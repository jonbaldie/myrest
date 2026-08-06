package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestRoleSwitchStatementActivatesTheRole(t *testing.T) {
	t.Parallel()

	statement, err := roleSwitchStatement("myrest_anon")
	if err != nil {
		t.Fatalf("roleSwitchStatement: %v", err)
	}
	if want := "SET ROLE 'myrest_anon'"; statement != want {
		t.Fatalf("statement = %q, want %q", statement, want)
	}
}

func TestRoleSwitchStatementRefusesANameItCannotQuote(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		role schemacache.Role
	}{
		{name: "an empty name", role: ""},
		{name: "a name that closes the quote", role: "myrest_anon'; DROP DATABASE shop; --"},
		{name: "a name with a host part", role: "myrest_anon@%"},
		{name: "a name with a space", role: "myrest anon"},
		{name: "a MySQL keyword with a space", role: "NONE "},
		{name: "a name with a dash", role: "myrest-anon"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			statement, err := roleSwitchStatement(one.role)
			if err == nil {
				t.Fatalf("roleSwitchStatement gave %q for the role %q", statement, one.role)
			}
		})
	}
}

func TestSimpleNameAcceptsLettersDigitsAndUnderscores(t *testing.T) {
	t.Parallel()

	for _, role := range []schemacache.Role{"a", "Z", "myrest_anon", "role9", "_"} {
		if !isSimpleName(role) {
			t.Errorf("isSimpleName(%q) = false, want true", role)
		}
	}
}
