package ols

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleState() *State {
	return &State{
		PanelCert:   "/var/lib/olspanel/panel-ssl/panel.crt",
		PanelKey:    "/var/lib/olspanel/panel-ssl/panel.key",
		PMARoot:     "/usr/local/olspanel/phpmyadmin",
		PMAPhpBin:   "/usr/local/lsws/lsphp83/bin/lsphp",
		SuspendRoot: "/usr/local/olspanel/share/suspended",
		DefaultRoot: "/usr/local/olspanel/share/default",
		Processors: []Processor{
			{Name: "alice_php83", User: "alice", Group: "alice", Version: "83", LsphpBin: "/usr/local/lsws/lsphp83/bin/lsphp"},
		},
		VHosts: []VHost{
			{
				Name: "example.com", User: "alice", Group: "alice", Root: "/home/alice/domains/example.com",
				Aliases: []string{"www.example.com", "example.net", "www.example.net"}, Processor: "alice_php83", PHPVersion: "83",
				SSLCert: "/var/lib/olspanel/ssl/example.com/fullchain.pem", SSLKey: "/var/lib/olspanel/ssl/example.com/privkey.pem",
				ForceHTTPS: true, ACMEDir: "/var/lib/olspanel/acme-challenge/example.com", OpenBase: "/home/alice/domains/example.com:/tmp",
			},
			{
				Name: "blog.example.com", User: "alice", Group: "alice", Root: "/home/alice/domains/blog.example.com",
				Aliases: []string{"www.blog.example.com"}, Processor: "alice_php83", PHPVersion: "83",
				ACMEDir: "/var/lib/olspanel/acme-challenge/blog.example.com", OpenBase: "/home/alice/domains/blog.example.com:/tmp",
			},
		},
	}
}

func TestRenderProducesAllFiles(t *testing.T) {
	files, err := Render(sampleState())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{FileProcessors, FileVHosts, FileListeners, "vhosts/_default/vhconf.conf", "vhosts/_pma/vhconf.conf", "vhosts/example.com/vhconf.conf", "vhosts/blog.example.com/vhconf.conf"} {
		if _, ok := files[want]; !ok {
			t.Errorf("missing rendered file %s", want)
		}
	}
	l := files[FileListeners]
	if !strings.Contains(l, "map                     example.com example.com, www.example.com, example.net, www.example.net") {
		t.Errorf("listener map line wrong:\n%s", l)
	}
	if !strings.Contains(l, "map                     _default *") {
		t.Errorf("catch-all map missing")
	}
	if !strings.Contains(l, "address                 127.0.0.1:8081") {
		t.Errorf("pma listener missing")
	}
	v := files["vhosts/example.com/vhconf.conf"]
	for _, want := range []string{
		"add                     lsapi:alice_php83 php",
		"keyFile                 /var/lib/olspanel/ssl/example.com/privkey.pem",
		"RewriteCond %{HTTPS} !on",
		"location                /var/lib/olspanel/acme-challenge/example.com/.well-known/acme-challenge",
		`php_admin_value open_basedir "/home/alice/domains/example.com:/tmp"`,
		"vhAliases                 www.example.com, example.net, www.example.net",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("vhconf missing %q:\n%s", want, v)
		}
	}
	b := files["vhosts/blog.example.com/vhconf.conf"]
	if strings.Contains(b, "vhssl") || strings.Contains(b, "RewriteCond %{HTTPS}") {
		t.Errorf("vhost without cert must not render vhssl / https redirect")
	}
	p := files[FileProcessors]
	if !strings.Contains(p, "extUser                 alice") || !strings.Contains(p, "extprocessor pma_php") {
		t.Errorf("processors wrong:\n%s", p)
	}
}

func TestRenderCustomPorts(t *testing.T) {
	st := sampleState()
	st.HTTPPort, st.HTTPSPort = 18080, 18443
	files, err := Render(st)
	if err != nil {
		t.Fatal(err)
	}
	l := files[FileListeners]
	if !strings.Contains(l, "address                 *:18080") || !strings.Contains(l, "address                 *:18443") {
		t.Errorf("custom ports not rendered:\n%s", l)
	}
	st.HTTPPort, st.HTTPSPort = 80, 80
	if _, err := Render(st); err == nil {
		t.Errorf("equal ports must be rejected")
	}
}

func TestRenderSuspended(t *testing.T) {
	st := sampleState()
	st.VHosts[0].Suspended = true
	files, err := Render(st)
	if err != nil {
		t.Fatal(err)
	}
	v := files["vhosts/example.com/vhconf.conf"]
	if !strings.Contains(v, "docRoot                   /usr/local/olspanel/share/suspended") {
		t.Errorf("suspended vhost should use suspend root:\n%s", v)
	}
	if strings.Contains(v, "scripthandler") {
		t.Errorf("suspended vhost must not run PHP")
	}
	if !strings.Contains(files[FileVHosts], "enableScript            0") {
		t.Errorf("suspended vhost must have enableScript 0")
	}
}

func TestRenderRejectsInjection(t *testing.T) {
	cases := []func(*State){
		func(s *State) { s.VHosts[0].Name = "evil.com }\nvirtualhost x {" },
		func(s *State) { s.VHosts[0].User = "root; rm -rf /" },
		func(s *State) { s.VHosts[0].Root = "/home/alice/../root" },
		func(s *State) { s.VHosts[0].Aliases = []string{"bad alias"} },
		func(s *State) { s.VHosts[0].Processor = "missing" },
		func(s *State) { s.VHosts[0].OpenBase = `/tmp" evil` },
		func(s *State) { s.VHosts = append(s.VHosts, s.VHosts[0]) },
		func(s *State) { s.PanelCert = "relative/path.crt" },
		func(s *State) { s.Processors[0].LsphpBin = "/usr/local/lsws/lsphp83/bin/lsphp\nextUser root" },
	}
	for i, mut := range cases {
		st := sampleState()
		mut(st)
		if _, err := Render(st); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func TestApplyDryRunWritesAndPrunes(t *testing.T) {
	dir := t.TempDir()
	// stale vhost that must be pruned
	stale := filepath.Join(dir, "vhosts", "old.example.org")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(stale, "vhconf.conf"), []byte("x"), 0o644)
	// stock Example vhost must survive
	_ = os.MkdirAll(filepath.Join(dir, "vhosts", "Example"), 0o755)

	m := &Manager{ConfDir: dir, DryRun: true, Build: func(context.Context) (*State, error) { return sampleState(), nil }}
	if err := m.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "olspanel", "20-listeners.conf")); err != nil {
		t.Errorf("listeners file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vhosts", "example.com", "vhconf.conf")); err != nil {
		t.Errorf("vhconf not written: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale vhost dir should be pruned")
	}
	if _, err := os.Stat(filepath.Join(dir, "vhosts", "Example")); err != nil {
		t.Errorf("Example vhost dir must be kept")
	}
	// second apply is idempotent
	if err := m.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotRestore(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{ConfDir: dir, DryRun: true}
	if err := m.ApplyState(context.Background(), sampleState()); err != nil {
		t.Fatal(err)
	}
	snap, err := m.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap) == 0 {
		t.Fatal("empty snapshot")
	}
	// mutate on disk, then restore
	target := filepath.Join(dir, "olspanel", "10-vhosts.conf")
	_ = os.WriteFile(target, []byte("broken"), 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "vhosts", "new.example"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "vhosts", "new.example", "vhconf.conf"), []byte("x"), 0o600)
	if err := m.restore(snap); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(target)
	if string(b) == "broken" {
		t.Errorf("restore did not revert file")
	}
	if _, err := os.Stat(filepath.Join(dir, "vhosts", "new.example", "vhconf.conf")); !os.IsNotExist(err) {
		t.Errorf("restore should remove files not in snapshot")
	}
}
