-- +goose Up
CREATE TABLE packages (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    disk_mb       INTEGER NOT NULL DEFAULT 1024,
    max_domains   INTEGER NOT NULL DEFAULT 1,
    max_subdomains INTEGER NOT NULL DEFAULT 5,
    max_databases INTEGER NOT NULL DEFAULT 1,
    max_ftp       INTEGER NOT NULL DEFAULT 1,
    max_cron      INTEGER NOT NULL DEFAULT 5,
    php_versions  TEXT NOT NULL DEFAULT '["83"]',
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    role          TEXT NOT NULL CHECK (role IN ('admin','user')),
    password_hash TEXT NOT NULL,
    email         TEXT NOT NULL DEFAULT '',
    package_id    INTEGER REFERENCES packages(id),
    uid           INTEGER,
    gid           INTEGER,
    home          TEXT NOT NULL DEFAULT '',
    disk_used_mb  INTEGER NOT NULL DEFAULT 0,
    suspended     INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE domains (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL UNIQUE,
    parent_id     INTEGER REFERENCES domains(id) ON DELETE CASCADE,
    type          TEXT NOT NULL CHECK (type IN ('domain','subdomain','alias')),
    php_version   TEXT NOT NULL DEFAULT '83',
    ssl_status    TEXT NOT NULL DEFAULT 'none',
    ssl_expires_at TEXT,
    force_https   INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_domains_user ON domains(user_id);

CREATE TABLE databases (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE db_users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    database_id INTEGER NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
    username    TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE ftp_accounts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    login       TEXT NOT NULL UNIQUE,
    home_subdir TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE cron_jobs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    schedule   TEXT NOT NULL,
    command    TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE sessions (
    token  TEXT PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry REAL NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions(expiry);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_id        INTEGER,
    impersonator_id INTEGER,
    action          TEXT NOT NULL,
    target          TEXT NOT NULL DEFAULT '',
    detail          TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- +goose Down
DROP TABLE audit_log;
DROP TABLE settings;
DROP TABLE sessions;
DROP TABLE cron_jobs;
DROP TABLE ftp_accounts;
DROP TABLE db_users;
DROP TABLE databases;
DROP TABLE domains;
DROP TABLE users;
DROP TABLE packages;
