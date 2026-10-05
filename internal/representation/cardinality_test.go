package representation_test

import (
	"errors"
	"testing"

	"github.com/jonbaldie/myrest/internal/representation"
)

// Seam under test: ValidateCardinality, the singular-object row count check.

func TestValidateCardinalityNeedsOneRowForSingularObject(t *testing.T) {
	t.Parallel()

	singular := representation.Spec{Kind: representation.KindSingularObject}
	if err := representation.ValidateCardinality(singular, 1); err != nil {
		t.Fatalf("one row: err = %v", err)
	}
	for _, count := range []int{0, 2, 5} {
		err := representation.ValidateCardinality(singular, count)
		var refusal representation.SingularObjectRefusal
		if !errors.As(err, &refusal) {
			t.Fatalf("%d rows: err = %T %v, want SingularObjectRefusal", count, err, err)
		}
		if refusal.RowCount != count {
			t.Fatalf("%d rows: RowCount = %d", count, refusal.RowCount)
		}
	}
	if got, want := (representation.SingularObjectRefusal{RowCount: 5}).Error(),
		"The result contains 5 rows, while 1 was expected"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestValidateCardinalityAcceptsAnyCountForArrayAndCSV(t *testing.T) {
	t.Parallel()

	for _, kind := range []representation.Kind{representation.KindJSONArray, representation.KindCSV} {
		for _, count := range []int{0, 1, 3} {
			if err := representation.ValidateCardinality(representation.Spec{Kind: kind}, count); err != nil {
				t.Fatalf("kind %v, %d rows: err = %v", kind, count, err)
			}
		}
	}
}

func TestRowOnlyIsEveryKindButTheJSONArray(t *testing.T) {
	t.Parallel()

	cases := map[representation.Kind]bool{
		representation.KindJSONArray:      false,
		representation.KindSingularObject: true,
		representation.KindCSV:            true,
	}
	for kind, want := range cases {
		if got := (representation.Spec{Kind: kind}).RowOnly(); got != want {
			t.Fatalf("kind %v: RowOnly = %v, want %v", kind, got, want)
		}
	}
}
