// Package domains orchestrates domains, subdomains and aliases and builds
// the OpenLiteSpeed state from the database.
package domains

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"olspanel/internal/db"
	"olspanel/internal/ols"
	"olspanel/internal/php"
	"olspanel/internal/system"
	"olspanel/internal/users"
	"olspanel/internal/validate"
)

// Service manages domains.
type Service struct {
	DB        *db.DB
	OLS       *ols.Manager
	LswsRoot  string
	DataDir   string // /var/lib/olspanel
	ShareDir  string // /usr/local/olspanel/share (default + suspended pages)
	PMARoot   string // "" when phpMyAdmin is not installed
	PanelCert string
	PanelKey  string
	HTTPPort  int
	HTTPSPort int
	NoSystem  bool
}

// CreateRequest is the user's input.
type CreateRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // domain | subdomain | alias
	ParentID   int64  `json:"parent_id"`
	PHPVersion string `json:"php_version"`
}

// Create adds a domain for user u, enforcing package limits.
func (s *Service) Create(ctx context.Context, u *db.User, req CreateRequest) (*db.Domain, error) {
	name, err := validate.Domain(req.Name)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.DomainByName(ctx, name); err == nil {
		return nil, errors.New("ta domena jest już dodana")
	}
	if strings.HasPrefix(name, "www.") {
		return nil, errors.New("dodaj domenę bez przedrostka www")
	}
	pkg, err := s.DB.PackageByID(ctx, u.PackageID)
	if err != nil {
		return nil, errors.New("konto nie ma przypisanego pakietu")
	}
	dm := &db.Domain{UserID: u.ID, Name: name, Type: req.Type}
	switch req.Type {
	case "domain":
		n, _ := s.DB.CountUserDomains(ctx, u.ID, "domain")
		if err := users.CheckLimit(n, pkg.MaxDomains, "domeny"); err != nil {
			return nil, err
		}
	case "subdomain", "alias":
		parent, err := s.DB.DomainByID(ctx, req.ParentID)
		if err != nil || parent.UserID != u.ID || parent.Type != "domain" {
			return nil, errors.New("nieprawidłowa domena nadrzędna")
		}
		if req.Type == "subdomain" {
			if !strings.HasSuffix(name, "."+parent.Name) {
				return nil, errors.New("subdomena musi kończyć się nazwą domeny nadrzędnej")
			}
			n, _ := s.DB.CountUserDomains(ctx, u.ID, "subdomain")
			if err := users.CheckLimit(n, pkg.MaxSubdomains, "subdomeny"); err != nil {
				return nil, err
			}
		} else {
			n, _ := s.DB.CountUserDomains(ctx, u.ID, "alias")
			if err := users.CheckLimit(n, pkg.MaxDomains, "aliasy"); err != nil {
				return nil, err
			}
		}
		pid := parent.ID
		dm.ParentID = &pid
	default:
		return nil, errors.New("nieprawidłowy typ domeny")
	}
	dm.PHPVersion = req.PHPVersion
	if dm.Type == "alias" {
		dm.PHPVersion = ""
	} else {
		if dm.PHPVersion == "" && len(pkg.PHPVersions) > 0 {
			dm.PHPVersion = pkg.PHPVersions[0]
		}
		if err := s.checkPHP(pkg, dm.PHPVersion); err != nil {
			return nil, err
		}
	}
	if dm.Type != "alias" {
		acc := &system.Account{Name: u.Username, UID: int(u.UID), GID: int(u.GID), Home: u.Home}
		if s.NoSystem {
			root := filepath.Join(u.Home, "domains", name)
			_ = os.MkdirAll(filepath.Join(root, "public_html"), 0o750)
			_ = os.MkdirAll(filepath.Join(root, "logs"), 0o750)
			s.writePlaceholder(root, acc, name)
		} else {
			root, err := system.EnsureDomainDirs(ctx, acc, name)
			if err != nil {
				return nil, fmt.Errorf("tworzenie katalogów: %w", err)
			}
			s.writePlaceholder(root, acc, name)
		}
		_ = os.MkdirAll(filepath.Join(s.DataDir, "acme-challenge", name, ".well-known", "acme-challenge"), 0o755)
	}
	id, err := s.DB.CreateDomain(ctx, dm)
	if err != nil {
		return nil, err
	}
	if err := s.OLS.Apply(ctx); err != nil {
		_ = s.DB.DeleteDomain(ctx, id)
		return nil, err
	}
	return s.DB.DomainByID(ctx, id)
}

func (s *Service) checkPHP(pkg *db.Package, v string) error {
	if err := validate.PHPVersion(v); err != nil {
		return err
	}
	allowed := false
	for _, a := range pkg.PHPVersions {
		if a == v {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("ta wersja PHP nie jest dostępna w Twoim pakiecie")
	}
	if !s.NoSystem && !php.Has(s.LswsRoot, v) {
		return errors.New("ta wersja PHP nie jest zainstalowana na serwerze")
	}
	return nil
}

func (s *Service) writePlaceholder(root string, acc *system.Account, name string) {
	idx := filepath.Join(root, "public_html", "index.html")
	if _, err := os.Stat(idx); err == nil {
		return
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "public_html")); len(entries) > 0 {
		return
	}
	html := strings.ReplaceAll(placeholderHTML, "{{domain}}", name)
	if err := os.WriteFile(idx, []byte(html), 0o644); err == nil && acc.UID > 0 {
		_ = os.Lchown(idx, acc.UID, acc.GID)
	}
}

// UpdateRequest edits a domain.
type UpdateRequest struct {
	PHPVersion string `json:"php_version"`
	ForceHTTPS *bool  `json:"force_https"`
}

// Update changes PHP version / HTTPS redirect.
func (s *Service) Update(ctx context.Context, u *db.User, id int64, req UpdateRequest) (*db.Domain, error) {
	dm, err := s.owned(ctx, u, id)
	if err != nil {
		return nil, err
	}
	if dm.Type == "alias" {
		return nil, errors.New("alias nie ma własnych ustawień")
	}
	if req.PHPVersion != "" && req.PHPVersion != dm.PHPVersion {
		pkg, err := s.DB.PackageByID(ctx, u.PackageID)
		if err != nil {
			return nil, err
		}
		if err := s.checkPHP(pkg, req.PHPVersion); err != nil {
			return nil, err
		}
		dm.PHPVersion = req.PHPVersion
	}
	if req.ForceHTTPS != nil {
		dm.ForceHTTPS = *req.ForceHTTPS
	}
	if err := s.DB.UpdateDomain(ctx, dm); err != nil {
		return nil, err
	}
	if err := s.OLS.Apply(ctx); err != nil {
		return nil, err
	}
	return s.DB.DomainByID(ctx, id)
}

// Delete removes a domain (and children) and its files.
func (s *Service) Delete(ctx context.Context, u *db.User, id int64) error {
	dm, err := s.owned(ctx, u, id)
	if err != nil {
		return err
	}
	children := []db.Domain{}
	all, _ := s.DB.ListUserDomains(ctx, u.ID)
	for _, c := range all {
		if c.ParentID != nil && *c.ParentID == dm.ID {
			children = append(children, c)
		}
	}
	if err := s.DB.DeleteDomain(ctx, id); err != nil {
		return err
	}
	if err := s.OLS.Apply(ctx); err != nil {
		slog.Warn("apply after domain delete", "err", err)
	}
	for _, d := range append(children, *dm) {
		if d.Type == "alias" {
			continue
		}
		if u.Home != "" {
			_ = os.RemoveAll(filepath.Join(u.Home, "domains", d.Name))
		}
		_ = os.RemoveAll(filepath.Join(s.DataDir, "acme-challenge", d.Name))
		_ = os.RemoveAll(filepath.Join(s.DataDir, "ssl", d.Name))
	}
	return nil
}

func (s *Service) owned(ctx context.Context, u *db.User, id int64) (*db.Domain, error) {
	dm, err := s.DB.DomainByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if dm.UserID != u.ID {
		return nil, db.ErrNotFound
	}
	return dm, nil
}

// CleanupUser implements users.Cleaner: removes domain files on account deletion.
func (s *Service) CleanupUser(ctx context.Context, u *db.User) error {
	list, err := s.DB.ListUserDomains(ctx, u.ID)
	if err != nil {
		return err
	}
	for _, d := range list {
		_ = os.RemoveAll(filepath.Join(s.DataDir, "acme-challenge", d.Name))
		_ = os.RemoveAll(filepath.Join(s.DataDir, "ssl", d.Name))
	}
	return nil
}

// SSLPaths returns cert/key paths for a domain and whether both exist.
func (s *Service) SSLPaths(name string) (cert, key string, ok bool) {
	cert = filepath.Join(s.DataDir, "ssl", name, "fullchain.pem")
	key = filepath.Join(s.DataDir, "ssl", name, "privkey.pem")
	_, e1 := os.Stat(cert)
	_, e2 := os.Stat(key)
	return cert, key, e1 == nil && e2 == nil
}

// BuildState renders the OLS state from the database.
func (s *Service) BuildState(ctx context.Context) (*ols.State, error) {
	st := &ols.State{
		PanelCert:   s.PanelCert,
		PanelKey:    s.PanelKey,
		PMARoot:     s.PMARoot,
		SuspendRoot: filepath.Join(s.ShareDir, "suspended"),
		DefaultRoot: filepath.Join(s.ShareDir, "default"),
		HTTPPort:    s.HTTPPort,
		HTTPSPort:   s.HTTPSPort,
	}
	versions := php.Installed(s.LswsRoot)
	if len(versions) > 0 {
		st.PMAPhpBin = versions[0].Bin
	} else {
		st.PMAPhpBin = php.Bin(s.LswsRoot, "83")
	}
	userList, err := s.DB.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]db.User{}
	for _, u := range userList {
		byID[u.ID] = u
	}
	all, err := s.DB.ListDomains(ctx)
	if err != nil {
		return nil, err
	}
	// aliases grouped by parent
	aliases := map[int64][]string{}
	for _, d := range all {
		if d.Type == "alias" && d.ParentID != nil {
			aliases[*d.ParentID] = append(aliases[*d.ParentID], d.Name, "www."+d.Name)
		}
	}
	procs := map[string]ols.Processor{}
	for _, d := range all {
		if d.Type == "alias" {
			continue
		}
		u, ok := byID[d.UserID]
		if !ok || u.Role != "user" {
			continue
		}
		vh := ols.VHost{
			Name:       d.Name,
			User:       u.Username,
			Group:      u.Username,
			Root:       filepath.Join(u.Home, "domains", d.Name),
			Aliases:    append([]string{"www." + d.Name}, aliases[d.ID]...),
			PHPVersion: d.PHPVersion,
			ForceHTTPS: d.ForceHTTPS,
			Suspended:  u.Suspended,
			ACMEDir:    filepath.Join(s.DataDir, "acme-challenge", d.Name),
			OpenBase:   filepath.Join(u.Home, "domains", d.Name) + ":" + filepath.Join(u.Home, "tmp") + ":/tmp:/usr/share/php",
		}
		if cert, key, ok := s.SSLPaths(d.Name); ok {
			vh.SSLCert, vh.SSLKey = cert, key
		}
		pname := u.Username + "_php" + d.PHPVersion
		vh.Processor = pname
		if _, ok := procs[pname]; !ok && !u.Suspended {
			procs[pname] = ols.Processor{Name: pname, User: u.Username, Group: u.Username, Version: d.PHPVersion, LsphpBin: php.Bin(s.LswsRoot, d.PHPVersion)}
		}
		st.VHosts = append(st.VHosts, vh)
	}
	for _, name := range sortedKeys(procs) {
		st.Processors = append(st.Processors, procs[name])
	}
	return st, nil
}

func sortedKeys(m map[string]ols.Processor) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

const placeholderHTML = `<!doctype html>
<html lang="pl"><head><meta charset="utf-8"><title>{{domain}}</title>
<style>body{font-family:system-ui,sans-serif;background:#0f172a;color:#e2e8f0;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}
.box{text-align:center;padding:2rem;border:1px solid #334155;border-radius:1rem;background:#1e293b}h1{margin:0 0 .5rem;font-size:1.5rem}p{color:#94a3b8;margin:0}</style></head>
<body><div class="box"><h1>{{domain}}</h1><p>Strona w budowie. Wgraj pliki do katalogu public_html.</p></div></body></html>
`
