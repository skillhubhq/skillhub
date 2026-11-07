package hub

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/skillhubhq/skillhub/internal/sqliteutil"
)

func TestMigrationPreservesVersionOneIdentityAndRejectsFutureSchema(t *testing.T) {
	root := t.TempDir()
	db, err := sqliteutil.Open(filepath.Join(root, "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE instance(id TEXT PRIMARY KEY,secret BLOB NOT NULL);
INSERT INTO instance VALUES('retained-registry',X'01020304');
CREATE TABLE users(id TEXT PRIMARY KEY,username TEXT UNIQUE NOT NULL,password_hash BLOB NOT NULL,administrator INTEGER NOT NULL,disabled INTEGER NOT NULL DEFAULT 0);
CREATE TABLE sessions(token_hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),expires_at INTEGER NOT NULL);
CREATE TABLE devices(device_hash TEXT PRIMARY KEY,user_code TEXT UNIQUE NOT NULL,expires_at INTEGER NOT NULL,user_id TEXT REFERENCES users(id),last_poll INTEGER NOT NULL DEFAULT 0);
CREATE TABLE audit(sequence INTEGER PRIMARY KEY AUTOINCREMENT,actor_id TEXT NOT NULL,action TEXT NOT NULL,resource TEXT NOT NULL,created_at INTEGER NOT NULL);
INSERT INTO audit(actor_id,action,resource,created_at) VALUES('old-admin','previous.action','retained',1);
PRAGMA user_version=1;`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.RegistryID() != "retained-registry" {
		t.Fatal("migration replaced instance identity")
	}
	admin, err := store.Bootstrap(context.Background(), "admin", "a-strong-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutNamespace(context.Background(), admin, "migration-create", PutNamespace{Name: "team", ReviewRequired: true}); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListAudit(context.Background(), admin, Page{})
	if err != nil || len(events.Items) != 3 || events.Items[0].Action != "previous.action" {
		t.Fatalf("audit migration: %v %v", events, err)
	}
	if _, err = store.db.Exec("PRAGMA user_version=999"); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if reopened, err := Open(root); err == nil {
		_ = reopened.Close()
		t.Fatal("future schema was accepted")
	}
}
