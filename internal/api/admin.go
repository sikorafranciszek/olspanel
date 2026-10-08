package api

import (
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"olspanel/internal/db"
	"olspanel/internal/ols"
	"olspanel/internal/php"
	"olspanel/internal/system"
	"olspanel/internal/users"
	"olspanel/internal/validate"
)

// ---- server / services ----

type serviceStatus struct {
	Unit   string `json:"unit"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
}

var managedUnits = []serviceStatus{
	{Unit: "lsws", Label: "OpenLiteSpeed"},
	{Unit: "mariadb", Label: "MariaDB"},
	{Unit: "pure-ftpd", Label: "Pure-FTPd"},
	{Unit: "cron", Label: "Cron"},
}

func (s *Server) serverInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hostname, _ := os.Hostname()
	usersN, _ := s.DB.CountUsers(ctx, "user")
	domainsAll, _ := s.DB.ListDomains(ctx)
	pkgs, _ := s.DB.ListPackages(ctx)
	services := make([]serviceStatus, 0, len(managedUnits))
	for _, u := range managedUnits {
		st := u
		if !s.Users.NoSystem {
			st.Active = system.UnitActive(ctx, u.Unit)
		}
		services = append(services, st)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hostname":  hostname,
		"version":   s.Version,
		"go":        runtime.Version(),
		"os":        runtime.GOOS + "/" + runtime.GOARCH,
		"uptime_s":  int64(time.Since(s.Started).Seconds()),
		"users":     usersN,
		"domains":   len(domainsAll),
		"packages":  len(pkgs),
		"services":  services,
		"php":       php.Installed(s.Cfg.LswsRoot),
		"ols_ok":    s.Users.NoSystem || ols.Healthy(),
		"resources": readResources(),
	})
}

func (s *Server) restartService(w http.ResponseWriter, r *http.Request) {
	unit := chi.URLParam(r, "unit")
	ok := false
	for _, u := range managedUnits {
		if u.Unit == unit {
			ok = true
		}
	}
	if !ok {
		writeErr(w, http.StatusBadRequest, "invalid", "nieznana usługa")
		return
	}
	if s.Users.NoSystem {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if err := system.Systemctl(r.Context(), "restart", unit); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "service.restart", unit, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) olsApply(w http.ResponseWriter, r *http.Request) {
	if err := s.OLS.Apply(r.Context()); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "ols.apply", "", "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- settings ----

var allowedSettings = map[string]bool{
	"panel_hostname": true, "acme_email": true, "acme_staging": true, "default_php": true,
	"ftp_passive_ip": true, "upload_max_mb": true,
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	all, err := s.DB.AllSettings(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, all)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	for k, v := range req {
		if !allowedSettings[k] {
			writeErr(w, http.StatusBadRequest, "invalid", "nieznane ustawienie: "+k)
			return
		}
		v = strings.TrimSpace(v)
		switch k {
		case "panel_hostname":
			if v != "" {
				d, err := validate.Domain(v)
				if err != nil {
					fail(w, err)
					return
				}
				v = d
			}
		case "acme_email":
			if err := validate.Email(v); err != nil {
				fail(w, err)
				return
			}
		case "default_php":
			if v != "" {
				if err := validate.PHPVersion(v); err != nil {
					fail(w, err)
					return
				}
			}
		}
		if err := s.DB.SetSetting(r.Context(), k, v); err != nil {
			fail(w, err)
			return
		}
	}
	s.audit1(r, "settings.update", "", "")
	s.getSettings(w, r)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListAudit(r.Context(), 200)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ---- users ----

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req users.CreateRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	u, err := s.Users.Create(r.Context(), req)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "user.create", u.Username, u.Role)
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var req users.UpdateRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	u, err := s.Users.Update(r.Context(), id, req)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "user.update", u.Username, "")
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	if id == ident(r).User.ID {
		writeErr(w, http.StatusBadRequest, "invalid", "nie można usunąć własnego konta")
		return
	}
	u, err := s.DB.UserByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Users.Delete(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "user.delete", u.Username, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) suspendUser(w http.ResponseWriter, r *http.Request)   { s.setSuspended(w, r, true) }
func (s *Server) unsuspendUser(w http.ResponseWriter, r *http.Request) { s.setSuspended(w, r, false) }

func (s *Server) setSuspended(w http.ResponseWriter, r *http.Request, v bool) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	u, err := s.Users.SetSuspended(r.Context(), id, v)
	if err != nil {
		fail(w, err)
		return
	}
	action := "user.unsuspend"
	if v {
		action = "user.suspend"
	}
	s.audit1(r, action, u.Username, "")
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) impersonate(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	target, err := s.DB.UserByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if target.Role != "user" {
		writeErr(w, http.StatusBadRequest, "invalid", "można podglądać tylko konta użytkowników")
		return
	}
	admin := ident(r).User
	if err := s.Auth.Impersonate(r.Context(), admin.ID, target.ID); err != nil {
		fail(w, err)
		return
	}
	s.DB.Audit(r.Context(), admin.ID, 0, "user.impersonate", target.Username, "")
	writeJSON(w, http.StatusOK, meResp{User: target, Impersonating: true, CSRF: s.Auth.CSRFToken(r.Context()), Version: s.Version})
}

// ---- packages ----

func (s *Server) listPackages(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListPackages(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func validPackage(p *db.Package) error {
	if err := validate.PackageName(p.Name); err != nil {
		return err
	}
	if p.DiskMB < 1 || p.MaxDomains < 0 || p.MaxSubdomains < 0 || p.MaxDatabases < 0 || p.MaxFTP < 0 || p.MaxCron < 0 {
		return validate.ErrInvalid
	}
	if len(p.PHPVersions) == 0 {
		p.PHPVersions = []string{"83"}
	}
	for _, v := range p.PHPVersions {
		if err := validate.PHPVersion(v); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) createPackage(w http.ResponseWriter, r *http.Request) {
	var p db.Package
	if err := decode(r, &p); err != nil {
		fail(w, err)
		return
	}
	if err := validPackage(&p); err != nil {
		fail(w, err)
		return
	}
	id, err := s.DB.CreatePackage(r.Context(), &p)
	if err != nil {
		fail(w, err)
		return
	}
	out, _ := s.DB.PackageByID(r.Context(), id)
	s.audit1(r, "package.create", p.Name, "")
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updatePackage(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	var p db.Package
	if err := decode(r, &p); err != nil {
		fail(w, err)
		return
	}
	p.ID = id
	if err := validPackage(&p); err != nil {
		fail(w, err)
		return
	}
	if err := s.DB.UpdatePackage(r.Context(), &p); err != nil {
		fail(w, err)
		return
	}
	out, _ := s.DB.PackageByID(r.Context(), id)
	s.audit1(r, "package.update", p.Name, "")
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deletePackage(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.DB.DeletePackage(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "package.delete", chi.URLParam(r, "id"), "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminDomains(w http.ResponseWriter, r *http.Request) {
	list, err := s.DB.ListDomains(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
