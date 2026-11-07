package hub

import (
	"context"
	"errors"
	"testing"
)

func TestCredentialRecoveryAndFederatedBoundary(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	admin, e := s.Login(ctx, "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	device, e := s.StartDevice(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveDevice(ctx, device.UserCode, "admin", "a-strong-test-password"); e != nil {
		t.Fatal(e)
	}
	if e = s.ChangePassword(ctx, admin.Identity, "a-strong-test-password", "new-admin-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteDevice(ctx, device.DeviceCode); e == nil {
		t.Fatal("approved device survived password change")
	}
	if _, e = s.Login(ctx, "admin", "a-strong-test-password"); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("old password accepted", e)
	}
	admin, e = s.Login(ctx, "admin", "new-admin-password")
	if e != nil {
		t.Fatal(e)
	}
	federated, e := s.FederatedLogin(ctx, "https://issuer.example", "person", "Person", true)
	if e != nil {
		t.Fatal(e)
	}
	if !federated.Identity.Federated {
		t.Fatal("missing federated identity")
	}
	if e = s.ResetUserPassword(ctx, admin.Identity, "sso-reset", federated.Identity.ID, "reset-test-password"); !errors.Is(e, ErrForbidden) {
		t.Fatal("SSO password reset", e)
	}
	if e = s.RecoverAdministrator(ctx, federated.Identity.Username, "recovery-password"); !errors.Is(e, ErrForbidden) {
		t.Fatal("SSO converted by recovery", e)
	}
	if _, e = s.SetUserAdministrator(ctx, admin.Identity, "sso-promote", federated.Identity.ID, true); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, federated.Token); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("SSO session survived promotion", e)
	}
	federated, e = s.FederatedLogin(ctx, "https://issuer.example", "person", "Person", false)
	if e != nil || !federated.Identity.Administrator {
		t.Fatal("SSO admin handoff", e)
	}
	if e = s.RecoverAdministrator(ctx, "missing", "recovery-password"); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if e = s.RecoverAdministrator(ctx, "admin", "recovery-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, admin.Token); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("recovery kept old session", e)
	}
	if _, e = s.Login(ctx, "admin", "recovery-password"); e != nil {
		t.Fatal(e)
	}
}
func TestServingLockExcludesRecoveryAndDuplicateServer(t *testing.T) {
	root := t.TempDir()
	unlock, e := AcquireServingLock(root)
	if e != nil {
		t.Fatal(e)
	}
	if second, e := AcquireServingLock(root); e == nil {
		second()
		unlock()
		t.Fatal("concurrent serving lock")
	}
	unlock()
	unlock, e = AcquireServingLock(root)
	if e != nil {
		t.Fatal(e)
	}
	unlock()
}
