// Package webui 提供与 Hub 同进程分发的 Web 静态资源。
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var assets embed.FS

// Handler 只分发构建产物；源代码检出未构建时明确返回不可用。
func Handler() http.Handler {
	sub, _ := fs.Sub(assets, "static/dist")
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if _, err := fs.Stat(sub, "index.html"); err != nil {
			http.Error(w, "Web UI not built; run npm run build:web in desktop/frontend before building the Hub", http.StatusServiceUnavailable)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api.") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		files.ServeHTTP(w, r)
	})
}
