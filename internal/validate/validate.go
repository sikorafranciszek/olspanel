// Package validate holds strict validators for every name that ends up in a
// config file, a shell argument or an SQL identifier. Nothing may be rendered
// into OpenLiteSpeed / crontab / pure-pw / MariaDB without passing through here.
package validate

import (
	"errors"
	"regexp"
	"strings"

	"golang.org/x/net/idna"
)

var (
	usernameRe = regexp.MustCompile(`^[a-z][a-z0-9]{2,15}$`)
	labelRe    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	suffixRe   = regexp.MustCompile(`^[a-z0-9_]{1,24}$`)
	phpRe      = regexp.MustCompile(`^8[0-9]$`)
	pkgNameRe  = regexp.MustCompile(`^[A-Za-z0-9 _.-]{1,40}$`)
)

// Reserved system/user names that may never become hosting accounts.
var reserved = map[string]bool{
	"root": true, "admin": true, "nobody": true, "nogroup": true, "lsadm": true, "mysql": true,
	"www-data": true, "daemon": true, "bin": true, "sys": true, "sync": true, "games": true,
	"man": true, "lp": true, "mail": true, "news": true, "uucp": true, "proxy": true, "backup": true,
	"list": true, "irc": true, "ftp": true, "sshd": true, "syslog": true, "ubuntu": true,
	"olspanel": true, "postfix": true, "dovecot": true, "pma": true, "default": true, "example": true,
}

// ErrInvalid is wrapped by every validation error.
var ErrInvalid = errors.New("nieprawidłowa wartość")

func invalid(msg string) error { return errors.Join(ErrInvalid, errors.New(msg)) }

// Username validates a hosting account name (also a Linux user name).
func Username(s string) error {
	if !usernameRe.MatchString(s) {
		return invalid("nazwa użytkownika: 3-16 znaków, małe litery i cyfry, zaczyna się literą")
	}
	if reserved[s] {
		return invalid("nazwa użytkownika jest zarezerwowana")
	}
	return nil
}

// Domain normalises and validates a fully-qualified domain name.
// Returns the ASCII (punycode) form.
func Domain(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(s, ".")))
	if s == "" {
		return "", invalid("domena jest wymagana")
	}
	ascii, err := idna.Lookup.ToASCII(s)
	if err != nil {
		return "", invalid("nieprawidłowa nazwa domeny")
	}
	if len(ascii) > 253 {
		return "", invalid("nazwa domeny jest za długa")
	}
	labels := strings.Split(ascii, ".")
	if len(labels) < 2 {
		return "", invalid("domena musi zawierać kropkę (np. example.com)")
	}
	for _, l := range labels {
		if !labelRe.MatchString(l) {
			return "", invalid("nieprawidłowa nazwa domeny")
		}
	}
	if ascii == "localhost" || strings.HasSuffix(ascii, ".localhost") {
		return "", invalid("nazwa domeny jest zarezerwowana")
	}
	return ascii, nil
}

// Suffix validates the user-chosen part of a database / db-user / ftp name.
func Suffix(s string) error {
	if !suffixRe.MatchString(s) {
		return invalid("nazwa: 1-24 znaki, małe litery, cyfry i podkreślenie")
	}
	return nil
}

// PHPVersion validates a short PHP version id such as "83".
func PHPVersion(s string) error {
	if !phpRe.MatchString(s) {
		return invalid("nieprawidłowa wersja PHP")
	}
	return nil
}

// PackageName validates a package display name.
func PackageName(s string) error {
	if !pkgNameRe.MatchString(s) {
		return invalid("nazwa pakietu: 1-40 znaków (litery, cyfry, spacja, _ . -)")
	}
	return nil
}

// Password enforces a minimum password policy.
func Password(s string) error {
	if len(s) < 8 {
		return invalid("hasło musi mieć co najmniej 8 znaków")
	}
	if len(s) > 128 {
		return invalid("hasło jest za długie")
	}
	for _, r := range s {
		if r == '\n' || r == '\r' || r == 0 {
			return invalid("hasło zawiera niedozwolone znaki")
		}
	}
	return nil
}

// Email validates a simple e-mail (may be empty).
func Email(s string) error {
	if s == "" {
		return nil
	}
	at := strings.Index(s, "@")
	if at < 1 || at == len(s)-1 || strings.ContainsAny(s, " \t\r\n") || len(s) > 254 {
		return invalid("nieprawidłowy adres e-mail")
	}
	return nil
}

// CronCommand validates the command part of a crontab entry.
func CronCommand(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return invalid("polecenie jest wymagane")
	}
	if len(s) > 1000 {
		return invalid("polecenie jest za długie")
	}
	if strings.ContainsAny(s, "\r\n\x00") {
		return invalid("polecenie zawiera niedozwolone znaki")
	}
	return nil
}

// RelPath validates a user-supplied relative path for the file manager:
// it must not be absolute and must not contain NUL bytes. Traversal is
// enforced by os.Root, not here.
func RelPath(s string) error {
	if strings.ContainsRune(s, 0) {
		return invalid("nieprawidłowa ścieżka")
	}
	if len(s) > 4096 {
		return invalid("ścieżka jest za długa")
	}
	return nil
}
