package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestDesktopObservesSharedCoreChanges(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SKILLHUB_HOME", root)
	service := &DesktopService{}
	if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer service.ServiceShutdown()
	other, err := client.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	source := t.TempDir()
	if err = os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: shared-check\ndescription: Verify shared inventory.\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, bundle, err := packageformat.Pack(source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "shared-check")
	plan, err := other.PlanInstall(context.Background(), client.Installation{RegistryID: "test-hub", RegistryURL: "https://example.invalid", Namespace: "tests", Name: "shared-check", Version: "1.0.0", Digest: bundle.Digest, Tool: "codex", Scope: "project", Target: target}, data)
	if err != nil {
		t.Fatal(err)
	}
	if err = other.Apply(context.Background(), *plan, data); err != nil {
		t.Fatal(err)
	}
	items, err := service.ListInstalled()
	if err != nil || len(items) != 1 || items[0].ID != plan.After.ID {
		t.Fatalf("shared inventory: %v %v", items, err)
	}
	remove, err := other.PlanRemove(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if err = other.Apply(context.Background(), *remove, nil); err != nil {
		t.Fatal(err)
	}
	items, err = service.ListInstalled()
	if err != nil || len(items) != 0 {
		t.Fatalf("shared removal: %v %v", items, err)
	}
}
