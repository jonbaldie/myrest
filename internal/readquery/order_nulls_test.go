package readquery_test

import (
	"errors"
	"net/url"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func TestParseOrderNullsOptionsAreFeatureGaps(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"id", "payload->>key"} {
		for _, option := range []string{"asc.nullsfirst", "asc.nullslast", "desc.nullsfirst", "desc.nullslast", "nullsfirst", "nullslast"} {
			t.Run(field+"."+option, func(t *testing.T) {
				_, err := readquery.Parse(url.Values{"order": {field + "." + option}}, readquery.CountNone)
				var failure readquery.ParseFailure
				if !errors.As(err, &failure) || !failure.Gap || failure.Message != "Order nulls options are not available with MySQL" {
					t.Fatalf("Parse error = %#v, want null-order feature gap", err)
				}
			})
		}
	}
}
