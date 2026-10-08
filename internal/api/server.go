// Package api wires HTTP routes for /api/v1 and the phpMyAdmin proxy.
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"olspanel/internal/acme"
	"olspanel/internal/auth"
	"olspanel/internal/config"
	"olspanel/internal/cron"
	"olspanel/internal/db"
	"olspanel/internal/domains"
	"olspanel/internal/files"
	"olspanel/internal/ftp"
	"olspanel/internal/mariadb"
	"olspanel/internal/ols"
	"olspanel/internal/users"
	"olspanel/internal/validate"
)

// Server holds every dependency the handlers need.
type Server struct {
	Cfg     config.Config
	DB      *db.DB
	Auth    *auth.Manager
	Users   *users.Service
	Domains *domains.Service
	OLS     *ols.Manager
	MariaDB *mariadb.Service
	Files   *files.Service
	FTP     *ftp.Service
	Cron    *cron.Service
	ACME    *acme.Service
	Version string
	Started time.Time
	SPA     http.Handler
}

// Router builds the full HTTP handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(s.Auth.Sessions.LoadAndSave)
	r.Use(s.Auth.Middleware)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", s.health)
		r.Post("/auth/login", s.login)
		r.Post("/auth/logout", s.logout)
		r.Get("/auth/me", s.me)
		r.Group(func(r chi.Router) {
			r.Use(auth.RequireUser)
			r.Put("/auth/password", s.changePassword)
			r.Post("/auth/stop-impersonation", s.stopImpersonation)
			r.Get("/usage", s.usage)
			r.Get("/php/versions", s.phpVersions)

			r.Get("/domains", s.listDomains)
			r.Post("/domains", s.createDomain)
			r.Put("/domains/{id}", s.updateDomain)
			r.Delete("/domains/{id}", s.deleteDomain)
			r.Post("/domains/{id}/ssl/issue", s.issueSSL)

			r.Get("/databases", s.listDatabases)
			r.Post("/databases", s.createDatabase)
			r.Delete("/databases/{id}", s.deleteDatabase)
			r.Post("/databases/{id}/users", s.createDBUser)
			r.Put("/databases/{id}/users/{uid}", s.updateDBUser)
			r.Delete("/databases/{id}/users/{uid}", s.deleteDBUser)

			r.Get("/files", s.filesList)
			r.Get("/files/content", s.filesRead)
			r.Put("/files/content", s.filesWrite)
			r.Get("/files/download", s.filesDownload)
			r.Post("/files/upload", s.filesUpload)
			r.Post("/files/mkdir", s.filesMkdir)
			r.Post("/files/rename", s.filesRename)
			r.Post("/files/delete", s.filesDelete)
			r.Post("/files/chmod", s.filesChmod)
			r.Post("/files/compress", s.filesCompress)
			r.Post("/files/extract", s.filesExtract)
			r.Post("/files/copy", s.filesCopy)

			r.Get("/ftp", s.listFTP)
			r.Post("/ftp", s.createFTP)
			r.Put("/ftp/{id}", s.updateFTP)
			r.Delete("/ftp/{id}", s.deleteFTP)

			r.Get("/cron", s.listCron)
			r.Post("/cron", s.createCron)
			r.Put("/cron/{id}", s.updateCron)
			r.Delete("/cron/{id}", s.deleteCron)
		})
		r.Route("/admin", func(r chi.Router) {
			r.Use(auth.RequireUser, auth.RequireAdmin)
			r.Get("/server", s.serverInfo)
			r.Post("/services/{unit}/restart", s.restartService)
			r.Get("/settings", s.getSettings)
			r.Put("/settings", s.putSettings)
			r.Get("/audit", s.audit)
			r.Post("/ols/apply", s.olsApply)

			r.Get("/users", s.listUsers)
			r.Post("/users", s.createUser)
			r.Put("/users/{id}", s.updateUser)
			r.Delete("/users/{id}", s.deleteUser)
			r.Post("/users/{id}/suspend", s.suspendUser)
			r.Post("/users/{id}/unsuspend", s.unsuspendUser)
			r.Post("/users/{id}/impersonate", s.impersonate)

			r.Get("/packages", s.listPackages)
			r.Post("/packages", s.createPackage)
			r.Put("/packages/{id}", s.updatePackage)
			r.Delete("/packages/{id}", s.deletePackage)

			r.Get("/domains", s.adminDomains)
		})
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeErr(w, http.StatusNotFound, "not_found", "nie znaleziono")
		})
	})

	r.Handle("/phpmyadmin", http.RedirectHandler("/phpmyadmin/", http.StatusFound))
	r.Handle("/phpmyadmin/*", auth.RequireUser(s.pmaProxy()))
	r.NotFound(s.SPA.ServeHTTP)
	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// ---- helpers ----

type errResp struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errResp{Error: msg, Code: code})
}

// fail maps service errors to HTTP responses.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "nie znaleziono")
	case errors.Is(err, validate.ErrInvalid):
		writeErr(w, http.StatusBadRequest, "invalid", trimJoin(err))
	case errors.Is(err, users.ErrOverQuota):
		writeErr(w, http.StatusForbidden, "quota", err.Error())
	case errors.Is(err, ols.ErrApplyFailed):
		slog.Error("ols apply", "err", err)
		writeErr(w, http.StatusInternalServerError, "ols", err.Error())
	default:
		slog.Warn("request failed", "err", err)
		writeErr(w, http.StatusBadRequest, "error", err.Error())
	}
}

// trimJoin strips the generic ErrInvalid prefix from joined errors.
func trimJoin(err error) string {
	msg := err.Error()
	prefix := validate.ErrInvalid.Error() + "\n"
	if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
		return msg[len(prefix):]
	}
	return msg
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.Join(validate.ErrInvalid, errors.New("nieprawidłowe dane żądania"))
	}
	return nil
}

// decodeLarge is decode without the 1 MB cap (caller sets MaxBytesReader).
func decodeLarge(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.Join(validate.ErrInvalid, errors.New("nieprawidłowe dane żądania"))
	}
	return nil
}

func idParam(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, db.ErrNotFound
	}
	return id, nil
}

func ident(r *http.Request) *auth.Identity { return auth.FromContext(r.Context()) }

func (s *Server) audit1(r *http.Request, action, target, detail string) {
	id := ident(r)
	if id == nil {
		return
	}
	s.DB.Audit(r.Context(), id.User.ID, id.ImpersonatorID, action, target, detail)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.Version})
}
