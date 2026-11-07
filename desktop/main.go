package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed frontend/public/assets/brand/app-icon.png
var appIcon []byte

func main() {
	services, err := nativeSmokeServices()
	if err != nil {
		slog.Error("desktop test setup failed", "error", err)
		os.Exit(1)
	}
	services = append(services, application.NewService(&DesktopService{}))
	app := application.New(application.Options{
		Name:        "SkillHub",
		Icon:        appIcon,
		Description: "Your skills, across sources and tools",
		Services:    services,
		Assets:      application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "SkillHub", Width: 1120, Height: 760, MinWidth: 840, MinHeight: 560, URL: "/",
	})
	if err := app.Run(); err != nil {
		slog.Error("desktop stopped", "error", err)
		os.Exit(1)
	}
}
