package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"time"
)

// Token layout: 8-byte big-endian expiry (unix seconds) followed by a 32-byte
// HMAC-SHA256 of those 8 bytes, base64url-encoded.

func (a *Authenticator) sign(expiryUnix int64) string {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, uint64(expiryUnix))
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(append(payload, mac.Sum(nil)...))
}

func (a *Authenticator) verify(token string, now time.Time) bool {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 8+sha256.Size {
		return false
	}
	payload, sig := raw[:8], raw[8:]
	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return false
	}
	expiry := int64(binary.BigEndian.Uint64(payload))
	return now.Unix() < expiry
}

// SetSession issues a fresh signed session cookie.
func (a *Authenticator) SetSession(w http.ResponseWriter, r *http.Request) {
	expiry := time.Now().Add(a.ttl)
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    a.sign(expiry.Unix()),
		Path:     a.cookiePath,
		HttpOnly: true,
		Secure:   a.secureFor(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiry,
		MaxAge:   int(a.ttl.Seconds()),
	})
}

// ClearSession expires the session cookie.
func (a *Authenticator) ClearSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     a.cookiePath,
		HttpOnly: true,
		Secure:   a.secureFor(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (a *Authenticator) secureFor(r *http.Request) bool {
	switch a.cookieSecure {
	case "true":
		return true
	case "false":
		return false
	default: // auto: secure when the request arrived over TLS (directly or via proxy)
		if r.TLS != nil {
			return true
		}
		return r.Header.Get("X-Forwarded-Proto") == "https"
	}
}
