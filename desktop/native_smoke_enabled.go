//go:build native_smoke

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/skillhubhq/skillhub/internal/appupdate"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// 此服务只编入独立测试二进制；生产构建没有报告写入或自动退出入口。
type NativeSmokeReporter struct{ root string }
type NativeSmokeReport struct {
	Passed     bool     `json:"passed"`
	Checks     []string `json:"checks"`
	Error      string   `json:"error"`
	UserAgent  string   `json:"user_agent"`
	Location   string   `json:"location"`
	Platform   string   `json:"platform"`
	DurationMS float64  `json:"duration_ms"`
}

func nativeSmokeServices() ([]application.Service, error) {
	root := os.Getenv("SKILLHUB_HOME")
	if os.Getenv("SKILLHUB_NATIVE_SMOKE") != "1" || root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("native smoke build requires an isolated fixture")
	}
	marker, err := os.ReadFile(filepath.Join(root, ".native-smoke-fixture"))
	if err != nil || string(marker) != "skillhub-native-smoke-v1\n" {
		return nil, errors.New("native smoke fixture marker is missing")
	}
	// 原生 UI 使用确定性发布夹具；真实 HTTP/解析/隔离由 appupdate 测试覆盖。
	applicationUpdateCheck = func(_ context.Context, _, _, _ string, preview bool) appupdate.Result {
		if preview {
			return appupdate.Result{Status: "offline", URL: appupdate.ReleasesURL, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
		}
		return appupdate.Result{Status: "available", Version: "9.0.0", Notes: "Fixture release notes <script>must remain text</script>", URL: appupdate.ReleasesURL + "/tag/v9.0.0", Asset: "skillhub-app-v9.0.0-macos-arm64.dmg", CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	}
	return []application.Service{application.NewService(&NativeSmokeReporter{root: root})}, nil
}

func (r *NativeSmokeReporter) Finish(report NativeSmokeReport) error {
	if len(report.Checks) > 128 || len(report.Error) > 4096 || len(report.UserAgent) > 4096 || len(report.Location) > 1024 {
		return errors.New("invalid native smoke report")
	}
	report.Platform = runtime.GOOS
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(r.root, "native-smoke.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	if err = errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
		return err
	}
	go application.Get().Quit()
	return nil
}
