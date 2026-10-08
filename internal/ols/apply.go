package ols

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"olspanel/internal/system"
)

// Manager serialises configuration changes: render → write → test → restart,
// with rollback to the last known-good files when the syntax test fails.
type Manager struct {
	ConfDir string // /usr/local/lsws/conf
	// Build returns the desired state (from the database). Set by the wiring layer.
	Build func(ctx context.Context) (*State, error)
	// DryRun skips the syntax test and restart (for tests / non-Linux dev).
	DryRun bool

	mu       sync.Mutex
	lsadmUID int
	lsadmGID int
	lookedUp bool
}

// ErrApplyFailed wraps syntax/restart failures after rollback.
var ErrApplyFailed = errors.New("nie udało się zastosować konfiguracji OpenLiteSpeed")

// Apply rebuilds the state and writes it to disk.
func (m *Manager) Apply(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, err := m.Build(ctx)
	if err != nil {
		return err
	}
	return m.applyState(ctx, st)
}

// ApplyState applies a given state (used by tests and the installer).
func (m *Manager) ApplyState(ctx context.Context, st *State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.applyState(ctx, st)
}

func (m *Manager) applyState(ctx context.Context, st *State) error {
	files, err := Render(st)
	if err != nil {
		return err
	}
	snapshot, err := m.snapshot()
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if err := m.writeFiles(files); err != nil {
		_ = m.restore(snapshot)
		return err
	}
	if err := m.pruneStale(files); err != nil {
		slog.Warn("prune stale vhost dirs", "err", err)
	}
	if m.DryRun {
		return nil
	}
	if err := m.syntaxTest(ctx); err != nil {
		slog.Error("OLS syntax test failed, rolling back", "err", err)
		if rerr := m.restore(snapshot); rerr != nil {
			slog.Error("rollback failed", "err", rerr)
		}
		return fmt.Errorf("%w: %v", ErrApplyFailed, err)
	}
	if err := m.restart(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrApplyFailed, err)
	}
	return nil
}

func (m *Manager) lookupLsadm() {
	if m.lookedUp {
		return
	}
	m.lookedUp = true
	if u, err := user.Lookup("lsadm"); err == nil {
		m.lsadmUID, _ = strconv.Atoi(u.Uid)
		m.lsadmGID, _ = strconv.Atoi(u.Gid)
	} else {
		m.lsadmUID, m.lsadmGID = -1, -1
	}
}

// writeFiles atomically writes each rendered file (tmp + rename).
func (m *Manager) writeFiles(files map[string]string) error {
	m.lookupLsadm()
	for _, rel := range SortedPaths(files) {
		full := filepath.Join(m.ConfDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			return err
		}
		if m.lsadmUID >= 0 {
			_ = os.Chown(filepath.Dir(full), m.lsadmUID, m.lsadmGID)
		}
		tmp := full + ".tmp"
		if err := os.WriteFile(tmp, []byte(files[rel]), 0o600); err != nil {
			return err
		}
		if m.lsadmUID >= 0 {
			_ = os.Chown(tmp, m.lsadmUID, m.lsadmGID)
		}
		if err := os.Rename(tmp, full); err != nil {
			return err
		}
	}
	return nil
}

// managedFiles lists every file the panel owns on disk.
func (m *Manager) managedFiles() ([]string, error) {
	var out []string
	for _, pattern := range []string{"olspanel/*.conf", "vhosts/*/vhconf.conf"} {
		matches, err := filepath.Glob(filepath.Join(m.ConfDir, pattern))
		if err != nil {
			return nil, err
		}
		out = append(out, matches...)
	}
	return out, nil
}

func (m *Manager) snapshot() (map[string][]byte, error) {
	snap := map[string][]byte{}
	files, err := m.managedFiles()
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		snap[f] = b
	}
	return snap, nil
}

func (m *Manager) restore(snap map[string][]byte) error {
	current, err := m.managedFiles()
	if err != nil {
		return err
	}
	var firstErr error
	for _, f := range current {
		if _, ok := snap[f]; !ok {
			if err := os.Remove(f); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	for f, b := range snap {
		if err := os.MkdirAll(filepath.Dir(f), 0o750); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := os.WriteFile(f, b, 0o600); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// pruneStale removes vhost directories that are no longer rendered.
func (m *Manager) pruneStale(files map[string]string) error {
	dirs, err := filepath.Glob(filepath.Join(m.ConfDir, "vhosts", "*"))
	if err != nil {
		return err
	}
	for _, d := range dirs {
		name := filepath.Base(d)
		if name == "Example" {
			continue // stock vhost, left alone
		}
		if _, ok := files["vhosts/"+name+"/vhconf.conf"]; ok {
			continue
		}
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) syntaxTest(ctx context.Context) error {
	res, err := system.Run(ctx, "openlitespeed", "", "-t")
	if err != nil {
		if res != nil {
			return fmt.Errorf("%s", strings.TrimSpace(res.Stderr+"\n"+res.Stdout))
		}
		return err
	}
	return nil
}

func (m *Manager) restart(ctx context.Context) error {
	if _, err := system.Run(ctx, "lswsctrl", "", "restart"); err != nil {
		return err
	}
	// Give the graceful restart a moment, then verify port 80 answers.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if portOpen("127.0.0.1:80") {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("OpenLiteSpeed nie odpowiada na porcie 80 po restarcie")
}

func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Healthy reports whether OLS serves HTTP on localhost.
func Healthy() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1/")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}
