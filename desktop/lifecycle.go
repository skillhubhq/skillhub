package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"golang.org/x/image/draw"
)

type DesktopPreferences struct {
	KeepRunning  bool   `json:"keep_running"`
	StartAtLogin bool   `json:"start_at_login"`
	Language     string `json:"language"`
}

var desktopLifecycle = struct {
	sync.Mutex
	preferences DesktopPreferences
	window      *application.WebviewWindow
	app         *application.App
	tray        *application.SystemTray
	action      string
}{preferences: DesktopPreferences{KeepRunning: true, Language: "en"}}

func preferencePath() (string, error) {
	root, err := client.DataDir()
	return filepath.Join(root, "desktop-preferences.json"), err
}
func readDesktopPreferences() (DesktopPreferences, error) {
	v := DesktopPreferences{KeepRunning: true, Language: "en"}
	path, err := preferencePath()
	if err != nil {
		return v, err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if os.IsNotExist(err) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	defer root.Close()
	file, err := root.Open(filepath.Base(path))
	if os.IsNotExist(err) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	defer file.Close()
	b, err := io.ReadAll(io.LimitReader(file, 16384))
	if os.IsNotExist(err) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(b, &v)
	return v, err
}
func (s *DesktopService) DesktopPreferences() DesktopPreferences {
	desktopLifecycle.Lock()
	defer desktopLifecycle.Unlock()
	return desktopLifecycle.preferences
}
func (s *DesktopService) SetDesktopPreferences(v DesktopPreferences) error {
	desktopLifecycle.Lock()
	defer desktopLifecycle.Unlock()
	if v.Language != "zh" {
		v.Language = "en"
	}
	path, err := preferencePath()
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	previous := desktopLifecycle.preferences
	raw, _ := json.Marshal(v)
	name := ".desktop-preferences-" + uuid.NewString()
	temp, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	if _, err = temp.Write(raw); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if v.StartAtLogin != previous.StartAtLogin {
		if err = setStartAtLogin(v.StartAtLogin); err != nil {
			return err
		}
	}
	if err == nil {
		err = root.Rename(name, filepath.Base(path))
	}
	if err != nil {
		if v.StartAtLogin != previous.StartAtLogin {
			_ = setStartAtLogin(previous.StartAtLogin)
		}
		return err
	}
	desktopLifecycle.preferences = v
	if desktopLifecycle.tray != nil {
		updateTrayMenuLocked()
	}
	return nil
}
func (s *DesktopService) TakeDesktopAction() string {
	desktopLifecycle.Lock()
	defer desktopLifecycle.Unlock()
	value := desktopLifecycle.action
	desktopLifecycle.action = ""
	return value
}

func showDesktop(action string) {
	desktopLifecycle.Lock()
	w := desktopLifecycle.window
	if action != "" {
		desktopLifecycle.action = action
	}
	desktopLifecycle.Unlock()
	if w != nil {
		w.Show()
		w.Restore()
		w.Focus()
		if action != "" {
			w.EmitEvent("desktop-action")
		}
	}
}
func updateTrayMenuLocked() {
	d := &desktopLifecycle
	label := func(en, zh string) string {
		if d.preferences.Language == "zh" {
			return zh
		}
		return en
	}
	menu := d.app.NewMenu()
	menu.Add(label("Open SkillHub", "打开 SkillHub")).OnClick(func(*application.Context) { showDesktop("") })
	menu.Add(label("Inbox", "通知")).OnClick(func(*application.Context) { showDesktop("inbox") })
	menu.Add(label("Check for updates…", "检查更新…")).OnClick(func(*application.Context) { showDesktop("updates") })
	menu.Add(label("Settings…", "设置…")).OnClick(func(*application.Context) { showDesktop("settings") })
	menu.AddSeparator()
	menu.Add(label("Quit SkillHub", "退出 SkillHub")).OnClick(func(*application.Context) { d.app.Quit() })
	d.tray.SetMenu(menu)
}

// 模板图只保留品牌标记的不透明像素，由系统按菜单栏背景着色。
func trayTemplate() []byte {
	src, err := png.Decode(bytes.NewReader(appIcon))
	if err != nil {
		return appIcon
	}
	dst := image.NewNRGBA(src.Bounds())
	for y := src.Bounds().Min.Y; y < src.Bounds().Max.Y; y++ {
		for x := src.Bounds().Min.X; x < src.Bounds().Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			if r > 60000 && g > 60000 && b > 60000 {
				dst.SetNRGBA(x, y, color.NRGBA{A: uint8(a >> 8)})
			}
		}
	}
	small := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	draw.CatmullRom.Scale(small, small.Bounds(), dst, image.Rect(190, 160, 830, 860), draw.Src, nil)
	var b bytes.Buffer
	_ = png.Encode(&b, small)
	return b.Bytes()
}
func configureLifecycle(app *application.App, w *application.WebviewWindow, v DesktopPreferences) {
	desktopLifecycle.Lock()
	desktopLifecycle.preferences = v
	desktopLifecycle.app = app
	desktopLifecycle.window = w
	tray := app.SystemTray.New()
	desktopLifecycle.tray = tray
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayTemplate())
	} else {
		tray.SetIcon(appIcon)
	}
	tray.OnClick(func() { tray.OpenMenu() })
	updateTrayMenuLocked()
	desktopLifecycle.Unlock()
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		desktopLifecycle.Lock()
		keep := desktopLifecycle.preferences.KeepRunning
		desktopLifecycle.Unlock()
		if keep {
			e.Cancel()
			w.Hide()
		} else {
			app.Quit()
		}
	})
	if runtime.GOOS == "darwin" {
		app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { showDesktop("") })
	}
}
