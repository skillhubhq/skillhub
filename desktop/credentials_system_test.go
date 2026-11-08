package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/packageformat"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// 仅在显式启用的隔离 CI 用户下访问真实凭据库；普通开发测试不触发系统提示。
func TestDesktopUsesSystemCredentialStore(t *testing.T) {
	if os.Getenv("SKILLHUB_TEST_OS_CREDENTIALS") != "1" {
		t.Skip("set SKILLHUB_TEST_OS_CREDENTIALS=1 on an isolated native runner")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Fatal("this acceptance requires a supported native desktop platform")
	}
	binary := "skillhub"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	binary, err := filepath.Abs(filepath.Join("..", "bin", binary))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(binary); err != nil {
		t.Fatal("build the real CLI before system credential acceptance")
	}
	root := t.TempDir()
	t.Setenv("SKILLHUB_HOME", root)
	service := &DesktopService{}
	if err = service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.ServiceShutdown() })
	credentials := client.SystemCredentials{}
	runCLI := func(stdin string, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, append([]string{"--data", root, "--json"}, args...)...)
		command.Stdin = strings.NewReader(stdin)
		output, err := command.Output()
		if err != nil {
			// 不把子进程输出或标准输入拼入错误，防止未来命令回显秘密。
			t.Fatalf("isolated CLI %s failed: %v", args[0], err)
		}
		return output
	}
	company, companyStore := desktopHub(t, true)
	partner, _ := desktopHub(t, true)
	sources := make(map[string]client.Source)
	for alias, url := range map[string]string{"system-company": company.URL, "system-partner": partner.URL} {
		if _, exists := os.LookupEnv("SKILLHUB_TOKEN_" + strings.ToUpper(strings.ReplaceAll(alias, "-", "_"))); exists {
			t.Fatal("injected source credentials would bypass system credential acceptance")
		}
		_, err := service.AddSource(alias, url)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			current, err := service.core.Source(context.Background(), alias)
			if err != nil {
				t.Errorf("cannot resolve isolated credential cleanup: %v", err)
				return
			}
			if err := credentials.Delete(current.CredentialRef); err != nil {
				t.Errorf("isolated credential cleanup failed: %v", err)
			}
		})
		if _, err = service.LoginPassword(alias, "admin", "desktop-test-password"); err != nil {
			t.Fatal(err)
		}
		// 首次验证身份会分配账号绑定的凭据引用，测试读取持久化后的引用。
		sources[alias], err = service.core.Source(context.Background(), alias)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(runCLI("", "whoami", "--source", alias), []byte("admin")) {
			t.Fatal("independent CLI did not read the App's system credential")
		}
	}
	ref := sources["system-company"].CredentialRef
	token, err := credentials.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = companyStore.Authenticate(context.Background(), token); err != nil {
		t.Fatal("saved system credential was not a valid Hub session")
	}
	runCLI("", "logout", "system-company")
	if _, err = credentials.Get(ref); !errors.Is(err, client.ErrCredentialMissing) {
		t.Fatal("CLI logout did not delete the system credential")
	}
	if _, err = companyStore.Authenticate(context.Background(), token); !errors.Is(err, hub.ErrUnauthenticated) {
		t.Fatal("CLI logout did not revoke the remote session")
	}
	if _, err = service.Identity("system-company"); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("App retained the logged-out CLI identity")
	}
	if _, err = service.Identity("system-partner"); err != nil {
		t.Fatal("logout affected another Hub's system credential")
	}
	runCLI("desktop-test-password\n", "login", "system-company", "--username", "admin", "--password-stdin")
	if identity, err := service.Identity("system-company"); err != nil || identity.Username != "admin" {
		t.Fatal("App did not read the independent CLI login")
	}
	if err = service.Logout("system-company"); err != nil {
		t.Fatal(err)
	}
	if _, err = credentials.Get(ref); !errors.Is(err, client.ErrCredentialMissing) {
		t.Fatal("App logout did not delete the system credential")
	}
	verifyLocalSystemConfiguration(t, service, runCLI)
}

func verifyLocalSystemConfiguration(t *testing.T, service *DesktopService, runCLI func(string, ...string) []byte) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{"SKILL.md": "---\nname: credential-check\ndescription: Isolated credential test.\n---\n", client.SkillConfigFile: `{"version":1,"variables":[{"name":"EXAMPLE_TOKEN","secret":true,"required":true}]}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive, bundle, err := packageformat.Pack(dir)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.store.PlanInstall(service.ctx, client.Installation{RegistryID: "fixture", RegistryURL: "https://skills.example.com", Namespace: "fixture", Name: "credential-check", Version: "1.0.0", Digest: bundle.Digest, Tool: "codex", Scope: "project", Target: filepath.Join(t.TempDir(), "credential-check")}, archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.store.Apply(service.ctx, *plan, archive); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.ClearSkillConfiguration(plan.Target); err != nil {
			t.Error("isolated local configuration cleanup failed")
		}
	})
	secret := "synthetic-local-config-secret"
	out := runCLI(secret+"\n", "config", "set", plan.Target, "EXAMPLE_TOKEN", "--mode", "keychain", "--value-stdin")
	if bytes.Contains(out, []byte(secret)) {
		t.Fatal("CLI printed secret")
	}
	report, err := service.SkillConfiguration(plan.Target)
	if err != nil || !report.Ready {
		t.Fatal("App did not observe CLI system-store configuration", err)
	}
	if err = service.SetSkillConfiguration(plan.Target, "EXAMPLE_TOKEN", "keychain", secret+"-rotated"); err != nil {
		t.Fatal(err)
	}
	out = runCLI("", "config", "doctor", plan.Target)
	if bytes.Contains(out, []byte(secret)) || !bytes.Contains(out, []byte(`"ready": true`)) {
		t.Fatal("CLI did not safely observe App system-store configuration")
	}
	runCLI("", "config", "clear", plan.Target, "--yes")
	report, err = service.SkillConfiguration(plan.Target)
	if err != nil || report.Ready {
		t.Fatal("App did not observe CLI configuration cleanup", err)
	}
}
