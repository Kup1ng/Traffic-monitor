// Package auth provides single-admin authentication: bcrypt password
// verification, an HMAC-signed stateless session cookie, and a small per-IP
// login rate limiter. It depends only on the standard library plus
// golang.org/x/crypto/bcrypt.
package auth

import (
	"crypto/sha256"
	"encoding/base64"
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
	cookiePath   string // session cookie Path (the app base path)
	trustProxy   bool   // trust X-Forwarded-For for the client IP
	limiter      *rateLimiter
}

// New creates an Authenticator. secret is the persisted HMAC session secret;
// basePath scopes the session cookie to the app's base path. trustProxy enables
// using X-Forwarded-For for the rate-limiter client IP (only safe behind a
// trusted reverse proxy).
func New(st *store.Store, secret []byte, ttl time.Duration, cookieSecure, basePath string, trustProxy bool) *Authenticator {
	if basePath == "" {
		basePath = "/"
	}
	return &Authenticator{
		store:        st,
		secret:       secret,
		ttl:          ttl,
		cookieSecure: cookieSecure,
		cookiePath:   basePath,
		trustProxy:   trustProxy,
		limiter:      newRateLimiter(5, 5*time.Minute, time.Minute),
	}
}

// prehash maps a password of any length to a fixed 44-byte token (base64 of its
// SHA-256) before bcrypt, which otherwise rejects inputs longer than 72 bytes.
func prehash(pw string) []byte {
	sum := sha256.Sum256([]byte(pw))
	return []byte(base64.StdEncoding.EncodeToString(sum[:]))
}

// HashPassword returns a bcrypt hash of pw (cost 10), pre-hashed so any password
// length is accepted.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword(prehash(pw), bcrypt.DefaultCost)
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
// false (not an error) when no password has been configured yet. It accepts both
// the current pre-hashed scheme and the legacy raw-bcrypt scheme so passwords set
// by older versions still work.
func (a *Authenticator) CheckPassword(pw string) (bool, error) {
	hash, ok, err := a.store.GetPasswordHash()
	if err != nil {
		return false, err
	}
	if !ok || hash == "" {
		return false, nil
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), prehash(pw)) == nil {
		return true, nil
	}
	// Legacy: raw password (bcrypt uses at most the first 72 bytes).
	legacy := []byte(pw)
	if len(legacy) > 72 {
		legacy = legacy[:72]
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), legacy) == nil, nil
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
	return a.limiter.allow(a.clientIP(r), time.Now())
}

// NoteLoginFailure records a failed attempt for rate limiting.
func (a *Authenticator) NoteLoginFailure(r *http.Request) {
	a.limiter.fail(a.clientIP(r), time.Now())
}

// NoteLoginSuccess clears the rate-limit state for the client IP.
func (a *Authenticator) NoteLoginSuccess(r *http.Request) {
	a.limiter.success(a.clientIP(r))
}

// clientIP returns the connecting peer's IP. X-Forwarded-For is honored only
// when trustProxy is set; otherwise it is ignored so a directly-exposed server
// cannot have its rate limiter bypassed by a spoofed header.
func (a *Authenticator) clientIP(r *http.Request) string {
	if a.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
