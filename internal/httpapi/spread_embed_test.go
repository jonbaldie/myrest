package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/rows"
)

// embed-006: ordinary spread embeds refuse with MYREST001 and a clean user-facing message.
func TestSpreadEmbedRefusesAtHTTP(t *testing.T) {
	t.Parallel()

	t.Run("many-to-one ordinary spread", func(t *testing.T) {
		t.Parallel()
		source := &reader{}
		response, body := get(t, serveEmbed(t, source), "/orders?select=id,...items(name)")
		failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
		wantMessage := "PostgREST spread embeds are not available with MySQL"
		if failure.Message != wantMessage {
			t.Fatalf("message = %q, want %q", failure.Message, wantMessage)
		}
		if source.table.ID.Name != "" {
			t.Fatalf("reader was called for unsupported spread: %#v", source.table.ID)
		}
	})

	t.Run("one-to-many ordinary spread without aggregates", func(t *testing.T) {
		t.Parallel()
		source := &reader{}
		response, body := get(t, serveEmbed(t, source), "/items?select=id,...orders(id)")
		failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
		wantMessage := "PostgREST spread embeds are not available with MySQL"
		if failure.Message != wantMessage {
			t.Fatalf("message = %q, want %q", failure.Message, wantMessage)
		}
		if source.table.ID.Name != "" {
			t.Fatalf("reader was called for unsupported spread: %#v", source.table.ID)
		}
	})

	t.Run("many-to-many ordinary spread without aggregates", func(t *testing.T) {
		t.Parallel()
		source := &reader{}
		response, body := get(t, serveManyToMany(t, source), "/items?select=id,...tags(name)")
		failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
		wantMessage := "PostgREST spread embeds are not available with MySQL"
		if failure.Message != wantMessage {
			t.Fatalf("message = %q, want %q", failure.Message, wantMessage)
		}
		if source.table.ID.Name != "" {
			t.Fatalf("reader was called for unsupported spread: %#v", source.table.ID)
		}
	})
}

func TestNormalEmbedsStillSucceedAtHTTP(t *testing.T) {
	t.Parallel()

	source := &multiReader{answers: []readAnswer{
		{rows: []rows.Row{{Columns: []string{"id", "item_id"}, Values: []any{int64(1), int64(1)}}}},
		{rows: []rows.Row{{Columns: []string{"id", "name"}, Values: []any{int64(1), "alpha"}}}},
	}}
	response, body := get(t, serveEmbed(t, source), "/orders?select=id,items(id,name)&id=eq.1")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.StatusCode, body)
	}
	want := `[{"id":1,"items":{"id":1,"name":"alpha"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

func TestAggregateSpreadRefusalPreservedAtHTTP(t *testing.T) {
	t.Parallel()

	resolved := aggregatesOn()
	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: resolved,
		Cache:    embedCache(),
		Reader:   &reader{},
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() { _ = service.Close() })

	t.Run("one-to-many spread with aggregate refuses with PGRST127", func(t *testing.T) {
		t.Parallel()
		response, body := get(t, service, "/items?select=id,...orders(count())")
		failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST127")
		if failure.Message != "Feature not implemented" {
			t.Fatalf("message = %q, want %q", failure.Message, "Feature not implemented")
		}
		if failure.Details != "Aggregates are not implemented for one-to-many or many-to-many spreads." {
			t.Fatalf("details = %#v", failure.Details)
		}
	})

	t.Run("many-to-many spread with aggregate refuses with PGRST127", func(t *testing.T) {
		t.Parallel()
		m2mService := serveManyToMany(t, &reader{})
		response, body := get(t, m2mService, "/items?select=id,...tags(count())")
		failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST127")
		if failure.Message != "Feature not implemented" {
			t.Fatalf("message = %q, want %q", failure.Message, "Feature not implemented")
		}
		if failure.Details != "Aggregates are not implemented for one-to-many or many-to-many spreads." {
			t.Fatalf("details = %#v", failure.Details)
		}
	})

	t.Run("many-to-one spread with aggregate refuses with MYREST001", func(t *testing.T) {
		t.Parallel()
		response, body := get(t, service, "/orders?select=id,...items(count())")
		failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
		wantMessage := "PostgREST spread embeds are not available with MySQL"
		if failure.Message != wantMessage {
			t.Fatalf("message = %q, want %q", failure.Message, wantMessage)
		}
	})
}
