package prefer_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/prefer"
	"github.com/jonbaldie/myrest/internal/readquery"
)

var surfaces = []prefer.Surface{prefer.SurfaceRead, prefer.SurfaceRPC, prefer.SurfaceWrite}

// TestRefusalBySurface holds the strict rule for every request surface: one
// header gives the same invalid tokens on table reads, RPC, and writes, except
// count=planned and count=estimated. Reads and RPC refuse those as a MySQL gap
// (readquery), so only the write surface names them invalid.
func TestRefusalBySurface(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		// invalid is the PGRST122 token list per surface; nil means no refusal.
		invalid map[prefer.Surface][]string
	}{
		{name: "no header", header: ""},
		{
			name:   "every supported token under strict",
			header: "handling=strict, return=representation, missing=default, max-affected=3, tx=commit, count=exact, resolution=merge-duplicates, all-rows",
		},
		{name: "unknown token under lenient", header: "bogus=1"},
		{name: "unknown token without handling", header: "bogus=1, count=bogus"},
		{name: "explicit lenient", header: "handling=lenient, bogus=1"},
		{
			name:    "unknown token under strict",
			header:  "handling=strict, bogus=1",
			invalid: everySurface("bogus=1"),
		},
		{
			name:    "bad count under strict",
			header:  "handling=strict, count=bogus",
			invalid: everySurface("count=bogus"),
		},
		{
			name:    "planned count under strict",
			header:  "handling=strict, count=planned",
			invalid: map[prefer.Surface][]string{prefer.SurfaceWrite: {"count=planned"}},
		},
		{
			name:    "estimated count under strict",
			header:  "handling=strict, count=estimated",
			invalid: map[prefer.Surface][]string{prefer.SurfaceWrite: {"count=estimated"}},
		},
		{
			name:    "bad values keep header and rule order",
			header:  "resolution=x, handling=strict, foo, count=bogus, return=, tx=sideways, max-affected=-1, missing=null, return=all",
			invalid: everySurface("foo", "return=", "return=all", "missing=null", "max-affected=-1", "tx=sideways", "count=bogus", "resolution=x"),
		},
		{
			name:   "gap count before another invalid token",
			header: "handling=strict, count=planned, resolution=x",
			invalid: map[prefer.Surface][]string{
				prefer.SurfaceRead:  {"resolution=x"},
				prefer.SurfaceRPC:   {"resolution=x"},
				prefer.SurfaceWrite: {"count=planned", "resolution=x"},
			},
		},
		{
			name:    "empty parts are skipped and bare known names are invalid",
			header:  "handling=strict,, missing, ,count",
			invalid: everySurface("missing", "count"),
		},
		{
			name:    "valued all-rows under strict",
			header:  "handling=strict, all-rows=true, all-rows=false, all-rows=",
			invalid: everySurface("all-rows=true", "all-rows=false", "all-rows="),
		},
		{
			name:    "bad handling value",
			header:  "handling=loose, bogus",
			invalid: nil,
		},
		{
			name:    "auth tokens are refused elsewhere, not invalid",
			header:  "handling=strict, row-security=on, jwt-claims, timezone=UTC, row-security, jwt-claims=x, timezone",
			invalid: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			preferences := prefer.Parse([]string{tc.header})
			for _, surface := range surfaces {
				err := preferences.Refusal(surface)
				want := tc.invalid[surface]
				if want == nil {
					if err != nil {
						t.Fatalf("surface %v: Refusal = %v, want nil", surface, err)
					}
					continue
				}
				var invalid prefer.InvalidError
				if !errors.As(err, &invalid) {
					t.Fatalf("surface %v: Refusal = %v, want InvalidError", surface, err)
				}
				if !reflect.DeepEqual(invalid.Tokens, want) {
					t.Fatalf("surface %v: tokens = %q, want %q", surface, invalid.Tokens, want)
				}
			}
		})
	}
}

func everySurface(tokens ...string) map[prefer.Surface][]string {
	return map[prefer.Surface][]string{
		prefer.SurfaceRead:  tokens,
		prefer.SurfaceRPC:   tokens,
		prefer.SurfaceWrite: tokens,
	}
}

func TestInvalidErrorContract(t *testing.T) {
	t.Parallel()

	err := prefer.InvalidError{Tokens: []string{"bogus=1", "count=bogus"}}
	if err.Error() != "Invalid preferences given with handling=strict" {
		t.Fatalf("Error = %q", err.Error())
	}
	if err.Details() != "Invalid preferences: bogus=1, count=bogus" {
		t.Fatalf("Details = %q", err.Details())
	}
}

func TestParseValues(t *testing.T) {
	t.Parallel()

	three, zero, nineteen := int64(3), int64(0), int64(19)
	cases := []struct {
		name    string
		headers []string
		want    prefer.Preferences
	}{
		{name: "none", headers: nil, want: prefer.Preferences{}},
		{
			name:    "every supported token",
			headers: []string{"Handling=Strict, RETURN=Representation, missing=Default", "max-affected=3, tx=Rollback, count=Exact, resolution=Ignore-Duplicates, all-rows"},
			want: prefer.Preferences{
				Strict:         true,
				Return:         prefer.ReturnRepresentation,
				MissingDefault: true,
				MaxAffected:    &three,
				Tx:             "rollback",
				Count:          readquery.CountExact,
				Resolution:     "ignore-duplicates",
				AllRows:        true,
			},
		},
		{name: "count planned", headers: []string{"count=planned"}, want: prefer.Preferences{Count: readquery.CountPlanned}},
		{name: "count estimated", headers: []string{"count=estimated"}, want: prefer.Preferences{Count: readquery.CountEstimated}},
		{name: "count bogus", headers: []string{"count=bogus"}, want: prefer.Preferences{}},
		{name: "return headers-only", headers: []string{"return=headers-only"}, want: prefer.Preferences{Return: prefer.ReturnHeadersOnly}},
		{name: "return minimal", headers: []string{"return=minimal"}, want: prefer.Preferences{Return: prefer.ReturnMinimal}},
		{name: "return bogus", headers: []string{"return=bogus"}, want: prefer.Preferences{}},
		{name: "last handling wins", headers: []string{"handling=strict, handling=lenient"}, want: prefer.Preferences{}},
		{name: "valued all-rows is not the flag", headers: []string{"all-rows=true, all-rows=false, all-rows="}, want: prefer.Preferences{}},
		{name: "bare all-rows among other tokens", headers: []string{"return=bogus, all-rows, missing=x"}, want: prefer.Preferences{AllRows: true}},
		{name: "bad max-affected", headers: []string{"max-affected=x"}, want: prefer.Preferences{}},
		{name: "negative max-affected", headers: []string{"max-affected=-1"}, want: prefer.Preferences{}},
		{name: "zero max-affected", headers: []string{"max-affected=0"}, want: prefer.Preferences{MaxAffected: &zero}},
		{name: "decimal max-affected", headers: []string{"max-affected=19"}, want: prefer.Preferences{MaxAffected: &nineteen}},
		{name: "spaces around name and value", headers: []string{" count = exact , return =  minimal "}, want: prefer.Preferences{Count: readquery.CountExact, Return: prefer.ReturnMinimal}},
		{name: "bad tx", headers: []string{"tx=sideways"}, want: prefer.Preferences{}},
		{name: "last resolution wins", headers: []string{"resolution=bogus, resolution=merge-duplicates"}, want: prefer.Preferences{Resolution: prefer.ResolutionMergeDuplicates}},
		{name: "bad resolution", headers: []string{"resolution=bogus"}, want: prefer.Preferences{BadResolution: true}},
		{name: "empty resolution", headers: []string{"resolution="}, want: prefer.Preferences{}},
		{
			name:    "auth tokens",
			headers: []string{"Row-Security=on, jwt-claims, timezone=UTC"},
			want:    prefer.Preferences{RowSecurity: true, JWTClaims: true, Timezone: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := prefer.Parse(tc.headers)
			// Compare only the exported values; the invalid list is checked
			// through Refusal.
			if !reflect.DeepEqual(exported(got), exported(tc.want)) {
				t.Fatalf("Parse = %+v, want %+v", exported(got), exported(tc.want))
			}
		})
	}
}

type values struct {
	Strict, MissingDefault, AllRows, BadResolution bool
	RowSecurity, JWTClaims, Timezone               bool
	Return, Tx, Resolution                         string
	Count                                          readquery.CountMode
	MaxAffected                                    int64
	HasMaxAffected                                 bool
}

func exported(p prefer.Preferences) values {
	v := values{
		Strict: p.Strict, MissingDefault: p.MissingDefault, AllRows: p.AllRows,
		BadResolution: p.BadResolution, RowSecurity: p.RowSecurity,
		JWTClaims: p.JWTClaims, Timezone: p.Timezone, Return: p.Return,
		Tx: p.Tx, Resolution: p.Resolution, Count: p.Count,
	}
	if p.MaxAffected != nil {
		v.MaxAffected, v.HasMaxAffected = *p.MaxAffected, true
	}
	return v
}
