// Package web embeds the built frontend (the Nuxt static SPA) so the single
// binary can serve the UI and the API on one port.
//
// The `all:` prefix is required because Nuxt places its hashed assets under a
// "_nuxt" directory, and Go's embed excludes names beginning with "_" or "."
// unless `all:` is used. Before the frontend is built, public/ contains only a
// stub index.html so this package still compiles.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:public
var embedded embed.FS

// FS returns the embedded frontend rooted at the public/ directory.
func FS() (fs.FS, error) {
	return fs.Sub(embedded, "public")
}
