// Package config holds runtime configuration for olspanel.
package config

import (
	"os"
	"path/filepath"
	"strconv"
)

// Config describes paths and listen settings. All paths are absolute.
type Config struct {
	ListenAddr   string // e.g. ":2222"
	DataDir      string // /var/lib/olspanel
	LogDir       string // /var/log/olspanel
	HomeRoot     string // /home
	LswsRoot     string // /usr/local/lsws
	PanelCert    string
	PanelKey     string
	DBPath       string
	PMAUpstream  string // http://127.0.0.1:8081
	MySQLSocket  string
	DevMode      bool   // serve SPA from Vite dev server instead of embed
	DevSPAOrigin string // http://localhost:5173
	InsecureHTTP bool   // listen on plain HTTP (development only)
}

// FromEnv builds a Config from environment variables with sane defaults.
func FromEnv() Config {
	c := Config{
		ListenAddr:   envOr("OLSPANEL_LISTEN", ":2222"),
		DataDir:      envOr("OLSPANEL_DATA_DIR", "/var/lib/olspanel"),
		LogDir:       envOr("OLSPANEL_LOG_DIR", "/var/log/olspanel"),
		HomeRoot:     envOr("OLSPANEL_HOME_ROOT", "/home"),
		LswsRoot:     envOr("OLSPANEL_LSWS_ROOT", "/usr/local/lsws"),
		PMAUpstream:  envOr("OLSPANEL_PMA_UPSTREAM", "http://127.0.0.1:8081"),
		MySQLSocket:  envOr("OLSPANEL_MYSQL_SOCKET", "/run/mysqld/mysqld.sock"),
		DevSPAOrigin: envOr("OLSPANEL_DEV_SPA", ""),
	}
	c.DevMode = c.DevSPAOrigin != ""
	c.InsecureHTTP = os.Getenv("OLSPANEL_INSECURE_HTTP") == "1"
	c.DBPath = envOr("OLSPANEL_DB", filepath.Join(c.DataDir, "panel.db"))
	c.PanelCert = envOr("OLSPANEL_TLS_CERT", filepath.Join(c.DataDir, "panel-ssl", "panel.crt"))
	c.PanelKey = envOr("OLSPANEL_TLS_KEY", filepath.Join(c.DataDir, "panel-ssl", "panel.key"))
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnvInt reads an integer env var with a default.
func EnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
