package system

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"olspanel/internal/validate"
)

// Account describes a Linux account created for a hosting user.
type Account struct {
	Name string
	UID  int
	GID  int
	Home string
}

// Lookup returns the Linux account for name, or an error if it does not exist.
func Lookup(name string) (*Account, error) {
	if err := validate.Username(name); err != nil {
		return nil, err
	}
	u, err := user.Lookup(name)
	if err != nil {
		return nil, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return &Account{Name: name, UID: uid, GID: gid, Home: u.HomeDir}, nil
}

// CreateAccount creates a Linux user with a private group, nologin shell and
// the standard directory layout. It is idempotent for an existing account.
func CreateAccount(ctx context.Context, name, homeRoot string) (*Account, error) {
	if err := validate.Username(name); err != nil {
		return nil, err
	}
	home := filepath.Join(homeRoot, name)
	if acc, err := Lookup(name); err == nil {
		return acc, EnsureLayout(ctx, acc)
	}
	if _, err := Run(ctx, "useradd", "", "-m", "-d", home, "-s", "/usr/sbin/nologin", "-U", "-K", "UMASK=027", name); err != nil {
		return nil, err
	}
	acc, err := Lookup(name)
	if err != nil {
		return nil, fmt.Errorf("konto utworzone, ale nie można go odczytać: %w", err)
	}
	return acc, EnsureLayout(ctx, acc)
}

// EnsureLayout creates /home/<u>/{domains,tmp} with safe permissions and grants
// the web server user traversal rights via ACL.
func EnsureLayout(ctx context.Context, acc *Account) error {
	if err := os.MkdirAll(acc.Home, 0o750); err != nil {
		return err
	}
	if err := os.Chmod(acc.Home, 0o750); err != nil {
		return err
	}
	if err := os.Lchown(acc.Home, acc.UID, acc.GID); err != nil {
		return err
	}
	for _, sub := range []string{"domains", "tmp", "backups"} {
		p := filepath.Join(acc.Home, sub)
		if err := os.MkdirAll(p, 0o750); err != nil {
			return err
		}
		_ = os.Chmod(p, 0o750)
		_ = os.Lchown(p, acc.UID, acc.GID)
	}
	// nobody (OLS worker) needs +x to traverse into public_html; no read on home itself.
	if Available("setfacl") {
		_, _ = Run(ctx, "setfacl", "", "-m", "u:nobody:x", acc.Home)
		_, _ = Run(ctx, "setfacl", "", "-m", "u:nobody:x", filepath.Join(acc.Home, "domains"))
	}
	return nil
}

// EnsureDomainDirs creates the per-domain tree and returns its root.
func EnsureDomainDirs(ctx context.Context, acc *Account, domain string) (string, error) {
	root := filepath.Join(acc.Home, "domains", domain)
	for _, sub := range []string{"", "public_html", "logs", "tmp"} {
		p := filepath.Join(root, sub)
		if err := os.MkdirAll(p, 0o750); err != nil {
			return "", err
		}
		_ = os.Lchown(p, acc.UID, acc.GID)
	}
	_ = os.Chmod(root, 0o750)
	_ = os.Chmod(filepath.Join(root, "public_html"), 0o750)
	_ = os.Chmod(filepath.Join(root, "logs"), 0o750)
	if Available("setfacl") {
		_, _ = Run(ctx, "setfacl", "", "-m", "u:nobody:x", root)
		_, _ = Run(ctx, "setfacl", "", "-m", "u:nobody:rx", filepath.Join(root, "public_html"))
		// Default ACL so files created later by PHP/FTP are readable by nobody.
		_, _ = Run(ctx, "setfacl", "", "-d", "-m", "u:nobody:rx", filepath.Join(root, "public_html"))
	}
	return root, nil
}

// DeleteAccount removes the Linux user and its home directory.
func DeleteAccount(ctx context.Context, name string) error {
	if err := validate.Username(name); err != nil {
		return err
	}
	if _, err := Lookup(name); err != nil {
		return nil // already gone
	}
	_, err := Run(ctx, "userdel", "", "-r", "-f", name)
	return err
}

// SetLocked locks or unlocks the account password (suspension).
func SetLocked(ctx context.Context, name string, locked bool) error {
	if err := validate.Username(name); err != nil {
		return err
	}
	flag := "-U"
	if locked {
		flag = "-L"
	}
	_, err := Run(ctx, "usermod", "", flag, name)
	return err
}

// DiskUsageMB returns `du -sm` of a path.
func DiskUsageMB(ctx context.Context, path string) (int64, error) {
	res, err := Run(ctx, "du", "", "-sm", path)
	if err != nil {
		return 0, err
	}
	f := strings.Fields(res.Stdout)
	if len(f) == 0 {
		return 0, fmt.Errorf("du: pusty wynik")
	}
	return strconv.ParseInt(f[0], 10, 64)
}

// ChownTree recursively sets ownership.
func ChownTree(ctx context.Context, path string, uid, gid int) error {
	_, err := Run(ctx, "chown", "", "-R", fmt.Sprintf("%d:%d", uid, gid), path)
	return err
}

// Systemctl runs an action on a unit.
func Systemctl(ctx context.Context, action, unit string) error {
	switch action {
	case "start", "stop", "restart", "reload", "enable", "disable":
	default:
		return fmt.Errorf("nieobsługiwana akcja %q", action)
	}
	for _, r := range unit {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
			return fmt.Errorf("nieprawidłowa nazwa usługi")
		}
	}
	_, err := Run(ctx, "systemctl", "", action, unit)
	return err
}

// UnitActive reports whether a systemd unit is active.
func UnitActive(ctx context.Context, unit string) bool {
	res, err := Run(ctx, "systemctl", "", "is-active", unit)
	if res == nil {
		return false
	}
	return err == nil && strings.TrimSpace(res.Stdout) == "active"
}
