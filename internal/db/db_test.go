package db

import (
	"context"
	"testing"
)

func open(t *testing.T) *DB {
	t.Helper()
	d, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestUsersPackagesDomainsCascade(t *testing.T) {
	ctx := context.Background()
	d := open(t)
	pid, err := d.CreatePackage(ctx, &Package{Name: "Basic", DiskMB: 100, MaxDomains: 2, MaxSubdomains: 2, MaxDatabases: 1, MaxFTP: 1, MaxCron: 1, PHPVersions: []string{"83", "84"}})
	if err != nil {
		t.Fatal(err)
	}
	uid, err := d.CreateUser(ctx, &User{Username: "alice", Role: "user", PasswordHash: "x", PackageID: pid, UID: 1001, GID: 1001, Home: "/home/alice"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := d.UserByID(ctx, uid)
	if err != nil || u.PackageName != "Basic" || u.UID != 1001 {
		t.Fatalf("user: %+v %v", u, err)
	}
	if err := d.DeletePackage(ctx, pid); err == nil {
		t.Errorf("deleting package in use must fail")
	}
	did, err := d.CreateDomain(ctx, &Domain{UserID: uid, Name: "example.com", Type: "domain", PHPVersion: "83"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateDomain(ctx, &Domain{UserID: uid, Name: "sub.example.com", Type: "subdomain", ParentID: &did, PHPVersion: "84"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateDomain(ctx, &Domain{UserID: uid, Name: "example.com", Type: "domain"}); err == nil {
		t.Errorf("duplicate domain must fail")
	}
	n, _ := d.CountUserDomains(ctx, uid, "subdomain")
	if n != 1 {
		t.Errorf("subdomain count = %d", n)
	}
	if _, err := d.CreateDatabase(ctx, uid, "alice_wp"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateCron(ctx, &CronJob{UserID: uid, Schedule: "* * * * *", Command: "x", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// deleting the parent domain cascades to the subdomain
	if err := d.DeleteDomain(ctx, did); err != nil {
		t.Fatal(err)
	}
	list, _ := d.ListUserDomains(ctx, uid)
	if len(list) != 0 {
		t.Errorf("cascade failed: %+v", list)
	}
	// deleting the user cascades to databases and cron
	if err := d.DeleteUser(ctx, uid); err != nil {
		t.Fatal(err)
	}
	dbs, _ := d.ListUserDatabases(ctx, uid)
	cr, _ := d.ListUserCron(ctx, uid)
	if len(dbs) != 0 || len(cr) != 0 {
		t.Errorf("user cascade failed")
	}
	if err := d.DeletePackage(ctx, pid); err != nil {
		t.Errorf("package delete after user gone: %v", err)
	}
}

func TestSettingsAndAudit(t *testing.T) {
	ctx := context.Background()
	d := open(t)
	if got := d.Setting(ctx, "x", "def"); got != "def" {
		t.Errorf("default = %q", got)
	}
	_ = d.SetSetting(ctx, "x", "1")
	_ = d.SetSetting(ctx, "x", "2")
	if got := d.Setting(ctx, "x", ""); got != "2" {
		t.Errorf("upsert = %q", got)
	}
	d.Audit(ctx, 0, 0, "login.failed", "bob", "1.2.3.4")
	list, err := d.ListAudit(ctx, 10)
	if err != nil || len(list) != 1 || list[0].Action != "login.failed" {
		t.Errorf("audit: %+v %v", list, err)
	}
}
