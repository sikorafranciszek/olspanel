package db

import (
	"context"
	"database/sql"
	"errors"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("nie znaleziono")

const userCols = `u.id, u.username, u.role, u.password_hash, u.email, COALESCE(u.package_id,0), COALESCE(p.name,''),
	COALESCE(u.uid,0), COALESCE(u.gid,0), u.home, u.disk_used_mb, u.suspended, u.created_at,
	(SELECT COUNT(*) FROM domains d WHERE d.user_id = u.id)`

type scanner interface{ Scan(dest ...any) error }

func scanUser(s scanner) (*User, error) {
	var u User
	var susp int
	err := s.Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.Email, &u.PackageID, &u.PackageName,
		&u.UID, &u.GID, &u.Home, &u.DiskUsedMB, &susp, &u.CreatedAt, &u.DomainCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Suspended = susp == 1
	return &u, nil
}

// UserByID fetches one user.
func (d *DB) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(d.QueryRowContext(ctx, `SELECT `+userCols+` FROM users u LEFT JOIN packages p ON p.id = u.package_id WHERE u.id = ?`, id))
}

// UserByName fetches one user by username.
func (d *DB) UserByName(ctx context.Context, name string) (*User, error) {
	return scanUser(d.QueryRowContext(ctx, `SELECT `+userCols+` FROM users u LEFT JOIN packages p ON p.id = u.package_id WHERE u.username = ?`, name))
}

// ListUsers returns all users ordered by role and name.
func (d *DB) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+userCols+` FROM users u LEFT JOIN packages p ON p.id = u.package_id ORDER BY u.role, u.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// CreateUser inserts a user and returns its id.
func (d *DB) CreateUser(ctx context.Context, u *User) (int64, error) {
	var pkg any
	if u.PackageID != 0 {
		pkg = u.PackageID
	}
	var uid, gid any
	if u.UID != 0 {
		uid, gid = u.UID, u.GID
	}
	res, err := d.ExecContext(ctx, `INSERT INTO users(username, role, password_hash, email, package_id, uid, gid, home) VALUES(?,?,?,?,?,?,?,?)`,
		u.Username, u.Role, u.PasswordHash, u.Email, pkg, uid, gid, u.Home)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateUser updates mutable fields.
func (d *DB) UpdateUser(ctx context.Context, u *User) error {
	var pkg any
	if u.PackageID != 0 {
		pkg = u.PackageID
	}
	susp := 0
	if u.Suspended {
		susp = 1
	}
	_, err := d.ExecContext(ctx, `UPDATE users SET email=?, package_id=?, suspended=?, uid=?, gid=?, home=?, disk_used_mb=? WHERE id=?`,
		u.Email, pkg, susp, u.UID, u.GID, u.Home, u.DiskUsedMB, u.ID)
	return err
}

// SetPassword updates the password hash.
func (d *DB) SetPassword(ctx context.Context, id int64, hash string) error {
	_, err := d.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	return err
}

// SetDiskUsed records the computed disk usage.
func (d *DB) SetDiskUsed(ctx context.Context, id, mb int64) error {
	_, err := d.ExecContext(ctx, `UPDATE users SET disk_used_mb=? WHERE id=?`, mb, id)
	return err
}

// DeleteUser removes a user (cascades through FK).
func (d *DB) DeleteUser(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	return err
}

// CountUsers returns number of users with the given role.
func (d *DB) CountUsers(ctx context.Context, role string) (int64, error) {
	var n int64
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role=?`, role).Scan(&n)
	return n, err
}
