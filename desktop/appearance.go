package main

import (
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ApplyAppearance 仅同步窗口外观；不改变系统主题或 Hub 身份。
func (s *DesktopService) ApplyAppearance(mode string, dark bool) error {
	if mode != "system" && mode != "light" && mode != "dark" {
		return errors.New("invalid appearance")
	}
	app := application.Get()
	if app == nil {
		return errors.New("native application is unavailable")
	}
	window, ok := app.Window.GetByName("main")
	if !ok {
		return errors.New("main window is unavailable")
	}
	webview, ok := window.(*application.WebviewWindow)
	if !ok {
		return errors.New("main window is not a webview")
	}
	if mode != "system" {
		dark = mode == "dark"
	}
	var err error
	application.InvokeSync(func() { err = applyWindowAppearance(webview.NativeWindow(), mode, dark) })
	return err
}
