package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopPreferencesPersistAndDefault(t *testing.T) {
	t.Setenv("SKILLHUB_HOME", t.TempDir())
	v, err := readDesktopPreferences()
	if err != nil || !v.KeepRunning || v.StartAtLogin || v.Language != "en" {
		t.Fatal("wrong defaults", err)
	}
	desktopLifecycle.Lock()
	previous := desktopLifecycle.preferences
	desktopLifecycle.preferences = v
	desktopLifecycle.Unlock()
	defer func() { desktopLifecycle.Lock(); desktopLifecycle.preferences = previous; desktopLifecycle.Unlock() }()
	service := &DesktopService{}
	v.KeepRunning = false
	v.Language = "zh"
	if err = service.SetDesktopPreferences(v); err != nil {
		t.Fatal(err)
	}
	saved, err := readDesktopPreferences()
	if err != nil || saved != v {
		t.Fatal("preferences lost", err)
	}
	path, _ := preferencePath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("preferences too permissive")
	}
	entries, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".desktop-preferences-*"))
	if len(entries) != 0 {
		t.Fatal("temporary preferences remain")
	}
}
func TestTrayTemplateHasVisibleMarkAndTransparency(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(trayTemplate()))
	if err != nil {
		t.Fatal(err)
	}
	transparent, opaque := 0, 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 0 {
				t.Fatal("template is not monochrome")
			}
			if a == 0 {
				transparent++
			}
			if a > 32767 {
				opaque++
			}
		}
	}
	if opaque < 100 || transparent < 300 {
		t.Fatal("tray mark is blank or a solid square", opaque, transparent)
	}
}

func TestDesktopPreferencesRejectEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SKILLHUB_HOME", root)
	outside := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(outside, []byte(`{"keep_running":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "desktop-preferences.json")); err != nil {
		t.Skip("symlinks unavailable", err)
	}
	if _, err := readDesktopPreferences(); err == nil {
		t.Fatal("read preferences outside the data directory")
	}
}
