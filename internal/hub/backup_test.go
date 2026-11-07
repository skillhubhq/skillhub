package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/skillhubhq/skillhub/internal/sqliteutil"
)

func backupFixture(t *testing.T) (*Store, Session, SubmitRelease, Release) {
	t.Helper()
	s := testStore(t)
	ctx := context.Background()
	session, err := s.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	actor := session.Identity
	if _, err = s.PutNamespace(ctx, actor, "backup-namespace", PutNamespace{Name: "team"}); err != nil {
		t.Fatal(err)
	}
	group, err := s.CreateGroup(ctx, actor, "backup-group", "engineering")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetGroupMember(ctx, actor, "backup-member", SetGroupMember{GroupID: group.ID, UserID: actor.ID, Member: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PutCategory(ctx, actor, "backup-category", PutCategory{Name: "operations", Title: "Operations"}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err = os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: backup-check\ndescription: Verify durable recovery.\n---\nFixed package content.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	archive, _, err := packageformat.Pack(directory)
	if err != nil {
		t.Fatal(err)
	}
	cmd := SubmitRelease{Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: taxonomy.Visibility_VISIBILITY_GROUPS, GroupIDs: []string{group.ID}, Category: "operations", Tags: []string{"recovery"}}
	release, err := s.SubmitRelease(ctx, actor, "backup-submit", cmd)
	if err != nil {
		t.Fatal(err)
	}
	return s, session, cmd, release
}

func TestOnlineBackupRestorePreservesIdentityGovernanceAndInvalidatesSessions(t *testing.T) {
	s, session, cmd, release := backupFixture(t)
	ctx := context.Background()
	device, err := s.StartDevice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ApproveDevice(ctx, device.UserCode, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "snapshot # 中文")
	info, err := s.Backup(ctx, backup)
	if err != nil || info.RegistryID != s.RegistryID() || info.Packages != 1 {
		t.Fatalf("backup: %+v %v", info, err)
	}
	if _, err = VerifyBackup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if store, err := Open(backup); err == nil {
		store.Close()
		t.Fatal("served an immutable backup as a live Hub")
	}
	// 快照完成后继续提交，恢复必须保持快照时刻，不混入后续版本。
	newCmd := cmd
	newCmd.Version = "1.1.0"
	if _, err = s.SubmitRelease(ctx, session.Identity, "after-backup", newCmd); err != nil {
		t.Fatal(err)
	}
	restoredRoot := filepath.Join(t.TempDir(), "restored # 中文")
	if _, err = RestoreBackup(ctx, backup, restoredRoot); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(restoredRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.RegistryID() != s.RegistryID() {
		t.Fatal("restore changed registry identity")
	}
	if _, err = restored.Authenticate(ctx, session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("restored an old live session")
	}
	var devices int
	if err = restored.db.QueryRow("SELECT count(*) FROM devices").Scan(&devices); err != nil || devices != 0 {
		t.Fatal("restored pending device authorization")
	}
	if _, err = s.Authenticate(ctx, session.Token); err != nil {
		t.Fatal("restore modified the running source")
	}
	login, err := restored.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	r, archive, err := restored.DownloadRelease(ctx, login.Identity, "team", "backup-check", "1.0.0")
	if err != nil || r.ID != release.ID || r.Digest != release.Digest || !bytes.Equal(archive, cmd.Archive) || r.Category != "operations" || len(r.GroupIDs) != 1 || len(r.Tags) != 1 {
		t.Fatalf("lost exact package or governance metadata: %+v %v", r, err)
	}
	if _, _, err = restored.DownloadRelease(ctx, Identity{}, "team", "backup-check", "1.0.0"); !errors.Is(err, ErrNotFound) {
		t.Fatal("restore made private package public")
	}
	if _, _, err = restored.DownloadRelease(ctx, login.Identity, "team", "backup-check", "1.1.0"); !errors.Is(err, ErrNotFound) {
		t.Fatal("snapshot included later publication")
	}
	replayed, err := restored.SubmitRelease(ctx, login.Identity, "backup-submit", cmd)
	if err != nil || replayed.ID != release.ID {
		t.Fatalf("restore broke mutation idempotency: %v", err)
	}
	audit, err := restored.ListAudit(ctx, login.Identity, Page{Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range audit.Items {
		found = found || event.Action == "instance.restore"
	}
	if !found {
		t.Fatal("restore was not recorded in audit")
	}
	if _, err = VerifyBackup(ctx, backup); err != nil {
		t.Fatalf("restore modified original snapshot: %v", err)
	}
}

func TestBackupDuringConcurrentPublicationRemainsComplete(t *testing.T) {
	s, session, cmd, _ := backupFixture(t)
	second, err := Open(s.root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx := context.Background()
	archives := make([][]byte, 12)
	packageDir := t.TempDir()
	for i := range archives {
		content := fmt.Sprintf("---\nname: backup-check\ndescription: Concurrent snapshot fixture.\n---\nVersion content %d.\n", i)
		if err := os.WriteFile(filepath.Join(packageDir, "SKILL.md"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		archives[i], _, err = packageformat.Pack(packageDir)
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	errorsOut := make(chan error, 1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 12; i++ {
			copy := cmd
			copy.Version = fmt.Sprintf("1.0.%d", i)
			copy.Archive = archives[i-1]
			if _, err := second.SubmitRelease(ctx, session.Identity, fmt.Sprintf("concurrent-%d", i), copy); err != nil {
				errorsOut <- err
				return
			}
		}
	}()
	defer wg.Wait()
	for i := 0; i < 3; i++ {
		destination := filepath.Join(t.TempDir(), "snapshot")
		info, err := s.Backup(ctx, destination)
		if err != nil {
			t.Fatal(err)
		}
		if info.Packages < 1 || info.Packages > 13 {
			t.Fatal("snapshot package count is outside concurrent publication bounds")
		}
		if _, err = VerifyBackup(ctx, destination); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	select {
	case err := <-errorsOut:
		t.Fatal(err)
	default:
	}
}

func TestBackupRejectsCorruptionAndProtectsExistingDestinations(t *testing.T) {
	s, _, _, release := backupFixture(t)
	ctx := context.Background()
	for _, scenario := range []string{"missing-package", "corrupt-package", "corrupt-database", "manifest", "incomplete", "schema", "sidecar"} {
		t.Run(scenario, func(t *testing.T) {
			backup := filepath.Join(t.TempDir(), "snapshot")
			info, err := s.Backup(ctx, backup)
			if err != nil {
				t.Fatal(err)
			}
			blob := filepath.Join(backup, "packages", release.Digest[7:]+".zip")
			switch scenario {
			case "missing-package":
				err = os.Remove(blob)
			case "corrupt-package":
				err = os.WriteFile(blob, []byte("corrupt"), 0600)
			case "corrupt-database":
				err = os.WriteFile(filepath.Join(backup, "hub.db"), []byte("corrupt"), 0600)
			case "manifest":
				err = os.WriteFile(filepath.Join(backup, backupManifestName), []byte(`{"format":999}`), 0600)
			case "incomplete":
				err = os.WriteFile(filepath.Join(backup, maintenanceMarker), nil, 0600)
			case "sidecar":
				err = os.WriteFile(filepath.Join(backup, "hub.db-wal"), nil, 0600)
			case "schema":
				var dbErr error
				db, openErr := sqliteutil.Open(filepath.Join(backup, "hub.db"))
				if openErr != nil {
					t.Fatal(openErr)
				}
				_, dbErr = db.Exec("PRAGMA user_version=999")
				err = errors.Join(dbErr, db.Close())
				if err == nil {
					info.DatabaseDigest, info.DatabaseBytes, err = hashBackupFile(ctx, filepath.Join(backup, "hub.db"))
				}
				if err == nil {
					raw, _ := json.Marshal(info)
					err = os.WriteFile(filepath.Join(backup, backupManifestName), raw, 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = VerifyBackup(ctx, backup); err == nil {
				t.Fatal("verified invalid backup")
			}
			destination := filepath.Join(t.TempDir(), "restore")
			if _, err = RestoreBackup(ctx, backup, destination); err == nil {
				t.Fatal("restored invalid backup")
			}
			if _, err = os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed restore left usable target")
			}
		})
	}
	backup := filepath.Join(t.TempDir(), "snapshot")
	if _, err := s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	existing := t.TempDir()
	marker := filepath.Join(existing, "retain.txt")
	if err := os.WriteFile(marker, []byte("retain"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreBackup(ctx, backup, existing); err == nil {
		t.Fatal("overwrote existing restore destination")
	}
	if _, err := s.Backup(ctx, existing); err == nil {
		t.Fatal("overwrote existing backup destination")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "retain" {
		t.Fatal("existing destination was changed")
	}
	if _, err := s.Backup(ctx, filepath.Join(s.root, "nested")); err == nil {
		t.Fatal("allowed a backup inside the data directory")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	destination := filepath.Join(t.TempDir(), "cancelled")
	if _, err := s.Backup(cancelled, destination); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled backup left destination")
	}
}

func TestIncompleteRestoreCannotStartAndEmptyRegistryCanBeBackedUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, maintenanceMarker), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(root); err == nil {
		s.Close()
		t.Fatal("started incomplete recovery")
	}
	if _, err := os.Stat(filepath.Join(root, "hub.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("startup initialized incomplete recovery")
	}
	s := testStore(t)
	backup := filepath.Join(t.TempDir(), "empty")
	info, err := s.Backup(context.Background(), backup)
	if err != nil || info.Packages != 0 {
		t.Fatalf("empty Hub backup: %+v %v", info, err)
	}
	if _, err = VerifyBackup(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
}
