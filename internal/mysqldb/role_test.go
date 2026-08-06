package mysqldb

import "testing"

func TestRoleSwitchStatementActivatesOneRole(t *testing.T) {
	t.Parallel()

	statement, err := roleSwitchStatement("myrest_anon")
	if err != nil {
		t.Fatalf("roleSwitchStatement: %v", err)
	}
	if want := "SET ROLE 'myrest_anon'"; statement != want {
		t.Fatalf("statement = %q, want %q", statement, want)
	}
}

func TestRoleSwitchStatementActivatesEveryRoleForTheCatalogRead(t *testing.T) {
	t.Parallel()

	statement, err := roleSwitchStatement(allRoles)
	if err != nil {
		t.Fatalf("roleSwitchStatement: %v", err)
	}
	if want := "SET ROLE ALL"; statement != want {
		t.Fatalf("statement = %q, want %q", statement, want)
	}
}

func TestRoleSwitchStatementRefusesANameItCannotQuote(t *testing.T) {
	t.Parallel()

	for name, role := range map[string]string{
		"empty":           "",
		"quote":           "myrest_anon'; DROP DATABASE shop; --",
		"host part":       "myrest_anon@%",
		"space":           "myrest anon",
		"another keyword": "NONE ",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := roleSwitchStatement(role); err == nil {
				t.Fatalf("roleSwitchStatement accepted %q", role)
			}
		})
	}
}
