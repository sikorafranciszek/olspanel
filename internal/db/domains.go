package db

import (
	"context"
	"database/sql"
	"errors"
)

const domainCols = `d.id, d.user_id, u.username, d.name, d.parent_id, d.type, d.php_version, d.ssl_status, d.ssl_expires_at, d.force_https, d.created_at`

func scanDomain(s scanner) (*Domain, error) {
	var dm Domain
	var force int
	err := s.Scan(&dm.ID, &dm.UserID, &dm.Username, &dm.Name, &dm.ParentID, &dm.Type, &dm.PHPVersion, &dm.SSLStatus, &dm.SSLExpiresAt, &force, &dm.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	dm.ForceHTTPS = force == 1
	return &dm, nil
}

func (d *DB) queryDomains(ctx context.Context, where string, args ...any) ([]Domain, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+domainCols+` FROM domains d JOIN users u ON u.id=d.user_id `+where+` ORDER BY d.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		dm, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *dm)
	}
	return out, rows.Err()
}

// ListDomains returns all domains (admin).
func (d *DB) ListDomains(ctx context.Context) ([]Domain, error) {
	return d.queryDomains(ctx, "")
}

// ListUserDomains returns a user's domains.
func (d *DB) ListUserDomains(ctx context.Context, userID int64) ([]Domain, error) {
	return d.queryDomains(ctx, `WHERE d.user_id=?`, userID)
}

// DomainByID fetches one domain.
func (d *DB) DomainByID(ctx context.Context, id int64) (*Domain, error) {
	return scanDomain(d.QueryRowContext(ctx, `SELECT `+domainCols+` FROM domains d JOIN users u ON u.id=d.user_id WHERE d.id=?`, id))
}

// DomainByName fetches one domain by name.
func (d *DB) DomainByName(ctx context.Context, name string) (*Domain, error) {
	return scanDomain(d.QueryRowContext(ctx, `SELECT `+domainCols+` FROM domains d JOIN users u ON u.id=d.user_id WHERE d.name=?`, name))
}

// CreateDomain inserts a domain.
func (d *DB) CreateDomain(ctx context.Context, dm *Domain) (int64, error) {
	force := 0
	if dm.ForceHTTPS {
		force = 1
	}
	res, err := d.ExecContext(ctx, `INSERT INTO domains(user_id, name, parent_id, type, php_version, force_https) VALUES(?,?,?,?,?,?)`,
		dm.UserID, dm.Name, dm.ParentID, dm.Type, dm.PHPVersion, force)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateDomain updates php_version, ssl fields and force_https.
func (d *DB) UpdateDomain(ctx context.Context, dm *Domain) error {
	force := 0
	if dm.ForceHTTPS {
		force = 1
	}
	_, err := d.ExecContext(ctx, `UPDATE domains SET php_version=?, ssl_status=?, ssl_expires_at=?, force_https=? WHERE id=?`,
		dm.PHPVersion, dm.SSLStatus, dm.SSLExpiresAt, force, dm.ID)
	return err
}

// DeleteDomain removes a domain and its children.
func (d *DB) DeleteDomain(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM domains WHERE id=?`, id)
	return err
}

// CountUserDomains counts domains of a type for a user.
func (d *DB) CountUserDomains(ctx context.Context, userID int64, typ string) (int64, error) {
	var n int64
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM domains WHERE user_id=? AND type=?`, userID, typ).Scan(&n)
	return n, err
}
