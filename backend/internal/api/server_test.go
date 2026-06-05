package api

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kup1ng/Traffic-monitor/internal/auth"
	"github.com/Kup1ng/Traffic-monitor/internal/collector"
	"github.com/Kup1ng/Traffic-monitor/internal/config"
	"github.com/Kup1ng/Traffic-monitor/internal/engine"
	"github.com/Kup1ng/Traffic-monitor/internal/shaper"
	"github.com/Kup1ng/Traffic-monitor/internal/store"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := auth.SetPassword(st, "pw"); err != nil {
		t.Fatal(err)
	}
	secret, _ := st.GetOrCreateSessionSecret()
	cfg := &config.Config{PollInterval: 2 * time.Second, FlushInterval: time.Minute, Location: time.UTC, Demo: true, BasePath: "/"}
	eng, err := engine.New(cfg, collector.NewFakeReader("eth0", "boot-1"), st)
	if err != nil {
		t.Fatal(err)
	}
	a := auth.New(st, secret, time.Hour, "false", "/", false)
	srv, err := NewServer(cfg, eng, a, shaper.New(eng.Iface(), cfg.Demo, nil), "test")
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler()
}

func TestAPIAuthFlow(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	get := func(path string) (int, string) {
		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	login := func(pw string) int {
		resp, err := client.Post(ts.URL+"/api/login", "application/json", strings.NewReader(`{"password":"`+pw+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if code, _ := get("/api/totals"); code != http.StatusUnauthorized {
		t.Fatalf("totals without session = %d, want 401", code)
	}
	if code := login("wrong"); code != http.StatusUnauthorized {
		t.Fatalf("login wrong = %d, want 401", code)
	}
	if code := login("pw"); code != http.StatusOK {
		t.Fatalf("login correct = %d, want 200", code)
	}
	if code, body := get("/api/totals"); code != http.StatusOK || !strings.Contains(body, "rx_total") {
		t.Fatalf("totals with session = %d body=%s", code, body)
	}
	if code, body := get("/api/session"); code != http.StatusOK || !strings.Contains(body, `"authenticated":true`) {
		t.Fatalf("session = %d body=%s", code, body)
	}
}

func TestShapingAPI(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	do := func(method, path, body string) (int, string) {
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req, _ := http.NewRequest(method, ts.URL+path, r)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	if code := func() int { c, _ := do("POST", "/api/login", `{"password":"pw"}`); return c }(); code != http.StatusOK {
		t.Fatalf("login = %d", code)
	}

	// Initial state: supported (demo mode simulates shaping) and disabled.
	if code, body := do("GET", "/api/shaping", ""); code != http.StatusOK ||
		!strings.Contains(body, `"limit_mbps":0`) || !strings.Contains(body, `"supported":true`) ||
		!strings.Contains(body, `"active":false`) {
		t.Fatalf("initial shaping = %d body=%s", code, body)
	}
	// Absurd values are rejected.
	if code, _ := do("POST", "/api/shaping", `{"mbps":2000000}`); code != http.StatusBadRequest {
		t.Fatalf("absurd mbps = %d, want 400", code)
	}
	// Setting a limit applies (simulated) and persists it.
	if code, body := do("POST", "/api/shaping", `{"mbps":100}`); code != http.StatusOK ||
		!strings.Contains(body, `"limit_mbps":100`) || !strings.Contains(body, `"active":true`) {
		t.Fatalf("set 100 = %d body=%s", code, body)
	}
	if _, body := do("GET", "/api/shaping", ""); !strings.Contains(body, `"limit_mbps":100`) {
		t.Fatalf("persisted limit not reflected on GET: %s", body)
	}
	// Disabling clears and deactivates the limit.
	if code, body := do("POST", "/api/shaping", `{"mbps":0}`); code != http.StatusOK ||
		!strings.Contains(body, `"limit_mbps":0`) || !strings.Contains(body, `"active":false`) {
		t.Fatalf("disable = %d body=%s", code, body)
	}
}

func TestSPARootServesIndex(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), "Traffic Monitor") {
		t.Fatalf("root = %d body=%q", resp.StatusCode, string(b))
	}
}

func TestStaticAssetMissingReturns404(t *testing.T) {
	ts := httptest.NewServer(newTestHandler(t))
	defer ts.Close()

	for _, p := range []string{"/_nuxt/missing.js", "/nope.css", "/x.woff2"} {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (a missing asset must not return the HTML shell)", p, resp.StatusCode)
		}
	}
	// An extension-less client route still falls back to index.html.
	resp, err := http.Get(ts.URL + "/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /dashboard = %d, want 200 (SPA fallback)", resp.StatusCode)
	}
}

func TestSecretBasePath(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := auth.SetPassword(st, "pw"); err != nil {
		t.Fatal(err)
	}
	secret, _ := st.GetOrCreateSessionSecret()
	base := "/s3cretpath/"
	cfg := &config.Config{PollInterval: 2 * time.Second, FlushInterval: time.Minute, Location: time.UTC, Demo: true, BasePath: base}
	eng, err := engine.New(cfg, collector.NewFakeReader("eth0", "boot-1"), st)
	if err != nil {
		t.Fatal(err)
	}
	a := auth.New(st, secret, time.Hour, "false", base, false)
	srv, err := NewServer(cfg, eng, a, shaper.New(eng.Iface(), cfg.Demo, nil), "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	get := func(p string) int {
		resp, err := client.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// Nothing is reachable outside the base path.
	if c := get("/"); c != http.StatusNotFound {
		t.Fatalf("GET / = %d, want 404", c)
	}
	if c := get("/api/totals"); c != http.StatusNotFound {
		t.Fatalf("GET /api/totals = %d, want 404", c)
	}
	if c := get("/wrongpath/"); c != http.StatusNotFound {
		t.Fatalf("GET /wrongpath/ = %d, want 404", c)
	}
	// No-trailing-slash probes of the base must 404 (not 301), so the secret path
	// isn't revealed by a differential response.
	if c := get(strings.TrimRight(base, "/")); c != http.StatusNotFound {
		t.Fatalf("GET %s = %d, want 404 (no 301 leak)", strings.TrimRight(base, "/"), c)
	}
	if c := get(strings.TrimRight(base+"api/", "/")); c != http.StatusNotFound {
		t.Fatalf("GET %s = %d, want 404 (no 301 leak)", strings.TrimRight(base+"api/", "/"), c)
	}

	// The app lives under the base path.
	if c := get(base); c != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", base, c)
	}
	if c := get(base + "api/totals"); c != http.StatusUnauthorized {
		t.Fatalf("GET %sapi/totals (no auth) = %d, want 401", base, c)
	}

	// Login under the base sets a cookie scoped to the base path.
	resp, err := client.Post(ts.URL+base+"api/login", "application/json", strings.NewReader(`{"password":"pw"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d, want 200", resp.StatusCode)
	}
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName {
			found = true
			if c.Path != base {
				t.Fatalf("cookie Path = %q, want %q", c.Path, base)
			}
		}
	}
	if !found {
		t.Fatal("no session cookie set on login")
	}

	if c := get(base + "api/totals"); c != http.StatusOK {
		t.Fatalf("GET %sapi/totals (authed) = %d, want 200", base, c)
	}
}
