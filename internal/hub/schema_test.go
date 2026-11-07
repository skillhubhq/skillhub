package hub

import (
	"context"
	"testing"

	"github.com/skillhubhq/skillhub/internal/hubdb"
)

func TestSchemaInitializationAndGuards(t *testing.T) {
	testSchemaInitializationAndGuards(t, hubdb.Config{Driver: "sqlite"})
}

func TestPostgresSchemaInitializationAndGuards(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testSchemaInitializationAndGuards(t, cfg)
}

func testSchemaInitializationAndGuards(t *testing.T, cfg hubdb.Config) {
	t.Helper()
	root := t.TempDir()
	s, err := OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := s.Bootstrap(context.Background(), "admin", "test-password-123")
	if err != nil {
		t.Fatal(err)
	}
	id := s.RegistryID()
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenDatabase(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.RegistryID() != id {
		t.Fatal("reopening changed registry identity")
	}
	user, err := s.Login(context.Background(), "admin", "test-password-123")
	if err != nil || user.Identity.ID != admin.ID {
		t.Fatal("reopening lost administrator", err)
	}
	if _, err = s.db.Exec("PRAGMA user_version=999;"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if opened, e := OpenDatabase(root, cfg); e == nil {
		opened.Close()
		t.Fatal("accepted unsupported schema")
	}
	db, err := hubdb.Open(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 999 {
		t.Fatal("unsupported schema was changed", err)
	}
	if _, err = db.Exec("PRAGMA user_version=1; DROP TABLE community_reports;"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if opened, e := OpenDatabase(root, cfg); e == nil {
		opened.Close()
		t.Fatal("accepted incomplete schema")
	}
	db, err = hubdb.Open(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if rows, e := db.Query("SELECT * FROM community_reports"); e == nil {
		rows.Close()
		t.Fatal("incomplete database silently repaired")
	}
	var preserved string
	if err = db.QueryRow("SELECT id FROM instance").Scan(&preserved); err != nil || preserved != id {
		t.Fatal("failure changed identity", err)
	}
}

func TestSchemaInitializationIsAtomic(t *testing.T) {
	cfg := hubdb.Config{Driver: "sqlite"}
	testSchemaInitializationIsAtomic(t, cfg)
}
func TestPostgresSchemaInitializationIsAtomic(t *testing.T) {
	cfg, _ := postgresFixture(t)
	testSchemaInitializationIsAtomic(t, cfg)
}
func testSchemaInitializationIsAtomic(t *testing.T, cfg hubdb.Config) {
	root := t.TempDir()
	db, err := hubdb.Open(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE users (sentinel TEXT); INSERT INTO users VALUES ('preserved');"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if s, e := OpenDatabase(root, cfg); e == nil {
		s.Close()
		t.Fatal("initialized over occupied database")
	}
	db, err = hubdb.Open(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err = db.QueryRow("SELECT sentinel FROM users").Scan(&sentinel); err != nil || sentinel != "preserved" {
		t.Fatal("initialization changed existing data", err)
	}
	if rows, e := db.Query("SELECT * FROM instance"); e == nil {
		rows.Close()
		t.Fatal("failed initialization left partial tables")
	}
}
