// Package skillconfig 定义可选配置声明；不持有值，也不执行提供方。
package skillconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

const FileName = "skillhub.config.json"

var namePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

func ValidName(name string) bool { return namePattern.MatchString(name) }

type Variable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Secret      bool   `json:"secret"`
	Required    bool   `json:"required"`
}
type Declaration struct {
	Version   int        `json:"version"`
	Variables []Variable `json:"variables"`
}

func Parse(raw []byte) (Declaration, error) {
	var result Declaration
	invalid := errors.New("invalid skillhub.config.json declaration")
	if len(raw) > 32<<10 || !utf8.Valid(raw) {
		return result, invalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&result); err != nil {
		return result, invalid
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return result, invalid
	}
	if result.Version != 1 || len(result.Variables) > 32 {
		return result, invalid
	}
	seen := map[string]bool{}
	for _, v := range result.Variables {
		if !ValidName(v.Name) || seen[v.Name] || len(v.Description) > 512 || strings.ContainsAny(v.Description, "\x00\r\n") {
			return result, invalid
		}
		seen[v.Name] = true
	}
	return result, nil
}
