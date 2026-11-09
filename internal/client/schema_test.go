package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/skillhubhq/skillhub/internal/sqliteutil"
)

func TestClientReopenPreservesStateAndRejectsUnsupportedSchema(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.Exec(`INSERT INTO installations VALUES('/example','id','{"id":"id","target":"/example","version":"1.0.0"}')`)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := sqliteutil.Open(filepath.Join(root, "client.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec("DROP TABLE suite_installations; UPDATE client_schema SET version=2"); err != nil {
		t.Fatal(err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err = store.db.QueryRow("SELECT version FROM client_schema").Scan(&version); err != nil || version != 3 {
		t.Fatal("complete client structure did not preserve schema identity", err)
	}
	items, err := store.ListInstallations(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != "id" {
		t.Fatal("reopen lost existing installations")
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
	db, err := sqliteutil.Open(filepath.Join(root, "client.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='projects'").Scan(&count); err != nil || count != 0 {
		t.Fatal("future schema was modified")
	}
}
