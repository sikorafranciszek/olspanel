// Package users orchestrates hosting accounts: database row, Linux account,
// directory layout and cascading deletion of everything an account owns.
package users

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"olspanel/internal/auth"
	"olspanel/internal/db"
	"olspanel/internal/system"
	"olspanel/internal/validate"
)

// Cleaner is implemented by subsystems that must drop resources of a deleted user.
type Cleaner interface {
	CleanupUser(ctx context.Context, u *db.User) error
}

// Applier re-renders OLS config after account changes.
type Applier interface {
	Apply(ctx context.Context) error
}

// Service manages hosting accounts.
type Service struct {
	DB       *db.DB
	HomeRoot string
	OLS      Applier
	Cleaners []Cleaner
	// NoSystem disables Linux account operations (dev on non-Linux).
	NoSystem bool
}

// CreateRequest is the admin's input for a new account.
type CreateRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	PackageID int64  `json:"package_id"`
}

// Create validates, creates the Linux account and the panel user.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*db.User, error) {
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if err := validate.Username(req.Username); err != nil {
		return nil, err
	}
	if err := validate.Password(req.Password); err != nil {
		return nil, err
	}
	if err := validate.Email(req.Email); err != nil {
		return nil, err
	}
	if req.Role != "admin" && req.Role != "user" {
		req.Role = "user"
	}
	if _, err := s.DB.UserByName(ctx, req.Username); err == nil {
		return nil, errors.New("użytkownik o tej nazwie już istnieje")
	}
	if req.Role == "user" {
		if req.PackageID == 0 {
			return nil, errors.New("wybierz pakiet")
		}
		if _, err := s.DB.PackageByID(ctx, req.PackageID); err != nil {
			return nil, errors.New("pakiet nie istnieje")
		}
	} else {
		req.PackageID = 0
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	u := &db.User{Username: req.Username, Role: req.Role, PasswordHash: hash, Email: req.Email, PackageID: req.PackageID}
	if req.Role == "user" && !s.NoSystem {
		acc, err := system.CreateAccount(ctx, req.Username, s.HomeRoot)
		if err != nil {
			return nil, fmt.Errorf("tworzenie konta systemowego: %w", err)
		}
		u.UID, u.GID, u.Home = int64(acc.UID), int64(acc.GID), acc.Home
	} else if req.Role == "user" {
		u.Home = filepath.Join(s.HomeRoot, req.Username)
		for _, sub := range []string{"domains", "tmp"} {
			_ = os.MkdirAll(filepath.Join(u.Home, sub), 0o750)
		}
	}
	id, err := s.DB.CreateUser(ctx, u)
	if err != nil {
		if req.Role == "user" && !s.NoSystem {
			_ = system.DeleteAccount(ctx, req.Username)
		}
		return nil, err
	}
	return s.DB.UserByID(ctx, id)
}

// UpdateRequest is the admin's input for editing an account.
type UpdateRequest struct {
	Email     string `json:"email"`
	PackageID int64  `json:"package_id"`
	Password  string `json:"password"`
}

// Update edits e-mail, package and optionally password.
func (s *Service) Update(ctx context.Context, id int64, req UpdateRequest) (*db.User, error) {
	u, err := s.DB.UserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validate.Email(req.Email); err != nil {
		return nil, err
	}
	u.Email = req.Email
	if u.Role == "user" && req.PackageID != 0 {
		if _, err := s.DB.PackageByID(ctx, req.PackageID); err != nil {
			return nil, errors.New("pakiet nie istnieje")
		}
		u.PackageID = req.PackageID
	}
	if err := s.DB.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	if req.Password != "" {
		if err := s.SetPassword(ctx, id, req.Password); err != nil {
			return nil, err
		}
	}
	return s.DB.UserByID(ctx, id)
}

// SetPassword changes a user's password.
func (s *Service) SetPassword(ctx context.Context, id int64, password string) error {
	if err := validate.Password(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return s.DB.SetPassword(ctx, id, hash)
}

// SetSuspended suspends or resumes an account: locks the Linux password and
// re-renders vhosts with the suspended page.
func (s *Service) SetSuspended(ctx context.Context, id int64, suspended bool) (*db.User, error) {
	u, err := s.DB.UserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.Role == "admin" {
		return nil, errors.New("nie można zawiesić administratora")
	}
	u.Suspended = suspended
	if err := s.DB.UpdateUser(ctx, u); err != nil {
		return nil, err
	}
	if !s.NoSystem {
		if err := system.SetLocked(ctx, u.Username, suspended); err != nil {
			slog.Warn("lock account", "user", u.Username, "err", err)
		}
	}
	if s.OLS != nil {
		if err := s.OLS.Apply(ctx); err != nil {
			return nil, err
		}
	}
	return s.DB.UserByID(ctx, id)
}

// Delete removes the account and everything it owns.
func (s *Service) Delete(ctx context.Context, id int64) error {
	u, err := s.DB.UserByID(ctx, id)
	if err != nil {
		return err
	}
	if u.Role == "admin" {
		n, _ := s.DB.CountUsers(ctx, "admin")
		if n <= 1 {
			return errors.New("nie można usunąć ostatniego administratora")
		}
	}
	for _, c := range s.Cleaners {
		if err := c.CleanupUser(ctx, u); err != nil {
			slog.Warn("cleanup user resource", "user", u.Username, "err", err)
		}
	}
	if err := s.DB.DeleteUser(ctx, id); err != nil {
		return err
	}
	if u.Role == "user" {
		if s.OLS != nil {
			if err := s.OLS.Apply(ctx); err != nil {
				slog.Warn("apply after delete", "err", err)
			}
		}
		if !s.NoSystem {
			if err := system.DeleteAccount(ctx, u.Username); err != nil {
				return fmt.Errorf("użytkownik usunięty z panelu, ale nie z systemu: %w", err)
			}
		}
	}
	return nil
}

// Usage summarises a user's consumption vs. package limits.
type Usage struct {
	Package    *db.Package `json:"package"`
	DiskUsedMB int64       `json:"disk_used_mb"`
	Domains    int64       `json:"domains"`
	Subdomains int64       `json:"subdomains"`
	Databases  int64       `json:"databases"`
	FTP        int64       `json:"ftp"`
	Cron       int64       `json:"cron"`
}

// Usage computes current usage for a user.
func (s *Service) Usage(ctx context.Context, u *db.User) (*Usage, error) {
	us := &Usage{DiskUsedMB: u.DiskUsedMB}
	if u.PackageID != 0 {
		p, err := s.DB.PackageByID(ctx, u.PackageID)
		if err == nil {
			us.Package = p
		}
	}
	us.Domains, _ = s.DB.CountUserDomains(ctx, u.ID, "domain")
	us.Subdomains, _ = s.DB.CountUserDomains(ctx, u.ID, "subdomain")
	us.Databases, _ = s.DB.CountUserDatabases(ctx, u.ID)
	us.FTP, _ = s.DB.CountUserFTP(ctx, u.ID)
	us.Cron, _ = s.DB.CountUserCron(ctx, u.ID)
	return us, nil
}

// RefreshDiskUsage recomputes disk usage for every hosting user.
func (s *Service) RefreshDiskUsage(ctx context.Context) {
	if s.NoSystem {
		return
	}
	list, err := s.DB.ListUsers(ctx)
	if err != nil {
		return
	}
	for _, u := range list {
		if u.Role != "user" || u.Home == "" {
			continue
		}
		mb, err := system.DiskUsageMB(ctx, u.Home)
		if err != nil {
			continue
		}
		_ = s.DB.SetDiskUsed(ctx, u.ID, mb)
	}
}

// ErrOverQuota is returned when a package limit is reached.
var ErrOverQuota = errors.New("limit pakietu został osiągnięty")

// CheckLimit returns ErrOverQuota when current >= limit (limit 0 = unlimited is NOT assumed; 0 means none).
func CheckLimit(current, limit int64, what string) error {
	if current >= limit {
		return fmt.Errorf("%w: %s (%d/%d)", ErrOverQuota, what, current, limit)
	}
	return nil
}
