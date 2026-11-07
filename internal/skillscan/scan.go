// Package skillscan 对已验证技能包进行有界、离线静态检查，不执行文件或访问网络。
package skillscan

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

const Engine = "builtin-static/1"
const MaxFindings = 100

type Finding struct {
	Rule     string `json:"rule"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Blocking bool   `json:"blocking"`
	Message  string `json:"message"`
}
type Report struct {
	Engine   string    `json:"engine"`
	Digest   string    `json:"digest"`
	Complete bool      `json:"complete"`
	Blocking bool      `json:"blocking"`
	Findings []Finding `json:"findings"`
}
type rule struct {
	id       string
	pattern  *regexp.Regexp
	blocking bool
	message  string
}

var rules = []rule{
	{"private-key", regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`), true, "Remove embedded private key material; rotate exposed credentials."},
	{"credential-token", regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{36,255}|AKIA[A-Z0-9]{16})\b`), true, "Remove a credential-shaped token; verify and rotate it outside the package."},
	{"download-execute", regexp.MustCompile(`(?i)(curl|wget)\b[^\n]{0,1000}\|\s*(sh|bash|zsh|python)\b`), false, "Remote content is piped into an interpreter. Review origin and integrity checks."},
	{"destructive-command", regexp.MustCompile(`(?i)\brm\s+(-[a-z]*[rf][a-z]*\s+){1,3}(/\s|/\*|~(/|\s))`), false, "Potentially destructive filesystem command. Review scope and safeguards."},
	{"instruction-override", regexp.MustCompile(`(?i)(ignore (all |any )?(previous|prior|system) instructions|忽略.{0,8}(系统|之前|所有).{0,4}指令)`), false, "Instruction override language requires human review in context."},
}

// Scan 的报告只包含规则、位置与固定说明，不能复制命中的秘密或正文。
func Scan(ctx context.Context, b *packageformat.Bundle) (Report, error) {
	r := Report{Engine: Engine, Digest: b.Digest, Complete: true, Findings: []Finding{}}
	add := func(f Finding) {
		if len(r.Findings) >= MaxFindings {
			r.Complete = false
			r.Blocking = true
			return
		}
		r.Findings = append(r.Findings, f)
		r.Blocking = r.Blocking || f.Blocking
	}
	for _, f := range b.Files {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		if !utf8.Valid(f.Data) || strings.IndexByte(string(f.Data), 0) >= 0 {
			r.Complete = false
			add(Finding{Rule: "uninspected-binary", Path: f.Path, Message: "Binary content was not inspected. An independent reviewer must inspect this file."})
			continue
		}
		text := string(f.Data)
		for _, rule := range rules {
			locations := rule.pattern.FindAllStringIndex(text, MaxFindings+1)
			for _, loc := range locations {
				if len(r.Findings) >= MaxFindings {
					r.Complete = false
					r.Blocking = true
					return r, nil
				}
				add(Finding{Rule: rule.id, Path: f.Path, Line: 1 + strings.Count(text[:loc[0]], "\n"), Blocking: rule.blocking, Message: rule.message})
			}
		}
	}
	return r, nil
}
