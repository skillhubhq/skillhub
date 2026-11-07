package hub

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestFederatedIdentityIsolationRevocationAndBackup(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admin, e := s.Login(ctx, "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.FederatedLogin(ctx, "https://issuer.example", "pending", "Person", false); !errors.Is(e, ErrPendingApproval) {
		t.Fatalf("closed registration: %v", e)
	}
	first, e := s.FederatedLogin(ctx, "https://issuer.example", "one", "Person", true)
	if e != nil {
		t.Fatal(e)
	}
	if first.Identity.Administrator || time.Until(first.ExpiresAt) > 8*time.Hour {
		t.Fatal("SSO privilege or lifetime")
	}
	again, e := s.FederatedLogin(ctx, "https://issuer.example", "one", "Different name", false)
	if e != nil || again.Identity.ID != first.Identity.ID {
		t.Fatal("unstable identity", e)
	}
	other, e := s.FederatedLogin(ctx, "https://other.example", "one", "Person", true)
	if e != nil || other.Identity.ID == first.Identity.ID {
		t.Fatal("issuer collision", e)
	}
	if _, e = s.Login(ctx, first.Identity.Username, ""); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("SSO password login")
	}
	users, e := s.ListUsers(ctx, admin.Identity, "", Page{})
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, u := range users.Items {
		if u.ID == first.Identity.ID {
			found = u.Federated && u.DisplayName == "Person"
		}
	}
	if !found {
		t.Fatal("SSO admin projection")
	}
	device, e := s.StartDevice(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveDeviceSession(ctx, device.UserCode, first.Token); e != nil {
		t.Fatal(e)
	}
	native, e := s.CompleteDevice(ctx, device.DeviceCode)
	if e != nil || native == nil || native.Identity.ID != first.Identity.ID || time.Until(native.ExpiresAt) > 8*time.Hour {
		t.Fatal("device SSO", e)
	}
	if _, e = s.SetUserDisabled(ctx, admin.Identity, "disable-oidc", SetUserDisabled{ID: first.Identity.ID, Disabled: true}); e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{first.Token, again.Token, native.Token} {
		if _, e = s.Authenticate(ctx, token); !errors.Is(e, ErrUnauthenticated) {
			t.Fatal("disabled session survives")
		}
	}
	if _, e = s.FederatedLogin(ctx, "https://issuer.example", "one", "Person", true); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("disabled reprovisioned")
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if _, e = s.Backup(ctx, backup); e != nil {
		t.Fatal(e)
	}
	restored := filepath.Join(t.TempDir(), "restored")
	if _, e = RestoreBackup(ctx, backup, restored); e != nil {
		t.Fatal(e)
	}
	copy, e := Open(restored)
	if e != nil {
		t.Fatal(e)
	}
	defer copy.Close()
	recovered, e := copy.FederatedLogin(ctx, "https://other.example", "one", "Person", false)
	if e != nil || recovered.Identity.ID != other.Identity.ID {
		t.Fatal("external identity lost in backup", e)
	}
}
func TestFederationMigrationFromSchema4(t *testing.T) {
	root := t.TempDir()
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	id, e := s.Bootstrap(context.Background(), "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec("DROP TABLE featured_skills; DROP INDEX releases_review_queue; DROP TABLE review_notifications; DROP TABLE release_scans; DROP TABLE platform_roles; DROP TABLE access_requests; ALTER TABLE releases DROP COLUMN display_name_en; ALTER TABLE releases DROP COLUMN display_name_zh; ALTER TABLE releases DROP COLUMN icon; DROP TABLE external_identities; PRAGMA user_version=4"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	session, e := s.Login(context.Background(), "admin", "a-strong-test-password")
	if e != nil || session.Identity.ID != id.ID {
		t.Fatal("migration lost local identity", e)
	}
	if _, e = s.FederatedLogin(context.Background(), "https://issuer.example", "one", "Person", true); e != nil {
		t.Fatal(e)
	}
}

func TestEnterpriseRequestApprovalRejectionAndRecovery(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admin, err := s.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err = s.FederatedLogin(ctx, "https://issuer.example", "app-subject", "Requested Person", false); !errors.Is(err, ErrPendingApproval) {
			t.Fatal("unapproved identity got access", err)
		}
	}
	users, err := s.ListUsers(ctx, admin.Identity, "", Page{})
	if err != nil {
		t.Fatal(err)
	}
	var requested User
	count := 0
	for _, u := range users.Items {
		if u.ApprovalPending {
			requested = u
			count++
		}
	}
	if count != 1 || !requested.Disabled || requested.Administrator || requested.OIDCIssuer != "https://issuer.example" || requested.OIDCSubject != "app-subject" {
		t.Fatal("pending projection or deduplication")
	}
	if _, err = s.SetUserDisabled(ctx, Identity{}, "unauthorized-approval", SetUserDisabled{ID: requested.ID}); err == nil {
		t.Fatal("anonymous approval")
	}
	if _, err = s.SetUserAdministrator(ctx, admin.Identity, "pending-admin", requested.ID, true); err == nil {
		t.Fatal("pending account made administrator")
	}
	if _, err = s.SetUserDisabled(ctx, admin.Identity, "approve-request", SetUserDisabled{ID: requested.ID}); err != nil {
		t.Fatal(err)
	}
	session, err := s.FederatedLogin(ctx, "https://issuer.example", "app-subject", "Requested Person", false)
	if err != nil || session.Identity.ID != requested.ID || session.Identity.Administrator {
		t.Fatal("approval did not preserve ordinary identity", err)
	}
	if _, err = s.SetUserDisabled(ctx, admin.Identity, "disable-approved", SetUserDisabled{ID: requested.ID, Disabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, session.Token); err == nil {
		t.Fatal("disabled SSO session survived")
	}
	if _, err = s.FederatedLogin(ctx, "https://issuer.example", "app-subject", "Requested Person", true); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("disabled account resurrected", err)
	}
	if _, err = s.FederatedLogin(ctx, "https://issuer.example", "rejected-subject", "Rejected Person", false); !errors.Is(err, ErrPendingApproval) {
		t.Fatal(err)
	}
	users, _ = s.ListUsers(ctx, admin.Identity, "", Page{})
	for _, u := range users.Items {
		if u.ApprovalPending {
			if _, err = s.SetUserDisabled(ctx, admin.Identity, "reject-request", SetUserDisabled{ID: u.ID, Disabled: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err = s.FederatedLogin(ctx, "https://issuer.example", "rejected-subject", "Rejected Person", false); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("rejected request reopened", err)
	}
	if _, err = s.FederatedLogin(ctx, "https://issuer.example", "pending-backup", "Waiting Person", false); !errors.Is(err, ErrPendingApproval) {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err = s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "restored")
	if _, err = RestoreBackup(ctx, backup, dest); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err = restored.FederatedLogin(ctx, "https://issuer.example", "pending-backup", "Waiting Person", false); !errors.Is(err, ErrPendingApproval) {
		t.Fatal("restored pending request bypassed approval", err)
	}
}
