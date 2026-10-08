package api

import (
	"net/http"
	"strings"

	"olspanel/internal/auth"
	"olspanel/internal/db"
	"olspanel/internal/php"
	"olspanel/internal/validate"
)

type meResp struct {
	User          *db.User `json:"user"`
	Impersonating bool     `json:"impersonating"`
	CSRF          string   `json:"csrf"`
	Version       string   `json:"version"`
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	id := ident(r)
	if id == nil {
		writeJSON(w, http.StatusOK, meResp{Version: s.Version})
		return
	}
	writeJSON(w, http.StatusOK, meResp{User: id.User, Impersonating: id.ImpersonatorID != 0, CSRF: s.Auth.CSRFToken(r.Context()), Version: s.Version})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i > 0 {
		ip = ip[:i]
	}
	if !s.Auth.Allow(ip) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "zbyt wiele prób logowania, spróbuj za chwilę")
		return
	}
	u, err := s.DB.UserByName(r.Context(), strings.ToLower(strings.TrimSpace(req.Username)))
	if err != nil || !auth.CheckPassword(req.Password, u.PasswordHash) {
		s.DB.Audit(r.Context(), 0, 0, "login.failed", req.Username, ip)
		writeErr(w, http.StatusUnauthorized, "bad_credentials", "nieprawidłowa nazwa użytkownika lub hasło")
		return
	}
	if u.Suspended {
		writeErr(w, http.StatusForbidden, "suspended", "konto jest zawieszone")
		return
	}
	if err := s.Auth.Login(r.Context(), u); err != nil {
		fail(w, err)
		return
	}
	s.DB.Audit(r.Context(), u.ID, 0, "login", u.Username, ip)
	writeJSON(w, http.StatusOK, meResp{User: u, CSRF: s.Auth.CSRFToken(r.Context()), Version: s.Version})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	_ = s.Auth.Logout(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	id := ident(r)
	if id.ImpersonatorID != 0 {
		writeErr(w, http.StatusForbidden, "impersonating", "zmiana hasła nie jest dostępna w trybie podglądu konta")
		return
	}
	if !auth.CheckPassword(req.Current, id.User.PasswordHash) {
		writeErr(w, http.StatusBadRequest, "bad_credentials", "obecne hasło jest nieprawidłowe")
		return
	}
	if err := s.Users.SetPassword(r.Context(), id.User.ID, req.New); err != nil {
		fail(w, err)
		return
	}
	s.audit1(r, "password.change", id.User.Username, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) stopImpersonation(w http.ResponseWriter, r *http.Request) {
	adminID, err := s.Auth.StopImpersonation(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if adminID == 0 {
		writeErr(w, http.StatusBadRequest, "not_impersonating", "nie jesteś w trybie podglądu konta")
		return
	}
	u, err := s.DB.UserByID(r.Context(), adminID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meResp{User: u, CSRF: s.Auth.CSRFToken(r.Context()), Version: s.Version})
}

func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	id := ident(r)
	us, err := s.Users.Usage(r.Context(), id.User)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, us)
}

// phpVersions lists versions available to the caller (installed ∩ package).
func (s *Server) phpVersions(w http.ResponseWriter, r *http.Request) {
	id := ident(r)
	installed := php.Installed(s.Cfg.LswsRoot)
	if s.Users.NoSystem && len(installed) == 0 {
		for _, v := range []string{"84", "83", "82", "81"} {
			installed = append(installed, php.Version{ID: v, Label: php.Label(v), Bin: php.Bin(s.Cfg.LswsRoot, v)})
		}
	}
	if id.IsAdmin() || id.User.PackageID == 0 {
		writeJSON(w, http.StatusOK, installed)
		return
	}
	pkg, err := s.DB.PackageByID(r.Context(), id.User.PackageID)
	if err != nil {
		writeJSON(w, http.StatusOK, []php.Version{})
		return
	}
	out := []php.Version{}
	for _, v := range installed {
		for _, a := range pkg.PHPVersions {
			if a == v.ID && validate.PHPVersion(a) == nil {
				out = append(out, v)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}
