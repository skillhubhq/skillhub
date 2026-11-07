package hubdb

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

func TestPostgresBinding(t *testing.T) {
	q, args, err := bind("postgres", `SELECT '@actor ?', "?", @actor, ?, @actor, @enabled`, []any{sql.Named("actor", "user"), sql.Named("unused", "ignored"), true, sql.Named("enabled", false)})
	if err != nil || q != `SELECT '@actor ?', "?", $1, $2, $1, $3` || !reflect.DeepEqual(args, []any{"user", int64(1), int64(0)}) {
		t.Fatalf("binding: %s %#v %v", q, args, err)
	}
	for _, input := range []struct {
		q string
		a []any
	}{{"SELECT ?", nil}, {"SELECT @missing", nil}, {"SELECT 1", []any{1}}} {
		if _, _, err = bind("postgres", input.q, input.a); err == nil {
			t.Fatal("accepted invalid binding")
		}
	}
	q, args, err = bind("sqlite", "SELECT @a", []any{sql.Named("a", true)})
	if err != nil || q != "SELECT @a" || args[0].(sql.NamedArg).Value != true {
		t.Fatal("SQLite binding changed")
	}
}
func TestPostgresSchemaAndQueries(t *testing.T) {
	q := postgresSQL("CREATE TABLE sample(sequence INTEGER PRIMARY KEY AUTOINCREMENT,body BLOB); PRAGMA user_version=1;")
	if !strings.Contains(q, "BIGSERIAL PRIMARY KEY") || !strings.Contains(q, "BYTEA") || !strings.Contains(q, "version=1") {
		t.Fatal(q)
	}
	q = postgresSQL("INSERT OR IGNORE INTO reactions VALUES(?,?,?)")
	if !strings.HasSuffix(q, "ON CONFLICT DO NOTHING") {
		t.Fatal(q)
	}
	if _, err := Open(t.TempDir(), Config{Driver: "mysql"}); err == nil {
		t.Fatal("unknown driver accepted")
	}
	if _, err := Open(t.TempDir(), Config{Driver: "sqlite", DSN: "secret"}); err == nil {
		t.Fatal("unexpected DSN accepted")
	}
	if _, err := Open(t.TempDir(), Config{Driver: "postgres", DSN: "postgres://user:secret@%invalid"}); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("invalid DSN was not safely rejected")
	}
}
