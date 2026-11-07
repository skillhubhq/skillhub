package main

import (
	"context"
	"runtime"

	"github.com/skillhubhq/skillhub/internal/appupdate"
)

// 构建时注入版本与来源；本地修改不会冒充正式发布包。
var appVersion = "dev"
var appRevision = "unknown"
var appBuild = "development"
var applicationUpdateCheck = appupdate.Check

type AppInfo struct {
	Version      string `json:"version"`
	Revision     string `json:"revision"`
	Build        string `json:"build"`
	Platform     string `json:"platform"`
	Architecture string `json:"architecture"`
}

func (s *DesktopService) ApplicationInfo() AppInfo {
	return AppInfo{Version: appVersion, Revision: appRevision, Build: appBuild, Platform: runtime.GOOS, Architecture: runtime.GOARCH}
}

func (s *DesktopService) CheckApplicationUpdate(preview bool) appupdate.Result {
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return applicationUpdateCheck(ctx, appVersion, runtime.GOOS, runtime.GOARCH, preview)
}
