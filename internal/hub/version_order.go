package hub

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// 排序键遵循 SemVer；默认版本优先选择稳定版，预发布仅在没有稳定版时使用。
func versionOrder(version string) string {
	v := "v" + version
	pre := semver.Prerelease(v)
	priority := "1"
	if pre != "" {
		priority = "0"
	}
	core := strings.SplitN(strings.SplitN(version, "+", 2)[0], "-", 2)[0]
	key := priority
	for _, n := range strings.Split(core, ".") {
		key += fmt.Sprintf("%03d%s", len(n), n)
	}
	if pre == "" {
		return key + "~"
	}
	for _, part := range strings.Split(strings.TrimPrefix(pre, "-"), ".") {
		numeric := true
		for _, r := range part {
			if r < '0' || r > '9' {
				numeric = false
				break
			}
		}
		if numeric {
			key += "0" + fmt.Sprintf("%03d%s", len(part), part) + "!"
		} else {
			key += "1" + part + "!"
		}
	}
	return key
}
