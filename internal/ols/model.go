// Package ols renders and applies OpenLiteSpeed configuration. The panel is
// the single source of truth: it owns conf/olspanel/*.conf (included from
// httpd_config.conf) and conf/vhosts/<domain>/vhconf.conf.
package ols

// Processor is one lsapi external application: one per (account, PHP version).
type Processor struct {
	Name     string // e.g. alice_php83
	User     string
	Group    string
	Version  string // "83"
	LsphpBin string // /usr/local/lsws/lsphp83/bin/lsphp
}

// VHost is one virtual host rendered from a domain row.
type VHost struct {
	Name       string // canonical domain, also vhost name
	User       string
	Group      string
	Root       string   // /home/<u>/domains/<d>
	Aliases    []string // additional host names mapped to this vhost (www., aliases)
	Processor  string   // Processor.Name
	PHPVersion string
	SSLKey     string // empty = no vhssl block
	SSLCert    string
	ForceHTTPS bool
	Suspended  bool
	ACMEDir    string // /var/lib/olspanel/acme-challenge/<d>
	OpenBase   string // open_basedir value
}

// State is everything needed to render the full OLS configuration.
type State struct {
	Processors  []Processor
	VHosts      []VHost
	PanelCert   string // fallback cert for the SSL listener
	PanelKey    string
	PMARoot     string // phpMyAdmin document root ("" = disabled)
	PMAPhpBin   string // lsphp binary used for phpMyAdmin
	SuspendRoot string // docroot shown for suspended accounts
	DefaultRoot string // docroot for unmatched hosts
}
