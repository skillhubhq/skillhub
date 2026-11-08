//go:build darwin

package main

/*
#cgo LDFLAGS: -framework CoreServices -framework CoreFoundation
#include <CoreServices/CoreServices.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

// 直接启动、覆盖安装时也刷新当前 bundle 的 URL 协议；测试 bundle 不参与系统关联。
func registerProtocol() error {
	if os.Getenv("SKILLHUB_NATIVE_SMOKE") == "1" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if !strings.HasSuffix(bundle, ".app") {
		return nil
	}
	if _, err := os.Stat(filepath.Join(bundle, "Contents", "Info.plist")); err != nil {
		return err
	}
	path := C.CString(bundle)
	defer C.free(unsafe.Pointer(path))
	url := C.CFURLCreateFromFileSystemRepresentation(C.kCFAllocatorDefault, (*C.UInt8)(unsafe.Pointer(path)), C.CFIndex(len(bundle)), C.Boolean(1))
	if url == 0 {
		return errors.New("cannot resolve application bundle")
	}
	defer C.CFRelease(C.CFTypeRef(url))
	if C.LSRegisterURL(url, C.Boolean(1)) != C.noErr {
		return errors.New("cannot register browser links for this application")
	}
	return nil
}
