package client

import (
	"os"
	"path/filepath"
)

// DataDir 是 CLI 与 App 唯一的本地数据根解析入口。
func DataDir() (string, error) {
	if root := os.Getenv("SKILLHUB_HOME"); root != "" {
		return filepath.Abs(root)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "skillhub"), nil
}
