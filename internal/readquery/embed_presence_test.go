package readquery_test

import (
	"errors"
	"net/url"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func TestEmbedPresenceFiltersRefuse(t *testing.T) {
	for _, raw := range []string{
		"select=id,orders(id)&orders=not.is.null",
		"select=id,orders(id)&orders=is.null",
		"select=id,purchases:orders(id)&purchases=not.is.null",
		"select=id,orders(id,items(id))&orders.items=is.null",
		"select=id,orders(id)&or=(id.eq.1,orders.not.is.null)",
		"select=id,orders(id)&and=(or(orders.is.null,id.eq.1))",
		"select=id,orders(id,items(id))&orders.or=(items.is.null,id.eq.1)",
	} {
		t.Run(raw, func(t *testing.T) {
			values, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatal(err)
			}
			_, err = readquery.Parse(values, readquery.CountNone)
			var failure readquery.ParseFailure
			if !errors.As(err, &failure) || !failure.Gap || failure.Message != "PostgREST embed presence filters are not supported" {
				t.Fatalf("Parse error = %v, want embed presence gap", err)
			}
		})
	}
}

func TestColumnNullFiltersStillParse(t *testing.T) {
	for _, raw := range []string{
		"select=id,orders(id)&id=not.is.null",
		"select=id,orders(id)&orders.id=is.null",
		"select=id,orders(id)&unknown=is.null",
		"select=id,purchases:orders(id)&orders=is.null",
		"select=id,orders(id)&orders=eq.null",
	} {
		t.Run(raw, func(t *testing.T) {
			values, err := url.ParseQuery(raw)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readquery.Parse(values, readquery.CountNone); err != nil {
				t.Fatal(err)
			}
		})
	}
}
