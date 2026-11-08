package client

import (
	"bytes"
	"context"
	"errors"
	registryv1 "github.com/skillhubhq/skillhub/gen/api/registry/v1"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"testing"
)

type officialTransport func(*http.Request) (*http.Response, error)

func (f officialTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestOfficialIdentityCannotBeClaimedByNameOrLookalike(t *testing.T) {
	for _, raw := range []string{"https://skills.codectn.com", "https://skills.codectn.com/"} {
		if !IsOfficialHub(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"http://skills.codectn.com", "https://skills.codectn.com.evil.example", "https://skills.codectn.com@evil.example", "https://skills.codectn.com/?official=true", "https://skills.codectn.com:444", "https://skills.example.com"} {
		if IsOfficialHub(raw) {
			t.Fatal(raw)
		}
	}
}
func TestOfficialOnboardingRetryAndOptOut(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	calls := 0
	offline := true
	core := NewClient(store, ClientOptions{HTTPClient: &http.Client{Transport: officialTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Host != "skills.codectn.com" || r.Header.Get("Authorization") != "" {
			t.Fatal("unexpected discovery request")
		}
		if offline {
			return nil, errors.New("offline")
		}
		raw, _ := proto.Marshal(&registryv1.GetRegistryResponse{RegistryId: "official-fixture", DisplayName: "SkillHub", ProtocolMajor: 2})
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/proto"}}, Body: io.NopCloser(bytes.NewReader(raw)), Request: r}, nil
	})}})
	ctx := context.Background()
	if err = core.EnsureOfficialSource(ctx); err == nil {
		t.Fatal("offline discovery accepted")
	}
	offline = false
	if err = core.EnsureOfficialSource(ctx); err != nil {
		t.Fatal(err)
	}
	sources, err := core.ListSources(ctx)
	if err != nil || len(sources) != 1 || !sources[0].Official {
		t.Fatal("official source missing", err)
	}
	// No credential was created: remove the source row as an explicit opt-out.
	if _, err = store.db.Exec("DELETE FROM sources WHERE alias='official'"); err != nil {
		t.Fatal(err)
	}
	next := NewClient(store, coreOptions(core))
	if err = next.EnsureOfficialSource(ctx); err != nil {
		t.Fatal(err)
	}
	sources, _ = next.ListSources(ctx)
	if len(sources) != 0 || calls != 2 {
		t.Fatal("removed source was re-added")
	}
}
func coreOptions(c *Client) ClientOptions { return ClientOptions{HTTPClient: c.http} }
func TestOfficialOnboardingPreservesExistingSources(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.db.Exec(`INSERT INTO sources VALUES('company','company-id','{"alias":"company","url":"https://company.example.com","registry_id":"company-id","display_name":"Official","official":true}')`)
	if err != nil {
		t.Fatal(err)
	}
	core := NewClient(store, ClientOptions{HTTPClient: &http.Client{Transport: officialTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected automatic network access")
		return nil, nil
	})}})
	if err = core.EnsureOfficialSource(context.Background()); err != nil {
		t.Fatal(err)
	}
	sources, err := core.ListSources(context.Background())
	if err != nil || len(sources) != 1 || sources[0].Official {
		t.Fatal("existing source changed or claimed official status", err)
	}
}
