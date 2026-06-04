// Package api wires the engine, store, and authenticator behind a stdlib HTTP
// server: a JSON/SSE REST API under /api and the embedded SPA on every other
// path.
package api

import (
	"io/fs"
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
	static  http.Handler
}

// NewServer constructs the API server and prepares the embedded static handler.
func NewServer(cfg *config.Config, eng *engine.Engine, a *auth.Authenticator, version string) (*Server, error) {
	sub, err := web.FS()
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, eng: eng, auth: a, version: version}
	s.static = s.makeStatic(sub)
	return s, nil
}

// Handler builds the routed, middleware-wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public endpoints.
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/session", s.handleSession)
	mux.HandleFunc("GET /api/version", s.handleVersion)

	// Protected endpoints (any other /api/* path requires a valid session).
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/totals", s.handleTotals)
	protected.HandleFunc("GET /api/live", s.handleLive)
	protected.HandleFunc("GET /api/live/recent", s.handleRecent)
	protected.HandleFunc("GET /api/live/stream", s.handleStream)
	protected.HandleFunc("GET /api/history", s.handleHistory)
	protected.HandleFunc("GET /api/interface", s.handleInterface)
	protected.HandleFunc("GET /api/summary", s.handleSummary)
	mux.Handle("/api/", s.auth.RequireAuth(protected))

	// Everything else: the embedded SPA.
	mux.Handle("/", s.static)

	return recoverMW(mux)
}

// makeStatic serves embedded files, falling back to index.html for unknown
// non-/api routes (client-side routing). Hashed assets under /_nuxt/ are served
// with a long immutable cache.
func (s *Server) makeStatic(sub fs.FS) http.Handler {
	fileServer := http.FileServerFS(sub)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		if f, err := sub.Open(name); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && !info.IsDir() {
				if strings.HasPrefix(r.URL.Path, "/_nuxt/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback to index.html.
		w.Header().Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		r2.URL.RawPath = ""
		fileServer.ServeHTTP(w, r2)
	})
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
