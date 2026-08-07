package mysqldb

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestRoleOfGranteeReadsEveryGranteeShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		grantee string
		want    schemacache.Role
	}{
		{name: "a quoted grantee with a host", grantee: "'myrest_anon'@'%'", want: "myrest_anon"},
		{name: "a bare grantee with a host", grantee: "myrest_anon@%", want: "myrest_anon"},
		{name: "a bare role name", grantee: "myrest_anon", want: "myrest_anon"},
		{name: "an unfinished quote", grantee: "'unfinished", want: ""},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			if role := roleOfGrantee(one.grantee); role != one.want {
				t.Fatalf("role of %q = %q, want %q", one.grantee, role, one.want)
			}
		})
	}
}

func TestPlaceholdersMatchTheNumberOfDatabases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		databases []string
		want      string
	}{
		{name: "no database", databases: nil, want: ""},
		{name: "one database", databases: []string{"shop"}, want: "?"},
		{name: "three databases", databases: []string{"shop", "warehouse", "audit"}, want: "?, ?, ?"},
	}
	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			t.Parallel()

			if list := placeholders(one.databases); list != one.want {
				t.Fatalf("placeholders = %q, want %q", list, one.want)
			}
		})
	}
}

func TestSplitPrivilegesReadsAPackedGrantSet(t *testing.T) {
	t.Parallel()

	got := splitPrivileges("Select,Insert")
	want := []string{"SELECT", "INSERT"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("splitPrivileges = %#v, want %#v", got, want)
	}
}

func TestAsArgumentsPassesEveryDatabase(t *testing.T) {
	t.Parallel()

	passed := asArguments([]string{"shop", "warehouse"})

	if len(passed) != 2 || passed[0] != "shop" || passed[1] != "warehouse" {
		t.Fatalf("arguments = %#v, want shop and warehouse", passed)
	}
}
