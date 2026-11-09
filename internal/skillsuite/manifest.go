// Package skillsuite 定义可发布的精确版本集合。
package skillsuite

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/skillhubhq/skillhub/internal/packageformat"
	"golang.org/x/mod/semver"
)

const Filename = "skillhub.suite.json"

type Member struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Digest    string `json:"digest"`
}
type Manifest struct {
	Schema  int      `json:"schema"`
	Entry   string   `json:"entry"`
	Members []Member `json:"members"`
}

func Parse(data []byte) (*Manifest, error) {
	if len(data) > 64<<10 {
		return nil, errors.New("suite manifest exceeds 64 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var m Manifest
	if err := d.Decode(&m); err != nil {
		return nil, errors.New("invalid suite manifest")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("invalid suite manifest")
	}
	if m.Schema != 1 || len(m.Members) < 1 || len(m.Members) > 50 {
		return nil, errors.New("suite requires 1–50 exact members")
	}
	names := map[string]bool{}
	entry := false
	for _, v := range m.Members {
		if !packageformat.ValidName(v.Namespace) || !packageformat.ValidName(v.Name) || len(v.Version) > 128 || !semver.IsValid("v"+v.Version) || strings.TrimPrefix(semver.Canonical("v"+v.Version), "v") != strings.SplitN(v.Version, "+", 2)[0] || !validDigest(v.Digest) || names[v.Name] {
			return nil, errors.New("invalid or conflicting suite member")
		}
		names[v.Name] = true
		if m.Entry == v.Namespace+"/"+v.Name {
			entry = true
		}
	}
	if !entry {
		return nil, errors.New("suite entry must identify one member")
	}
	return &m, nil
}
func validDigest(d string) bool {
	if len(d) != 71 || !strings.HasPrefix(d, "sha256:") {
		return false
	}
	for _, r := range d[7:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func FromBundle(b *packageformat.Bundle) (*Manifest, error) {
	for _, f := range b.Files {
		if f.Path == Filename {
			return Parse(f.Data)
		}
	}
	return nil, nil
}
