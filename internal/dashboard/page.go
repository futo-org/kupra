package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// embedded is the built page, which `mise run web:build` copies into page/. Without it, the dashboard says so instead.
//
//go:embed all:page
var embedded embed.FS

// builtPage returns the embedded page.
func builtPage() fs.FS {
	page, _ := fs.Sub(embedded, "page")
	return page
}

// spa serves the built page, and its index for any path that isn't a file, so the page's own routes load.
func spa(page fs.FS) http.Handler {
	files := http.FileServerFS(page)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/auth/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if info, err := fs.Stat(page, name); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(page, "index.html"); err != nil {
			http.Error(w, "this dashboard was built without its page: run mise run web:build before building it", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, page, "index.html")
	})
}
