package hubhttp

import (
	"net/url"
	"strings"
)

// 仅允许站内社区路由和设备确认页，不接受外部地址或任意服务端端点。
func returnPath(raw string) string {
	if raw == "authorize" {
		return "/authorize"
	}
	if len(raw) > 4096 || strings.ContainsAny(raw, "\\\r\n\x00") || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil || u.RawPath != "" || (u.Path != "/" && u.Path != "/authorize") {
		return "/"
	}
	return u.String()
}
