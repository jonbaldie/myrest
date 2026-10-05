package representation_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jonbaldie/myrest/internal/representation"
)

// Seam under test: Negotiate, the Accept negotiation for row data.

func TestNegotiateClaimsJSONArrayByDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		accept []string
		want   representation.Spec
	}{
		{accept: nil, want: representation.Spec{Kind: representation.KindJSONArray, ContentType: "application/json"}},
		{accept: []string{""}, want: representation.Spec{Kind: representation.KindJSONArray, ContentType: "application/json"}},
		{accept: []string{"application/json"}, want: representation.Spec{Kind: representation.KindJSONArray, ContentType: "application/json"}},
		{accept: []string{"*/*"}, want: representation.Spec{Kind: representation.KindJSONArray, ContentType: "application/json"}},
		{accept: []string{"application/vnd.pgrst.array+json"}, want: representation.Spec{Kind: representation.KindJSONArray, ContentType: "application/vnd.pgrst.array+json"}},
		{accept: []string{"application/vnd.pgrst.array"}, want: representation.Spec{Kind: representation.KindJSONArray, ContentType: "application/vnd.pgrst.array+json"}},
	}
	for _, tc := range cases {
		got, err := representation.Negotiate(tc.accept)
		if err != nil {
			t.Fatalf("Accept %q: err = %v", tc.accept, err)
		}
		if got != tc.want {
			t.Fatalf("Accept %q: spec = %+v, want %+v", tc.accept, got, tc.want)
		}
	}
}

func TestNegotiateClaimsSingularObjectAndCSV(t *testing.T) {
	t.Parallel()

	cases := []struct {
		accept string
		want   representation.Spec
	}{
		{accept: "application/vnd.pgrst.object+json", want: representation.Spec{Kind: representation.KindSingularObject, ContentType: "application/vnd.pgrst.object+json"}},
		{accept: "application/vnd.pgrst.object", want: representation.Spec{Kind: representation.KindSingularObject, ContentType: "application/vnd.pgrst.object+json"}},
		{accept: "Application/VND.pgrst.Object+JSON", want: representation.Spec{Kind: representation.KindSingularObject, ContentType: "application/vnd.pgrst.object+json"}},
		{accept: "text/csv", want: representation.Spec{Kind: representation.KindCSV, ContentType: "text/csv; charset=utf-8"}},
		{accept: " text/csv ; charset=utf-8", want: representation.Spec{Kind: representation.KindCSV, ContentType: "text/csv; charset=utf-8"}},
	}
	for _, tc := range cases {
		got, err := representation.Negotiate([]string{tc.accept})
		if err != nil {
			t.Fatalf("Accept %q: err = %v", tc.accept, err)
		}
		if got != tc.want {
			t.Fatalf("Accept %q: spec = %+v, want %+v", tc.accept, got, tc.want)
		}
	}
}

func TestNegotiateSelectsHighestQuality(t *testing.T) {
	t.Parallel()

	cases := []struct {
		accept []string
		want   representation.Kind
	}{
		{accept: []string{"application/json;q=0.1, text/csv;q=1"}, want: representation.KindCSV},
		{accept: []string{"text/csv;q=0.5", "application/vnd.pgrst.object+json;Q=0.9"}, want: representation.KindSingularObject},
		// Equal quality keeps the first claimed type.
		{accept: []string{"text/csv, application/json"}, want: representation.KindCSV},
		// q=0 is not acceptable, so the unclaimed type does not matter.
		{accept: []string{"text/csv;q=0, application/json;q=0.2"}, want: representation.KindJSONArray},
		// An unclaimed type with higher quality does not win.
		{accept: []string{"text/html, text/csv;q=0.3"}, want: representation.KindCSV},
		// A parameter without a value and a non-q parameter are ignored.
		{accept: []string{"text/csv;foo;level=1;q=0.4, application/json;q=0.3"}, want: representation.KindCSV},
		{accept: []string{"text/csv;level=1;q=0.2, application/json;q=0.3"}, want: representation.KindJSONArray},
		// q=1 is the highest valid quality. Spaces round q and its value are allowed.
		{accept: []string{"application/json;q=0.9, text/csv;q=1"}, want: representation.KindCSV},
		{accept: []string{"application/json;q=0.5, text/csv; Q = 0.6 "}, want: representation.KindCSV},
		// A bad quality value makes the type not acceptable.
		{accept: []string{"text/csv;q=bad, application/json;q=0.1"}, want: representation.KindJSONArray},
		{accept: []string{"text/csv;q=2, application/json;q=0.1"}, want: representation.KindJSONArray},
		{accept: []string{"text/csv;q=-1, application/json;q=0.1"}, want: representation.KindJSONArray},
		{accept: []string{"text/csv;q=NaN, application/json;q=0.1"}, want: representation.KindJSONArray},
		// An empty member between commas is skipped.
		{accept: []string{" , text/csv"}, want: representation.KindCSV},
	}
	for _, tc := range cases {
		got, err := representation.Negotiate(tc.accept)
		if err != nil {
			t.Fatalf("Accept %q: err = %v", tc.accept, err)
		}
		if got.Kind != tc.want {
			t.Fatalf("Accept %q: kind = %v, want %v", tc.accept, got.Kind, tc.want)
		}
	}
}

func TestNegotiateRefusesUnclaimedMediaTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		accept  []string
		offered []string
		message string
	}{
		{
			accept:  []string{"text/html"},
			offered: []string{"text/html"},
			message: "None of these media types are available: text/html",
		},
		{
			accept:  []string{"Text/HTML;q=1, application/json;q=0", "image/png"},
			offered: []string{"text/html", "application/json", "image/png"},
			message: "None of these media types are available: text/html, application/json, image/png",
		},
	}
	for _, tc := range cases {
		_, err := representation.Negotiate(tc.accept)
		var refusal representation.UnsupportedMedia
		if !errors.As(err, &refusal) {
			t.Fatalf("Accept %q: err = %T %v, want UnsupportedMedia", tc.accept, err, err)
		}
		if !reflect.DeepEqual(refusal.Offered, tc.offered) {
			t.Fatalf("Accept %q: offered = %q, want %q", tc.accept, refusal.Offered, tc.offered)
		}
		if refusal.Error() != tc.message {
			t.Fatalf("Accept %q: message = %q, want %q", tc.accept, refusal.Error(), tc.message)
		}
	}
}

func TestRefuseListsEveryOfferedMediaType(t *testing.T) {
	t.Parallel()

	refusal := representation.Refuse([]string{"text/csv;q=1, , Application/JSON", "*/*"})
	want := []string{"text/csv", "application/json", "*/*"}
	if !reflect.DeepEqual(refusal.Offered, want) {
		t.Fatalf("offered = %q, want %q", refusal.Offered, want)
	}
}

// Every Accept value either claims a known representation or refuses.
func FuzzNegotiate(f *testing.F) {
	f.Add("application/json;q=NaN")
	f.Add("text/csv;q=0.5, application/vnd.pgrst.object+json")
	f.Fuzz(func(t *testing.T, accept string) {
		spec, err := representation.Negotiate([]string{accept})
		if err != nil {
			var refusal representation.UnsupportedMedia
			if !errors.As(err, &refusal) {
				t.Fatalf("Accept %q: err = %T %v", accept, err, err)
			}
			return
		}
		switch spec.ContentType {
		case representation.MediaJSON, representation.MediaArrayJSON,
			representation.MediaObjectJSON, representation.MediaCSVCharset:
		default:
			t.Fatalf("Accept %q: spec = %+v", accept, spec)
		}
	})
}
