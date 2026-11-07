package hubhttp

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/hubdb"
)

// 设置测试专用 DSN 后，原有 RPC 行为测试逐个使用隔离 schema 在真实 PostgreSQL 上重跑。
func testDatabase(t *testing.T, root string) (*hub.Store, error) {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_POSTGRES_DSN")
	if dsn == "" {
		return hub.Open(root)
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme != "postgres" {
		t.Fatal("test PostgreSQL DSN must be a postgres URL")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("cannot open test PostgreSQL")
	}
	schema := "skillhub_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = db.ExecContext(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); _ = db.Close() })
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return hub.OpenDatabase(root, hubdb.Config{Driver: "postgres", DSN: u.String()})
}
