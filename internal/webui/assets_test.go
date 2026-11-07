package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDocumentationRoutesAndIsolation(t *testing.T) {
	h := handler(fstest.MapFS{
		"static/dist/index.html":          {Data: []byte("community")},
		"static/docs/index.html":          {Data: []byte("docs")},
		"static/docs/usage/index.html":    {Data: []byte("install guide")},
		"static/docs/api/search":          {Data: []byte(`{"indexes":[]}`)},
		"static/docs/_next/static/app.js": {Data: []byte("script")},
		"static/docs/csp.json":            {Data: []byte(`{"index.html":"script-src 'self' 'sha256-fixture'","usage/index.html":"script-src 'self'"}`)},
	})
	for _, tc := range []struct {
		path string
		code int
		body string
	}{
		{"/", 200, "community"}, {"/docs/", 200, "docs"}, {"/docs/usage/", 200, "install guide"},
		{"/docs", 301, ""}, {"/docs/usage", 301, ""}, {"/docs/api/search", 200, "indexes"},
		{"/docs/_next/static/app.js", 200, "script"}, {"/docs/missing", 404, ""},
		{"/docs/_next/", 404, ""}, {"/docs/csp.json", 404, ""},
		{"/docs/../dist/index.html", 404, ""}, {"/docs/%2e%2e/dist/index.html", 404, ""},
		{"/api.invalid.Service/Test", 404, ""}, {"/server.go", 404, ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			res := httptest.NewRecorder()
			h.ServeHTTP(res, httptest.NewRequest("GET", tc.path, nil))
			if res.Code != tc.code || !strings.Contains(res.Body.String(), tc.body) {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
			if tc.path == "/docs/" && !strings.Contains(res.Header().Get("Content-Security-Policy"), "sha256-fixture") {
				t.Fatal("missing inline script hashes")
			}
		})
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest("POST", "/docs/", nil))
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatal(res.Code)
	}
}

func TestDocumentationFailsClosedWithoutPolicy(t *testing.T) {
	h := handler(fstest.MapFS{"static/docs/index.html": {Data: []byte("not verified")}})
	res := httptest.NewRecorder()
	h.ServeHTTP(res, httptest.NewRequest("GET", "/docs/", nil))
	if res.Code != http.StatusServiceUnavailable || strings.Contains(res.Body.String(), "not verified") {
		t.Fatal(res.Code, res.Body.String())
	}
}

func TestEmbeddedDocumentationIncludesNextAssets(t *testing.T) {
	page, err := fs.ReadFile(assets, "static/docs/index.html")
	if err != nil {
		t.Skip("documentation export is checked by make web")
	}
	refs := regexp.MustCompile(`(?:src|href)="(/docs/[^"?#]+\.(?:js|css))"`).FindAllStringSubmatch(string(page), -1)
	if len(refs) == 0 {
		t.Fatal("documentation has no compiled assets")
	}
	for _, ref := range refs {
		res := httptest.NewRecorder()
		Handler().ServeHTTP(res, httptest.NewRequest("GET", ref[1], nil))
		if res.Code != http.StatusOK {
			t.Errorf("embedded asset %s returned %d", ref[1], res.Code)
		}
	}
}
