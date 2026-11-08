//go:build windows

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

// 当前用户范围注册链接；不请求管理员权限，也不覆盖其他产品的处理器。
func registerProtocol() error {
	if os.Getenv("SKILLHUB_NATIVE_SMOKE") == "1" {
		return nil
	}
	path, err := os.Executable()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\skillhub`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	label, _, err := key.GetStringValue("")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	if label != "" && label != "URL:SkillHub Protocol" {
		return errors.New("skillhub URL scheme is owned by another application")
	}
	if err = key.SetStringValue("", "URL:SkillHub Protocol"); err != nil {
		return err
	}
	if err = key.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}
	command, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\skillhub\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer command.Close()
	return command.SetStringValue("", `"`+path+`" "%1"`)
}
