package client

import (
	"testing"

	catalogv1 "github.com/skillhubhq/skillhub/gen/api/catalog/v1"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

func TestPreviewUsesVerifiedArchiveInsteadOfUnboundMarkdown(t *testing.T) {
	archive, digest := testArchive(t, "Actual reviewed instructions.")
	release := &catalogv1.Release{Name: "check", Digest: digest, SkillMarkdown: "Misleading separate preview"}
	preview, err := inspectPreview(release, archive)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Release.Markdown == release.SkillMarkdown || len(preview.Files) == 0 {
		t.Fatal("trusted unbound preview text")
	}
	release.Digest = "sha256:invalid"
	if _, err = inspectPreview(release, archive); err == nil {
		t.Fatal("preview accepted mismatched digest")
	}
}

func TestPreviewFilePreservesTextAndRefusesFilesystemPaths(t *testing.T) {
	script := "#!/bin/sh\n# This file is displayed, never executed.\necho '<script>unsafe()</script>'\n"
	bundle := &packageformat.Bundle{Files: []packageformat.File{
		{Path: "scripts/check.sh", Data: []byte(script), Executable: true},
		{Path: "assets/binary", Data: []byte{0, 0xff, 0x01}},
		{Path: "empty.txt"},
	}}
	content, err := previewFile(bundle, "scripts/check.sh")
	if err != nil || !content.Text || content.Content != script {
		t.Fatalf("script content changed: %+v, %v", content, err)
	}
	content, err = previewFile(bundle, "assets/binary")
	if err != nil || content.Text || content.Content != "" {
		t.Fatalf("binary content converted to text: %+v, %v", content, err)
	}
	content, err = previewFile(bundle, "empty.txt")
	if err != nil || !content.Text || content.Content != "" {
		t.Fatalf("empty text file not supported: %+v, %v", content, err)
	}
	for _, path := range []string{"../SKILL.md", "/etc/passwd", "scripts/../scripts/check.sh", "missing"} {
		if _, err := previewFile(bundle, path); err == nil {
			t.Fatalf("accepted non-member path %q", path)
		}
	}
}
