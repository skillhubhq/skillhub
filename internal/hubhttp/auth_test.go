package hubhttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"connectrpc.com/connect"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/gen/api/auth/v1/authv1connect"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	registryv1 "github.com/skillhubhq/skillhub/gen/api/registry/v1"
	"github.com/skillhubhq/skillhub/gen/api/registry/v1/registryv1connect"
)

func testServer(t *testing.T) (*httptest.Server, authv1connect.AuthServiceClient) {
	t.Helper()
	store, err := testDatabase(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err = store.Bootstrap(context.Background(), "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	server, err := New(store, Config{PublicURL: "http://127.0.0.1", DisplayName: "Test Hub", Private: true})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	return httpServer, authv1connect.NewAuthServiceClient(httpServer.Client(), httpServer.URL)
}

func login(t *testing.T, client authv1connect.AuthServiceClient) string {
	t.Helper()
	response, err := client.Login(context.Background(), connect.NewRequest(&authv1.LoginRequest{Username: "admin", Password: "a-strong-test-password"}))
	if err != nil {
		t.Fatal(err)
	}
	return response.Msg.AccessToken
}

func authenticated[T any](message *T, token string) *connect.Request[T] {
	r := connect.NewRequest(message)
	r.Header().Set("Authorization", "Bearer "+token)
	return r
}

func TestHTTPAuthIsolationValidationAndRevocation(t *testing.T) {
	a, aClient := testServer(t)
	_, bClient := testServer(t)
	ctx := context.Background()
	token := login(t, aClient)
	if _, err := bClient.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, token)); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("cross-registry identity: %v", err)
	}
	identity, err := aClient.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, token))
	if err != nil || identity.Msg.Identity.Username != "admin" {
		t.Fatalf("identity: %v %v", identity, err)
	}
	if _, err = aClient.Login(ctx, connect.NewRequest(&authv1.LoginRequest{Password: "secret-not-for-errors"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("validation: %v", err)
	} else {
		ce := err.(*connect.Error)
		if strings.Contains(err.Error(), "secret-not-for-errors") {
			t.Fatal("error leaked request")
		}
		if len(ce.Details()) != 1 {
			t.Fatal("missing typed error")
		}
		detail, err := ce.Details()[0].Value()
		if err != nil || detail.(*commonv1.ErrorDetail).ErrorCode != "INVALID_REQUEST" {
			t.Fatalf("typed error: %v %v", detail, err)
		}
	}
	if _, err = aClient.Logout(ctx, authenticated(&authv1.LogoutRequest{}, token)); err != nil {
		t.Fatal(err)
	}
	if _, err = aClient.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, token)); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("revoked token accepted")
	}
	registry := registryv1connect.NewRegistryServiceClient(a.Client(), a.URL)
	info, err := registry.GetRegistry(ctx, connect.NewRequest(&registryv1.GetRegistryRequest{}))
	if err != nil || info.Msg.RegistryId == "" || !info.Msg.PrivateMode {
		t.Fatalf("registry discovery: %v %v", info, err)
	}
	if info.Header().Get("Cache-Control") != "no-store" || info.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing safe response headers")
	}
}

func TestBrowserApprovalRequiresCSRFAndIssuesOneSession(t *testing.T) {
	server, client := testServer(t)
	ctx := context.Background()
	device, err := client.StartDeviceLogin(ctx, connect.NewRequest(&authv1.StartDeviceLoginRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	get, err := server.Client().Get(server.URL + "/authorize")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(get.Body)
	_ = get.Body.Close()
	if get.StatusCode != 200 || !strings.Contains(string(body), "Authorize this device") {
		t.Fatal("missing authorization form")
	}
	cookies := get.Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing CSRF cookie")
	}
	cookie := cookies[0]
	form := url.Values{"code": {device.Msg.UserCode}, "username": {"admin"}, "password": {"a-strong-test-password"}, "csrf": {cookie.Value}}
	post := func(origin string, includeCookie bool) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/authorize", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin)
		if includeCookie {
			req.AddCookie(cookie)
		}
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response
	}
	if result := post("http://127.0.0.1", false); result.StatusCode != 403 {
		t.Fatal("missing cookie accepted")
	}
	if result := post("https://attacker.example", true); result.StatusCode != 403 {
		t.Fatal("foreign origin accepted")
	}
	if result := post("http://127.0.0.1", true); result.StatusCode != 200 {
		t.Fatalf("approval: %d", result.StatusCode)
	}
	complete, err := client.CompleteDeviceLogin(ctx, connect.NewRequest(&authv1.CompleteDeviceLoginRequest{DeviceCode: device.Msg.DeviceCode}))
	if err != nil || complete.Msg.Login == nil || complete.Msg.Pending {
		t.Fatalf("complete: %v %v", complete, err)
	}
	if _, err = client.GetIdentity(ctx, authenticated(&authv1.GetIdentityRequest{}, complete.Msg.Login.AccessToken)); err != nil {
		t.Fatal(err)
	}
	if _, err = client.CompleteDeviceLogin(ctx, connect.NewRequest(&authv1.CompleteDeviceLoginRequest{DeviceCode: device.Msg.DeviceCode})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("device secret reused")
	}
}

func TestTransportBoundsAndRateLimits(t *testing.T) {
	server, client := testServer(t)
	_, err := client.Login(context.Background(), connect.NewRequest(&authv1.LoginRequest{Username: "admin", Password: strings.Repeat("x", 9000)}))
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("unbounded request: %v", err)
	}
	for i := 0; i < 22; i++ {
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/api.auth.v1.AuthService/Login", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if i == 21 && response.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("not rate limited: %d", response.StatusCode)
		}
	}
}
