package validate

import (
	"strings"
	"testing"
)

func TestUsername(t *testing.T) {
	good := []string{"alice", "bob123", "a12", "abcdefghijklmnop"}
	bad := []string{"", "Al", "1abc", "root", "admin", "nobody", "lsadm", "mysql", "a-b", "ab", "abcdefghijklmnopq", "al ice", "a;b", "ä"}
	for _, g := range good {
		if err := Username(g); err != nil {
			t.Errorf("%q should be valid: %v", g, err)
		}
	}
	for _, b := range bad {
		if err := Username(b); err == nil {
			t.Errorf("%q should be invalid", b)
		}
	}
}

func TestDomain(t *testing.T) {
	cases := map[string]string{
		"Example.COM":    "example.com",
		"example.com.":   "example.com",
		"  blog.a-b.pl ": "blog.a-b.pl",
		"sub.sub.x.org":  "sub.sub.x.org",
	}
	for in, want := range cases {
		got, err := Domain(in)
		if err != nil || got != want {
			t.Errorf("Domain(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if got, err := Domain("zażółć.pl"); err != nil || !strings.HasPrefix(got, "xn--") || !strings.HasSuffix(got, ".pl") {
		t.Errorf("IDN not converted to punycode: %q %v", got, err)
	}
	bad := []string{"", "localhost", "foo.localhost", "nodot", "-bad.com", "bad-.com", "a..b", "a b.com", "evil.com }\nvirtualhost", "a_b.com", "x.com;rm"}
	for _, b := range bad {
		if _, err := Domain(b); err == nil {
			t.Errorf("%q should be invalid", b)
		}
	}
}

func TestSuffixAndPHP(t *testing.T) {
	if err := Suffix("wp_1"); err != nil {
		t.Error(err)
	}
	for _, b := range []string{"", "A", "a-b", "x y", "abcdefghijklmnopqrstuvwxy"} {
		if err := Suffix(b); err == nil {
			t.Errorf("suffix %q should be invalid", b)
		}
	}
	for _, g := range []string{"81", "84"} {
		if err := PHPVersion(g); err != nil {
			t.Error(err)
		}
	}
	for _, b := range []string{"7", "8.3", "83x", "9"} {
		if err := PHPVersion(b); err == nil {
			t.Errorf("php %q should be invalid", b)
		}
	}
}

func TestCronCommand(t *testing.T) {
	if err := CronCommand("php /home/a/domains/x/public_html/cron.php"); err != nil {
		t.Error(err)
	}
	for _, b := range []string{"", "echo hi\nrm -rf /", "x\x00y"} {
		if err := CronCommand(b); err == nil {
			t.Errorf("%q should be invalid", b)
		}
	}
}

func TestPassword(t *testing.T) {
	if err := Password("abcdefgh"); err != nil {
		t.Error(err)
	}
	for _, b := range []string{"short", "with\nnewline123"} {
		if err := Password(b); err == nil {
			t.Errorf("%q should be invalid", b)
		}
	}
}
