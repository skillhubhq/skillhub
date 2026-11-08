package client

import (
	"errors"
	"net/url"
	"strings"

	"golang.org/x/mod/semver"
)

// OpenRequest 只携带来源与精确版本；不接受凭据、本地路径或自动安装指令。
type OpenRequest struct {
	Hub       string `json:"hub"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

func ParseOpenLink(raw string) (OpenRequest, error) {
	invalid := errors.New("invalid SkillHub link")
	if len(raw) > 2048 {
		return OpenRequest{}, invalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "skillhub" || u.Host != "open" || u.Path != "" || u.User != nil || u.Fragment != "" {
		return OpenRequest{}, invalid
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 3 {
		return OpenRequest{}, invalid
	}
	for _, key := range []string{"hub", "skill", "version"} {
		if len(q[key]) != 1 || q.Get(key) == "" {
			return OpenRequest{}, invalid
		}
	}
	hub, err := canonicalURL(q.Get("hub"))
	if err != nil {
		return OpenRequest{}, invalid
	}
	coordinate, err := ParseCoordinate("source/"+q.Get("skill")+"@"+q.Get("version"), true)
	if err != nil || !semver.IsValid("v"+coordinate.Version) || len(coordinate.Version) > 128 || strings.TrimPrefix(semver.Canonical("v"+coordinate.Version), "v") != strings.SplitN(coordinate.Version, "+", 2)[0] {
		return OpenRequest{}, invalid
	}
	return OpenRequest{Hub: hub, Namespace: coordinate.Namespace, Name: coordinate.Name, Version: coordinate.Version}, nil
}
func (r OpenRequest) WebURL() string {
	q := url.Values{"skill": {r.Namespace + "/" + r.Name}, "version": {r.Version}, "tab": {"readme"}}
	return r.Hub + "/#discover?" + q.Encode()
}
