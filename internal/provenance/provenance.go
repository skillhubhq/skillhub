// Package provenance 定义收录来源声明，不将外部作者映射为 Hub 登录账号。
package provenance

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

// Record 随版本固定；维护者由版本所属命名空间确定，操作账号由认证身份确定。
type Record struct {
	AuthorName  string `json:"author_name"`
	AuthorURL   string `json:"author_url"`
	SourceURL   string `json:"source_url"`
	Revision    string `json:"revision"`
	License     string `json:"license"`
	LicenseFile string `json:"license_file"`
	Modified    bool   `json:"modified"`
	Changes     string `json:"changes"`
}

var revision = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var license = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]{0,79}$`)

func cleanText(v string, max int) bool {
	return strings.TrimSpace(v) == v && utf8.ValidString(v) && utf8.RuneCountInString(v) <= max && strings.IndexFunc(v, func(r rune) bool {
		return unicode.IsControl(r) || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069
	}) < 0
}
func publicLink(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && len(raw) <= 2048 && cleanText(raw, 2048) && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Opaque == ""
}

// Validate 验证声明和随包许可证；不联网，也不声称验证了作者身份或许可证适用性。
func Validate(p *Record, bundle *packageformat.Bundle) error {
	if p == nil {
		return nil
	}
	if p.AuthorName == "" || !cleanText(p.AuthorName, 120) || !publicLink(p.AuthorURL) || !publicLink(p.SourceURL) || !revision.MatchString(p.Revision) || !license.MatchString(p.License) || !cleanText(p.Changes, 1000) || p.Modified && p.Changes == "" || !p.Modified && p.Changes != "" {
		return errors.New("invalid upstream attribution")
	}
	for _, f := range bundle.Files {
		if f.Path == p.LicenseFile && len(f.Data) > 0 && len(f.Data) <= 256<<10 && utf8.Valid(f.Data) {
			return nil
		}
	}
	return errors.New("upstream license file is missing or invalid")
}
func Encode(p *Record) string { b, _ := json.Marshal(p); return string(b) }
func Decode(raw string) (*Record, error) {
	var p *Record
	e := json.Unmarshal([]byte(raw), &p)
	return p, e
}

// Read 读取显式提供的声明文件；未知属性或尾随内容会被拒绝。
func Read(r io.Reader) (*Record, error) {
	b, e := io.ReadAll(io.LimitReader(r, 16385))
	if e != nil || len(b) > 16384 {
		return nil, errors.New("attribution document exceeds limits")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	var p *Record
	if e = d.Decode(&p); e != nil || p == nil {
		return nil, errors.New("invalid attribution document")
	}
	if e = d.Decode(new(any)); !errors.Is(e, io.EOF) {
		return nil, errors.New("invalid attribution document trailer")
	}
	return p, nil
}
