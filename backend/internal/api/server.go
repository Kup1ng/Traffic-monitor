// Package api wires the engine, store, and authenticator behind a stdlib HTTP
// server. The entire app (JSON API, SSE, and the embedded SPA) is served under a
// configurable secret base path; anything outside that path returns 404.
package api

import (
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/Kup1ng/Traffic-monitor/internal/auth"
	"github.com/Kup1ng/Traffic-monitor/internal/config"
	"github.com/Kup1ng/Traffic-monitor/internal/engine"
	"github.com/Kup1ng/Traffic-monitor/web"
)

// Server holds the dependencies shared by the HTTP handlers.
type Server struct {
	cfg     *config.Config
	eng     *engine.Engine
	auth    *auth.Authenticator
	version string
	files   map[string]web.File
}

// NewServer constructs the API server and prepares the embedded static files
// (with the base-path placeholder rewritten to the configured base path).
func NewServer(cfg *config.Config, eng *engine.Engine, a *auth.Authenticator, version string) (*Server, error) {
	files, err := web.BuildFS(cfg.BasePath)
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, eng: eng, auth: a, version: version, files: files}, nil
}

// Handler builds the routed, middleware-wrapped HTTP handler. Every route lives
// under cfg.BasePath; unmatched paths (including "/") fall through to a 404.
func (s *Server) Handler() http.Handler {
	base := s.cfg.BasePath
	mux := http.NewServeMux()

	// Public endpoints.
	mux.HandleFunc("POST "+base+"api/login", s.handleLogin)
	mux.HandleFunc("POST "+base+"api/logout", s.handleLogout)
	mux.HandleFunc("GET "+base+"api/session", s.handleSession)
	mux.HandleFunc("GET "+base+"api/version", s.handleVersion)

	// Protected endpoints (any other base+api/* path requires a valid session).
	protected := http.NewServeMux()
	protected.HandleFunc("GET "+base+"api/totals", s.handleTotals)
	protected.HandleFunc("GET "+base+"api/live", s.handleLive)
	protected.HandleFunc("GET "+base+"api/live/recent", s.handleRecent)
	protected.HandleFunc("GET "+base+"api/live/stream", s.handleStream)
	protected.HandleFunc("GET "+base+"api/history", s.handleHistory)
	protected.HandleFunc("GET "+base+"api/interface", s.handleInterface)
	protected.HandleFunc("GET "+base+"api/summary", s.handleSummary)
	mux.Handle(base+"api/", s.auth.RequireAuth(protected))

	// The embedded SPA on every other path under the base.
	mux.Handle(base, http.HandlerFunc(s.serveStatic))

	// Without these, ServeMux would 301-redirect the no-trailing-slash forms of
	// the subtree patterns (e.g. /<base> -> /<base>/), revealing that the secret
	// base path is valid. Return 404 for those probes instead so they look like
	// any other miss. (Guarded because TrimRight("/","/")=="" would panic.)
	if base != "/" {
		mux.HandleFunc(strings.TrimRight(base, "/"), http.NotFound)
		mux.HandleFunc(strings.TrimRight(base+"api/", "/"), http.NotFound)
	}

	return recoverMW(mux)
}

// serveStatic serves embedded files relative to the base path, falling back to
// index.html for unknown non-asset routes (client-side routing).
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, s.cfg.BasePath)
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	if rel == "" || rel == "." {
		rel = "index.html"
	}

	if f, ok := s.files[rel]; ok {
		if strings.HasPrefix(rel, "_nuxt/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		w.Header().Set("Content-Type", f.ContentType)
		_, _ = w.Write(f.Data)
		return
	}

	// A missing asset (hashed chunk, font, anything with a file extension) must
	// 404 — returning the HTML shell would make the browser fail with a MIME
	// error instead of a clean miss the SPA can recover from. Only extension-less
	// client routes fall through to index.html.
	if strings.HasPrefix(rel, "_nuxt/") || path.Ext(rel) != "" {
		http.NotFound(w, r)
		return
	}

	// SPA fallback to index.html for client-side routes.
	idx, ok := s.files["index.html"]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", idx.ContentType)
	_, _ = w.Write(idx.Data)
}

// recoverMW turns a handler panic into a 500 instead of crashing the server.
func recoverMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("api: panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
