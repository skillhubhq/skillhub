package hub

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/skillhubhq/skillhub/internal/hubdb"
)

func TestDeliverySchemaUpgradePreservesIdentityAndPackages(t *testing.T) {
	testDeliverySchemaUpgrade(t, hubdb.Config{Driver: "sqlite"})
}
func TestPostgresDeliverySchemaUpgradePreservesIdentityAndPackages(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testDeliverySchemaUpgrade(t, cfg)
}
func testDeliverySchemaUpgrade(t *testing.T, cfg hubdb.Config) {
	ctx := context.Background()
	root := t.TempDir()
	s, e := OpenDatabase(root, cfg)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Bootstrap(ctx, "admin", "a-strong-test-password"); e != nil {
		t.Fatal(e)
	}
	_, session, _, release := backupFixtureStore(t, s)
	id := s.RegistryID()
	if _, e = s.db.Exec("DROP TABLE subscriptions; DROP TABLE access_tokens;"); e != nil {
		t.Fatal(e)
	}
	dropRenewableSchema(t, s)
	if _, e = s.db.Exec("PRAGMA user_version=14;"); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = OpenDatabase(root, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.RegistryID() != id {
		t.Fatal("identity changed")
	}
	actor, e := s.Authenticate(ctx, session.Token)
	if e != nil {
		t.Fatal("lost credentials", e)
	}
	if _, _, e = s.DownloadRelease(ctx, actor, release.Namespace, release.Name, release.Version); e != nil {
		t.Fatal("lost package", e)
	}
	if e = s.SetSubscription(ctx, actor, "upgrade-follow", release.Namespace, release.Name, true); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.CreateAccessToken(ctx, actor, "CI", "read", "", 1); e != nil {
		t.Fatal(e)
	}
}
func TestAccessTokenExpiryRevocationAndRestore(t *testing.T) {
	ctx := context.Background()
	s, session, _, r := backupFixture(t)
	actor := session.Identity
	token, secret, e := s.CreateAccessToken(ctx, actor, "CI", "read", "", 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, secret); e != nil {
		t.Fatal(e)
	}
	savedNow := s.now
	s.now = func() time.Time { return time.Unix(token.ExpiresAt, 0) }
	if _, e = s.Authenticate(ctx, secret); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("expiry ignored", e)
	}
	s.now = savedNow
	if e = s.SetSubscription(ctx, actor, "restore-follow", r.Namespace, r.Name, true); e != nil {
		t.Fatal(e)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if _, e = s.Backup(ctx, snapshot); e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(t.TempDir(), "restored")
	if _, e = RestoreBackup(ctx, snapshot, root); e != nil {
		t.Fatal(e)
	}
	restored, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if _, e = restored.Authenticate(ctx, secret); !errors.Is(e, ErrUnauthenticated) {
		t.Fatal("restored token remained valid", e)
	}
	next, e := restored.Login(ctx, "admin", "a-strong-test-password")
	if e != nil {
		t.Fatal(e)
	}
	following, e := restored.ListSubscriptions(ctx, next.Identity, Page{})
	if e != nil || len(following.Items) != 1 {
		t.Fatal("subscription lost", e)
	}
	var hashes int
	if e = s.db.QueryRow("SELECT count(*) FROM access_tokens WHERE token_hash=?", secret).Scan(&hashes); e != nil || hashes != 0 {
		t.Fatal("plaintext token stored", e)
	}
}
