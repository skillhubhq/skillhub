package hub

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if _, err = s.Bootstrap(context.Background(), "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLocalCredentialsExpireAndRevoke(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	for _, username := range []string{"admin", "missing"} {
		if _, err := s.Login(ctx, username, "incorrect-password"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("invalid login: %v", err)
		}
	}
	value, err := s.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if user, err := s.Authenticate(ctx, value.Token); err != nil || !user.Administrator || user.Username != "admin" {
		t.Fatalf("identity: %+v %v", user, err)
	}
	var stored string
	if err = s.db.QueryRow("SELECT token_hash FROM sessions").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == value.Token || stored != secretHash(value.Token) {
		t.Fatal("raw token persisted")
	}
	if err = s.Logout(ctx, value.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, value.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("revoked token accepted")
	}
	value, err = s.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	now = value.ExpiresAt
	if _, err = s.Authenticate(ctx, value.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired token accepted")
	}
}

func TestDisableInvalidatesSessionsAndDevices(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	value, err := s.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	device, err := s.StartDevice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ApproveDevice(ctx, device.UserCode, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE users SET disabled=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, value.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("disabled session accepted")
	}
	if _, err = s.CompleteDevice(ctx, device.DeviceCode); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("disabled device accepted")
	}
	if _, err = s.Login(ctx, "admin", "a-strong-test-password"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("disabled password accepted")
	}
}

func TestDevicePollingExpiryAndSingleConsumptionAcrossStores(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Bootstrap(ctx, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	other.now = s.now
	device, err := s.StartDevice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := s.CompleteDevice(ctx, device.DeviceCode); err != nil || value != nil {
		t.Fatalf("pending: %v %v", value, err)
	}
	if _, err = s.CompleteDevice(ctx, device.DeviceCode); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("poll frequency: %v", err)
	}
	if _, err = s.CompleteDevice(ctx, device.UserCode); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("short code accepted")
	}
	if err = s.ApproveDevice(ctx, device.UserCode, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	if err = s.ApproveDevice(ctx, device.UserCode, "admin", "a-strong-test-password"); !errors.Is(err, ErrNotFound) {
		t.Fatal("approved twice")
	}
	now = now.Add(2 * time.Second)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, store := range []*Store{s, other} {
		wg.Go(func() { _, err := store.CompleteDevice(ctx, device.DeviceCode); results <- err })
	}
	wg.Wait()
	close(results)
	success, denied := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrUnauthenticated) {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("single consumption: success=%d denied=%d", success, denied)
	}
	device, err = s.StartDevice(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now = device.ExpiresAt
	if _, err = s.CompleteDevice(ctx, device.DeviceCode); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("expired device accepted")
	}
}

func TestStableInstanceAndBootstrapOnce(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	id := s.RegistryID()
	if _, err = s.Bootstrap(ctx, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.RegistryID() != id {
		t.Fatal("instance identity changed")
	}
	if _, err = s.Bootstrap(ctx, "another", "a-strong-test-password"); !errors.Is(err, ErrConflict) {
		t.Fatal("bootstrap repeated")
	}
}
