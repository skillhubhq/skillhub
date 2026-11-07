// Package evaluation 校验外部评测文件，不执行任何模型、脚本或网络请求。
package evaluation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

const MaxBytes = 256 << 10
const MaxCases = 50

var ErrInvalid = errors.New("invalid evaluation document; check schema, limits and case results")
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Case struct {
	ID       string `json:"id"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
}
type Suite struct {
	SchemaVersion string `json:"schema_version"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Cases         []Case `json:"cases"`
}
type Outcome struct {
	Status   string `json:"status"`
	Observed string `json:"observed"`
}
type Result struct {
	CaseID string `json:"case_id"`
	Outcome
	Baseline *Outcome `json:"baseline,omitempty"`
}
type Report struct {
	SchemaVersion string   `json:"schema_version"`
	SkillDigest   string   `json:"skill_digest"`
	Suite         Suite    `json:"suite"`
	Agent         string   `json:"agent"`
	Model         string   `json:"model"`
	Runner        string   `json:"runner"`
	Settings      string   `json:"settings"`
	ExecutedAt    string   `json:"executed_at"`
	Results       []Result `json:"results"`
}
type Summary struct {
	Passed         uint32 `json:"passed"`
	Failed         uint32 `json:"failed"`
	Errors         uint32 `json:"errors"`
	Skipped        uint32 `json:"skipped"`
	BaselinePassed uint32 `json:"baseline_passed"`
	BaselineTotal  uint32 `json:"baseline_total"`
}

func textOK(s string, max int, required bool) bool {
	return utf8.ValidString(s) && len(s) <= max && (!required || strings.TrimSpace(s) != "") && strings.IndexFunc(s, func(r rune) bool {
		return (unicode.IsControl(r) && r != '\n' && r != '\t') || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069
	}) < 0
}
func validSuite(s Suite) bool {
	if s.SchemaVersion != "skillhub.cases.v1" || !packageformat.ValidName(s.Name) || !textOK(s.Version, 80, true) || len(s.Cases) == 0 || len(s.Cases) > MaxCases {
		return false
	}
	seen := map[string]bool{}
	for _, c := range s.Cases {
		if !packageformat.ValidName(c.ID) || seen[c.ID] || !textOK(c.Input, 8192, true) || !textOK(c.Expected, 4096, true) {
			return false
		}
		seen[c.ID] = true
	}
	return true
}
func validOutcome(o Outcome) bool {
	return (o.Status == "pass" || o.Status == "fail" || o.Status == "error" || o.Status == "skipped") && textOK(o.Observed, 8192, true)
}

// 拒绝重复键和深层 JSON，防止不同执行器对同一证据产生不同解释。
func tokens(d *json.Decoder, depth int) error {
	if depth > 16 {
		return ErrInvalid
	}
	t, e := d.Token()
	if e != nil {
		return ErrInvalid
	}
	if t == nil {
		return ErrInvalid
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return ErrInvalid
			}
			key, ok := k.(string)
			if !ok || seen[key] || key != strings.ToLower(key) {
				return ErrInvalid
			}
			seen[key] = true
			if e = tokens(d, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e = tokens(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return ErrInvalid
	}
	_, e = d.Token()
	return e
}
func decode(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > MaxBytes || !utf8.Valid(raw) {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := tokens(d, 0); err != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return ErrInvalid
	}
	return nil
}
func ParseSuite(raw []byte) (Suite, error) {
	var s Suite
	if err := decode(raw, &s); err != nil || !validSuite(s) {
		return Suite{}, ErrInvalid
	}
	return s, nil
}
func Parse(raw []byte) (Report, error) {
	var r Report
	if err := decode(raw, &r); err != nil {
		return r, err
	}
	if r.SchemaVersion != "skillhub.evaluation.v1" || !digestPattern.MatchString(r.SkillDigest) || !validSuite(r.Suite) || !textOK(r.Agent, 160, true) || !textOK(r.Model, 160, true) || !textOK(r.Runner, 160, true) || !textOK(r.Settings, 2048, true) || len(r.Results) != len(r.Suite.Cases) {
		return Report{}, ErrInvalid
	}
	at, err := time.Parse(time.RFC3339, r.ExecutedAt)
	if err != nil || at.Year() < 2020 {
		return Report{}, ErrInvalid
	}
	cases := map[string]bool{}
	for _, c := range r.Suite.Cases {
		cases[c.ID] = true
	}
	for _, result := range r.Results {
		if !cases[result.CaseID] || !validOutcome(result.Outcome) || (result.Baseline != nil && !validOutcome(*result.Baseline)) {
			return Report{}, ErrInvalid
		}
		delete(cases, result.CaseID)
	}
	return r, nil
}
func (r Report) Canonical() []byte   { b, _ := json.Marshal(r); return b }
func (r Report) SuiteDigest() string { b, _ := json.Marshal(r.Suite); return packageformat.Digest(b) }
func (r Report) Summary() Summary {
	var s Summary
	for _, c := range r.Results {
		switch c.Status {
		case "pass":
			s.Passed++
		case "fail":
			s.Failed++
		case "error":
			s.Errors++
		case "skipped":
			s.Skipped++
		}
		if c.Baseline != nil {
			s.BaselineTotal++
			if c.Baseline.Status == "pass" {
				s.BaselinePassed++
			}
		}
	}
	return s
}
