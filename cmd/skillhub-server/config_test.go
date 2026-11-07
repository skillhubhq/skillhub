package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckConfigOverridesWithoutCreatingData(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "hub.yaml")
	data := filepath.Join(root, "not-created")
	if err := os.WriteFile(path, []byte("listen: invalid\nprivate: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"check-config", "--config", path, "--data", data}); err == nil {
		t.Fatal("invalid listener accepted")
	}
	if err := run([]string{"check-config", "--config", path, "--data", data, "--listen", "127.0.0.1:8080"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatal("check-config touched data directory")
	}
}
