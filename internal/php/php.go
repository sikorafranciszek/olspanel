// Package php discovers installed LSPHP versions.
package php

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var dirRe = regexp.MustCompile(`^lsphp(\d{2})$`)

// Version describes one installed lsphp build.
type Version struct {
	ID    string `json:"id"`    // "83"
	Label string `json:"label"` // "PHP 8.3"
	Bin   string `json:"bin"`   // /usr/local/lsws/lsphp83/bin/lsphp
}

// Installed lists lsphp versions under lswsRoot, newest first.
func Installed(lswsRoot string) []Version {
	out := []Version{}
	entries, err := os.ReadDir(lswsRoot)
	if err != nil {
		return out
	}
	for _, e := range entries {
		m := dirRe.FindStringSubmatch(e.Name())
		if m == nil || !e.IsDir() {
			continue
		}
		bin := filepath.Join(lswsRoot, e.Name(), "bin", "lsphp")
		if st, err := os.Stat(bin); err != nil || st.IsDir() {
			continue
		}
		out = append(out, Version{ID: m[1], Label: Label(m[1]), Bin: bin})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

// Label formats "83" as "PHP 8.3".
func Label(id string) string {
	if len(id) != 2 {
		return "PHP " + id
	}
	return fmt.Sprintf("PHP %c.%c", id[0], id[1])
}

// Bin returns the lsphp binary path for a version id (no existence check).
func Bin(lswsRoot, id string) string {
	return filepath.Join(lswsRoot, "lsphp"+id, "bin", "lsphp")
}

// Has reports whether version id is installed.
func Has(lswsRoot, id string) bool {
	for _, v := range Installed(lswsRoot) {
		if v.ID == id {
			return true
		}
	}
	return false
}
