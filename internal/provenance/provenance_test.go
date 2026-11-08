package provenance

import (
	"strings"
	"testing"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func record() *Record {
	return &Record{AuthorName: "Example Authors", AuthorURL: "https://example.org/authors", SourceURL: "https://example.org/source/" + strings.Repeat("a", 40), Revision: strings.Repeat("a", 40), License: "Apache-2.0", LicenseFile: "LICENSE.txt"}
}
func TestValidateRequiresBoundedAttributionAndPackagedLicense(t *testing.T) {
	b := &packageformat.Bundle{Files: []packageformat.File{{Path: "LICENSE.txt", Data: []byte("License text")}}}
	if e := Validate(record(), b); e != nil {
		t.Fatal(e)
	}
	if e := Validate(nil, b); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*Record){"script": func(p *Record) { p.SourceURL = "javascript:alert(1)" }, "credentials": func(p *Record) { p.AuthorURL = "https://user:pass@example.org" }, "token": func(p *Record) { p.SourceURL += "?token=secret" }, "missing-license": func(p *Record) { p.LicenseFile = "missing" }, "branch": func(p *Record) { p.Revision = "main" }, "bidi": func(p *Record) { p.AuthorName = "Example\u202e" }, "unnamed": func(p *Record) { p.AuthorName = "" }, "modified-without-note": func(p *Record) { p.Modified = true }, "contradictory": func(p *Record) { p.Changes = "Modified" }, "oversized": func(p *Record) { p.Changes = strings.Repeat("x", 1001) }} {
		t.Run(name, func(t *testing.T) {
			p := record()
			change(p)
			if Validate(p, b) == nil {
				t.Fatal("accepted invalid attribution")
			}
		})
	}
	if Validate(record(), &packageformat.Bundle{Files: []packageformat.File{{Path: "LICENSE.txt", Data: []byte{255}}}}) == nil {
		t.Fatal("accepted binary license")
	}
}
func TestReadRejectsMalformedAndOversizedDocuments(t *testing.T) {
	p := record()
	raw := Encode(p)
	parsed, e := Read(strings.NewReader(raw))
	if e != nil || *parsed != *p {
		t.Fatal(parsed, e)
	}
	for _, s := range []string{"null", raw + " {}", `{"author_name":"A","unknown":true}`, strings.Repeat(" ", 16385)} {
		if _, e := Read(strings.NewReader(s)); e == nil {
			t.Fatal("accepted invalid document")
		}
	}
}
