package httpapi

import (
	"math"
	"strconv"
	"testing"

	"github.com/jonbaldie/myrest/internal/rows"
)

func FuzzEmbedKeySeparatesCompositeValues(f *testing.F) {
	f.Add("a", "b\x1fc", "a\x1fb", "c")
	f.Fuzz(func(t *testing.T, leftFirst, leftSecond, rightFirst, rightSecond string) {
		if leftFirst == "" || leftSecond == "" || rightFirst == "" || rightSecond == "" {
			t.Skip()
		}
		if leftFirst == rightFirst && leftSecond == rightSecond {
			t.Skip()
		}
		read := []rows.Row{
			{Columns: []string{"first", "second"}, Values: []any{leftFirst, leftSecond}},
			{Columns: []string{"first", "second"}, Values: []any{rightFirst, rightSecond}},
		}
		if len(uniqueKeyTuples(read, []string{"first", "second"})) != 2 {
			t.Fatalf("distinct composite keys collapsed: %#v", read)
		}
	})
}

func FuzzEmbedKeyKeepsEmptyString(f *testing.F) {
	f.Add("")
	f.Fuzz(func(t *testing.T, value string) {
		if value != "" {
			t.Skip()
		}
		read := []rows.Row{{Columns: []string{"id"}, Values: []any{value}}}
		if len(uniqueKeyTuples(read, []string{"id"})) != 1 {
			t.Fatalf("empty string key was dropped: %#v", read)
		}
	})
}

func FuzzEmbedKeySeparatesEmptyFromText(f *testing.F) {
	f.Add("0:")
	f.Fuzz(func(t *testing.T, value string) {
		if value == "" {
			t.Skip()
		}
		read := []rows.Row{
			{Columns: []string{"id"}, Values: []any{""}},
			{Columns: []string{"id"}, Values: []any{value}},
		}
		if len(uniqueKeyTuples(read, []string{"id"})) != 2 {
			t.Fatalf("empty key collided with %q", value)
		}
	})
}

func FuzzEmbedKeyKeepsFractionalFloat(f *testing.F) {
	f.Add(1.5)
	f.Fuzz(func(t *testing.T, value float64) {
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) == value {
			t.Skip()
		}
		want := strconv.FormatFloat(value, 'g', -1, 64)
		if got := stringifyValue(value); got != want {
			t.Fatalf("float key = %q, want %q for %v", got, want, value)
		}
	})
}

func TestParseMediaPreferenceRejectsNaNQuality(t *testing.T) {
	t.Parallel()

	_, quality, ok := parseMediaPreference("application/json;q=NaN")
	if !ok || quality != 0 {
		t.Fatalf("quality = %v, ok = %v; want invalid quality 0", quality, ok)
	}
}

func FuzzMediaQualityIsFinite(f *testing.F) {
	f.Add("application/json;q=NaN")
	f.Fuzz(func(t *testing.T, part string) {
		_, quality, ok := parseMediaPreference(part)
		if ok && (math.IsNaN(quality) || math.IsInf(quality, 0)) {
			t.Fatalf("non-finite quality %v for %q", quality, part)
		}
	})
}
