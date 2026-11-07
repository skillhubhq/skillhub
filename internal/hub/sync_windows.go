package hub

// Windows 不支持目录句柄的相同同步语义；包文件在改名之前已 FlushFileBuffers。
func syncPackageDirectory(string) error { return nil }
