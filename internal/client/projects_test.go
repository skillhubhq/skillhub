package client

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestProjectMetadataBoundsAndTargetValidation(t *testing.T) {
	core := NewClient(testStore(t), ClientOptions{})
	ctx := context.Background()
	good := ProjectManifest{Schema: 1, Skills: []ProjectSkill{{Source: "company", Namespace: "team", Name: "safe-skill", Version: "1.0.0", Tool: "codex"}}}
	for name, raw := range map[string]string{
		"future-schema":     `{"schema":2,"skills":[]}`,
		"unknown-field":     `{"schema":1,"skills":[],"token":"secret"}`,
		"missing-skills":    `{"schema":1}`,
		"trailing-document": `{"schema":1,"skills":[]} {}`,
		"oversized":         strings.Repeat(" ", maxProjectFile+1),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ManifestFile), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := core.ReadProject(ctx, root); err == nil {
				t.Fatal("accepted invalid project metadata")
			}
		})
	}
	for _, version := range []string{"latest", "1", "1.2", "^1.0.0", "v1.0.0", "1.0.0/../../outside"} {
		m := good
		m.Skills = append([]ProjectSkill{}, good.Skills...)
		m.Skills[0].Version = version
		if err := validateManifest(m); err == nil {
			t.Fatalf("accepted non-exact version %s", version)
		}
	}
	for _, version := range []string{"1.2.3", "1.2.3-rc.1", "1.2.3+build.1"} {
		m := good
		m.Skills = append([]ProjectSkill{}, good.Skills...)
		m.Skills[0].Version = version
		if err := validateManifest(m); err != nil {
			t.Fatalf("valid version %s: %v", version, err)
		}
	}
	duplicate := good
	duplicate.Skills = append(append([]ProjectSkill{}, good.Skills...), good.Skills[0])
	duplicate.Skills[1].Source = "public"
	if err := validateManifest(duplicate); err == nil {
		t.Fatal("accepted two physical owners")
	}
	many := ProjectManifest{Schema: 1, Skills: make([]ProjectSkill, maxProjectEntries+1)}
	if err := validateManifest(many); err == nil {
		t.Fatal("accepted unbounded project entries")
	}
	lock := ProjectLock{Schema: 1, ManifestDigest: metadataDigest(good), Skills: []LockedSkill{{ProjectSkill: good.Skills[0], RegistryID: "registry", RegistryURL: "https://skills.example.com", Digest: "sha256:" + strings.Repeat("0", 64), Target: ".agents/skills/safe-skill"}}}
	if err := validateLock(good, lock); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside", "/tmp/outside", `C:\outside`, `.agents\skills\safe-skill`, ".agents/skills/../outside"} {
		bad := lock
		bad.Skills = append([]LockedSkill{}, lock.Skills...)
		bad.Skills[0].Target = path
		if err := validateLock(good, bad); err == nil {
			t.Fatalf("accepted locked path %s", path)
		}
	}
}

func TestProjectOperationsCoordinateDifferentClientDatabases(t *testing.T) {
	a := NewClient(testStore(t), ClientOptions{})
	b := NewClient(testStore(t), ClientOptions{})
	ctx := context.Background()
	root := t.TempDir()
	if _, err := a.InitProject(ctx, root); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, core := range []*Client{a, b} {
		wg.Go(func() {
			tool := "codex"
			if i == 1 {
				tool = "claude-code"
			}
			_, err := core.SetProjectSkill(ctx, root, ProjectSkill{Source: "company", Namespace: "team", Name: "safe-skill", Version: "1.0.0", Tool: tool})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := a.ReadProject(ctx, root)
	if err != nil || len(state.Manifest.Skills) != 2 || !state.NeedsLock {
		t.Fatalf("concurrent update lost: %+v %v", state, err)
	}
	if _, err = b.RemoveProjectSkill(ctx, root, "codex", "safe-skill"); err != nil {
		t.Fatal(err)
	}
	state, err = a.ReadProject(ctx, root)
	if err != nil || len(state.Manifest.Skills) != 1 || state.Manifest.Skills[0].Tool != "claude-code" {
		t.Fatal("wrong manifest entry removed")
	}
}

func TestProjectMetadataAndTargetsRejectSymlinkEscapes(t *testing.T) {
	ctx := context.Background()
	core := NewClient(testStore(t), ClientOptions{})
	root, outside := t.TempDir(), t.TempDir()
	m := ProjectManifest{Schema: 1, Skills: []ProjectSkill{}}
	raw, _ := json.Marshal(m)
	external := filepath.Join(outside, ManifestFile)
	if err := os.WriteFile(external, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, ManifestFile)); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := core.ReadProject(ctx, root); err == nil {
		t.Fatal("read external manifest through symlink")
	}
	if err := os.Remove(filepath.Join(root, ManifestFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".agents")); err != nil {
		t.Fatal(err)
	}
	physicalRoot, err := projectRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	skill := LockedSkill{ProjectSkill: ProjectSkill{Source: "company", Namespace: "team", Name: "safe-skill", Version: "1.0.0", Tool: "codex"}, Target: ".agents/skills/safe-skill"}
	if _, err = projectTarget(physicalRoot, skill); err == nil {
		t.Fatal("project target escaped its root")
	}
	if _, err = os.Stat(filepath.Join(outside, "skills")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("validation modified target")
	}
}
