package client

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAllToolAdaptersResolveBothScopesAndContainTargets(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	for _, tool := range Tools() {
		for _, scope := range []string{"global", "project"} {
			t.Run(tool.ID+"-"+scope, func(t *testing.T) {
				selectedProject := project
				if scope == "global" {
					selectedProject = ""
				}
				target, _, e := ResolveTarget(tool.ID, scope, selectedProject, home, "sample")
				if e != nil {
					t.Fatal(e)
				}
				root, dir := home, tool.GlobalDirectory
				if scope == "project" {
					root, dir = project, tool.ProjectDirectory
				}
				expected, e := canonicalTarget(filepath.Join(root, dir, "sample"))
				if e != nil || target != expected {
					t.Fatalf("unexpected target: %s", target)
				}
				if !strings.HasSuffix(target, "sample") {
					t.Fatal(target)
				}
			})
		}
	}
	if _, _, e := ResolveTarget("unknown", "project", project, home, "sample"); e == nil {
		t.Fatal("accepted unknown adapter")
	}
}
