// Package appupdate 只读取官方发布元数据，不读取 Hub 身份或安装任何程序。
package appupdate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const ReleasesURL = "https://github.com/skillhubhq/skillhub/releases"
const endpoint = "https://api.github.com/repos/skillhubhq/skillhub/releases?per_page=100"

type Result struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	URL       string `json:"url"`
	Asset     string `json:"asset"`
	CheckedAt string `json:"checked_at"`
}
type release struct {
	Tag        string `json:"tag_name"`
	Body       string `json:"body"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name  string `json:"name"`
		State string `json:"state"`
		Size  int64  `json:"size"`
	} `json:"assets"`
}

// Check 仅在用户明确检查时调用；不使用 GitHub Token，也不跟随重定向。
func Check(ctx context.Context, current, platform, arch string, preview bool) Result {
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return check(ctx, client, endpoint, current, platform, arch, preview)
}

func check(ctx context.Context, client *http.Client, address, current, platform, arch string, preview bool) Result {
	r := Result{Status: "unavailable", URL: ReleasesURL, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if platform == "darwin" {
		platform = "macos"
	}
	if !(platform == "macos" && arch == "arm64" || platform == "windows" && arch == "amd64") {
		r.Status = "unsupported"
		return r
	}
	current = "v" + strings.TrimPrefix(current, "v")
	if !semver.IsValid(current) {
		r.Status = "development"
		return r
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return r
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "SkillHub-update-check")
	response, err := client.Do(req)
	if err != nil {
		r.Status = "offline"
		return r
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		r.Status = "restricted"
		return r
	}
	if response.StatusCode == 429 || response.StatusCode == 403 {
		r.Status = "rate_limited"
		return r
	}
	if response.StatusCode != http.StatusOK {
		return r
	}
	const limit = 2 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(data) > limit {
		return r
	}
	var releases []release
	if json.Unmarshal(data, &releases) != nil || len(releases) >= 100 {
		return r
	}
	best := ""
	for _, item := range releases {
		if item.Draft || !semver.IsValid(item.Tag) || semver.Canonical(item.Tag) != item.Tag || !preview && (item.Prerelease || semver.Prerelease(item.Tag) != "") {
			continue
		}
		if best != "" && semver.Compare(item.Tag, best) <= 0 {
			continue
		}
		best = item.Tag
		r.Version = strings.TrimPrefix(item.Tag, "v")
		r.Notes = item.Body
		if len(r.Notes) > 16000 {
			r.Notes = string([]rune(item.Body)[:min(4000, len([]rune(item.Body)))]) + "…"
		}
		// 发布页由已验证的 tag 构造，不接受远端提供的任意外链。
		r.URL = ReleasesURL + "/tag/" + item.Tag
		r.Asset = ""
		prefix := "skillhub-app-" + item.Tag + "-" + platform + "-" + arch
		for _, asset := range item.Assets {
			if asset.State != "uploaded" || asset.Size <= 0 {
				continue
			}
			if asset.Name == prefix+".dmg" && platform == "macos" {
				r.Asset = asset.Name
				break
			}
			if asset.Name == prefix+".zip" {
				r.Asset = asset.Name
			}
		}
	}
	if best == "" {
		r.Status = "no_release"
	} else if semver.Compare(best, current) <= 0 {
		r.Status = "up_to_date"
	} else if r.Asset == "" {
		r.Status = "no_package"
	} else {
		r.Status = "available"
	}
	return r
}
