package packageformat

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const validSkill = "---\nname: release-check\ndescription: Verify release readiness.\n---\nRun the documented checks.\n"

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestInspectRejectsUnsafeArchives(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:/windows", "a\\b", "a/../b", "CON.txt", "a/./b", "file.", "file "} {
		t.Run(name, func(t *testing.T) {
			if _, err := Inspect(archive(t, map[string]string{"SKILL.md": validSkill, name: "bad"})); err == nil {
				t.Fatal("accepted unsafe path")
			}
		})
	}
	for _, files := range []map[string]string{
		{"SKILL.md": validSkill, "skill.md": "collision"},
		{"SKILL.md": validSkill, "x": "file", "x/y": "child"},
		{"SKILL.md": validSkill, "Scripts/a.sh": "first", "scripts/b.sh": "second"},
		{"SKILL.md": "---\nname: bad--name\ndescription: test\n---"},
		{"SKILL.md": "---\nname: valid\nname: duplicate\ndescription: test\n---"},
	} {
		if _, err := Inspect(archive(t, files)); err == nil {
			t.Fatal("accepted invalid archive")
		}
	}
}

func TestRejectsSymlinkAndOversizedEntry(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "link"}
	h.SetMode(os.ModeSymlink | 0777)
	f, _ := w.CreateHeader(h)
	_, _ = f.Write([]byte("/etc/passwd"))
	_ = w.Close()
	if _, err := Inspect(b.Bytes()); err == nil {
		t.Fatal("accepted symlink")
	}
	if _, err := Inspect(archive(t, map[string]string{"SKILL.md": validSkill, "large": string(bytes.Repeat([]byte("x"), MaxFileBytes+1))})); err == nil {
		t.Fatal("accepted oversized file")
	}
}

func TestPackRoundTripAndFingerprint(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(validSkill), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(source, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "check.sh"), []byte("echo hello\n"), 0755); err != nil {
		t.Fatal(err)
	}
	data, bundle, err := Pack(source)
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := Pack(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		t.Fatal("pack is not reproducible")
	}
	target := t.TempDir()
	if err := bundle.Extract(target); err != nil {
		t.Fatal(err)
	}
	files, err := ReadDirectory(target)
	if err != nil {
		t.Fatal(err)
	}
	if Fingerprint(files) != bundle.Fingerprint {
		t.Fatal("round trip changed fingerprint")
	}
	if err := bundle.Extract(target); err == nil {
		t.Fatal("overwrote nonempty directory")
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte(validSkill+"modified"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err = ReadDirectory(target)
	if err != nil {
		t.Fatal(err)
	}
	if Fingerprint(files) == bundle.Fingerprint {
		t.Fatal("missed local modification")
	}
}

func TestPackRejectsExternalSymlink(t *testing.T) {
	source := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(source, "outside")); err != nil {
		t.Skip(err)
	}
	if _, _, err := Pack(source); err == nil {
		t.Fatal("packed symlink")
	}
}
