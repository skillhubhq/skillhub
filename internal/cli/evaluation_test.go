package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluationOfflineValidationAndSafeErrors(t *testing.T) {
	state := filepath.Join(t.TempDir(), "unused")
	for _, args := range [][]string{{"eval", "validate", "../../examples/evaluations/cases.json", "--cases"}, {"eval", "validate", "../../examples/evaluations/report.json"}} {
		var out, stderr bytes.Buffer
		if err := Execute(context.Background(), append([]string{"--data", state, "--json"}, args...), strings.NewReader(""), &out, &stderr, Options{}); err != nil || !strings.Contains(out.String(), `"valid": true`) {
			t.Fatal(out.String(), err)
		}
	}
	if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("offline validation initialized state")
	}
	file := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(file, []byte(`{"secret":"private-fixture-marker"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	err := Execute(context.Background(), []string{"--data", state, "eval", "validate", file}, strings.NewReader(""), &out, &stderr, Options{})
	if err == nil || strings.Contains(out.String()+stderr.String()+err.Error(), "private-fixture-marker") {
		t.Fatal("unsafe invalid document feedback")
	}
}
