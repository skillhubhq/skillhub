package main

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
)

func setStartAtLogin(enabled bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "io.github.skillhubhq.skillhub.plist")
	if !enabled {
		err = os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	var escaped bytes.Buffer
	if err = xml.EscapeText(&escaped, []byte(exe)); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	text := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>io.github.skillhubhq.skillhub</string><key>ProgramArguments</key><array><string>` + escaped.String() + `</string><string>--background</string></array><key>RunAtLoad</key><true/></dict></plist>`
	return os.WriteFile(path, []byte(text), 0600)
}
