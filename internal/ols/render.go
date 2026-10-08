package ols

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"text/template"
)

// Values that end up unquoted in OLS config must match these.
var (
	safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,253}$`)
	safePath = regexp.MustCompile(`^/[A-Za-z0-9._/-]*$`)
	safeUser = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
)

// okPath accepts absolute paths without ".." segments. On Windows (dev only,
// DryRun) a drive letter prefix and backslashes are tolerated.
func okPath(p string) bool {
	if runtime.GOOS == "windows" {
		p = filepath.ToSlash(p)
		if len(p) > 2 && p[1] == ':' {
			p = p[2:]
		}
	}
	if !safePath.MatchString(p) {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// Render produces every config file (path relative to $SERVER_ROOT/conf)
// for the given state. It validates all values that are rendered unquoted.
func Render(st *State) (map[string]string, error) {
	if err := validateState(st); err != nil {
		return nil, err
	}
	out := map[string]string{}
	exec := func(name string, t *template.Template, data any) error {
		var b bytes.Buffer
		if err := t.Execute(&b, data); err != nil {
			return fmt.Errorf("render %s: %w", name, err)
		}
		out[name] = b.String()
		return nil
	}
	if err := exec(FileProcessors, tplProcessors, st); err != nil {
		return nil, err
	}
	if err := exec(FileVHosts, tplVHosts, st); err != nil {
		return nil, err
	}
	if err := exec(FileListeners, tplListeners, st); err != nil {
		return nil, err
	}
	if err := exec("vhosts/_default/vhconf.conf", tplDefaultVHConf, map[string]string{"ACMEDir": path.Join(path.Dir(firstACME(st)), "_default")}); err != nil {
		return nil, err
	}
	if st.PMARoot != "" {
		if err := exec("vhosts/_pma/vhconf.conf", tplPMAVHConf, st); err != nil {
			return nil, err
		}
	}
	for i := range st.VHosts {
		vh := &st.VHosts[i]
		data := struct {
			*VHost
			SuspendRoot string
		}{vh, st.SuspendRoot}
		if err := exec("vhosts/"+vh.Name+"/vhconf.conf", tplVHConf, data); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func firstACME(st *State) string {
	if len(st.VHosts) > 0 && st.VHosts[0].ACMEDir != "" {
		return st.VHosts[0].ACMEDir
	}
	return "/var/lib/olspanel/acme-challenge/_"
}

func validateState(st *State) error {
	for _, p := range []string{st.PanelCert, st.PanelKey, st.SuspendRoot, st.DefaultRoot} {
		if !okPath(p) {
			return fmt.Errorf("nieprawidłowa ścieżka w konfiguracji: %q", p)
		}
	}
	if st.PMARoot != "" && (!okPath(st.PMARoot) || !okPath(st.PMAPhpBin)) {
		return fmt.Errorf("nieprawidłowa ścieżka phpMyAdmin")
	}
	seenProc := map[string]bool{}
	for _, p := range st.Processors {
		if !safeUser.MatchString(p.Name) || !safeUser.MatchString(p.User) || !safeUser.MatchString(p.Group) || !okPath(p.LsphpBin) {
			return fmt.Errorf("nieprawidłowy procesor PHP %q", p.Name)
		}
		if seenProc[p.Name] {
			return fmt.Errorf("zduplikowany procesor PHP %q", p.Name)
		}
		seenProc[p.Name] = true
	}
	seen := map[string]bool{}
	for _, vh := range st.VHosts {
		if !safeName.MatchString(vh.Name) {
			return fmt.Errorf("nieprawidłowa nazwa vhosta %q", vh.Name)
		}
		if seen[vh.Name] {
			return fmt.Errorf("zduplikowany vhost %q", vh.Name)
		}
		seen[vh.Name] = true
		if !safeUser.MatchString(vh.User) || !safeUser.MatchString(vh.Group) {
			return fmt.Errorf("nieprawidłowy użytkownik vhosta %q", vh.Name)
		}
		for _, p := range []string{vh.Root, vh.ACMEDir} {
			if !okPath(p) {
				return fmt.Errorf("nieprawidłowa ścieżka vhosta %q", vh.Name)
			}
		}
		if vh.SSLCert != "" && (!okPath(vh.SSLCert) || !okPath(vh.SSLKey)) {
			return fmt.Errorf("nieprawidłowa ścieżka certyfikatu %q", vh.Name)
		}
		if !vh.Suspended && !seenProc[vh.Processor] {
			return fmt.Errorf("vhost %q wskazuje nieistniejący procesor PHP %q", vh.Name, vh.Processor)
		}
		for _, a := range vh.Aliases {
			if !safeName.MatchString(a) {
				return fmt.Errorf("nieprawidłowy alias %q", a)
			}
			if seen[a] {
				return fmt.Errorf("alias %q koliduje z innym vhostem", a)
			}
		}
		if vh.OpenBase != "" {
			for _, c := range vh.OpenBase {
				if c == '"' || c == '\n' || c == '\r' || c == 0 {
					return fmt.Errorf("nieprawidłowy open_basedir dla %q", vh.Name)
				}
			}
		}
	}
	return nil
}

// SortedPaths returns rendered file names in a stable order (for tests/logs).
func SortedPaths(files map[string]string) []string {
	out := make([]string, 0, len(files))
	for k := range files {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
