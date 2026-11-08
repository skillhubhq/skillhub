package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/hubhttp"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIConnectionPreferences(t *testing.T) {
	ctx := context.Background()
	h, err := hub.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	handler, err := hubhttp.New(h, hubhttp.Config{PublicURL: "http://127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	state := t.TempDir()
	run := func(args ...string) (client.Source, error) {
		var out, stderr bytes.Buffer
		err := Execute(ctx, append([]string{"--data", state, "--json", "source"}, args...), strings.NewReader(""), &out, &stderr, Options{})
		var source client.Source
		if err == nil {
			err = json.Unmarshal(out.Bytes(), &source)
		}
		return source, err
	}
	initial, err := run("add", "work", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = run("edit", "work"); err == nil {
		t.Fatal("empty edit accepted")
	}
	if _, err = run("edit", "work", "--name", "Team library", "--note", "Reviewer"); err != nil {
		t.Fatal(err)
	}
	next, err := run("edit", "work", "--note", "")
	if err != nil {
		t.Fatal(err)
	}
	if next.Label != "Team library" || next.Note != "" || next.Alias != initial.Alias || next.CredentialRef != initial.CredentialRef {
		t.Fatal("partial edit lost fields or identity")
	}
	if _, err = run("refresh", "work"); err != nil {
		t.Fatal(err)
	}
	server.Close()
	next, err = run("edit", "work", "--name", "")
	if err != nil || next.Label != "" {
		t.Fatal("offline reset failed", err)
	}
}
