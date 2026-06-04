// Package auth provides single-admin authentication: bcrypt password
// verification, an HMAC-signed stateless session cookie, and a small per-IP
// login rate limiter. It depends only on the standard library plus
// golang.org/x/crypto/bcrypt.
package auth

import (
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

// CookieName is the session cookie name.
const CookieName = "tm_session"

// Authenticator verifies credentials and issues/validates session cookies.
type Authenticator struct {
	store        *store.Store
	secret       []byte
	ttl          time.Duration
	cookieSecure string // "auto" | "true" | "false"
	limiter      *rateLimiter
}

// New creates an Authenticator. secret is the persisted HMAC session secret.
func New(st *store.Store, secret []byte, ttl time.Duration, cookieSecure string) *Authenticator {
	return &Authenticator{
		store:        st,
		secret:       secret,
		ttl:          ttl,
		cookieSecure: cookieSecure,
		limiter:      newRateLimiter(5, 5*time.Minute, time.Minute),
	}
}

// HashPassword returns a bcrypt hash of pw (cost 10).
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// SetPassword hashes pw and stores it as the admin password.
func SetPassword(st *store.Store, pw string) error {
	h, err := HashPassword(pw)
	if err != nil {
		return err
	}
	return st.SetPasswordHash(h)
}

// CheckPassword reports whether pw matches the stored admin password. It returns
// false (not an error) when no password has been configured yet.
func (a *Authenticator) CheckPassword(pw string) (bool, error) {
	hash, ok, err := a.store.GetPasswordHash()
	if err != nil {
		return false, err
	}
	if !ok || hash == "" {
		return false, nil
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil, nil
}

// PasswordConfigured reports whether an admin password has been set.
func (a *Authenticator) PasswordConfigured() bool {
	hash, ok, err := a.store.GetPasswordHash()
	return err == nil && ok && hash != ""
}

// RequireAuth wraps a handler, returning 401 when the request has no valid
// session.
func (a *Authenticator) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Authenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Authenticated reports whether the request carries a valid, unexpired session.
func (a *Authenticator) Authenticated(r *http.Request) bool {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return false
	}
	return a.verify(c.Value, time.Now())
}

// AllowLogin reports whether the client IP may attempt a login right now.
func (a *Authenticator) AllowLogin(r *http.Request) bool {
	return a.limiter.allow(clientIP(r), time.Now())
}

// NoteLoginFailure records a failed attempt for rate limiting.
func (a *Authenticator) NoteLoginFailure(r *http.Request) {
	a.limiter.fail(clientIP(r), time.Now())
}

// NoteLoginSuccess clears the rate-limit state for the client IP.
func (a *Authenticator) NoteLoginSuccess(r *http.Request) {
	a.limiter.success(clientIP(r))
}

// clientIP extracts the best-effort client IP, honoring a single
// X-Forwarded-For hop (for use behind a trusted reverse proxy).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
