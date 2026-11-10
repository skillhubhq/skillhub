package main

import (
	"context"
	"github.com/skillhubhq/skillhub/internal/client"
	"github.com/wailsapp/wails/v3/pkg/application"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recoveryTransport struct{ fail bool }

func (r *recoveryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.fail && strings.HasSuffix(req.URL.Path, "/CompleteDeviceLogin") {
		r.fail = false
		return nil, io.ErrUnexpectedEOF
	}
	return http.DefaultTransport.RoundTrip(req)
}
func TestDeviceLoginSurvivesInterruptedPoll(t *testing.T) {
	t.Setenv("SKILLHUB_HOME", t.TempDir())
	transport := &recoveryTransport{fail: true}
	service := &DesktopService{options: client.ClientOptions{Credentials: &desktopCredentials{values: map[string]string{}}, HTTPClient: &http.Client{Transport: transport}, Home: t.TempDir()}}
	if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer service.ServiceShutdown()
	server, store := desktopHub(t, true)
	if _, err := service.AddSource("company", server.URL); err != nil {
		t.Fatal(err)
	}
	login, err := service.BeginLogin("company")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ApproveDevice(context.Background(), login.Code, "admin", "desktop-test-password"); err != nil {
		t.Fatal(err)
	}
	service.logins[login.Handle].next = time.Time{}
	if _, err = service.PollLogin(login.Handle); err == nil {
		t.Fatal("expected interrupted poll")
	}
	if service.logins[login.Handle] == nil {
		t.Fatal("network failure discarded authorization")
	}
	service.logins[login.Handle].next = time.Time{}
	result, err := service.PollLogin(login.Handle)
	if err != nil || result.Pending || result.Identity.Username != "admin" {
		t.Fatalf("login did not recover: %v", err)
	}
	if service.logins[login.Handle] != nil {
		t.Fatal("completed authorization retained")
	}
}
