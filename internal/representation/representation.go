// Package representation owns the response representation of row data: the
// Accept negotiation that picks it, the singular-object cardinality check, and
// the JSON and CSV body formats. It knows nothing of HTTP responses; callers
// map its typed refusals to status codes and error bodies.
package representation

import (
	"fmt"
	"strings"
)

// Claimed response media types for row data.
const (
	MediaJSON       = "application/json"
	MediaArrayJSON  = "application/vnd.pgrst.array+json"
	MediaObjectJSON = "application/vnd.pgrst.object+json"
	MediaCSV        = "text/csv"
	MediaCSVCharset = "text/csv; charset=utf-8"
)

// Kind is the claimed response shape for row data.
type Kind int

const (
	// KindJSONArray is a JSON array of row objects. It is the zero Kind.
	KindJSONArray Kind = iota
	// KindSingularObject is one JSON row object: the result must hold one row.
	KindSingularObject
	// KindCSV is RFC 4180 CSV with a header record.
	KindCSV
)

// Spec is one negotiated representation: its kind and the response
// Content-Type. The zero Spec is the JSON array with no Content-Type.
type Spec struct {
	Kind        Kind
	ContentType string
}

// Default is the JSON array representation of an empty Accept header.
func Default() Spec {
	return Spec{Kind: KindJSONArray, ContentType: MediaJSON}
}

// RowOnly says the representation needs a row set: a scalar or an object
// result cannot be written as a singular row object or as CSV.
func (s Spec) RowOnly() bool {
	return s.Kind != KindJSONArray
}

// UnsupportedMedia says no offered Accept media type is claimed. Its message,
// like the SingularObjectRefusal message, is the PostgREST wire text, so it
// starts with a capital letter.
type UnsupportedMedia struct {
	Offered []string
}

func (e UnsupportedMedia) Error() string {
	return "None of these media types are available: " + strings.Join(e.Offered, ", ")
}

// Refuse is the UnsupportedMedia refusal of the Accept headers: it lists every
// offered media type, also the claimed ones.
func Refuse(acceptHeaders []string) UnsupportedMedia {
	var offered []string
	for _, header := range acceptHeaders {
		for _, part := range strings.Split(header, ",") {
			if mediaType := mediaTypeOf(part); mediaType != "" {
				offered = append(offered, mediaType)
			}
		}
	}
	return UnsupportedMedia{Offered: offered}
}

// SingularObjectRefusal says a singular-object representation claimed one
// row, but the result held RowCount rows.
type SingularObjectRefusal struct {
	RowCount int
}

func (e SingularObjectRefusal) Error() string {
	return fmt.Sprintf("The result contains %d rows, while 1 was expected", e.RowCount)
}

// ValidateCardinality checks a result row count against the representation.
// Only the singular object constrains the count: it needs exactly one row.
func ValidateCardinality(spec Spec, rowCount int) error {
	if spec.Kind == KindSingularObject && rowCount != 1 {
		return SingularObjectRefusal{RowCount: rowCount}
	}
	return nil
}
