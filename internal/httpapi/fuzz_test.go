package httpapi

import (
	"math"
	"testing"
)

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
