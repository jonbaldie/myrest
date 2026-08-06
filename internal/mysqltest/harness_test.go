package mysqltest_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"github.com/jonbaldie/myrest/internal/mysqltest"
)

func TestHarnessStartsMySQL8AndLoadsFixtureSQL(t *testing.T) {
	harness, err := mysqltest.Start()
	if err != nil {
		t.Fatalf("start MySQL harness: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := harness.Stop(); stopErr != nil {
			t.Errorf("stop harness: %v", stopErr)
		}
	})

	fixture := filepath.Join("..", "..", "testdata", "fixtures", "schema.sql")
	if err := harness.LoadSQL(fixture); err != nil {
		t.Fatalf("load fixture SQL: %v", err)
	}

	db, err := sql.Open("mysql", harness.DSN())
	if err != nil {
		t.Fatalf("open DSN: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var version string
	if err := db.QueryRow("SELECT VERSION()").Scan(&version); err != nil {
		t.Fatalf("query VERSION(): %v", err)
	}
	if len(version) == 0 || version[0] != '8' {
		t.Fatalf("MySQL version = %q, want 8.x", version)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM myrest_fixture.items").Scan(&count); err != nil {
		t.Fatalf("count fixture rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("fixture row count = %d, want 2", count)
	}
}
