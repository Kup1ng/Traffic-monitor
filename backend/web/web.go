// Package web embeds the built frontend (the Nuxt static SPA) and rewrites its
// build-time base-path placeholder to the runtime secret base path.
//
// The frontend is built once with a fixed placeholder base ("/__TM_BASE__/")
// baked into every asset URL, the router base, and Vite's import.meta.env.BASE_URL.
// At startup the server replaces that placeholder with the configured base path,
// so a single binary can be served under any secret path chosen at install time
// without rebuilding.
//
// The `all:` prefix is required because Nuxt places hashed assets under a "_nuxt"
// directory, and Go's embed excludes names beginning with "_" or "." otherwise.
package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:public
var embedded embed.FS

// BasePlaceholder is the base path baked into the production build. It must match
// TM_BUILD_BASE used by the build scripts.
const BasePlaceholder = "/__TM_BASE__/"

// File is a prepared, ready-to-serve asset.
type File struct {
	ContentType string
	Data        []byte
}

// BuildFS reads the embedded frontend and returns a map of relative path -> File
// with the base-path placeholder rewritten to basePath in every text asset.
func BuildFS(basePath string) (map[string]File, error) {
	sub, err := fs.Sub(embedded, "public")
	if err != nil {
		return nil, err
	}
	files := make(map[string]File)
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		if isTextAsset(p) {
			data = bytes.ReplaceAll(data, []byte(BasePlaceholder), []byte(basePath))
		}
		// Some Nuxt builds nest output under the base dir; normalize the key.
		key := strings.TrimPrefix(p, strings.Trim(BasePlaceholder, "/")+"/")
		files[key] = File{ContentType: contentType(p, data), Data: data}
		return nil
	})
	return files, err
}

func isTextAsset(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".html", ".js", ".mjs", ".css", ".json", ".svg", ".txt", ".map", ".webmanifest":
		return true
	}
	return false
}

func contentType(p string, data []byte) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json", ".map", ".webmanifest":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".ttf":
		return "font/ttf"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return http.DetectContentType(data)
	}
}
