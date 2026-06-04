package auth

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

func newTestAuth(t *testing.T) (*Authenticator, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, []byte("test-secret-32-bytes-long-padding!!"), time.Hour, "auto", "/", false), st
}

func TestPasswordHashAndCheck(t *testing.T) {
	a, st := newTestAuth(t)

	if a.PasswordConfigured() {
		t.Fatal("password should not be configured initially")
	}
	if ok, _ := a.CheckPassword("anything"); ok {
		t.Fatal("CheckPassword must be false before a password is set")
	}

	if err := SetPassword(st, "s3cret-pass"); err != nil {
		t.Fatal(err)
	}
	if !a.PasswordConfigured() {
		t.Fatal("password should be configured after SetPassword")
	}
	if ok, err := a.CheckPassword("s3cret-pass"); err != nil || !ok {
		t.Fatalf("correct password rejected: ok=%v err=%v", ok, err)
	}
	if ok, _ := a.CheckPassword("wrong"); ok {
		t.Fatal("wrong password accepted")
	}
}

func TestLongPassword(t *testing.T) {
	a, st := newTestAuth(t)
	long := strings.Repeat("p", 100) // > 72 bytes, which raw bcrypt rejects
	if err := SetPassword(st, long); err != nil {
		t.Fatalf("SetPassword(long): %v", err)
	}
	if ok, err := a.CheckPassword(long); err != nil || !ok {
		t.Fatalf("CheckPassword(long) = ok=%v err=%v, want true", ok, err)
	}
	if ok, _ := a.CheckPassword(strings.Repeat("p", 99)); ok {
		t.Fatal("a different long password must not match")
	}
}

func TestClientIPTrustProxy(t *testing.T) {
	a, _ := newTestAuth(t)
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "10.0.0.5:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")

	a.trustProxy = false
	if ip := a.clientIP(r); ip != "10.0.0.5" {
		t.Errorf("untrusted clientIP = %q, want 10.0.0.5 (XFF must be ignored)", ip)
	}
	a.trustProxy = true
	if ip := a.clientIP(r); ip != "1.2.3.4" {
		t.Errorf("trusted clientIP = %q, want 1.2.3.4", ip)
	}
}

func TestCookieSignVerify(t *testing.T) {
	a, _ := newTestAuth(t)
	now := time.Unix(1_000_000, 0)

	tok := a.sign(now.Add(time.Hour).Unix())
	if !a.verify(tok, now) {
		t.Fatal("freshly signed token must verify")
	}
	// Expired.
	if a.verify(a.sign(now.Add(-time.Second).Unix()), now) {
		t.Fatal("expired token must not verify")
	}
	// Tampered.
	bad := []byte(tok)
	bad[len(bad)-1] ^= 0x01
	if a.verify(string(bad), now) {
		t.Fatal("tampered token must not verify")
	}
	// Wrong secret.
	other := New(nil, []byte("a-totally-different-secret-key-123456"), time.Hour, "auto", "/", false)
	if other.verify(tok, now) {
		t.Fatal("token must not verify under a different secret")
	}
	// Garbage.
	if a.verify("not-base64-@@@", now) {
		t.Fatal("garbage token must not verify")
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(3, time.Minute, time.Minute)
	now := time.Unix(0, 0)
	ip := "1.2.3.4"

	if !l.allow(ip, now) {
		t.Fatal("first attempt should be allowed")
	}
	l.fail(ip, now)
	l.fail(ip, now)
	if !l.allow(ip, now) {
		t.Fatal("should still be allowed after 2 failures")
	}
	l.fail(ip, now) // 3rd failure -> lock
	if l.allow(ip, now) {
		t.Fatal("should be locked after reaching max failures")
	}
	if !l.allow(ip, now.Add(2*time.Minute)) {
		t.Fatal("should be allowed again after lockout expires")
	}

	// Success clears state.
	l.fail(ip, now)
	l.fail(ip, now)
	l.fail(ip, now)
	l.success(ip)
	if !l.allow(ip, now) {
		t.Fatal("success must clear the lockout")
	}
}
