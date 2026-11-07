//go:build !windows

package main

// macOS 通过应用包的 CFBundleURLTypes 注册；不改动系统级设置。
func registerProtocol() error { return nil }
