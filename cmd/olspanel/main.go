// Command olspanel is the hosting control panel daemon and CLI.
//
//	olspanel serve                      run the HTTPS server (systemd)
//	olspanel init --admin-password PW   create database, settings and admin
//	olspanel admin-reset-password       reset the "admin" password
//	olspanel doctor                     check host prerequisites
//	olspanel version
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"olspanel/internal/acme"
	"olspanel/internal/api"
	"olspanel/internal/auth"
	"olspanel/internal/config"
	"olspanel/internal/cron"
	"olspanel/internal/db"
	"olspanel/internal/domains"
	"olspanel/internal/files"
	"olspanel/internal/ftp"
	"olspanel/internal/mariadb"
	"olspanel/internal/ols"
	"olspanel/internal/php"
	"olspanel/internal/scheduler"
	"olspanel/internal/system"
	"olspanel/internal/users"
	"olspanel/internal/web"
)

// Version is set at build time with -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	logLevel := slog.LevelInfo
	if os.Getenv("OLSPANEL_DEBUG") != "" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "init":
		err = initCmd(os.Args[2:])
	case "admin-reset-password":
		err = resetPassword(os.Args[2:])
	case "doctor":
		err = doctor()
	case "version":
		fmt.Println("olspanel", Version, runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "błąd:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "użycie: olspanel <serve|init|admin-reset-password|doctor|version> [opcje]")
}

// build wires every service. noSystem disables privileged host operations.
func build(ctx context.Context, cfg config.Config, noSystem bool) (*api.Server, error) {
	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, err
	}
	shareDir := "/usr/local/olspanel/share"
	pmaRoot := "/usr/local/olspanel/phpmyadmin"
	if _, err := os.Stat(filepath.Join(pmaRoot, "index.php")); err != nil {
		pmaRoot = ""
	}
	olsMgr := &ols.Manager{ConfDir: filepath.Join(cfg.LswsRoot, "conf"), DryRun: noSystem}
	domainSvc := &domains.Service{
		DB: database, OLS: olsMgr, LswsRoot: cfg.LswsRoot, DataDir: cfg.DataDir, ShareDir: shareDir,
		PMARoot: pmaRoot, PanelCert: cfg.PanelCert, PanelKey: cfg.PanelKey, NoSystem: noSystem,
	}
	olsMgr.Build = domainSvc.BuildState
	mariaSvc := &mariadb.Service{DB: database, Socket: cfg.MySQLSocket, NoSystem: noSystem}
	cronSvc := &cron.Service{DB: database, NoSystem: noSystem}
	ftpSvc := &ftp.Service{DB: database, NoSystem: noSystem}
	userSvc := &users.Service{DB: database, HomeRoot: cfg.HomeRoot, OLS: olsMgr, NoSystem: noSystem,
		Cleaners: []users.Cleaner{domainSvc, mariaSvc, cronSvc, ftpSvc}}
	acmeSvc := &acme.Service{DB: database, DataDir: cfg.DataDir, OLS: olsMgr, NoSystem: noSystem}
	acmeSvc.OnPanelCert = func(certPEM, keyPEM []byte) {
		_ = os.WriteFile(cfg.PanelKey, keyPEM, 0o600)
		_ = os.WriteFile(cfg.PanelCert, certPEM, 0o644)
		if err := ftp.InstallCert(ctx, certPEM, keyPEM); err != nil {
			slog.Warn("ftp cert install", "err", err)
		}
		if err := olsMgr.Apply(ctx); err != nil {
			slog.Warn("ols apply after panel cert", "err", err)
		}
	}
	uploadMB := int64(config.EnvInt("OLSPANEL_UPLOAD_MAX_MB", 512))
	srv := &api.Server{
		Cfg: cfg, DB: database, Auth: auth.New(database, !cfg.DevMode && !cfg.InsecureHTTP), Users: userSvc, Domains: domainSvc,
		OLS: olsMgr, MariaDB: mariaSvc, Files: &files.Service{MaxUploadMB: uploadMB}, FTP: ftpSvc, Cron: cronSvc,
		ACME: acmeSvc, Version: Version, Started: time.Now(), SPA: web.Handler(cfg.DevSPAOrigin),
	}
	return srv, nil
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	noSystem := fs.Bool("no-system", runtime.GOOS != "linux", "nie wykonuj operacji systemowych (tryb deweloperski)")
	_ = fs.Parse(args)
	cfg := config.FromEnv()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := build(ctx, cfg, *noSystem)
	if err != nil {
		return err
	}
	defer srv.DB.Close()

	hostname, _ := os.Hostname()
	if err := acme.EnsureSelfSigned(cfg.PanelCert, cfg.PanelKey, []string{hostname, "localhost", "127.0.0.1"}); err != nil {
		return fmt.Errorf("certyfikat panelu: %w", err)
	}

	sched := scheduler.New()
	sched.Every("disk-usage", time.Hour, func(ctx context.Context) { srv.Users.RefreshDiskUsage(ctx) })
	sched.Every("acme-renew", 12*time.Hour, func(ctx context.Context) { srv.ACME.RenewDue(ctx) })
	go sched.Run(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: certLoader(cfg.PanelCert, cfg.PanelKey),
		},
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	slog.Info("olspanel listening", "addr", cfg.ListenAddr, "version", Version, "no_system", *noSystem, "tls", !cfg.InsecureHTTP)
	var err2 error
	if cfg.InsecureHTTP {
		slog.Warn("OLSPANEL_INSECURE_HTTP=1: serving plain HTTP (development only)")
		err2 = httpSrv.ListenAndServe()
	} else {
		err2 = httpSrv.ListenAndServeTLS("", "")
	}
	if err2 != nil && !errors.Is(err2, http.ErrServerClosed) {
		return err2
	}
	return nil
}

// certLoader reloads the certificate from disk when it changes (ACME renewals).
func certLoader(certPath, keyPath string) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	var (
		cached  *tls.Certificate
		modTime time.Time
		last    time.Time
	)
	return func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		if cached != nil && time.Since(last) < 30*time.Second {
			return cached, nil
		}
		last = time.Now()
		st, err := os.Stat(certPath)
		if err == nil && cached != nil && st.ModTime().Equal(modTime) {
			return cached, nil
		}
		c, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			if cached != nil {
				return cached, nil
			}
			return nil, err
		}
		cached = &c
		if st != nil {
			modTime = st.ModTime()
		}
		return cached, nil
	}
}

func initCmd(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	adminPass := fs.String("admin-password", "", "hasło administratora (wymagane przy pierwszym uruchomieniu)")
	hostname := fs.String("hostname", "", "nazwa hosta panelu (opcjonalnie)")
	email := fs.String("email", "", "e-mail do Let's Encrypt (opcjonalnie)")
	_ = fs.Parse(args)
	cfg := config.FromEnv()
	ctx := context.Background()
	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()

	n, err := database.CountUsers(ctx, "admin")
	if err != nil {
		return err
	}
	if n == 0 {
		if *adminPass == "" {
			return errors.New("podaj --admin-password")
		}
		hash, err := auth.HashPassword(*adminPass)
		if err != nil {
			return err
		}
		if _, err := database.CreateUser(ctx, &db.User{Username: "admin", Role: "admin", PasswordHash: hash}); err != nil {
			return err
		}
		fmt.Println("utworzono administratora: admin")
	}
	pkgs, _ := database.ListPackages(ctx)
	if len(pkgs) == 0 {
		versions := []string{}
		for _, v := range php.Installed(cfg.LswsRoot) {
			versions = append(versions, v.ID)
		}
		if len(versions) == 0 {
			versions = []string{"83"}
		}
		_, _ = database.CreatePackage(ctx, &db.Package{Name: "Podstawowy", DiskMB: 5120, MaxDomains: 3, MaxSubdomains: 10, MaxDatabases: 3, MaxFTP: 3, MaxCron: 5, PHPVersions: versions})
		fmt.Println("utworzono pakiet: Podstawowy")
	}
	if *hostname != "" {
		_ = database.SetSetting(ctx, "panel_hostname", strings.ToLower(*hostname))
	}
	if *email != "" {
		_ = database.SetSetting(ctx, "acme_email", *email)
	}
	for _, d := range []string{filepath.Join(cfg.DataDir, "ssl"), filepath.Join(cfg.DataDir, "acme"), filepath.Join(cfg.DataDir, "acme-challenge", "_default", ".well-known", "acme-challenge"), filepath.Join(cfg.DataDir, "panel-ssl"), cfg.LogDir} {
		_ = os.MkdirAll(d, 0o755)
	}
	_ = os.Chmod(filepath.Join(cfg.DataDir, "ssl"), 0o700)
	_ = os.Chmod(filepath.Join(cfg.DataDir, "acme"), 0o700)
	host, _ := os.Hostname()
	if err := acme.EnsureSelfSigned(cfg.PanelCert, cfg.PanelKey, []string{host, *hostname, "localhost", "127.0.0.1"}); err != nil {
		return err
	}
	// Render the initial OLS configuration so the include files exist.
	srv, err := build(ctx, cfg, runtime.GOOS != "linux" || os.Getenv("OLSPANEL_NO_SYSTEM") != "")
	if err != nil {
		return err
	}
	defer srv.DB.Close()
	if err := srv.OLS.Apply(ctx); err != nil {
		slog.Warn("początkowa konfiguracja OLS", "err", err)
	}
	fmt.Println("inicjalizacja zakończona")
	return nil
}

func resetPassword(args []string) error {
	fs := flag.NewFlagSet("admin-reset-password", flag.ExitOnError)
	pass := fs.String("password", "", "nowe hasło")
	user := fs.String("user", "admin", "nazwa administratora")
	_ = fs.Parse(args)
	if *pass == "" {
		*pass = auth.RandomToken(12)
		fmt.Println("wygenerowane hasło:", *pass)
	}
	cfg := config.FromEnv()
	ctx := context.Background()
	database, err := db.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()
	u, err := database.UserByName(ctx, *user)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(*pass)
	if err != nil {
		return err
	}
	if err := database.SetPassword(ctx, u.ID, hash); err != nil {
		return err
	}
	fmt.Println("hasło zmienione dla", u.Username)
	return nil
}

func doctor() error {
	cfg := config.FromEnv()
	ok := true
	check := func(name string, good bool, hint string) {
		mark := "OK "
		if !good {
			mark = "BRAK"
			ok = false
		}
		fmt.Printf("[%s] %s %s\n", mark, name, hint)
	}
	check("OpenLiteSpeed", system.Available("openlitespeed"), cfg.LswsRoot)
	check("lswsctrl", system.Available("lswsctrl"), "")
	versions := php.Installed(cfg.LswsRoot)
	ids := []string{}
	for _, v := range versions {
		ids = append(ids, v.ID)
	}
	check("LSPHP", len(versions) > 0, strings.Join(ids, ","))
	check("MariaDB socket", fileExists(cfg.MySQLSocket), cfg.MySQLSocket)
	check("pure-pw", system.Available("pure-pw"), "")
	check("crontab", system.Available("crontab"), "")
	check("setfacl", system.Available("setfacl"), "")
	check("katalog danych", fileExists(cfg.DataDir), cfg.DataDir)
	check("certyfikat panelu", fileExists(cfg.PanelCert) && fileExists(cfg.PanelKey), cfg.PanelCert)
	check("httpd_config include", includeConfigured(cfg.LswsRoot), "include $SERVER_ROOT/conf/olspanel/*.conf")
	check("phpMyAdmin", fileExists("/usr/local/olspanel/phpmyadmin/index.php"), "/usr/local/olspanel/phpmyadmin")
	if runtime.GOOS == "linux" {
		check("OLS odpowiada na :80", ols.Healthy(), "")
	}
	if !ok {
		return errors.New("niektóre wymagania nie są spełnione")
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func includeConfigured(lsws string) bool {
	b, err := os.ReadFile(filepath.Join(lsws, "conf", "httpd_config.conf"))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "conf/olspanel/*.conf")
}
