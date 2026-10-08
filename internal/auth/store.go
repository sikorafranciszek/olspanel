package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"olspanel/internal/db"
)

// sqliteStore implements scs.Store / scs.CtxStore on the sessions table.
type sqliteStore struct {
	db *db.DB
}

func newSQLiteStore(d *db.DB) *sqliteStore {
	s := &sqliteStore{db: d}
	go s.cleanupLoop()
	return s
}

func (s *sqliteStore) cleanupLoop() {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for range t.C {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE expiry < ?`, float64(time.Now().UnixNano())/1e9)
	}
}

func (s *sqliteStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

func (s *sqliteStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	var data []byte
	now := float64(time.Now().UnixNano()) / 1e9
	err := s.db.QueryRowContext(ctx, `SELECT data FROM sessions WHERE token = ? AND expiry >= ?`, token, now).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *sqliteStore) Commit(token string, b []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, b, expiry)
}

func (s *sqliteStore) CommitCtx(ctx context.Context, token string, b []byte, expiry time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(token, data, expiry) VALUES(?,?,?)
		ON CONFLICT(token) DO UPDATE SET data=excluded.data, expiry=excluded.expiry`,
		token, b, float64(expiry.UnixNano())/1e9)
	return err
}

func (s *sqliteStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}

func (s *sqliteStore) DeleteCtx(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}
