package client

// Windows 不支持普通目录句柄的 fsync；文件内容已在暂存时 FlushFileBuffers。
func syncDirectory(_ string) error { return nil }
