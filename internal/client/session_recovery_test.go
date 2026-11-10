package client

import (
	"connectrpc.com/connect"
	"context"
	auth "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	"github.com/skillhubhq/skillhub/gen/api/auth/v1/authv1connect"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIdentityRecoveryRetriesOnlyTransientReads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  connect.Code
		login bool
		calls int
	}{
		{"network", connect.CodeUnavailable, false, 2},
		{"expired", connect.CodeUnauthenticated, false, 1},
		{"forbidden", connect.CodePermissionDenied, false, 1},
		{"login-is-not-replayed", connect.CodeUnavailable, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					_ = connect.NewErrorWriter().Write(w, r, connect.NewError(tc.code, nil))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"identity":{"userId":"member"}}`))
			}))
			defer server.Close()
			api := authv1connect.NewAuthServiceClient(server.Client(), server.URL, connect.WithProtoJSON(), connect.WithInterceptors(identityRetryInterceptor()))
			var err error
			if tc.login {
				_, err = api.Login(context.Background(), connect.NewRequest(&auth.LoginRequest{}))
			} else {
				_, err = api.GetIdentity(context.Background(), connect.NewRequest(&auth.GetIdentityRequest{}))
			}
			if calls != tc.calls {
				t.Fatalf("calls=%d want=%d", calls, tc.calls)
			}
			if tc.calls == 2 && err != nil {
				t.Fatal(err)
			}
			if tc.calls == 1 && connect.CodeOf(err) != tc.code {
				t.Fatalf("changed error: %v", err)
			}
		})
	}
}
