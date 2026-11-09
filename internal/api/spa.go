package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the embedded single-page app. Unknown paths fall back to
// index.html so that client-side routes survive a page reload.
func spaHandler(static fs.FS) http.Handler {
	if static == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "web UI is not built into this binary", http.StatusNotFound)
		})
	}
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if st, err := fs.Stat(static, p); err != nil || st.IsDir() {
			if path.Ext(p) != "" && p != "index.html" {
				// A missing asset is a real 404, not a client route.
				http.NotFound(w, r)
				return
			}
			p = "index.html"
		}
		if p == "index.html" {
			serveIndex(w, r, static)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			// Vite puts content hashes into asset file names.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

// serveIndex writes index.html directly: http.ServeFileFS would redirect
// "/index.html" to "/", which is pointless for an SPA.
func serveIndex(w http.ResponseWriter, r *http.Request, static fs.FS) {
	b, err := fs.ReadFile(static, "index.html")
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method != http.MethodHead {
		_, _ = w.Write(b)
	}
}
