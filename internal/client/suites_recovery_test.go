package client

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInterruptedSuiteUpdateCanRemoveActualOwnedVersion(t *testing.T) {
	for _, updated := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-member-write", true: "after-member-write"}[updated], func(t *testing.T) {
			ctx := context.Background()
			store := testStore(t)
			oldRaw, oldDigest := testArchive(t, "old")
			newRaw, newDigest := testArchive(t, "new")
			target := filepath.Join(t.TempDir(), "check")
			raw, digest := oldRaw, oldDigest
			if updated {
				raw, digest = newRaw, newDigest
			}
			plan, err := store.PlanInstall(ctx, record(target, digest), raw)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.Apply(ctx, *plan, raw); err != nil {
				t.Fatal(err)
			}
			target = plan.Target
			parent := filepath.Join(filepath.Dir(target), "suite")
			receipt := SuiteInstallation{Target: parent, Members: []SuiteMemberInstallation{
				{Target: target, Digest: newDigest, Managed: true},
				{Target: target, Digest: oldDigest, Managed: true},
			}}
			value, err := json.Marshal(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.db.ExecContext(ctx, "INSERT INTO suite_installations(target,record) VALUES(?,?)", parent, string(value)); err != nil {
				t.Fatal(err)
			}
			core := NewClient(store, ClientOptions{})
			removal, err := core.PlanSuiteRemove(ctx, parent)
			if err != nil {
				t.Fatal(err)
			}
			if len(removal.Plans) != 1 || len(removal.Retained) != 0 {
				t.Fatalf("incorrect interrupted removal: %+v", removal)
			}
			if _, err = core.ApplySuite(ctx, removal); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("owned member remains", err)
			}
			receipts, err := store.Suites(ctx)
			if err != nil || len(receipts) != 0 {
				t.Fatal("recovery receipt remains", err)
			}
		})
	}
}
