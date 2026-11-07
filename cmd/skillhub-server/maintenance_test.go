package main

import (
	"strings"
	"testing"
)

func TestMaintenanceSummaryEscapesBackupMetadata(t *testing.T) {
	for _, registry := range []string{"registry-id", "registry\nforged record", "registry\rforged record", "registry\x1b[2J", "registry\x00"} {
		line := maintenanceSummary("verify-backup", registry, 1, 2)
		if strings.Count(line, "\n") != 1 || !strings.HasSuffix(line, "\n") || strings.ContainsAny(line, "\r\x1b\x00") {
			t.Fatalf("unsafe maintenance output: %q", line)
		}
		if !strings.HasPrefix(line, "verify-backup complete: registry ") || !strings.HasSuffix(line, ", schema 1, 2 packages\n") {
			t.Fatalf("incomplete maintenance output: %q", line)
		}
	}
}
