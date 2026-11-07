package client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/google/uuid"
)

func newProjectClient(t *testing.T) (*client.Client, *client.Store) {
	t.Helper()
	store, err := client.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return client.NewClient(store, client.ClientOptions{Credentials: &credentialStore{values: map[string]string{}}}), store
}
func projectMutation() *commonv1.Mutation {
	return &commonv1.Mutation{IdempotencyKey: uuid.NewString()}
}

func TestProjectRestoreAcrossClientsUsesLockedIdentityDigestAndCurrentAuthorization(t *testing.T) {
	ctx := context.Background()
	author, _ := newProjectClient(t)
	company, public := httptest.NewServer(handler(t)), httptest.NewServer(handler(t))
	defer company.Close()
	defer public.Close()
	remotes := map[string]*client.Remote{}
	ids := map[string]string{}
	dir := t.TempDir()
	original := []byte("---\nname: project-check\ndescription: Restore an exact reviewed package.\n---\nVersion one.\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), original, 0600); err != nil {
		t.Fatal(err)
	}
	archive, _, err := packageformat.Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	for alias, url := range map[string]string{"company": company.URL, "public": public.URL} {
		if _, err := author.AddSource(ctx, alias, url); err != nil {
			t.Fatal(err)
		}
		if _, err := author.LoginPassword(ctx, alias, "admin", "a-strong-test-password"); err != nil {
			t.Fatal(err)
		}
		remote, err := author.Remote(ctx, alias)
		if err != nil {
			t.Fatal(err)
		}
		remotes[alias] = remote
		if _, err = remote.Admin.PutNamespace(ctx, connect.NewRequest(&adminv1.PutNamespaceRequest{Mutation: projectMutation(), Name: "team"})); err != nil {
			t.Fatal(err)
		}
		visibility := taxonomy.Visibility_VISIBILITY_MEMBERS
		if alias == "public" {
			visibility = taxonomy.Visibility_VISIBILITY_PUBLIC
		}
		response, err := remote.Publishing.SubmitRelease(ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: projectMutation(), Namespace: "team", Version: "1.0.0", Archive: archive, Visibility: visibility, DisplayNameEn: "Project Check", DisplayNameZh: "项目检查", Icon: taxonomy.SkillIcon_SKILL_ICON_WORKFLOW}))
		if err != nil {
			t.Fatal(err)
		}
		ids[alias] = response.Msg.Release.ReleaseId
	}
	project := t.TempDir()
	if _, err := author.InitProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	if _, err := author.InitProject(ctx, project); err == nil {
		t.Fatal("overwrote existing manifest")
	}
	for tool, alias := range map[string]string{"codex": "company", "claude-code": "public"} {
		if _, err := author.SetProjectSkill(ctx, project, client.ProjectSkill{Source: alias, Namespace: "team", Name: "project-check", Version: "1.0.0", Tool: tool}); err != nil {
			t.Fatal(err)
		}
	}
	lock, err := author.LockProject(ctx, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Skills) != 2 {
		t.Fatal("lost lock entry")
	}
	for _, s := range lock.Skills {
		if s.Digest != packageformat.Digest(archive) || strings.Contains(s.Target, project) {
			t.Fatal("invalid portable lock")
		}
	}
	// 新工作区与新客户端数据库，只复制可提交到 Git 的两份文件。
	freshProject := t.TempDir()
	for _, name := range []string{client.ManifestFile, client.ProjectLockFile} {
		data, err := os.ReadFile(filepath.Join(project, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("credential")) || bytes.Contains(data, []byte("password")) {
			t.Fatal("credential leaked into project")
		}
		if err = os.WriteFile(filepath.Join(freshProject, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	consumer, inventory := newProjectClient(t)
	if _, err := consumer.PlanProjectRestore(ctx, freshProject); err == nil {
		t.Fatal("lock implicitly trusted an unconfigured source")
	}
	// 本机别名不同，但 URL 和不可变实例 ID 必须一致。
	for alias, url := range map[string]string{"work": company.URL, "community": public.URL} {
		if _, err = consumer.AddSource(ctx, alias, url); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = consumer.PlanProjectRestore(ctx, freshProject); err == nil {
		t.Fatal("private content restored without credentials")
	}
	if _, err = consumer.LoginPassword(ctx, "work", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	plan, err := consumer.PlanProjectRestore(ctx, freshProject)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plan.Plans {
		if p.After.DisplayNameEN != "Project Check" || p.After.DisplayNameZH != "项目检查" || p.After.Icon != taxonomy.SkillIcon_SKILL_ICON_WORKFLOW {
			t.Fatal("restore lost release presentation")
		}
	}

	if items, err := inventory.ListInstallations(ctx); err != nil || len(items) != 0 {
		t.Fatal("planning installed files")
	}
	session, err := consumer.Remote(ctx, "work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = session.Auth.Logout(ctx, connect.NewRequest(&authv1.LogoutRequest{})); err != nil {
		t.Fatal(err)
	}
	results, err := consumer.ApplyProjectRestore(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	var succeeded, failed int
	for _, result := range results {
		if result.Applied {
			succeeded++
		} else {
			failed++
		}
	}
	installed, err := inventory.ListInstallations(ctx)
	if err != nil || len(installed) != 1 || installed[0].Source != "community" || installed[0].SourceRef == "" {
		t.Fatalf("restore lost the actual local connection: %+v %v", installed, err)
	}
	if succeeded != 1 || failed != 1 {
		t.Fatalf("authorization was not rechecked after planning: %+v", results)
	}
	if _, err := os.Stat(filepath.Join(freshProject, ".agents", "skills", "project-check")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("revoked identity installed private skill")
	}
	if _, err = consumer.LoginPassword(ctx, "work", "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	// 发布更新不会改变锁定版本。
	updated := append(append([]byte{}, original...), []byte("Version two.\n")...)
	if err = os.WriteFile(filepath.Join(dir, "SKILL.md"), updated, 0600); err != nil {
		t.Fatal(err)
	}
	newArchive, _, err := packageformat.Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = remotes["public"].Publishing.SubmitRelease(ctx, connect.NewRequest(&publishingv1.SubmitReleaseRequest{Mutation: projectMutation(), Namespace: "team", Version: "1.1.0", Archive: newArchive, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC})); err != nil {
		t.Fatal(err)
	}
	plan, err = consumer.PlanProjectRestore(ctx, freshProject)
	if err != nil {
		t.Fatal(err)
	}
	results, err = consumer.ApplyProjectRestore(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if !result.Applied {
			t.Fatalf("restore failed: %+v", result)
		}
	}
	for _, toolDir := range []string{".agents", ".claude"} {
		data, err := os.ReadFile(filepath.Join(freshProject, toolDir, "skills", "project-check", "SKILL.md"))
		if err != nil || !bytes.Equal(data, original) {
			t.Fatal("restore changed locked version")
		}
	}
	plan, err = consumer.PlanProjectRestore(ctx, freshProject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = consumer.SetProjectSkill(ctx, freshProject, client.ProjectSkill{Source: "public", Namespace: "team", Name: "project-check", Version: "1.1.0", Tool: "claude-code"}); err != nil {
		t.Fatal(err)
	}
	if _, err = consumer.ApplyProjectRestore(ctx, plan); err == nil {
		t.Fatal("applied changed manifest")
	}
	state, err := consumer.ReadProject(ctx, freshProject)
	if err != nil || !state.NeedsLock {
		t.Fatal("did not detect stale lock")
	}
	originalManifest, err := os.ReadFile(filepath.Join(project, client.ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(freshProject, client.ManifestFile), originalManifest, 0600); err != nil {
		t.Fatal(err)
	}
	// 手工改摘要不能依赖源返回的摘要来覆盖锁定内容。
	broken := lock
	broken.Skills = append([]client.LockedSkill{}, lock.Skills...)
	broken.Skills[0].Digest = "sha256:" + strings.Repeat("0", 64)
	raw, _ := json.Marshal(broken)
	if err = os.WriteFile(filepath.Join(freshProject, client.ProjectLockFile), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = consumer.PlanProjectRestore(ctx, freshProject); err == nil {
		t.Fatal("accepted wrong locked digest")
	}
	raw, _ = json.Marshal(lock)
	if err = os.WriteFile(filepath.Join(freshProject, client.ProjectLockFile), raw, 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(freshProject, ".agents", "skills", "project-check", "SKILL.md")
	if err = os.WriteFile(target, []byte("local edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = consumer.PlanProjectRestore(ctx, freshProject); !errors.Is(err, client.ErrModified) {
		t.Fatalf("ignored local edit: %v", err)
	}
	if err = os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	plan, err = consumer.PlanProjectRestore(ctx, freshProject)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = remotes["public"].Publishing.YankRelease(ctx, connect.NewRequest(&publishingv1.YankReleaseRequest{Mutation: projectMutation(), ReleaseId: ids["public"], Reason: "Withdraw before apply"})); err != nil {
		t.Fatal(err)
	}
	results, err = consumer.ApplyProjectRestore(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if strings.HasPrefix(result.Target, ".claude/") && result.Applied {
			t.Fatal("restore bypassed yank")
		}
	}
}
