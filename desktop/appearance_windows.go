package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var dwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

func applyWindowAppearance(handle unsafe.Pointer, _ string, dark bool) error {
	// 使用系统绘制的标题栏和窗口按钮，只同步颜色；前端已解析跟随系统模式。
	var enabled uint32
	colour := uint32(0x00ffffff)
	if dark {
		enabled = 1
		colour = 0x00221710
	}
	for _, setting := range []struct {
		attribute uintptr
		value     uint32
	}{{20, enabled}, {35, colour}} {
		result, _, _ := dwmSetWindowAttribute.Call(uintptr(handle), setting.attribute, uintptr(unsafe.Pointer(&setting.value)), 4)
		if int32(result) < 0 {
			return fmt.Errorf("window appearance failed: 0x%x", result)
		}
	}
	return nil
}
