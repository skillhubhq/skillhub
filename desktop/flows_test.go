package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"github.com/skillhubhq/skillhub/internal/cli"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/hubhttp"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type desktopCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func (c *desktopCredentials) Get(key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.values[key]
	if !ok {
		return "", client.ErrCredentialMissing
	}
	return value, nil
}
func (c *desktopCredentials) Set(key, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] = value
	return nil
}
func (c *desktopCredentials) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.values, key)
	return nil
}
func desktopHub(t *testing.T, private bool) (*httptest.Server, *hub.Store) {
	t.Helper()
	store, err := hub.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err = store.Bootstrap(context.Background(), "admin", "desktop-test-password"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	handler, err := hubhttp.New(store, hubhttp.Config{PublicURL: "http://" + server.Listener.Addr().String(), Private: private})
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	t.Cleanup(server.Close)
	return server, store
}
func TestDesktopFlowsShareCredentialsAndInventoryWithCLI(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SKILLHUB_HOME", root)
	credentials := &desktopCredentials{values: map[string]string{}}
	options := client.ClientOptions{Credentials: credentials, Home: t.TempDir()}
	service := &DesktopService{options: options}
	if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer service.ServiceShutdown()
	company, companyStore := desktopHub(t, true)
	public, _ := desktopHub(t, false)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cliRun := func(args ...string) []byte {
		t.Helper()
		var out, errOut bytes.Buffer
		must(cli.Execute(context.Background(), append([]string{"--data", root, "--json"}, args...), strings.NewReader(""), &out, &errOut, cli.Options{Client: options}))
		return out.Bytes()
	}
	for alias, url := range map[string]string{"company": company.URL, "public": public.URL} {
		_, err := service.AddSource(alias, url)
		must(err)
		identity, err := service.LoginPassword(alias, "admin", "desktop-test-password")
		must(err)
		if !identity.Administrator {
			t.Fatal("lost identity role")
		}
		must(service.AdminChange(AdminForm{Source: alias, Action: "namespace", RequestID: service.NewRequestID(), Name: "team", Enabled: alias == "company", Revision: "0"}))
	}
	choices, err := service.AdminList("company", "submission-namespaces", "", "")
	must(err)
	if len(choices.Rows) != 1 || choices.Rows[0].Name != "team" {
		t.Fatal("administrator submission choices missing")
	}
	if !bytes.Contains(cliRun("whoami", "--source", "company"), []byte("admin")) {
		t.Fatal("CLI did not observe App login")
	}
	must(service.AdminChange(AdminForm{Source: "company", Action: "user-add", RequestID: service.NewRequestID(), Name: "reviewer", Password: "reviewer-test-password"}))
	users, err := service.AdminList("company", "users", "", "")
	must(err)
	reviewer := ""
	for _, u := range users.Rows {
		if u.Name == "reviewer" {
			reviewer = u.ID
		}
	}
	if reviewer == "" {
		t.Fatal("created user missing")
	}
	must(service.AdminChange(AdminForm{Source: "company", Action: "grant", RequestID: service.NewRequestID(), Namespace: "team", Subject: reviewer, SubjectKind: taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, Role: taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, Enabled: true}))
	must(service.AdminChange(AdminForm{Source: "public", Action: "group-add", RequestID: service.NewRequestID(), Name: "engineering"}))
	groups, err := service.AdminList("public", "groups", "", "")
	must(err)
	if len(groups.Rows) != 1 {
		t.Fatal("group missing")
	}
	must(service.AdminChange(AdminForm{Source: "public", Action: "category", RequestID: service.NewRequestID(), Name: "operations", Title: "Operations", Enabled: true, Revision: "0"}))
	categories, err := service.AdminList("public", "categories", "", "")
	must(err)
	if len(categories.Rows) != 1 || categories.Rows[0].Revision != "1" {
		t.Fatal("category revision mapping failed")
	}
	directory := t.TempDir()
	content := []byte("---\nname: desktop-check\ndescription: Exercise real desktop APIs.\n---\nReviewed fixture.\n")
	must(os.WriteFile(filepath.Join(directory, "SKILL.md"), content, 0600))
	draft, err := service.InspectDirectory(directory)
	must(err)
	if draft.Name != "desktop-check" || draft.Markdown != string(content) || len(draft.Files) != 1 {
		t.Fatal("invalid local draft preview")
	}
	form := PublishForm{Source: "company", Directory: directory, Namespace: "team", Version: "1.0.0", Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS, RequestID: service.NewRequestID()}
	must(os.WriteFile(filepath.Join(directory, "SKILL.md"), append(append([]byte{}, content...), []byte("Changed after preview.\n")...), 0600))
	if _, err = service.PublishChecked(form, draft.Digest); err == nil {
		t.Fatal("published changed directory after preview")
	}
	must(os.WriteFile(filepath.Join(directory, "SKILL.md"), content, 0600))
	pending, err := service.PublishChecked(form, draft.Digest)
	must(err)
	if pending.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PENDING {
		t.Fatal("pending publication shown as published")
	}
	preview, err := service.PreviewSubmission("company", pending.ID)
	must(err)
	if preview.Release.Markdown != string(content) || len(preview.Files) != 1 {
		t.Fatal("invalid fixed preview")
	}
	file, err := service.ReadPreviewFile(preview.Target, "SKILL.md")
	must(err)
	if !file.Text || file.Content != string(content) {
		t.Fatal("file preview did not match approved content")
	}
	wrongTarget := preview.Target
	wrongTarget.Digest = "sha256:wrong"
	if _, err = service.ReadPreviewFile(wrongTarget, "SKILL.md"); err == nil {
		t.Fatal("accepted file preview with wrong digest")
	}
	if _, err = service.Review("company", pending.ID, "review", service.NewRequestID(), true); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("self review accepted: %v", err)
	}
	_, err = service.LoginPassword("company", "reviewer", "reviewer-test-password")
	must(err)
	if !bytes.Contains(cliRun("whoami", "--source", "company"), []byte("reviewer")) {
		t.Fatal("CLI used stale source credentials")
	}
	choices, err = service.AdminList("company", "submission-namespaces", "", "")
	must(err)
	if len(choices.Rows) != 0 {
		t.Fatal("review-only identity offered a publication namespace")
	}
	approved, err := service.Review("company", pending.ID, "Checked package", service.NewRequestID(), true)
	must(err)
	if approved.Status != taxonomy.ReleaseStatus_RELEASE_STATUS_PUBLISHED {
		t.Fatal("approval not observed")
	}
	_, err = service.Publish(PublishForm{Source: "public", Directory: directory, Namespace: "team", Version: "1.0.0", Category: "operations", Tags: []string{"checked"}, Visibility: taxonomy.Visibility_VISIBILITY_PUBLIC, RequestID: service.NewRequestID()})
	must(err)
	search, err := service.Search("", "desktop-check", "", nil, "", 0, false)
	must(err)
	if len(search) != 2 || len(search[0].Skills) != 1 || len(search[1].Skills) != 1 {
		t.Fatal("two-source desktop search failed")
	}
	project := t.TempDir()
	for tool, source := range map[string]string{"codex": "company", "claude-code": "public"} {
		plan, err := service.PlanInstall(source+"/team/desktop-check@1.0.0", tool, "project", project)
		must(err)
		if len(plan.Changes) != 1 {
			t.Fatal("missing concrete plan")
		}
		results, err := service.ApplyPlan(plan.Handle)
		must(err)
		if len(results) != 1 || !results[0].Applied {
			t.Fatal("installation failed")
		}
		if _, err = service.ApplyPlan(plan.Handle); err == nil {
			t.Fatal("replayed consumed plan")
		}
	}
	var installations []client.Installation
	must(json.Unmarshal(cliRun("list"), &installations))
	if len(installations) != 2 {
		t.Fatal("CLI inventory did not observe App installation")
	}
	// 全局安装使用注入的临时 home，验证两种工具的真实落盘而不污染开发者目录。
	for _, tool := range []string{"codex", "claude-code"} {
		globalPlan, err := service.PlanInstall("public/team/desktop-check@1.0.0", tool, "global", "")
		must(err)
		_, err = service.ApplyPlan(globalPlan.Handle)
		must(err)
		globalTarget, _, err := client.ResolveTarget(tool, "global", "", options.Home, "desktop-check")
		must(err)
		actual, err := os.ReadFile(filepath.Join(globalTarget, "SKILL.md"))
		must(err)
		if !bytes.Equal(actual, content) {
			t.Fatal("global installation content differs")
		}
		globalRemove, err := service.PlanRemove(globalTarget)
		must(err)
		_, err = service.ApplyPlan(globalRemove.Handle)
		must(err)
		if _, err = os.Stat(globalTarget); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("global uninstall left managed files")
		}
	}
	// 新安装记住连接；同一 Hub 新增账号不应打断更新，错误来源与失效凭据仍不能回退。
	updateTarget := filepath.Join(project, ".agents", "skills", "desktop-check")
	_, err = service.AddSource("company-alt", company.URL)
	must(err)
	ambiguous, err := service.CheckUpdatesFrom(updateTarget, "")
	must(err)
	if len(ambiguous) != 1 || ambiguous[0].Error != "" || ambiguous[0].Source != "company" {
		t.Fatal("update did not retain installation connection")
	}
	unauthorized, err := service.CheckUpdatesFrom(updateTarget, "company-alt")
	must(err)
	if len(unauthorized) != 1 || unauthorized[0].Error == "" {
		t.Fatal("unauthenticated update fell back to another account")
	}
	retained, err := service.CheckUpdatesFrom(updateTarget, "")
	must(err)
	if len(retained) != 1 || retained[0].Error != "" || retained[0].Source != "company" {
		t.Fatal("failed explicit check changed the remembered account")
	}
	wrong, err := service.CheckUpdatesFrom(updateTarget, "public")
	must(err)
	if len(wrong) != 1 || wrong[0].Error == "" {
		t.Fatal("update accepted a different Hub")
	}
	if _, err = service.PlanUpdateFrom(updateTarget, "1.0.0", "public"); !errors.Is(err, client.ErrRegistryChanged) {
		t.Fatalf("wrong Hub update plan: %v", err)
	}
	_, err = service.LoginPassword("company-alt", "reviewer", "reviewer-test-password")
	must(err)
	explicit, err := service.CheckUpdatesFrom(updateTarget, "company-alt")
	must(err)
	if len(explicit) != 1 || explicit[0].Error != "" || explicit[0].Source != "company-alt" {
		t.Fatalf("explicit update check failed: %+v", explicit)
	}
	updatePlan, err := service.PlanUpdateFrom(updateTarget, "1.0.0", "company-alt")
	must(err)
	service.DiscardPlan(updatePlan.Handle)
	if !bytes.Contains(cliRun("updates", updateTarget, "--source", "company-alt"), []byte("company-alt")) {
		t.Fatal("CLI lost explicit update source")
	}
	remembered, err := service.CheckUpdatesFrom(updateTarget, "")
	must(err)
	if len(remembered) != 1 || remembered[0].Error != "" || remembered[0].Source != "company-alt" {
		t.Fatal("successful explicit selection was not remembered")
	}
	must(service.RemoveSource("company-alt"))
	missing, err := service.CheckUpdatesFrom(updateTarget, "")
	must(err)
	if len(missing) != 1 || !missing[0].ConnectionRequired {
		t.Fatal("removed connection silently fell back to another account")
	}
	_, err = service.CheckUpdatesFrom(updateTarget, "company")
	must(err)
	var cache client.CacheInfo
	must(json.Unmarshal(cliRun("cache", "info"), &cache))
	if cache.Entries != 2 {
		t.Fatal("CLI did not observe App cached downloads")
	}
	cliRun("cache", "clear", "--yes")
	cache, err = service.CacheInfo()
	must(err)
	if cache.Entries != 0 {
		t.Fatal("App did not observe CLI cache cleanup")
	}
	_, err = service.OpenProject(project, true)
	must(err)
	for tool, source := range map[string]string{"codex": "company", "claude-code": "public"} {
		_, err = service.SetProjectSkill(project, client.ProjectSkill{Source: source, Namespace: "team", Name: "desktop-check", Version: "1.0.0", Tool: tool})
		must(err)
	}
	state, err := service.LockProject(project)
	must(err)
	if state.NeedsLock {
		t.Fatal("project remained unlocked")
	}
	cliRun("project", "restore", "--project", project, "--yes")
	projects, err := service.ListProjects()
	must(err)
	if len(projects) != 1 {
		t.Fatal("shared project missing")
	}
	restore, err := service.PlanRestore(project)
	must(err)
	results, err := service.ApplyPlan(restore.Handle)
	must(err)
	for _, r := range results {
		if !r.Applied {
			t.Fatalf("restore: %+v", r)
		}
	}
	_, err = service.ClearCache()
	must(err)
	must(json.Unmarshal(cliRun("cache", "info"), &cache))
	if cache.Entries != 0 {
		t.Fatal("CLI did not observe App cache cleanup")
	}
	target := filepath.Join(project, ".agents", "skills", "desktop-check")
	remove, err := service.PlanRemove(target)
	must(err)
	must(os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("local edits"), 0600))
	if _, err = service.ApplyPlan(remove.Handle); !errors.Is(err, client.ErrModified) {
		t.Fatalf("discarded post-plan local changes: %v", err)
	}
	must(os.WriteFile(filepath.Join(target, "SKILL.md"), content, 0600))
	remove, err = service.PlanRemove(target)
	must(err)
	service.DiscardPlan(remove.Handle)
	if _, err = service.ApplyPlan(remove.Handle); err == nil {
		t.Fatal("applied discarded preview")
	}
	// 浏览器授权只把用户码交给界面，设备秘密保持在 Go 层。
	login, err := service.BeginLogin("company")
	must(err)
	raw, err := json.Marshal(login)
	must(err)
	service.mu.Lock()
	secret := service.logins[login.Handle].login.DeviceCode
	service.mu.Unlock()
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatal("device secret exposed to frontend")
	}
	must(companyStore.ApproveDevice(context.Background(), login.Code, "reviewer", "reviewer-test-password"))
	time.Sleep(time.Duration(login.IntervalSeconds) * time.Second)
	poll, err := service.PollLogin(login.Handle)
	must(err)
	if poll.Pending || poll.Identity.Username != "reviewer" {
		t.Fatal("desktop browser login did not complete")
	}
	_, err = service.PollLogin(login.Handle)
	if err == nil {
		t.Fatal("consumed device authorization replayed")
	}
	audit, err := service.AdminList("public", "audit", "", "")
	must(err)
	if len(audit.Rows) == 0 || audit.Rows[0].CreatedAt == "" {
		t.Fatal("audit projection missing")
	}
	must(service.Logout("company"))
	if _, err = service.ReadPreviewFile(preview.Target, "SKILL.md"); err == nil {
		t.Fatal("file preview bypassed revoked source authorization")
	}
	search, err = service.Search("", "desktop-check", "", nil, "", 0, false)
	must(err)
	if search[0].Error == "" || len(search[1].Skills) != 1 {
		t.Fatal("one source failure hid other results")
	}
	cliRun("remove", target, "--yes")
	items, err := service.ListInstalled()
	must(err)
	if len(items) != 1 {
		t.Fatal("App did not observe CLI removal")
	}
}
