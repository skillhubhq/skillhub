//go:build !darwin && !windows

package main

import "unsafe"

// 其他平台由窗口管理器提供装饰，当前发行范围为 macOS 和 Windows。
func applyWindowAppearance(_ unsafe.Pointer, _ string, _ bool) error { return nil }
