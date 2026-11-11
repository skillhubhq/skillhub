package main

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
)

func setStartAtLogin(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if !enabled {
		err = key.DeleteValue("SkillHub")
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return key.SetStringValue("SkillHub", windows.EscapeArg(exe)+" --background")
}
