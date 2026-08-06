package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestRoleSwitchStatementActivatesTheRole(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		role schemacache.Role
		want string
	}{
		{name: "a simple name", role: "myrest_anon", want: "SET ROLE 'myrest_anon'"},
		// MySQL takes any of these as a role name, so myrest must too.
		{name: "a name with a dash", role: "web-anon", want: "SET ROLE 'web-anon'"},
		{name: "a name with a space", role: "web anon", want: "SET ROLE 'web anon'"},
		{name: "a name with a host", role: "web-anon@localhost", want: "SET ROLE 'web-anon'@'localhost'"},
		{name: "a name with the host %", role: "web_anon@%", want: "SET ROLE 'web_anon'@'%'"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			statement, err := roleSwitchStatement(one.role)
			if err != nil {
				t.Fatalf("roleSwitchStatement(%q): %v", one.role, err)
			}
			if statement != one.want {
				t.Fatalf("statement = %q, want %q", statement, one.want)
			}
		})
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
		{name: "a name with a double quote", role: `myrest"anon`},
		{name: "a name with a back quote", role: "myrest`anon"},
		{name: "a name with an escape character", role: `myrest\anon`},
		{name: "a name with a new line", role: "myrest\nanon"},
		{name: "a name with the delete character", role: "myrest\x7fanon"},
		{name: "a host that closes the quote", role: "myrest_anon@'"},
		{name: "an empty host", role: "myrest_anon@"},
		{name: "a host with no name", role: "@localhost"},
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
