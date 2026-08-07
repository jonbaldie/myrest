package mysqldb

import (
	"testing"
)

func TestPreRequestCallStatementNamesTheRoutine(t *testing.T) {
	t.Parallel()

	got, err := preRequestCallStatement("myrest_fixture.before_request")
	if err != nil {
		t.Fatalf("statement: %v", err)
	}
	want := "CALL `myrest_fixture`.`before_request`()"
	if got != want {
		t.Fatalf("statement = %q, want %q", got, want)
	}
}

func TestPreRequestCallStatementKeepsABackQuoteInsideTheName(t *testing.T) {
	t.Parallel()

	got, err := preRequestCallStatement("db.od`d")
	if err != nil {
		t.Fatalf("statement: %v", err)
	}
	want := "CALL `db`.`od``d`()"
	if got != want {
		t.Fatalf("statement = %q, want %q", got, want)
	}
}

func TestPreRequestCallStatementNeedsDatabaseAndRoutine(t *testing.T) {
	t.Parallel()

	cases := []string{"", "only_routine", "too.many.parts", ".missing", "missing."}
	for _, name := range cases {
		if _, err := preRequestCallStatement(name); err == nil {
			t.Fatalf("name %q was accepted", name)
		}
	}
}
