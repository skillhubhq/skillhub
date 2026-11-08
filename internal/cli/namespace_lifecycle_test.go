package cli

import (
	"bytes"
	"context"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/hubhttp"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLINamespaceLifecycleRequiresExplicitConfirmation(t *testing.T) {
	ctx := context.Background()
	s, e := hub.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	admin, e := s.Bootstrap(ctx, "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.PutNamespace(ctx, admin, "create-team", hub.PutNamespace{Name: "team"}); e != nil {
		t.Fatal(e)
	}
	h, e := hubhttp.New(s, hubhttp.Config{PublicURL: "http://127.0.0.1"})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	session, e := s.Login(ctx, "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("SKILLHUB_TOKEN_TEST", session.Token)
	state := t.TempDir()
	store, e := client.OpenStore(state)
	if e != nil {
		t.Fatal(e)
	}
	core := client.NewClient(store, client.ClientOptions{})
	if _, e = core.AddSource(ctx, "test", server.URL); e != nil {
		t.Fatal(e)
	}
	store.Close()
	run := func(args ...string) (string, error) {
		var out, errout bytes.Buffer
		err := Execute(ctx, append([]string{"--data", state, "--json", "admin"}, args...), strings.NewReader(""), &out, &errout, Options{})
		return out.String(), err
	}
	if _, e = run("namespace-archive", "team", "--source", "test", "--revision", "1"); e == nil {
		t.Fatal("unconfirmed archive")
	}
	if _, e = run("namespace-archive", "team", "--source", "test", "--revision", "1", "--yes"); e != nil {
		t.Fatal(e)
	}
	if _, e = run("namespace-purge", "team", "--source", "test", "--revision", "2", "--yes"); e == nil {
		t.Fatal("missing typed confirmation")
	}
	if _, e = run("namespace-inspect", "team", "--source", "test"); e != nil {
		t.Fatal(e)
	}
	if _, e = run("namespace-purge", "team", "--source", "test", "--revision", "2", "--yes", "--confirm", "team"); e != nil {
		t.Fatal(e)
	}
}
