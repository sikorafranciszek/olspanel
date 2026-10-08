package db

import (
	"context"
	"database/sql"
	"errors"
)

// ListUserDatabases returns databases with their users.
func (d *DB) ListUserDatabases(ctx context.Context, userID int64) ([]Database, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, user_id, name, created_at FROM databases WHERE user_id=? ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Database{}
	for rows.Next() {
		var x Database
		if err := rows.Scan(&x.ID, &x.UserID, &x.Name, &x.CreatedAt); err != nil {
			return nil, err
		}
		x.Users = []DBUser{}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	urows, err := d.QueryContext(ctx, `SELECT id, user_id, database_id, username, created_at FROM db_users WHERE user_id=? ORDER BY username`, userID)
	if err != nil {
		return nil, err
	}
	defer urows.Close()
	for urows.Next() {
		var u DBUser
		if err := urows.Scan(&u.ID, &u.UserID, &u.DatabaseID, &u.Username, &u.CreatedAt); err != nil {
			return nil, err
		}
		for i := range out {
			if out[i].ID == u.DatabaseID {
				out[i].Users = append(out[i].Users, u)
			}
		}
	}
	return out, urows.Err()
}

// DatabaseByID fetches one database.
func (d *DB) DatabaseByID(ctx context.Context, id int64) (*Database, error) {
	var x Database
	err := d.QueryRowContext(ctx, `SELECT id, user_id, name, created_at FROM databases WHERE id=?`, id).Scan(&x.ID, &x.UserID, &x.Name, &x.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &x, err
}

// CreateDatabase inserts a database row.
func (d *DB) CreateDatabase(ctx context.Context, userID int64, name string) (int64, error) {
	res, err := d.ExecContext(ctx, `INSERT INTO databases(user_id, name) VALUES(?,?)`, userID, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteDatabase removes a database row.
func (d *DB) DeleteDatabase(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM databases WHERE id=?`, id)
	return err
}

// CountUserDatabases counts a user's databases.
func (d *DB) CountUserDatabases(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM databases WHERE user_id=?`, userID).Scan(&n)
	return n, err
}

// DBUserByID fetches one db user.
func (d *DB) DBUserByID(ctx context.Context, id int64) (*DBUser, error) {
	var u DBUser
	err := d.QueryRowContext(ctx, `SELECT id, user_id, database_id, username, created_at FROM db_users WHERE id=?`, id).Scan(&u.ID, &u.UserID, &u.DatabaseID, &u.Username, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// ListDBUsersOf lists db users of a database.
func (d *DB) ListDBUsersOf(ctx context.Context, dbID int64) ([]DBUser, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, user_id, database_id, username, created_at FROM db_users WHERE database_id=?`, dbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DBUser{}
	for rows.Next() {
		var u DBUser
		if err := rows.Scan(&u.ID, &u.UserID, &u.DatabaseID, &u.Username, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CreateDBUser inserts a db user row.
func (d *DB) CreateDBUser(ctx context.Context, userID, dbID int64, username string) (int64, error) {
	res, err := d.ExecContext(ctx, `INSERT INTO db_users(user_id, database_id, username) VALUES(?,?,?)`, userID, dbID, username)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteDBUser removes a db user row.
func (d *DB) DeleteDBUser(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM db_users WHERE id=?`, id)
	return err
}

// ListUserFTP lists FTP accounts of a user.
func (d *DB) ListUserFTP(ctx context.Context, userID int64) ([]FTPAccount, error) {
	return d.queryFTP(ctx, `WHERE user_id=?`, userID)
}

// ListAllFTP lists every FTP account.
func (d *DB) ListAllFTP(ctx context.Context) ([]FTPAccount, error) {
	return d.queryFTP(ctx, "")
}

func (d *DB) queryFTP(ctx context.Context, where string, args ...any) ([]FTPAccount, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, user_id, login, home_subdir, created_at FROM ftp_accounts `+where+` ORDER BY login`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FTPAccount{}
	for rows.Next() {
		var a FTPAccount
		if err := rows.Scan(&a.ID, &a.UserID, &a.Login, &a.HomeSubdir, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// FTPByID fetches one FTP account.
func (d *DB) FTPByID(ctx context.Context, id int64) (*FTPAccount, error) {
	var a FTPAccount
	err := d.QueryRowContext(ctx, `SELECT id, user_id, login, home_subdir, created_at FROM ftp_accounts WHERE id=?`, id).Scan(&a.ID, &a.UserID, &a.Login, &a.HomeSubdir, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &a, err
}

// CreateFTP inserts an FTP account row.
func (d *DB) CreateFTP(ctx context.Context, a *FTPAccount) (int64, error) {
	res, err := d.ExecContext(ctx, `INSERT INTO ftp_accounts(user_id, login, home_subdir) VALUES(?,?,?)`, a.UserID, a.Login, a.HomeSubdir)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteFTP removes an FTP account row.
func (d *DB) DeleteFTP(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM ftp_accounts WHERE id=?`, id)
	return err
}

// CountUserFTP counts a user's FTP accounts.
func (d *DB) CountUserFTP(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM ftp_accounts WHERE user_id=?`, userID).Scan(&n)
	return n, err
}

// ListUserCron lists cron jobs of a user.
func (d *DB) ListUserCron(ctx context.Context, userID int64) ([]CronJob, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, user_id, schedule, command, enabled, created_at FROM cron_jobs WHERE user_id=? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CronJob{}
	for rows.Next() {
		var j CronJob
		var en int
		if err := rows.Scan(&j.ID, &j.UserID, &j.Schedule, &j.Command, &en, &j.CreatedAt); err != nil {
			return nil, err
		}
		j.Enabled = en == 1
		out = append(out, j)
	}
	return out, rows.Err()
}

// CronByID fetches one cron job.
func (d *DB) CronByID(ctx context.Context, id int64) (*CronJob, error) {
	var j CronJob
	var en int
	err := d.QueryRowContext(ctx, `SELECT id, user_id, schedule, command, enabled, created_at FROM cron_jobs WHERE id=?`, id).Scan(&j.ID, &j.UserID, &j.Schedule, &j.Command, &en, &j.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	j.Enabled = en == 1
	return &j, err
}

// CreateCron inserts a cron job.
func (d *DB) CreateCron(ctx context.Context, j *CronJob) (int64, error) {
	en := 0
	if j.Enabled {
		en = 1
	}
	res, err := d.ExecContext(ctx, `INSERT INTO cron_jobs(user_id, schedule, command, enabled) VALUES(?,?,?,?)`, j.UserID, j.Schedule, j.Command, en)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateCron updates a cron job.
func (d *DB) UpdateCron(ctx context.Context, j *CronJob) error {
	en := 0
	if j.Enabled {
		en = 1
	}
	_, err := d.ExecContext(ctx, `UPDATE cron_jobs SET schedule=?, command=?, enabled=? WHERE id=?`, j.Schedule, j.Command, en, j.ID)
	return err
}

// DeleteCron removes a cron job.
func (d *DB) DeleteCron(ctx context.Context, id int64) error {
	_, err := d.ExecContext(ctx, `DELETE FROM cron_jobs WHERE id=?`, id)
	return err
}

// CountUserCron counts a user's cron jobs.
func (d *DB) CountUserCron(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM cron_jobs WHERE user_id=?`, userID).Scan(&n)
	return n, err
}
