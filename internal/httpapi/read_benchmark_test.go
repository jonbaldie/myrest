package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/jonbaldie/myrest/internal/readquery"
)

func BenchmarkParseOrdinaryReadQuery(b *testing.B) {
	request := httptest.NewRequest(
		"GET",
		"/items?select=id,name,details-%3E%3Ecategory,orders(id,total,status)&status=in.(open,pending)&price=gte.10&or=(stock.gt.0,featured.eq.true)&orders.order=total.desc&orders.limit=5&order=name.asc&limit=20&offset=10",
		nil,
	)
	request.Header.Set("Prefer", "count=exact")
	values := request.URL.Query()
	prefer := request.Header.Values("Prefer")

	b.ReportAllocs()
	for b.Loop() {
		query, err := readquery.Parse(values, prefer)
		if err != nil {
			b.Fatal(err)
		}
		if len(query.Columns) != 3 || len(query.Embeds) != 1 {
			b.Fatal("unexpected parsed query")
		}
	}
}
