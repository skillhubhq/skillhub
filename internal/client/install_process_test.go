package client

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/skillhubhq/skillhub/internal/packageformat"
)

type processInstallRequest struct {
	Root    string
	Plan    InstallPlan
	Archive []byte
	CrashAt string
	Blocked bool
}

// 子进程直接退出，不执行 defer，验证真实进程中断后的锁释放和持久日志恢复。
func TestInstallProcessHelper(t *testing.T) {
	path := os.Getenv("SKILLHUB_TEST_INSTALL_PROCESS")
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var request processInstallRequest
	if err = json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(request.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	timeout := 20 * time.Second
	if request.Blocked {
		timeout = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err = store.apply(ctx, request.Plan, request.Archive, func(point string) error {
		if point == request.CrashAt {
			os.Exit(73)
		}
		return nil
	})
	if request.Blocked {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("second process did not wait for the target lock: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
}

func runInstallProcess(t *testing.T, request processInstallRequest, crash bool) {
	t.Helper()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "request.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstallProcessHelper$")
	command.Env = append(os.Environ(), "SKILLHUB_TEST_INSTALL_PROCESS="+path)
	output, err := command.CombinedOutput()
	if crash {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 73 {
			t.Fatalf("expected process interruption at %s: %v\n%s", request.CrashAt, err, output)
		}
	} else if err != nil {
		t.Fatalf("install subprocess failed: %v\n%s", err, output)
	}
}

func TestInstallUpdateRemoveRecoverAfterProcessExit(t *testing.T) {
	for _, action := range []string{"install", "update", "remove"} {
		for _, point := range []string{"prepared", "old-moved", "new-moved", "committed"} {
			t.Run(action+"/"+point, func(t *testing.T) {
				ctx := context.Background()
				root := t.TempDir()
				s, err := OpenStore(root)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if s != nil {
						_ = s.Close()
					}
				})
				target := filepath.Join(t.TempDir(), "check")
				old, digest := testArchive(t, "old")
				if action != "install" {
					plan, err := s.PlanInstall(ctx, record(target, digest), old)
					if err != nil {
						t.Fatal(err)
					}
					if err = s.Apply(ctx, *plan, old); err != nil {
						t.Fatal(err)
					}
				}
				next, digest := testArchive(t, "new")
				desired := record(target, digest)
				desired.Version = "2.0.0"
				var plan *InstallPlan
				if action == "remove" {
					plan, err = s.PlanRemove(ctx, target)
					next = nil
				} else {
					plan, err = s.PlanInstall(ctx, desired, next)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err = s.Close(); err != nil {
					t.Fatal(err)
				}
				runInstallProcess(t, processInstallRequest{Root: root, Plan: *plan, Archive: next, CrashAt: point}, true)
				s, err = OpenStore(root)
				if err != nil {
					t.Fatal(err)
				}
				if err = s.Recover(ctx); err != nil {
					t.Fatal(err)
				}
				installed, err := s.checkedInstallation(ctx, plan.Target)
				if err != nil {
					t.Fatal(err)
				}
				absent := (action == "install" && point != "committed") || (action == "remove" && point == "committed")
				if absent {
					if installed != nil {
						t.Fatal("recovery retained an installation that should be absent")
					}
				} else {
					want := "1.0.0"
					archive := old
					if point == "committed" {
						want, archive = "2.0.0", next
					}
					if installed == nil || installed.Version != want || installed.Digest != packageformat.Digest(archive) {
						t.Fatal("recovered version or archive identity is incorrect")
					}
				}
				var journals int
				if err = s.db.QueryRow("SELECT COUNT(*) FROM install_journals").Scan(&journals); err != nil || journals != 0 {
					t.Fatal("recovery left a persistent journal")
				}
				backups, err := s.LocalBackups(ctx)
				if err != nil {
					t.Fatal(err)
				}
				wantBackups := 0
				if point == "committed" && action != "install" {
					wantBackups = 1
				}
				if len(backups) != wantBackups {
					t.Fatalf("recovery points: got %d, want %d", len(backups), wantBackups)
				}
				if matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), ".skillhub-txn-*")); err != nil || len(matches) != wantBackups {
					t.Fatal("recovery left untracked transaction directories", err)
				}
			})
		}
	}
}

func TestInstallTargetLockExcludesAnotherProcess(t *testing.T) {
	s := testStore(t)
	archive, digest := testArchive(t, "process lock")
	plan, err := s.PlanInstall(context.Background(), record(filepath.Join(t.TempDir(), "check"), digest), archive)
	if err != nil {
		t.Fatal(err)
	}
	request := processInstallRequest{Root: s.root, Plan: *plan, Archive: archive, Blocked: true}
	err = s.withTarget(context.Background(), plan.Target, func() error {
		runInstallProcess(t, request, false)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	request.Blocked = false
	runInstallProcess(t, request, false)
	if installed, err := s.checkedInstallation(context.Background(), plan.Target); err != nil || installed == nil {
		t.Fatal("second process could not install after lock release")
	}
}
