package sqliteutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpensExpectedFileAndRetainsPragmas(t *testing.T) {
	root := filepath.Join(t.TempDir(), "space # percent % 中文")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "store.db")
	db, err := Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("CREATE TABLE example(value TEXT); INSERT INTO example VALUES('retained')"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filename); err != nil {
		t.Fatalf("database created at wrong path: %v", err)
	}
	var foreignKeys int
	if err = db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Fatalf("foreign keys: %d %v", foreignKeys, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err = db.QueryRow("SELECT value FROM example").Scan(&value); err != nil || value != "retained" {
		t.Fatalf("reopen: %s %v", value, err)
	}
}

func TestSnapshotIsReadOnlyAndDoesNotCreateMissingDatabase(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "snapshot # %.db")
	db, err := Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE example(value TEXT); INSERT INTO example VALUES('snapshot')"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := OpenSnapshot(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var value string
	if err = snapshot.QueryRow("SELECT value FROM example").Scan(&value); err != nil || value != "snapshot" {
		t.Fatalf("snapshot read: %s %v", value, err)
	}
	if _, err = snapshot.Exec("DELETE FROM example"); err == nil {
		t.Fatal("snapshot allowed writes")
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	db, err = OpenSnapshot(missing)
	if err == nil {
		err = db.Ping()
		db.Close()
	}
	if err == nil {
		t.Fatal("opened missing snapshot")
	}
	if _, err = os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("read-only snapshot created a database")
	}
}
