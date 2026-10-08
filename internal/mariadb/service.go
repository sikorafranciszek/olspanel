// Package mariadb manages per-user databases and database users through the
// local MariaDB server (root over unix socket).
package mariadb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"olspanel/internal/db"
	"olspanel/internal/users"
	"olspanel/internal/validate"
)

var identRe = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// Service manages MariaDB resources.
type Service struct {
	DB       *db.DB
	Socket   string
	NoSystem bool // skip real MariaDB calls (dev)
}

func (s *Service) conn(ctx context.Context) (*sql.DB, error) {
	dsn := fmt.Sprintf("root@unix(%s)/?charset=utf8mb4&parseTime=true&timeout=5s", s.Socket)
	c, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := c.PingContext(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("połączenie z MariaDB: %w", err)
	}
	return c, nil
}

// fullName builds "<user>_<suffix>" and validates it as an identifier.
func fullName(u *db.User, suffix string) (string, error) {
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	if err := validate.Suffix(suffix); err != nil {
		return "", err
	}
	name := u.Username + "_" + suffix
	if !identRe.MatchString(name) {
		return "", errors.New("nazwa jest za długa (maks. 32 znaki razem z prefiksem)")
	}
	return name, nil
}

func quoteIdent(s string) string { return "`" + s + "`" }

// Create creates a database named <user>_<suffix>.
func (s *Service) Create(ctx context.Context, u *db.User, suffix string) (*db.Database, error) {
	name, err := fullName(u, suffix)
	if err != nil {
		return nil, err
	}
	pkg, err := s.DB.PackageByID(ctx, u.PackageID)
	if err != nil {
		return nil, errors.New("konto nie ma przypisanego pakietu")
	}
	n, _ := s.DB.CountUserDatabases(ctx, u.ID)
	if err := users.CheckLimit(n, pkg.MaxDatabases, "bazy danych"); err != nil {
		return nil, err
	}
	if !s.NoSystem {
		c, err := s.conn(ctx)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		if _, err := c.ExecContext(ctx, "CREATE DATABASE "+quoteIdent(name)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			return nil, fmt.Errorf("tworzenie bazy: %w", err)
		}
	}
	id, err := s.DB.CreateDatabase(ctx, u.ID, name)
	if err != nil {
		return nil, err
	}
	return s.DB.DatabaseByID(ctx, id)
}

// Delete drops a database and its users.
func (s *Service) Delete(ctx context.Context, u *db.User, id int64) error {
	d, err := s.DB.DatabaseByID(ctx, id)
	if err != nil {
		return err
	}
	if d.UserID != u.ID {
		return db.ErrNotFound
	}
	dbUsers, _ := s.DB.ListDBUsersOf(ctx, id)
	if !s.NoSystem {
		c, err := s.conn(ctx)
		if err != nil {
			return err
		}
		defer c.Close()
		for _, du := range dbUsers {
			_, _ = c.ExecContext(ctx, "DROP USER IF EXISTS ?@'localhost'", du.Username)
		}
		if _, err := c.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(d.Name)); err != nil {
			return fmt.Errorf("usuwanie bazy: %w", err)
		}
	}
	return s.DB.DeleteDatabase(ctx, id)
}

// CreateUser creates <user>_<suffix>@localhost with all privileges on one database.
func (s *Service) CreateUser(ctx context.Context, u *db.User, dbID int64, suffix, password string) (*db.DBUser, error) {
	d, err := s.DB.DatabaseByID(ctx, dbID)
	if err != nil {
		return nil, err
	}
	if d.UserID != u.ID {
		return nil, db.ErrNotFound
	}
	name, err := fullName(u, suffix)
	if err != nil {
		return nil, err
	}
	if err := validate.Password(password); err != nil {
		return nil, err
	}
	if !s.NoSystem {
		c, err := s.conn(ctx)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		if _, err := c.ExecContext(ctx, "CREATE USER ?@'localhost' IDENTIFIED BY ?", name, password); err != nil {
			return nil, fmt.Errorf("tworzenie użytkownika bazy: %w", err)
		}
		if _, err := c.ExecContext(ctx, "GRANT ALL PRIVILEGES ON "+quoteIdent(d.Name)+".* TO ?@'localhost'", name); err != nil {
			_, _ = c.ExecContext(ctx, "DROP USER IF EXISTS ?@'localhost'", name)
			return nil, fmt.Errorf("nadawanie uprawnień: %w", err)
		}
		_, _ = c.ExecContext(ctx, "FLUSH PRIVILEGES")
	}
	id, err := s.DB.CreateDBUser(ctx, u.ID, dbID, name)
	if err != nil {
		return nil, err
	}
	return s.DB.DBUserByID(ctx, id)
}

// SetUserPassword changes a db user's password.
func (s *Service) SetUserPassword(ctx context.Context, u *db.User, dbID, uid int64, password string) error {
	du, err := s.DB.DBUserByID(ctx, uid)
	if err != nil {
		return err
	}
	if du.UserID != u.ID || du.DatabaseID != dbID {
		return db.ErrNotFound
	}
	if err := validate.Password(password); err != nil {
		return err
	}
	if s.NoSystem {
		return nil
	}
	c, err := s.conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.ExecContext(ctx, "ALTER USER ?@'localhost' IDENTIFIED BY ?", du.Username, password)
	return err
}

// DeleteUser drops a db user.
func (s *Service) DeleteUser(ctx context.Context, u *db.User, dbID, uid int64) error {
	du, err := s.DB.DBUserByID(ctx, uid)
	if err != nil {
		return err
	}
	if du.UserID != u.ID || du.DatabaseID != dbID {
		return db.ErrNotFound
	}
	if !s.NoSystem {
		c, err := s.conn(ctx)
		if err != nil {
			return err
		}
		defer c.Close()
		if _, err := c.ExecContext(ctx, "DROP USER IF EXISTS ?@'localhost'", du.Username); err != nil {
			return err
		}
	}
	return s.DB.DeleteDBUser(ctx, uid)
}

// CleanupUser implements users.Cleaner: drops every database and db user of u.
func (s *Service) CleanupUser(ctx context.Context, u *db.User) error {
	list, err := s.DB.ListUserDatabases(ctx, u.ID)
	if err != nil {
		return err
	}
	if s.NoSystem {
		return nil
	}
	c, err := s.conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	for _, d := range list {
		for _, du := range d.Users {
			_, _ = c.ExecContext(ctx, "DROP USER IF EXISTS ?@'localhost'", du.Username)
		}
		_, _ = c.ExecContext(ctx, "DROP DATABASE IF EXISTS "+quoteIdent(d.Name))
	}
	return nil
}

// SizeMB returns the on-disk size of a user's databases.
func (s *Service) SizeMB(ctx context.Context, u *db.User) (int64, error) {
	if s.NoSystem {
		return 0, nil
	}
	c, err := s.conn(ctx)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	var bytes sql.NullInt64
	err = c.QueryRowContext(ctx, `SELECT SUM(data_length + index_length) FROM information_schema.tables WHERE table_schema LIKE ?`, u.Username+"\\_%").Scan(&bytes)
	if err != nil {
		return 0, err
	}
	return bytes.Int64 / (1024 * 1024), nil
}
