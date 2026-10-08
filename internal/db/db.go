// Package db opens the SQLite database and runs migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"olspanel/migrations"
)

// DB wraps *sql.DB with typed repositories.
type DB struct {
	*sql.DB
}

// Open opens (creating if needed) the SQLite database at path and migrates it.
// Use ":memory:" for tests.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := path
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
		dsn = "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	} else {
		dsn = "file::memory:?cache=shared&_pragma=foreign_keys(1)"
	}
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite: one writer. Keep a single connection to avoid SQLITE_BUSY storms.
	sdb.SetMaxOpenConns(1)
	if err := sdb.PingContext(ctx); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(ctx, sdb); err != nil {
		sdb.Close()
		return nil, err
	}
	if path != ":memory:" {
		_ = os.Chmod(path, 0o600)
	}
	return &DB{DB: sdb}, nil
}

func migrate(ctx context.Context, sdb *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, sdb, "."); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Setting reads a key from settings; returns def when absent.
func (d *DB) Setting(ctx context.Context, key, def string) string {
	var v string
	err := d.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return def
	}
	return v
}

// SetSetting upserts a setting.
func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := d.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// AllSettings returns every setting.
func (d *DB) AllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// Audit appends an audit log entry.
func (d *DB) Audit(ctx context.Context, actorID, impersonatorID int64, action, target, detail string) {
	var imp any
	if impersonatorID != 0 {
		imp = impersonatorID
	}
	_, _ = d.ExecContext(ctx, `INSERT INTO audit_log(actor_id, impersonator_id, action, target, detail) VALUES(?,?,?,?,?)`,
		actorID, imp, action, target, detail)
}

// AuditEntry is one audit row.
type AuditEntry struct {
	ID             int64  `json:"id"`
	ActorID        int64  `json:"actor_id"`
	ActorName      string `json:"actor"`
	ImpersonatorID int64  `json:"impersonator_id"`
	Action         string `json:"action"`
	Target         string `json:"target"`
	Detail         string `json:"detail"`
	CreatedAt      string `json:"created_at"`
}

// ListAudit returns the newest entries.
func (d *DB) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := d.QueryContext(ctx, `SELECT a.id, COALESCE(a.actor_id,0), COALESCE(u.username,''), COALESCE(a.impersonator_id,0), a.action, a.target, a.detail, a.created_at
		FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id ORDER BY a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorName, &e.ImpersonatorID, &e.Action, &e.Target, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if out == nil {
		out = []AuditEntry{}
	}
	return out, rows.Err()
}
