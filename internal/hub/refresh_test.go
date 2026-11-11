package hub

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/hubdb"
	"testing"
	"time"
)

func dropRenewableSchema(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.db.Exec("DROP INDEX sessions_refresh; DROP INDEX sessions_previous_refresh;"); err != nil {
		t.Fatal(err)
	}
	for _, col := range []string{"previous_access_hash", "previous_access_expires_at", "refresh_hash", "previous_refresh_hash", "refresh_request", "refresh_expires_at", "absolute_expires_at"} {
		if _, err := s.db.Exec("ALTER TABLE sessions DROP COLUMN " + col); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec("ALTER TABLE devices DROP COLUMN renewable"); err != nil {
		t.Fatal(err)
	}
}
func TestRefreshRotationAndRecovery(t *testing.T) {
	testRefreshRotation(t, hubdb.Config{Driver: "sqlite"})
}
func TestPostgresRefreshRotationAndRecovery(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testRefreshRotation(t, cfg)
}
func testRefreshRotation(t *testing.T, cfg hubdb.Config) {
	ctx := context.Background()
	s, err := OpenDatabase(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Bootstrap(ctx, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	s.now = func() time.Time { return now }
	first, err := s.Login(ctx, "admin", "a-strong-test-password", true)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExpiresAt.Sub(now) != accessDuration || first.RefreshExpiresAt.Sub(now) != refreshDuration {
		t.Fatal("wrong lifetimes")
	}
	if _, err = s.Authenticate(ctx, first.RefreshToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("refresh accepted as access")
	}
	var stored string
	if err = s.db.QueryRow("SELECT refresh_hash FROM sessions").Scan(&stored); err != nil || stored != secretHash(first.RefreshToken) {
		t.Fatal("refresh secret was not hashed", err)
	}
	now = now.Add(14 * time.Minute)
	request := uuid.NewString()
	next, err := s.RefreshSession(ctx, first.RefreshToken, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE users SET administrator=0 WHERE id=?", first.Identity.ID); err != nil {
		t.Fatal(err)
	}
	replayPermissions, err := s.RefreshSession(ctx, first.RefreshToken, request)
	if err != nil || replayPermissions.Identity.Administrator {
		t.Fatal("refresh retained removed permissions", err)
	}
	if _, err = s.db.Exec("UPDATE users SET administrator=1 WHERE id=?", first.Identity.ID); err != nil {
		t.Fatal(err)
	}
	if next.Token == first.Token || next.RefreshToken == first.RefreshToken {
		t.Fatal("credentials not rotated")
	}
	if _, err = s.Authenticate(ctx, first.Token); err != nil {
		t.Fatal("in-flight request overlap lost", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err = s.Authenticate(ctx, first.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("old access survived overlap")
	}
	replay, err := s.RefreshSession(ctx, first.RefreshToken, request)
	if err != nil || replay != next {
		t.Fatal("lost response retry changed session", err)
	}
	if _, err = s.RefreshSession(ctx, first.RefreshToken, uuid.NewString()); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("reused refresh accepted")
	}
	if _, err = s.Authenticate(ctx, next.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("reused family remained active")
	}
	if _, err = s.RefreshSession(ctx, next.RefreshToken, uuid.NewString()); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("revoked refresh accepted")
	}
}
func TestRefreshExpiryRevocationAndAccountState(t *testing.T) {
	for _, mode := range []string{"idle", "absolute", "disabled", "password", "logout", "expired-access-logout"} {
		t.Run(mode, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			now := time.Now().UTC().Truncate(time.Second)
			s.now = func() time.Time { return now }
			v, err := s.Login(ctx, "admin", "a-strong-test-password", true)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "idle":
				now = v.RefreshExpiresAt
			case "absolute":
				start := now
				for i := 0; i < 4; i++ {
					now = now.Add(20 * 24 * time.Hour)
					v, err = s.RefreshSession(ctx, v.RefreshToken, uuid.NewString())
					if err != nil {
						t.Fatal(err)
					}
				}
				now = start.Add(sessionDuration)
			case "disabled":
				_, err = s.db.Exec("UPDATE users SET disabled=1")
			case "password":
				err = s.ChangePassword(ctx, v.Identity, "a-strong-test-password", "another-strong-password")
			case "logout":
				err = s.Logout(ctx, v.Token)
			case "expired-access-logout":
				now = v.ExpiresAt
				err = s.RevokeRefreshSession(ctx, v.RefreshToken)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.RefreshSession(ctx, v.RefreshToken, uuid.NewString()); !errors.Is(err, ErrUnauthenticated) {
				t.Fatal("refresh survived termination", err)
			}
		})
	}
}
func TestRefreshRetainsExpiredAccessDuringOtherLogin(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now()
	s.now = func() time.Time { return now }
	v, err := s.Login(ctx, "admin", "a-strong-test-password", true)
	if err != nil {
		t.Fatal(err)
	}
	now = v.ExpiresAt.Add(time.Hour)
	if _, err = s.Login(ctx, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RefreshSession(ctx, v.RefreshToken, uuid.NewString()); err != nil {
		t.Fatal("valid refresh was purged", err)
	}
	device, err := s.StartDevice(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ApproveDevice(ctx, device.UserCode, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	result, err := s.CompleteDevice(ctx, device.DeviceCode)
	if err != nil || result.RefreshToken == "" {
		t.Fatal("device login lacks renewal", err)
	}
}
func TestRefreshSchemaUpgradePreservesSessions(t *testing.T) {
	testRefreshSchemaUpgrade(t, hubdb.Config{Driver: "sqlite"})
}
func TestPostgresRefreshSchemaUpgradePreservesSessions(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testRefreshSchemaUpgrade(t, cfg)
}
func testRefreshSchemaUpgrade(t *testing.T, cfg hubdb.Config) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Bootstrap(ctx, "admin", "a-strong-test-password"); err != nil {
		t.Fatal(err)
	}
	v, err := s.Login(ctx, "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	id := s.RegistryID()
	dropRenewableSchema(t, s)
	if _, err = s.db.Exec("PRAGMA user_version=15;"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.RegistryID() != id {
		t.Fatal("registry changed")
	}
	if _, err = s.Authenticate(ctx, v.Token); err != nil {
		t.Fatal("existing session lost", err)
	}
	if _, err = s.Login(ctx, "admin", "a-strong-test-password", true); err != nil {
		t.Fatal(err)
	}
}
