package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/skillhubhq/skillhub/internal/client"
	"strings"
	"testing"
)

func TestCLIAccountRotationAndAdministratorHandoff(t *testing.T) {
	server := cliHub(t, true)
	state := t.TempDir()
	options := Options{Client: client.ClientOptions{Credentials: &memoryCredentials{values: map[string]string{}}}}
	run := func(input string, expectError bool, args ...string) []byte {
		t.Helper()
		var out, errOut bytes.Buffer
		e := Execute(context.Background(), append([]string{"--data", state, "--json"}, args...), strings.NewReader(input), &out, &errOut, options)
		if (e != nil) != expectError {
			t.Fatalf("command %s error %v", args[0], e)
		}
		for _, secret := range []string{"initial-member-password", "new-member-password", "reset-member-password"} {
			if strings.Contains(out.String()+errOut.String(), secret) {
				t.Fatal("secret in output")
			}
		}
		return out.Bytes()
	}
	run("", false, "source", "add", "owner", server.URL)
	run("test-password-strong\n", false, "login", "owner", "--username", "admin", "--password-stdin")
	raw := run("initial-member-password\n", false, "admin", "user-add", "member", "--source", "owner", "--password-stdin")
	var created struct {
		User struct {
			ID string `json:"user_id"`
		} `json:"user"`
	}
	if e := json.Unmarshal(raw, &created); e != nil || created.User.ID == "" {
		t.Fatal("missing user", e)
	}
	run("", false, "source", "add", "member", server.URL)
	run("initial-member-password\n", false, "login", "member", "--username", "member", "--password-stdin")
	run("initial-member-password\nnew-member-password\n", false, "password-change", "--source", "member", "--passwords-stdin")
	run("", true, "whoami", "--source", "member")
	run("new-member-password\n", false, "login", "member", "--username", "member", "--password-stdin")
	run("reset-member-password\n", false, "admin", "user-password-reset", created.User.ID, "--source", "owner", "--password-stdin")
	run("", true, "whoami", "--source", "member")
	run("", false, "admin", "user-administrator", created.User.ID, "--source", "owner", "--enabled=true")
	run("reset-member-password\n", false, "login", "member", "--username", "member", "--password-stdin")
	identity := run("", false, "whoami", "--source", "member")
	var account struct {
		Identity struct {
			Administrator bool `json:"administrator"`
		} `json:"identity"`
	}
	if e := json.Unmarshal(identity, &account); e != nil || !account.Identity.Administrator {
		t.Fatalf("not administrator: %s", identity)
	}
}
