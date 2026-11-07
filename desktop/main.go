package main

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed frontend/public/assets/brand/app-icon.png
var appIcon []byte

func main() {
	if err := registerProtocol(); err != nil {
		slog.Warn("browser link registration unavailable", "error", err)
	}
	services, err := nativeSmokeServices()
	if err != nil {
		slog.Error("desktop test setup failed", "error", err)
		os.Exit(1)
	}
	services = append(services, application.NewService(&DesktopService{}))
	root, err := client.DataDir()
	if err != nil {
		slog.Error("client directory unavailable", "error", err)
		os.Exit(1)
	}
	var window atomic.Pointer[application.WebviewWindow]
	handleLink := func(raw string) {
		queueOpenLink(raw)
		if active := window.Load(); active != nil {
			active.Show()
			active.Restore()
			active.Focus()
		}
	}
	for _, arg := range os.Args[1:] {
		queueOpenLink(arg)
	}
	app := application.New(application.Options{
		Name: "SkillHub",
		SingleInstance: &application.SingleInstanceOptions{UniqueID: fmt.Sprintf("io.github.skillhubhq.skillhub.%x", sha256.Sum256([]byte(root))), OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
			for _, arg := range data.Args {
				handleLink(arg)
			}
		}},
		Icon:        appIcon,
		Description: "Your skills, across sources and tools",
		Services:    services,
		Assets:      application.AssetOptions{Handler: application.BundledAssetFileServer(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) { handleLink(event.Context().URL()) })
	window.Store(app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "SkillHub", Width: 1120, Height: 760, MinWidth: 840, MinHeight: 560, URL: "/",
	}))
	if err := app.Run(); err != nil {
		slog.Error("desktop stopped", "error", err)
		os.Exit(1)
	}
}
