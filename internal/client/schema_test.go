package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/skillhubhq/skillhub/internal/sqliteutil"
)

func TestClientMigrationPreservesV1AndRejectsFutureSchema(t *testing.T) {
	root := t.TempDir()
	db, err := sqliteutil.Open(filepath.Join(root, "client.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE client_schema(version INTEGER NOT NULL); INSERT INTO client_schema VALUES(1);
CREATE TABLE installations(target TEXT PRIMARY KEY,id TEXT UNIQUE NOT NULL,record TEXT NOT NULL);
CREATE TABLE install_journals(target TEXT PRIMARY KEY,record TEXT NOT NULL);
CREATE TABLE sources(alias TEXT PRIMARY KEY,registry_id TEXT NOT NULL,record TEXT NOT NULL);
INSERT INTO installations VALUES('/example','id','{"id":"id","target":"/example","version":"1.0.0"}');`)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListInstallations(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != "id" {
		t.Fatal("migration lost existing installations")
	}
	core := NewClient(store, ClientOptions{})
	project := t.TempDir()
	state, err := core.InitProject(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	other := NewClient(second, ClientOptions{})
	roots, err := other.ListProjects(context.Background())
	if err != nil || len(roots) != 1 || roots[0] != state.Root {
		t.Fatal("client instances do not share project inventory")
	}
	if err = os.RemoveAll(project); err != nil {
		t.Fatal(err)
	}
	if err = other.ForgetProject(context.Background(), state.Root); err != nil {
		t.Fatal(err)
	}
	roots, err = core.ListProjects(context.Background())
	if err != nil || len(roots) != 0 {
		t.Fatal("deleted project cannot be forgotten")
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec("UPDATE client_schema SET version=999; DROP TABLE projects"); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := OpenStore(root); err == nil {
		reopened.Close()
		t.Fatal("opened future schema")
	}
	db, err = sqliteutil.Open(filepath.Join(root, "client.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='projects'").Scan(&count); err != nil || count != 0 {
		t.Fatal("future schema was modified")
	}
}
