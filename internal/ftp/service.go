// Package ftp manages pure-ftpd virtual users (PureDB backend).
package ftp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"olspanel/internal/db"
	"olspanel/internal/system"
	"olspanel/internal/users"
	"olspanel/internal/validate"
)

const (
	passwdFile = "/etc/pure-ftpd/pureftpd.passwd"
	pdbFile    = "/etc/pure-ftpd/pureftpd.pdb"
	certFile   = "/etc/ssl/private/pure-ftpd.pem"
)

// Service manages FTP accounts.
type Service struct {
	DB       *db.DB
	NoSystem bool
}

// CreateRequest is the user's input.
type CreateRequest struct {
	Suffix     string `json:"suffix"` // "" = main account login (<user>)
	Password   string `json:"password"`
	HomeSubdir string `json:"home_subdir"` // relative to /home/<u>, "" = whole home
}

// UpdateRequest edits an account.
type UpdateRequest struct {
	Password   string  `json:"password"`
	HomeSubdir *string `json:"home_subdir"`
}

func login(u *db.User, suffix string) (string, error) {
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	if suffix == "" {
		return u.Username, nil
	}
	if err := validate.Suffix(suffix); err != nil {
		return "", err
	}
	return u.Username + "_" + suffix, nil
}

// cleanSubdir normalises a home sub-directory and rejects traversal.
func cleanSubdir(s string) (string, error) {
	s = strings.Trim(strings.TrimSpace(s), "/")
	if s == "" {
		return "", nil
	}
	c := filepath.ToSlash(filepath.Clean("/" + s))
	if strings.Contains(c, "..") || strings.ContainsAny(c, "\x00\n\r") {
		return "", errors.Join(validate.ErrInvalid, errors.New("nieprawidłowy katalog"))
	}
	return strings.TrimPrefix(c, "/"), nil
}

func (s *Service) homeFor(u *db.User, sub string) string {
	if sub == "" {
		return u.Home
	}
	return filepath.Join(u.Home, sub)
}

// Create adds an FTP account.
func (s *Service) Create(ctx context.Context, u *db.User, req CreateRequest) (*db.FTPAccount, error) {
	lg, err := login(u, req.Suffix)
	if err != nil {
		return nil, err
	}
	if err := validate.Password(req.Password); err != nil {
		return nil, err
	}
	sub, err := cleanSubdir(req.HomeSubdir)
	if err != nil {
		return nil, err
	}
	pkg, err := s.DB.PackageByID(ctx, u.PackageID)
	if err != nil {
		return nil, errors.New("konto nie ma przypisanego pakietu")
	}
	n, _ := s.DB.CountUserFTP(ctx, u.ID)
	if err := users.CheckLimit(n, pkg.MaxFTP, "konta FTP"); err != nil {
		return nil, err
	}
	if !s.NoSystem {
		home := s.homeFor(u, sub)
		if err := os.MkdirAll(home, 0o750); err != nil {
			return nil, err
		}
		_ = os.Lchown(home, int(u.UID), int(u.GID))
		args := []string{"useradd", lg, "-f", passwdFile, "-u", strconv.FormatInt(u.UID, 10), "-g", strconv.FormatInt(u.GID, 10), "-d", home, "-m"}
		if _, err := system.Run(ctx, "pure-pw", req.Password+"\n"+req.Password+"\n", args...); err != nil {
			return nil, fmt.Errorf("pure-pw: %w", err)
		}
		s.mkdb(ctx)
	}
	id, err := s.DB.CreateFTP(ctx, &db.FTPAccount{UserID: u.ID, Login: lg, HomeSubdir: sub})
	if err != nil {
		if !s.NoSystem {
			_, _ = system.Run(ctx, "pure-pw", "", "userdel", lg, "-f", passwdFile, "-m")
		}
		return nil, err
	}
	return s.DB.FTPByID(ctx, id)
}

// Update changes password and/or home directory.
func (s *Service) Update(ctx context.Context, u *db.User, id int64, req UpdateRequest) (*db.FTPAccount, error) {
	a, err := s.owned(ctx, u, id)
	if err != nil {
		return nil, err
	}
	if req.Password != "" {
		if err := validate.Password(req.Password); err != nil {
			return nil, err
		}
		if !s.NoSystem {
			if _, err := system.Run(ctx, "pure-pw", req.Password+"\n"+req.Password+"\n", "passwd", a.Login, "-f", passwdFile, "-m"); err != nil {
				return nil, fmt.Errorf("pure-pw: %w", err)
			}
		}
	}
	if req.HomeSubdir != nil {
		sub, err := cleanSubdir(*req.HomeSubdir)
		if err != nil {
			return nil, err
		}
		if !s.NoSystem {
			home := s.homeFor(u, sub)
			if err := os.MkdirAll(home, 0o750); err != nil {
				return nil, err
			}
			_ = os.Lchown(home, int(u.UID), int(u.GID))
			if _, err := system.Run(ctx, "pure-pw", "", "usermod", a.Login, "-f", passwdFile, "-d", home, "-m"); err != nil {
				return nil, fmt.Errorf("pure-pw: %w", err)
			}
		}
		_ = s.DB.DeleteFTP(ctx, id)
		if _, err := s.DB.CreateFTP(ctx, &db.FTPAccount{UserID: u.ID, Login: a.Login, HomeSubdir: sub}); err != nil {
			return nil, err
		}
		list, _ := s.DB.ListUserFTP(ctx, u.ID)
		for _, x := range list {
			if x.Login == a.Login {
				return &x, nil
			}
		}
	}
	return s.DB.FTPByID(ctx, id)
}

// Delete removes an FTP account.
func (s *Service) Delete(ctx context.Context, u *db.User, id int64) error {
	a, err := s.owned(ctx, u, id)
	if err != nil {
		return err
	}
	if !s.NoSystem {
		if _, err := system.Run(ctx, "pure-pw", "", "userdel", a.Login, "-f", passwdFile, "-m"); err != nil {
			return fmt.Errorf("pure-pw: %w", err)
		}
	}
	return s.DB.DeleteFTP(ctx, id)
}

func (s *Service) owned(ctx context.Context, u *db.User, id int64) (*db.FTPAccount, error) {
	a, err := s.DB.FTPByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.UserID != u.ID {
		return nil, db.ErrNotFound
	}
	return a, nil
}

func (s *Service) mkdb(ctx context.Context) {
	_, _ = system.Run(ctx, "pure-pw", "", "mkdb", pdbFile, "-f", passwdFile)
}

// CleanupUser implements users.Cleaner.
func (s *Service) CleanupUser(ctx context.Context, u *db.User) error {
	list, err := s.DB.ListUserFTP(ctx, u.ID)
	if err != nil {
		return err
	}
	if s.NoSystem {
		return nil
	}
	for _, a := range list {
		_, _ = system.Run(ctx, "pure-pw", "", "userdel", a.Login, "-f", passwdFile)
	}
	s.mkdb(ctx)
	return nil
}

// InstallCert writes key+cert into the pure-ftpd PEM bundle and restarts the service.
func InstallCert(ctx context.Context, certPEM, keyPEM []byte) error {
	if err := os.MkdirAll(filepath.Dir(certFile), 0o700); err != nil {
		return err
	}
	bundle := append(append([]byte{}, keyPEM...), certPEM...)
	if err := os.WriteFile(certFile, bundle, 0o600); err != nil {
		return err
	}
	if system.Available("systemctl") && system.UnitActive(ctx, "pure-ftpd") {
		return system.Systemctl(ctx, "restart", "pure-ftpd")
	}
	return nil
}
