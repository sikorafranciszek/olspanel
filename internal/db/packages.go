package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

const pkgCols = `p.id, p.name, p.disk_mb, p.max_domains, p.max_subdomains, p.max_databases, p.max_ftp, p.max_cron, p.php_versions, p.created_at,
	(SELECT COUNT(*) FROM users u WHERE u.package_id = p.id)`

func scanPackage(s scanner) (*Package, error) {
	var p Package
	var phpJSON string
	err := s.Scan(&p.ID, &p.Name, &p.DiskMB, &p.MaxDomains, &p.MaxSubdomains, &p.MaxDatabases, &p.MaxFTP, &p.MaxCron, &phpJSON, &p.CreatedAt, &p.UserCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(phpJSON), &p.PHPVersions)
	if p.PHPVersions == nil {
		p.PHPVersions = []string{}
	}
	return &p, nil
}

// PackageByID fetches a package.
func (d *DB) PackageByID(ctx context.Context, id int64) (*Package, error) {
	return scanPackage(d.QueryRowContext(ctx, `SELECT `+pkgCols+` FROM packages p WHERE p.id=?`, id))
}

// ListPackages returns all packages.
func (d *DB) ListPackages(ctx context.Context) ([]Package, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+pkgCols+` FROM packages p ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Package{}
	for rows.Next() {
		p, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// CreatePackage inserts a package.
func (d *DB) CreatePackage(ctx context.Context, p *Package) (int64, error) {
	php, _ := json.Marshal(p.PHPVersions)
	res, err := d.ExecContext(ctx, `INSERT INTO packages(name, disk_mb, max_domains, max_subdomains, max_databases, max_ftp, max_cron, php_versions) VALUES(?,?,?,?,?,?,?,?)`,
		p.Name, p.DiskMB, p.MaxDomains, p.MaxSubdomains, p.MaxDatabases, p.MaxFTP, p.MaxCron, string(php))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdatePackage updates a package.
func (d *DB) UpdatePackage(ctx context.Context, p *Package) error {
	php, _ := json.Marshal(p.PHPVersions)
	_, err := d.ExecContext(ctx, `UPDATE packages SET name=?, disk_mb=?, max_domains=?, max_subdomains=?, max_databases=?, max_ftp=?, max_cron=?, php_versions=? WHERE id=?`,
		p.Name, p.DiskMB, p.MaxDomains, p.MaxSubdomains, p.MaxDatabases, p.MaxFTP, p.MaxCron, string(php), p.ID)
	return err
}

// ErrPackageInUse is returned when deleting a package that users reference.
var ErrPackageInUse = errors.New("pakiet jest przypisany do użytkowników")

// DeletePackage removes a package; fails if users reference it.
func (d *DB) DeletePackage(ctx context.Context, id int64) error {
	var n int64
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE package_id=?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrPackageInUse
	}
	_, err := d.ExecContext(ctx, `DELETE FROM packages WHERE id=?`, id)
	return err
}
