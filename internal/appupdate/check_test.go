package appupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// 固定发布来源与下载链接，防止迁移后仍请求其他仓库或信任响应中的外链。
func TestOfficialDistributionRequest(t *testing.T) {
	for _, code := range []int{http.StatusOK, http.StatusNotFound} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodGet || r.URL.String() != "https://api.github.com/repos/skillhubhq/skillhub/releases?per_page=100" {
					t.Fatalf("unexpected update request: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Fatal("update request contains credentials")
				}
				body := `[{"tag_name":"v1.1.0","html_url":"https://untrusted.invalid/release","assets":[{"name":"skillhub-app-v1.1.0-macos-arm64.dmg","state":"uploaded","size":12,"browser_download_url":"https://untrusted.invalid/app"}]}]`
				return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			got := check(context.Background(), client, endpoint, "1.0.0", "darwin", "arm64", false)
			wantURL, wantStatus := "https://github.com/skillhubhq/skillhub/releases", "restricted"
			if code == http.StatusOK {
				wantURL += "/tag/v1.1.0"
				wantStatus = "available"
			}
			if calls != 1 || got.URL != wantURL || got.Status != wantStatus {
				t.Fatalf("unexpected distribution result (%d calls): %+v", calls, got)
			}
		})
	}
}

func TestCheckReleaseSelection(t *testing.T) {
	for _, test := range []struct {
		name, body, platform, arch string
		preview                    bool
		status, version, asset     string
	}{
		{"stable skips preview", `[ {"tag_name":"v2.0.0-beta.1","prerelease":true}, {"tag_name":"v1.1.0","body":"<script>untrusted</script>","html_url":"https://evil.invalid","assets":[{"name":"skillhub-app-v1.1.0-macos-arm64.zip","state":"uploaded","size":9},{"name":"skillhub-app-v1.1.0-macos-arm64.dmg","state":"uploaded","size":12}]} ]`, "darwin", "arm64", false, "available", "1.1.0", "skillhub-app-v1.1.0-macos-arm64.dmg"},
		{"preview opt in", `[{"tag_name":"v2.0.0-beta.1","prerelease":true,"assets":[{"name":"skillhub-app-v2.0.0-beta.1-windows-amd64.zip","state":"uploaded","size":9}]}]`, "windows", "amd64", true, "available", "2.0.0-beta.1", "skillhub-app-v2.0.0-beta.1-windows-amd64.zip"},
		{"no downgrade", `[{"tag_name":"v0.9.0"}]`, "darwin", "arm64", false, "up_to_date", "0.9.0", ""},
		{"missing package is not current", `[{"tag_name":"v2.0.0","assets":[{"name":"skillhub-app-v2.0.0-windows-amd64.zip","state":"uploaded","size":9}]}]`, "darwin", "arm64", false, "no_package", "2.0.0", ""},
		{"draft and invalid tag", `[{"tag_name":"v9.0.0","draft":true},{"tag_name":"../../evil"}]`, "darwin", "arm64", false, "no_release", "", ""},
		{"preview only stable channel", `[{"tag_name":"v2.0.0-beta.1"}]`, "darwin", "arm64", false, "no_release", "", ""},
		{"malformed", `{`, "darwin", "arm64", false, "unavailable", "", ""},
		{"platform", `[]`, "linux", "amd64", false, "unsupported", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("Referer") != "" {
					t.Error("credential or source leaked")
				}
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			result := check(context.Background(), server.Client(), server.URL, "1.0.0", test.platform, test.arch, test.preview)
			if result.Status != test.status || result.Version != test.version || result.Asset != test.asset {
				t.Fatalf("unexpected result: %+v", result)
			}
			if !strings.HasPrefix(result.URL, ReleasesURL) {
				t.Fatal("untrusted release URL")
			}
		})
	}
}

func TestCheckFailures(t *testing.T) {
	for code, status := range map[int]string{404: "restricted", 403: "rate_limited", 429: "rate_limited", 500: "unavailable", 302: "unavailable"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) }))
		r := check(context.Background(), server.Client(), server.URL, "1.0.0", "darwin", "arm64", false)
		server.Close()
		if r.Status != status {
			t.Fatalf("status %d: %s", code, r.Status)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", (2<<20)+1)) }))
	defer server.Close()
	if r := check(context.Background(), server.Client(), server.URL, "1.0.0", "darwin", "arm64", false); r.Status != "unavailable" {
		t.Fatal("unbounded response")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := check(ctx, server.Client(), server.URL, "1.0.0", "darwin", "arm64", false); r.Status != "offline" {
		t.Fatal("cancel not reported")
	}
}
