// Package web serves the embedded React SPA (or proxies to Vite in dev mode).
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler serving the SPA with history fallback.
// When devOrigin is non-empty, requests are proxied to the Vite dev server.
func Handler(devOrigin string) http.Handler {
	if devOrigin != "" {
		target, err := url.Parse(devOrigin)
		if err == nil {
			return httputil.NewSingleHostReverseProxy(target)
		}
	}
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := sub.Open(p); err == nil {
			f.Close()
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
