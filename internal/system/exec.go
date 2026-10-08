// Package system is the only place that runs privileged commands. Every
// binary is referenced by absolute path from an allowlist and arguments are
// always passed as a slice; nothing is ever interpreted by a shell.
package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Binaries that olspanel may execute.
var allowed = map[string]string{
	"useradd":       "/usr/sbin/useradd",
	"userdel":       "/usr/sbin/userdel",
	"usermod":       "/usr/sbin/usermod",
	"setfacl":       "/usr/bin/setfacl",
	"du":            "/usr/bin/du",
	"crontab":       "/usr/bin/crontab",
	"systemctl":     "/usr/bin/systemctl",
	"pure-pw":       "/usr/bin/pure-pw",
	"lswsctrl":      "/usr/local/lsws/bin/lswsctrl",
	"openlitespeed": "/usr/local/lsws/bin/openlitespeed",
	"id":            "/usr/bin/id",
	"getent":        "/usr/bin/getent",
	"chown":         "/usr/bin/chown",
}

// ErrNotAllowed is returned for binaries outside the allowlist.
var ErrNotAllowed = errors.New("polecenie nie jest dozwolone")

// Result captures a finished command.
type Result struct {
	Stdout string
	Stderr string
	Code   int
}

// Run executes an allowlisted binary with args and optional stdin.
func Run(ctx context.Context, name string, stdin string, args ...string) (*Result, error) {
	path, ok := allowed[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotAllowed, name)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("brak programu %s: %w", path, err)
	}
	for _, a := range args {
		if strings.ContainsAny(a, "\x00\n\r") {
			return nil, fmt.Errorf("argument zawiera niedozwolone znaki")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8"}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	res := &Result{Stdout: out.String(), Stderr: errb.String()}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.Code = ee.ExitCode()
		} else {
			res.Code = -1
		}
		slog.Warn("command failed", "cmd", name, "args", args, "code", res.Code, "stderr", strings.TrimSpace(res.Stderr))
		return res, fmt.Errorf("%s: %s", name, firstLine(res.Stderr, err.Error()))
	}
	slog.Debug("command ok", "cmd", name, "args", args)
	return res, nil
}

func firstLine(s, def string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Path returns the absolute path of an allowlisted binary (for existence checks).
func Path(name string) string { return allowed[name] }

// Available reports whether an allowlisted binary exists on this host.
func Available(name string) bool {
	p, ok := allowed[name]
	if !ok {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}
