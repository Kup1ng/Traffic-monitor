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
	cfg := &config.Config{PollInterval: 2 * time.Second, FlushInterval: time.Minute, Location: time.UTC, Demo: true}
	eng, err := engine.New(cfg, collector.NewFakeReader("eth0", "boot-1"), st)
	if err != nil {
		t.Fatal(err)
	}
	a := auth.New(st, secret, time.Hour, "false")
	srv, err := NewServer(cfg, eng, a, "test")
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
