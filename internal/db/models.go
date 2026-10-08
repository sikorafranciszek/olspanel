package db

// User is a panel account (admin or hosting user).
type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
	Email        string `json:"email"`
	PackageID    int64  `json:"package_id"`
	PackageName  string `json:"package_name"`
	UID          int64  `json:"uid"`
	GID          int64  `json:"gid"`
	Home         string `json:"home"`
	DiskUsedMB   int64  `json:"disk_used_mb"`
	Suspended    bool   `json:"suspended"`
	CreatedAt    string `json:"created_at"`
	DomainCount  int64  `json:"domain_count"`
}

// Package defines hosting limits.
type Package struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	DiskMB        int64    `json:"disk_mb"`
	MaxDomains    int64    `json:"max_domains"`
	MaxSubdomains int64    `json:"max_subdomains"`
	MaxDatabases  int64    `json:"max_databases"`
	MaxFTP        int64    `json:"max_ftp"`
	MaxCron       int64    `json:"max_cron"`
	PHPVersions   []string `json:"php_versions"`
	UserCount     int64    `json:"user_count"`
	CreatedAt     string   `json:"created_at"`
}

// Domain is a vhost (domain, subdomain or alias).
type Domain struct {
	ID           int64   `json:"id"`
	UserID       int64   `json:"user_id"`
	Username     string  `json:"username"`
	Name         string  `json:"name"`
	ParentID     *int64  `json:"parent_id"`
	Type         string  `json:"type"`
	PHPVersion   string  `json:"php_version"`
	SSLStatus    string  `json:"ssl_status"`
	SSLExpiresAt *string `json:"ssl_expires_at"`
	ForceHTTPS   bool    `json:"force_https"`
	CreatedAt    string  `json:"created_at"`
}

// Database is a MariaDB database owned by a user.
type Database struct {
	ID        int64    `json:"id"`
	UserID    int64    `json:"user_id"`
	Name      string   `json:"name"`
	Users     []DBUser `json:"users"`
	CreatedAt string   `json:"created_at"`
}

// DBUser is a MariaDB user bound to one database.
type DBUser struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	DatabaseID int64  `json:"database_id"`
	Username   string `json:"username"`
	CreatedAt  string `json:"created_at"`
}

// FTPAccount is a pure-ftpd virtual user.
type FTPAccount struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Login      string `json:"login"`
	HomeSubdir string `json:"home_subdir"`
	CreatedAt  string `json:"created_at"`
}

// CronJob is one crontab line.
type CronJob struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Schedule  string `json:"schedule"`
	Command   string `json:"command"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}
