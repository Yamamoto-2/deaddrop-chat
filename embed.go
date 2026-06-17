package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// distFS holds the built frontend. Run `npm run build` in ./web before
// `go build` so this directory contains real assets.
//
//go:embed all:web/dist
var distFS embed.FS

// spaHandler serves the embedded frontend, falling back to index.html for
// unknown paths. (Routing is hash-based, so the fragment never reaches us.)
func spaHandler() http.Handler {
	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if _, err := fs.Stat(sub, r.URL.Path[1:]); err != nil {
				r = r.Clone(r.Context())
				r.URL.Path = "/"
			}
		}
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Hashed assets are immutable; index.html must revalidate so clients
		// pick up new builds instead of running a stale cached app.
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}
