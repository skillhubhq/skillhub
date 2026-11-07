// Package webui 提供与 Hub 同进程分发的 Web 静态资源。
package webui

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:static
var assets embed.FS

// Handler 只分发构建产物；源代码检出未构建时明确返回不可用。
func Handler() http.Handler {
	return handler(assets)
}

func handler(assets fs.FS) http.Handler {
	sub, _ := fs.Sub(assets, "static/dist")
	files := http.FileServer(http.FS(sub))
	docs, _ := fs.Sub(assets, "static/docs")
	docsFiles := http.StripPrefix("/docs/", http.FileServer(http.FS(docs)))
	policies := map[string]string{}
	if raw, err := fs.ReadFile(docs, "csp.json"); err == nil {
		_ = json.Unmarshal(raw, &policies)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/docs" {
			http.Redirect(w, r, "/docs/", http.StatusMovedPermanently)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/docs/") {
			name := strings.TrimPrefix(r.URL.Path, "/docs/")
			if name == "" {
				name = "index.html"
			}
			if strings.HasSuffix(name, "/") {
				name += "index.html"
			}
			if !fs.ValidPath(name) || name == "csp.json" || strings.HasSuffix(name, ".map") {
				http.NotFound(w, r)
				return
			}
			info, err := fs.Stat(docs, name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if info.IsDir() {
				if _, err := fs.Stat(docs, path.Join(name, "index.html")); err != nil {
					http.NotFound(w, r)
					return
				}
				http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
				return
			}
			policy := "default-src 'none'; frame-ancestors 'none'; base-uri 'none'"
			if strings.HasSuffix(name, ".html") {
				var ok bool
				policy, ok = policies[name]
				if !ok {
					http.Error(w, "Documentation not built; run make web", http.StatusServiceUnavailable)
					return
				}
			}
			w.Header().Set("Content-Security-Policy", policy)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "same-origin")
			docsFiles.ServeHTTP(w, r)
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
