package hubhttp

import (
	"connectrpc.com/connect"
	"context"
	adminv1 "github.com/skillhubhq/skillhub/gen/api/admin/v1"
	authv1 "github.com/skillhubhq/skillhub/gen/api/auth/v1"
	commonv1 "github.com/skillhubhq/skillhub/gen/api/common/v1"
	governancev1 "github.com/skillhubhq/skillhub/gen/api/governance/v1"
	"github.com/skillhubhq/skillhub/gen/api/governance/v1/governancev1connect"
	publishingv1 "github.com/skillhubhq/skillhub/gen/api/publishing/v1"
	taxonomy "github.com/skillhubhq/skillhub/gen/common/registry/v1"
	"testing"
)

func TestGovernanceRPCIsolationAndNotificationRevocation(t *testing.T) {
	for _, jsonCodec := range []bool{false, true} {
		t.Run(map[bool]string{false: "protobuf", true: "json"}[jsonCodec], func(t *testing.T) {
			f := registry(t, true)
			ctx := context.Background()
			opts := []connect.ClientOption{}
			if jsonCodec {
				opts = append(opts, connect.WithProtoJSON())
			}
			g := governancev1connect.NewGovernanceServiceClient(f.server.Client(), f.server.URL, opts...)
			publisherID, publisher := f.user(t, "publisher")
			reviewerID, reviewer := f.user(t, "reviewer")
			_, outsider := f.user(t, "outsider")
			f.grant(t, publisherID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, true)
			f.grant(t, reviewerID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_REVIEWER, true)
			r, e := f.publishing.SubmitRelease(ctx, authenticated(&publishingv1.SubmitReleaseRequest{Mutation: mutation(), Namespace: "team", Version: "1.0.0", Archive: skillArchive(t, "governance", "Write a report."), Visibility: taxonomy.Visibility_VISIBILITY_MEMBERS}, publisher))
			if e != nil {
				t.Fatal(e)
			}
			id := r.Msg.Release.ReleaseId
			scan, e := g.GetScanReport(ctx, authenticated(&governancev1.GetScanReportRequest{ReleaseId: id}, reviewer))
			if e != nil || scan.Msg.Report == nil || scan.Msg.Report.Digest != r.Msg.Release.Digest || !scan.Msg.Report.Complete {
				t.Fatal("scan projection", scan, e)
			}
			for _, token := range []string{outsider, ""} {
				_, e = g.GetScanReport(ctx, rpcRequest(&governancev1.GetScanReportRequest{ReleaseId: id}, token))
				if connect.CodeOf(e) != connect.CodeNotFound && connect.CodeOf(e) != connect.CodeUnauthenticated {
					t.Fatal("report leakage", e)
				}
			}
			_, e = f.publishing.ReviewRelease(ctx, authenticated(&publishingv1.ReviewReleaseRequest{Mutation: mutation(), ReleaseId: id, Approve: true}, reviewer))
			if e != nil {
				t.Fatal(e)
			}
			notes, e := g.ListNotifications(ctx, authenticated(&governancev1.ListNotificationsRequest{Page: &commonv1.PageRequest{PageSize: 1}}, publisher))
			if e != nil || len(notes.Msg.Notifications) != 1 {
				t.Fatal("notifications", notes, e)
			}
			n := notes.Msg.Notifications[0]
			_, e = g.MarkNotificationRead(ctx, authenticated(&governancev1.MarkNotificationReadRequest{NotificationId: n.NotificationId}, outsider))
			if connect.CodeOf(e) != connect.CodeNotFound {
				t.Fatal("notification ownership", e)
			}
			f.grant(t, publisherID, taxonomy.PrincipalKind_PRINCIPAL_KIND_USER, taxonomy.NamespaceRole_NAMESPACE_ROLE_MAINTAINER, false)
			hidden, e := g.ListNotifications(ctx, authenticated(&governancev1.ListNotificationsRequest{}, publisher))
			if e != nil || len(hidden.Msg.Notifications) != 0 {
				t.Fatal("revoked notification leak", e)
			}
			_, e = g.MarkNotificationRead(ctx, authenticated(&governancev1.MarkNotificationReadRequest{NotificationId: n.NotificationId}, publisher))
			if connect.CodeOf(e) != connect.CodeNotFound {
				t.Fatal("revoked read", e)
			}
			_, e = g.SetPlatformRole(ctx, authenticated(&governancev1.SetPlatformRoleRequest{Mutation: mutation(), UserId: publisherID, Role: taxonomy.PlatformRole_PLATFORM_ROLE_USER_ADMIN, Granted: true}, f.owner))
			if e != nil {
				t.Fatal(e)
			}
			_, e = g.GetPlatformAccess(ctx, authenticated(&governancev1.GetPlatformAccessRequest{}, publisher))
			if connect.CodeOf(e) != connect.CodeUnauthenticated {
				t.Fatal("old session survives", e)
			}
			login, e := f.auth.Login(ctx, connect.NewRequest(&authv1.LoginRequest{Username: "publisher", Password: "a-user-test-password"}))
			if e != nil {
				t.Fatal(e)
			}
			access, e := g.GetPlatformAccess(ctx, authenticated(&governancev1.GetPlatformAccessRequest{}, login.Msg.AccessToken))
			if e != nil || !access.Msg.CanManageUsers || access.Msg.Administrator || access.Msg.CanReadAudit {
				t.Fatal("duty projection", e)
			}
			_, e = g.GetScanReport(ctx, authenticated(&governancev1.GetScanReportRequest{ReleaseId: id}, login.Msg.AccessToken))
			if connect.CodeOf(e) != connect.CodeNotFound {
				t.Fatal("people duty restored revoked namespace access", e)
			}
			_, e = f.admin.ListUsers(ctx, authenticated(&adminv1.ListUsersRequest{}, login.Msg.AccessToken))
			if e != nil {
				t.Fatal(e)
			}
			_, e = f.admin.CreateUser(ctx, authenticated(&adminv1.CreateUserRequest{Mutation: mutation(), Username: "bad", Password: "strong-test-password", Administrator: true}, login.Msg.AccessToken))
			if connect.CodeOf(e) != connect.CodePermissionDenied {
				t.Fatal("role escalation", e)
			}
		})
	}
}
