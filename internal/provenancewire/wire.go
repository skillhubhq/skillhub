// Package provenancewire 仅在协议边界转换上游来源。
package provenancewire

import (
	api "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	"github.com/skillhubhq/skillhub/internal/provenance"
)

func Encode(p *provenance.Record) *api.Provenance {
	if p == nil {
		return nil
	}
	return &api.Provenance{AuthorName: p.AuthorName, AuthorUrl: p.AuthorURL, SourceUrl: p.SourceURL, Revision: p.Revision, License: p.License, LicenseFile: p.LicenseFile, Modified: p.Modified, Changes: p.Changes}
}
func Decode(p *api.Provenance) *provenance.Record {
	if p == nil {
		return nil
	}
	return &provenance.Record{AuthorName: p.AuthorName, AuthorURL: p.AuthorUrl, SourceURL: p.SourceUrl, Revision: p.Revision, License: p.License, LicenseFile: p.LicenseFile, Modified: p.Modified, Changes: p.Changes}
}
