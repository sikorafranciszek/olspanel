// Package files implements the file manager. Every operation goes through
// os.Root opened at the user's home, so paths can never escape it (including
// via symlinks). New files are chowned to the account.
package files

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"olspanel/internal/db"
	"olspanel/internal/validate"
)

// MaxEditSize caps files opened in the editor.
const MaxEditSize = 2 << 20

// Service holds file-manager settings.
type Service struct {
	MaxUploadMB int64
}

// Entry describes a directory entry.
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Dir     bool      `json:"dir"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"` // octal, e.g. "0644"
	ModTime time.Time `json:"mtime"`
	Symlink bool      `json:"symlink"`
}

var errInvalidPath = errors.Join(validate.ErrInvalid, errors.New("nieprawidłowa ścieżka"))

// clean normalises a user path to a root-relative form without leading slash.
func clean(p string) (string, error) {
	if err := validate.RelPath(p); err != nil {
		return "", err
	}
	p = strings.ReplaceAll(p, "\\", "/")
	c := path.Clean("/" + p)
	if strings.HasPrefix(c, "/..") {
		return "", errInvalidPath
	}
	return strings.TrimPrefix(c, "/"), nil
}

func (s *Service) root(u *db.User) (*os.Root, error) {
	if u.Home == "" {
		return nil, errors.New("konto nie ma katalogu domowego")
	}
	return os.OpenRoot(u.Home)
}

func (s *Service) chown(r *os.Root, u *db.User, rel string) {
	if u.UID != 0 {
		_ = r.Lchown(rel, int(u.UID), int(u.GID))
	}
}

// List returns entries of a directory.
func (s *Service) List(u *db.User, dir string) ([]Entry, error) {
	rel, err := clean(dir)
	if err != nil {
		return nil, err
	}
	r, err := s.root(u)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if rel == "" {
		rel = "."
	}
	f, err := r.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		p := e.Name()
		if rel != "." {
			p = rel + "/" + e.Name()
		}
		ent := Entry{Name: e.Name(), Path: p, Dir: e.IsDir(), Size: info.Size(), Mode: fmt.Sprintf("%04o", info.Mode().Perm()), ModTime: info.ModTime(), Symlink: info.Mode()&fs.ModeSymlink != 0}
		if ent.Symlink {
			if st, err := r.Stat(p); err == nil {
				ent.Dir = st.IsDir()
			}
		}
		out = append(out, ent)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// Read returns a text file's contents (size-limited).
func (s *Service) Read(u *db.User, p string) ([]byte, error) {
	rel, err := clean(p)
	if err != nil {
		return nil, err
	}
	r, err := s.root(u)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	st, err := r.Stat(rel)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, errors.New("to jest katalog")
	}
	if st.Size() > MaxEditSize {
		return nil, errors.New("plik jest za duży do edycji w przeglądarce (limit 2 MB)")
	}
	return r.ReadFile(rel)
}

// Write replaces a file's contents.
func (s *Service) Write(u *db.User, p string, data []byte) error {
	rel, err := clean(p)
	if err != nil {
		return err
	}
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	exists := false
	if _, err := r.Stat(rel); err == nil {
		exists = true
	}
	if err := r.WriteFile(rel, data, 0o644); err != nil {
		return err
	}
	if !exists {
		s.chown(r, u, rel)
	}
	return nil
}

// Open opens a file for download.
func (s *Service) Open(u *db.User, p string) (*os.File, os.FileInfo, error) {
	rel, err := clean(p)
	if err != nil {
		return nil, nil, err
	}
	r, err := s.root(u)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	f, err := r.Open(rel)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		f.Close()
		return nil, nil, errors.New("nie można pobrać katalogu")
	}
	return f, st, nil
}

// Upload stores a stream as dir/name.
func (s *Service) Upload(u *db.User, dir, name string, src io.Reader) error {
	if strings.ContainsAny(name, "/\\\x00") || name == "" || name == "." || name == ".." {
		return errInvalidPath
	}
	rel, err := clean(path.Join(dir, name))
	if err != nil {
		return err
	}
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := r.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		_ = r.Remove(rel)
		return err
	}
	f.Close()
	s.chown(r, u, rel)
	return nil
}

// Mkdir creates a directory (and parents).
func (s *Service) Mkdir(u *db.User, p string) error {
	rel, err := clean(p)
	if err != nil {
		return err
	}
	if rel == "" {
		return errInvalidPath
	}
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.MkdirAll(rel, 0o755); err != nil {
		return err
	}
	// chown every newly created component
	parts := strings.Split(rel, "/")
	for i := range parts {
		s.chown(r, u, strings.Join(parts[:i+1], "/"))
	}
	return nil
}

// Rename moves a file or directory.
func (s *Service) Rename(u *db.User, from, to string) error {
	f, err := clean(from)
	if err != nil {
		return err
	}
	t, err := clean(to)
	if err != nil {
		return err
	}
	if f == "" || t == "" {
		return errInvalidPath
	}
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.Rename(f, t)
}

// Delete removes files/directories.
func (s *Service) Delete(u *db.User, paths []string) error {
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, p := range paths {
		rel, err := clean(p)
		if err != nil {
			return err
		}
		if rel == "" || rel == "domains" {
			return errors.New("nie można usunąć tego katalogu")
		}
		if err := r.RemoveAll(rel); err != nil {
			return err
		}
	}
	return nil
}

// Chmod sets permission bits (no setuid/setgid/sticky).
func (s *Service) Chmod(u *db.User, p string, mode uint32) error {
	rel, err := clean(p)
	if err != nil {
		return err
	}
	if mode&^0o777 != 0 {
		return errors.Join(validate.ErrInvalid, errors.New("nieprawidłowe uprawnienia"))
	}
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	st, err := r.Lstat(rel)
	if err != nil {
		return err
	}
	if st.Mode()&fs.ModeSymlink != 0 {
		return errors.New("nie można zmienić uprawnień dowiązania")
	}
	return r.Chmod(rel, fs.FileMode(mode))
}

// Copy duplicates a file or directory tree.
func (s *Service) Copy(u *db.User, from, to string) error {
	f, err := clean(from)
	if err != nil {
		return err
	}
	t, err := clean(to)
	if err != nil {
		return err
	}
	if f == "" || t == "" || t == f || strings.HasPrefix(t, f+"/") {
		return errInvalidPath
	}
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	return s.copyTree(r, u, f, t)
}

func (s *Service) copyTree(r *os.Root, u *db.User, from, to string) error {
	st, err := r.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case st.IsDir():
		if err := r.Mkdir(to, st.Mode().Perm()); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		s.chown(r, u, to)
		d, err := r.Open(from)
		if err != nil {
			return err
		}
		entries, err := d.ReadDir(-1)
		d.Close()
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := s.copyTree(r, u, from+"/"+e.Name(), to+"/"+e.Name()); err != nil {
				return err
			}
		}
		return nil
	case st.Mode()&fs.ModeSymlink != 0:
		return nil // skip symlinks
	default:
		src, err := r.Open(from)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := r.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			return err
		}
		dst.Close()
		s.chown(r, u, to)
		return nil
	}
}

// Compress zips paths into dest (a .zip file path).
func (s *Service) Compress(u *db.User, paths []string, dest string) error {
	d, err := clean(dest)
	if err != nil {
		return err
	}
	if d == "" || !strings.HasSuffix(d, ".zip") {
		return errors.Join(validate.ErrInvalid, errors.New("nazwa archiwum musi kończyć się .zip"))
	}
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	out, err := r.OpenFile(d, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	for _, p := range paths {
		rel, err := clean(p)
		if err != nil {
			return err
		}
		if rel == "" || rel == d {
			continue
		}
		base := path.Dir(rel)
		if err := s.addToZip(r, zw, rel, base, d); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	s.chown(r, u, d)
	return nil
}

func (s *Service) addToZip(r *os.Root, zw *zip.Writer, rel, base, skip string) error {
	st, err := r.Lstat(rel)
	if err != nil {
		return err
	}
	name := strings.TrimPrefix(rel, base+"/")
	if base == "." || base == "" {
		name = rel
	}
	if st.IsDir() {
		if _, err := zw.Create(name + "/"); err != nil {
			return err
		}
		d, err := r.Open(rel)
		if err != nil {
			return err
		}
		entries, err := d.ReadDir(-1)
		d.Close()
		if err != nil {
			return err
		}
		for _, e := range entries {
			child := rel + "/" + e.Name()
			if child == skip {
				continue
			}
			if err := s.addToZip(r, zw, child, base, skip); err != nil {
				return err
			}
		}
		return nil
	}
	if st.Mode()&fs.ModeSymlink != 0 {
		return nil
	}
	hdr, err := zip.FileInfoHeader(st)
	if err != nil {
		return err
	}
	hdr.Name = name
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	f, err := r.Open(rel)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// Extract unzips an archive into destDir (zip-slip safe: all paths go through os.Root).
func (s *Service) Extract(u *db.User, zipPath, destDir string) error {
	z, err := clean(zipPath)
	if err != nil {
		return err
	}
	d, err := clean(destDir)
	if err != nil {
		return err
	}
	r, err := s.root(u)
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := r.Open(z)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return fmt.Errorf("nieprawidłowe archiwum zip: %w", err)
	}
	var total int64
	for _, zf := range zr.File {
		name := path.Clean("/" + strings.ReplaceAll(zf.Name, "\\", "/"))
		if strings.HasPrefix(name, "/..") {
			continue
		}
		target := strings.TrimPrefix(path.Join("/"+d, name), "/")
		if target == "" {
			continue
		}
		if zf.FileInfo().IsDir() {
			if err := r.MkdirAll(target, 0o755); err != nil {
				return err
			}
			s.chown(r, u, target)
			continue
		}
		if dir := path.Dir(target); dir != "." {
			if err := r.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			s.chown(r, u, dir)
		}
		total += int64(zf.UncompressedSize64)
		if total > 4<<30 {
			return errors.New("archiwum jest za duże (limit 4 GB)")
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := r.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, zf.Mode().Perm()|0o600)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
		s.chown(r, u, target)
	}
	return nil
}
