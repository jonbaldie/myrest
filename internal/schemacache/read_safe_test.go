package schemacache_test

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/schemacache"
)

func TestReadSafeFollowsSQLDataAccess(t *testing.T) {
	t.Parallel()

	cases := []struct {
		access string
		safe   bool
	}{
		{access: "NO SQL", safe: true},
		{access: "CONTAINS SQL", safe: true},
		{access: "READS SQL DATA", safe: true},
		{access: "MODIFIES SQL DATA", safe: false},
		{access: "", safe: false},
		{access: "unknown", safe: false},
		{access: "no sql", safe: true},
	}
	for _, test := range cases {
		got := schemacache.RoutineFact{SQLDataAccess: test.access}.ReadSafe()
		if got != test.safe {
			t.Errorf("SQLDataAccess %q: ReadSafe = %v, want %v", test.access, got, test.safe)
		}
	}
}
