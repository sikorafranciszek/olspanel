package files

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"olspanel/internal/db"
)

func setup(t *testing.T) (*Service, *db.User, string) {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	outside := filepath.Join(base, "outside")
	_ = os.MkdirAll(filepath.Join(home, "domains", "x.com", "public_html"), 0o755)
	_ = os.MkdirAll(outside, 0o755)
	_ = os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644)
	_ = os.WriteFile(filepath.Join(home, "domains", "x.com", "public_html", "index.php"), []byte("<?php echo 1;"), 0o644)
	if runtime.GOOS != "windows" {
		_ = os.Symlink(outside, filepath.Join(home, "escape"))
	}
	return &Service{}, &db.User{Home: home}, outside
}

func TestListAndRead(t *testing.T) {
	s, u, _ := setup(t)
	list, err := s.List(u, "domains/x.com/public_html")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "index.php" || list[0].Dir {
		t.Fatalf("unexpected listing: %+v", list)
	}
	b, err := s.Read(u, "/domains/x.com/public_html/index.php")
	if err != nil || string(b) != "<?php echo 1;" {
		t.Fatalf("read: %v %q", err, b)
	}
}

func TestTraversalBlocked(t *testing.T) {
	s, u, _ := setup(t)
	for _, p := range []string{"../outside/secret.txt", "domains/../../outside/secret.txt", "/../outside/secret.txt"} {
		if _, err := s.Read(u, p); err == nil {
			t.Errorf("traversal %q should fail", p)
		}
	}
	if runtime.GOOS != "windows" {
		if _, err := s.Read(u, "escape/secret.txt"); err == nil {
			t.Errorf("symlink escape should fail")
		}
		if _, err := s.List(u, "escape"); err == nil {
			t.Errorf("symlink escape listing should fail")
		}
	}
}

func TestWriteMkdirRenameDelete(t *testing.T) {
	s, u, _ := setup(t)
	if err := s.Mkdir(u, "a/b/c"); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(u, "a/b/c/f.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(u, "a/b/c/f.txt", "a/g.txt"); err != nil {
		t.Fatal(err)
	}
	b, err := s.Read(u, "a/g.txt")
	if err != nil || string(b) != "hello" {
		t.Fatalf("after rename: %v %q", err, b)
	}
	if err := s.Delete(u, []string{"a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(u, "a"); err == nil {
		t.Errorf("deleted dir still listable")
	}
	if err := s.Delete(u, []string{""}); err == nil {
		t.Errorf("deleting home root must fail")
	}
	if err := s.Delete(u, []string{"domains"}); err == nil {
		t.Errorf("deleting domains must fail")
	}
}

func TestUploadCompressExtract(t *testing.T) {
	s, u, _ := setup(t)
	if err := s.Upload(u, "domains/x.com/public_html", "up.txt", strings.NewReader("uploaded")); err != nil {
		t.Fatal(err)
	}
	if err := s.Upload(u, "domains", "../../evil.txt", strings.NewReader("x")); err == nil {
		t.Errorf("upload with traversal name must fail")
	}
	if err := s.Compress(u, []string{"domains/x.com/public_html"}, "domains/x.com/site.zip"); err != nil {
		t.Fatal(err)
	}
	if err := s.Mkdir(u, "restore"); err != nil {
		t.Fatal(err)
	}
	if err := s.Extract(u, "domains/x.com/site.zip", "restore"); err != nil {
		t.Fatal(err)
	}
	b, err := s.Read(u, "restore/public_html/up.txt")
	if err != nil || string(b) != "uploaded" {
		t.Fatalf("extracted content: %v %q", err, b)
	}
	if err := s.Copy(u, "restore", "restore2"); err != nil {
		t.Fatal(err)
	}
	b, err = s.Read(u, "restore2/public_html/index.php")
	if err != nil || !bytes.HasPrefix(b, []byte("<?php")) {
		t.Fatalf("copy: %v %q", err, b)
	}
}

func TestChmod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on windows")
	}
	s, u, _ := setup(t)
	if err := s.Chmod(u, "domains/x.com/public_html/index.php", 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Chmod(u, "domains/x.com/public_html/index.php", 0o4755); err == nil {
		t.Errorf("setuid must be rejected")
	}
	list, _ := s.List(u, "domains/x.com/public_html")
	if list[0].Mode != "0600" {
		t.Errorf("mode = %s", list[0].Mode)
	}
}
