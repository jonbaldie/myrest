package mysqlname_test

import (
	"github.com/jonbaldie/myrest/internal/mysqlname"
	"testing"
)

func TestParseRole(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		role, name, host string
		hasHost          bool
	}{
		{"web anon", "web anon", "", false},
		{"web-anon@%", "web-anon", "%", true},
		{"name@part@localhost", "name@part", "localhost", true},
	} {
		role, err := mysqlname.ParseRole(tc.role)
		if err != nil || role.Name != tc.name || role.Host != tc.host || role.HasHost != tc.hasHost {
			t.Errorf("ParseRole(%q) = %+v, %v", tc.role, role, err)
		}
	}
	for _, role := range []string{"", "@host", "name@", "bad'role", "bad\"role", "bad`role", `bad\role`, "bad\nrole", "bad\x7frole", "role@'", "role@\n"} {
		if _, err := mysqlname.ParseRole(role); err == nil {
			t.Errorf("accepted %q", role)
		}
	}
}

func TestSplitPreRequest(t *testing.T) {
	t.Parallel()
	database, routine, err := mysqlname.SplitPreRequest("db.od`d")
	if err != nil || database != "db" || routine != "od`d" {
		t.Fatalf("split = %q, %q, %v", database, routine, err)
	}
	for _, name := range []string{"", "nodot", "too.many.parts", ".routine", "db."} {
		if _, _, err := mysqlname.SplitPreRequest(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}
