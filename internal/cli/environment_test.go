package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/packageformat"
)

// 测试子进程充当 CLI，不启动 Agent，也不使用真实系统密钥。
func TestEnvironmentProcessHelper(t *testing.T) {
	if os.Getenv("SKILLHUB_ENV_TEST_HELPER") != "1" {
		return
	}
	start := 0
	for n, arg := range os.Args {
		if arg == "--" {
			start = n + 1
			break
		}
	}
	if start == 0 {
		os.Exit(2)
	}
	err := Execute(context.Background(), os.Args[start:], strings.NewReader(""), os.Stdout, os.Stderr, Options{})
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(0)
}

func TestShellEnvironmentRoundTripAndRemoval(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store, err := client.OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "skill with ' quotes")
	for name, value := range map[string]string{"SKILL.md": "---\nname: env-check\ndescription: Environment fixture.\n---\n", client.SkillConfigFile: `{"version":1,"variables":[{"name":"SERVICE_VALUE","required":true},{"name":"SERVICE_REGION","required":true}]}`} {
		if err = os.WriteFile(filepath.Join(source, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	archive, bundle, err := packageformat.Pack(source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := store.PlanInstall(ctx, client.Installation{RegistryID: "fixture", RegistryURL: "https://skills.example.com", Namespace: "team", Name: "env-check", Version: "1.0.0", Digest: bundle.Digest, Tool: "codex", Scope: "project", Target: target}, archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Apply(ctx, *plan, archive); err != nil {
		t.Fatal(err)
	}
	target = plan.Target
	core := client.NewClient(store, client.ClientOptions{})
	value := "synthetic-'\"$(touch INJECTION)\n雪\\literal"
	if err = core.SetSkillConfiguration(ctx, target, "SERVICE_VALUE", "value", value); err != nil {
		t.Fatal(err)
	}
	if err = core.SetSkillConfiguration(ctx, target, "SERVICE_REGION", "value", "fixture-region"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, shell := range []string{"bash", "zsh", "pwsh"} {
		t.Run(shell, func(t *testing.T) {
			binary, e := exec.LookPath(shell)
			if e != nil {
				if runtime.GOOS == "windows" && shell == "pwsh" {
					t.Fatal("PowerShell regression is required on Windows")
				}
				t.Skip("shell not installed")
			}
			quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
			template := environmentShell
			if shell == "pwsh" {
				quote = func(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }
				template = environmentPowerShell
			}
			script := strings.ReplaceAll(strings.ReplaceAll(template, "@EXECUTABLE@", quote(executable)+" -test.run=TestEnvironmentProcessHelper --"), "@DATA@", quote(root))
			var checks string
			if shell == "pwsh" {
				checks = "\nskillhub_env use " + quote(target) + "\nskillhub_env doctor " + quote(target) + "\nif ($LASTEXITCODE -ne 0) {throw 'doctor failed'}\nskillhub_env deactivate\nif (Test-Path Env:SERVICE_VALUE) {throw 'value retained'}\nif ($env:SERVICE_REGION -ne 'fixture-region') {throw 'existing environment changed'}\n$env:SERVICE_VALUE='conflict'\ntry { skillhub_env use " + quote(target) + "; throw 'accepted conflict' } catch { if ($_.Exception.Message -eq 'accepted conflict') {throw} }\nif ($env:SERVICE_VALUE -ne 'conflict') {throw 'conflict overwritten'}\n"
			} else {
				checks = "\nskillhub_env use " + quote(target) + " || exit 10\nskillhub_env doctor " + quote(target) + " || exit 11\nskillhub_env deactivate || exit 12\n[ -z \"${SERVICE_VALUE+x}\" ] || exit 13\n[ \"$SERVICE_REGION\" = fixture-region ] || exit 14\nSERVICE_VALUE=unexported\nif skillhub_env use " + quote(target) + "; then exit 15; fi\n[ \"$SERVICE_VALUE\" = unexported ] || exit 16\nunset SERVICE_VALUE\nset -x\nif skillhub_env use " + quote(target) + "; then exit 17; fi\nset +x\n[ -z \"${SERVICE_VALUE+x}\" ] || exit 18\n"
			}
			file := filepath.Join(t.TempDir(), "roundtrip.sh")
			if shell == "pwsh" {
				file += ".ps1"
			}
			if err = os.WriteFile(file, []byte(script+checks), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{file}
			if shell == "pwsh" {
				args = []string{"-NoProfile", "-NonInteractive", "-File", file}
			}
			command := exec.Command(binary, args...)
			command.Dir = t.TempDir()
			command.Env = append(os.Environ(), "SKILLHUB_ENV_TEST_HELPER=1", "SERVICE_REGION=fixture-region")
			output, err := command.CombinedOutput()
			if bytes.Contains(output, []byte(value)) || bytes.Contains(output, []byte("雪\\literal")) {
				t.Fatal("value leaked into shell output")
			}
			if err != nil {
				t.Fatalf("shell handoff failed: %v\n%s", err, output)
			}
			if _, err = os.Stat(filepath.Join(command.Dir, "INJECTION")); !os.IsNotExist(err) {
				t.Fatal("value executed as code")
			}
		})
	}
	var output bytes.Buffer
	err = Execute(ctx, []string{"--data", root, "env", "_read", target}, strings.NewReader(""), &output, io.Discard, Options{})
	if err == nil || output.Len() != 0 {
		t.Fatal("transfer accepted diagnostic writer")
	}
}
