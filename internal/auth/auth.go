// Package auth implements password hashing, server-side sessions and the
// request-scoped identity (including admin impersonation).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/alexedwards/scs/v2"
	"golang.org/x/time/rate"

	"olspanel/internal/db"
)

const (
	keyUserID        = "uid"
	keyImpersonator  = "imp"
	keyCSRF          = "csrf"
	CookieName       = "olspanel_session"
	CSRFHeader       = "X-CSRF-Token"
	sessionLifetime  = 12 * time.Hour
	sessionIdleLimit = 2 * time.Hour
)

// Identity is the authenticated principal attached to a request.
type Identity struct {
	User           *db.User
	ImpersonatorID int64 // non-zero when an admin is acting as this user
}

// IsAdmin reports whether the effective user is an admin.
func (i *Identity) IsAdmin() bool { return i != nil && i.User.Role == "admin" }

type ctxKey struct{}

// FromContext returns the identity stored by Middleware, or nil.
func FromContext(ctx context.Context) *Identity {
	id, _ := ctx.Value(ctxKey{}).(*Identity)
	return id
}

// Manager owns sessions and login throttling.
type Manager struct {
	Sessions *scs.SessionManager
	db       *db.DB
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// New builds a Manager backed by the sessions table.
func New(d *db.DB, secureCookie bool) *Manager {
	sm := scs.New()
	sm.Store = newSQLiteStore(d)
	sm.Lifetime = sessionLifetime
	sm.IdleTimeout = sessionIdleLimit
	sm.Cookie.Name = CookieName
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = secureCookie
	sm.Cookie.SameSite = http.SameSiteStrictMode
	sm.Cookie.Path = "/"
	return &Manager{Sessions: sm, db: d, limiters: map[string]*rate.Limiter{}}
}

// HashPassword returns an argon2id PHC string.
func HashPassword(pw string) (string, error) {
	return argon2id.CreateHash(pw, argon2id.DefaultParams)
}

// CheckPassword compares a password with a stored hash.
func CheckPassword(pw, hash string) bool {
	ok, err := argon2id.ComparePasswordAndHash(pw, hash)
	return err == nil && ok
}

// RandomToken returns a URL-safe random string of n bytes entropy.
func RandomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Allow reports whether a login attempt from ip may proceed (5 per minute, burst 10).
func (m *Manager) Allow(ip string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.limiters[ip]
	if !ok {
		l = rate.NewLimiter(rate.Every(12*time.Second), 10)
		m.limiters[ip] = l
	}
	return l.Allow()
}

// Login rotates the session token and binds it to the user.
func (m *Manager) Login(ctx context.Context, u *db.User) error {
	if err := m.Sessions.RenewToken(ctx); err != nil {
		return err
	}
	m.Sessions.Put(ctx, keyUserID, u.ID)
	m.Sessions.Remove(ctx, keyImpersonator)
	m.Sessions.Put(ctx, keyCSRF, RandomToken(32))
	return nil
}

// Impersonate switches the session to act as target while remembering the admin.
func (m *Manager) Impersonate(ctx context.Context, adminID, targetID int64) error {
	if err := m.Sessions.RenewToken(ctx); err != nil {
		return err
	}
	m.Sessions.Put(ctx, keyUserID, targetID)
	m.Sessions.Put(ctx, keyImpersonator, adminID)
	m.Sessions.Put(ctx, keyCSRF, RandomToken(32))
	return nil
}

// StopImpersonation returns the session to the admin.
func (m *Manager) StopImpersonation(ctx context.Context) (int64, error) {
	adminID := m.Sessions.GetInt64(ctx, keyImpersonator)
	if adminID == 0 {
		return 0, nil
	}
	if err := m.Sessions.RenewToken(ctx); err != nil {
		return 0, err
	}
	m.Sessions.Put(ctx, keyUserID, adminID)
	m.Sessions.Remove(ctx, keyImpersonator)
	m.Sessions.Put(ctx, keyCSRF, RandomToken(32))
	return adminID, nil
}

// Logout destroys the session.
func (m *Manager) Logout(ctx context.Context) error {
	return m.Sessions.Destroy(ctx)
}

// CSRFToken returns the token bound to this session.
func (m *Manager) CSRFToken(ctx context.Context) string {
	return m.Sessions.GetString(ctx, keyCSRF)
}

// Load resolves the identity for the current session, or nil.
func (m *Manager) Load(ctx context.Context) *Identity {
	uid := m.Sessions.GetInt64(ctx, keyUserID)
	if uid == 0 {
		return nil
	}
	u, err := m.db.UserByID(ctx, uid)
	if err != nil {
		return nil
	}
	return &Identity{User: u, ImpersonatorID: m.Sessions.GetInt64(ctx, keyImpersonator)}
}

// Middleware attaches the Identity (if any) to the request context and enforces
// CSRF on state-changing requests.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := m.Load(r.Context())
		if id != nil {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				want := m.CSRFToken(r.Context())
				got := r.Header.Get(CSRFHeader)
				if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
					http.Error(w, `{"error":"nieprawidłowy token CSRF","code":"csrf"}`, http.StatusForbidden)
					return
				}
			}
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, id))
		}
		next.ServeHTTP(w, r)
	})
}

// RequireUser rejects anonymous and suspended requests.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := FromContext(r.Context())
		if id == nil {
			http.Error(w, `{"error":"wymagane logowanie","code":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if id.User.Suspended && id.ImpersonatorID == 0 {
			http.Error(w, `{"error":"konto jest zawieszone","code":"suspended"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin rejects non-admin requests. Impersonating admins are not admins.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := FromContext(r.Context())
		if id == nil || !id.IsAdmin() {
			http.Error(w, `{"error":"brak uprawnień","code":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
