package httpapi_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/httpapi"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Seam under test: the HTTP API boundary for Accept-Profile and
// Content-Profile (repr-002). The reader and writer answer in memory.

// multiSchemaCache holds the same table name in two configured databases.
func multiSchemaCache() *schemacache.Cache {
	shopItems := schemacache.TableID{Database: "shop", Name: "items"}
	warehouseItems := schemacache.TableID{Database: "warehouse", Name: "items"}

	return schemacache.Build(schemacache.Catalog{
		Tables: []schemacache.TableID{shopItems, warehouseItems},
		Columns: []schemacache.ColumnFact{
			{Table: shopItems, Name: "id"},
			{Table: shopItems, Name: "name"},
			{Table: warehouseItems, Name: "id"},
			{Table: warehouseItems, Name: "sku"},
		},
		Selects: []schemacache.SelectFact{
			{Role: "myrest_anon", Table: shopItems},
			{Role: "myrest_anon", Table: warehouseItems},
		},
		TablePrivileges: []schemacache.TablePrivilegeFact{
			{Role: "myrest_anon", Table: shopItems, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: shopItems, Privilege: "INSERT"},
			{Role: "myrest_anon", Table: warehouseItems, Privilege: "SELECT"},
			{Role: "myrest_anon", Table: warehouseItems, Privilege: "INSERT"},
		},
	})
}

func multiSchemaSettings() config.Settings {
	resolved := settings()
	resolved.DB.Schemas = []string{"shop", "warehouse"}
	return resolved
}

func serveProfiles(t *testing.T, source httpapi.Reader, sink httpapi.Writer) *httpapi.Service {
	t.Helper()

	service, err := httpapi.Listen(httpapi.Options{
		Addr:     "127.0.0.1:0",
		Settings: multiSchemaSettings(),
		Cache:    multiSchemaCache(),
		Reader:   source,
		Writer:   sink,
	})
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = service.Serve() }()
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("close service: %v", err)
		}
	})
	return service
}

// repr-002: Accept-Profile selects a second configured database for a read.
func TestAcceptProfileSelectsAConfiguredDatabase(t *testing.T) {
	t.Parallel()

	source := &reader{}
	headers := http.Header{}
	headers.Set("Accept-Profile", "warehouse")
	response, body := apitest.Do(
		t,
		http.MethodGet,
		serveProfiles(t, source, nil).URL()+"/items",
		headers,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := (schemacache.TableID{Database: "warehouse", Name: "items"}); source.table.ID != want {
		t.Fatalf("read table %v, want %v", source.table.ID, want)
	}
}

// repr-002: a profile outside db-schemas refuses in the PostgREST shape.
func TestAcceptProfileOutsideDbSchemasRefuses(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("Accept-Profile", "tenant3")
	response, body := apitest.Do(
		t,
		http.MethodGet,
		serveProfiles(t, &reader{}, nil).URL()+"/items",
		headers,
	)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST106")
	if want := "The schema must be one of the following: shop, warehouse"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// With no profile header the read still goes to the default database.
func TestReadWithoutProfileUsesTheDefaultDatabase(t *testing.T) {
	t.Parallel()

	source := &reader{}
	response, body := get(t, serveProfiles(t, source, nil), "/items")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := (schemacache.TableID{Database: "shop", Name: "items"}); source.table.ID != want {
		t.Fatalf("read table %v, want %v", source.table.ID, want)
	}
}

// Content-Profile on a GET does not select the database; Accept-Profile does.
func TestContentProfileDoesNotSelectTheDatabaseForARead(t *testing.T) {
	t.Parallel()

	source := &reader{}
	headers := http.Header{}
	headers.Set("Content-Profile", "warehouse")
	response, body := apitest.Do(
		t,
		http.MethodGet,
		serveProfiles(t, source, nil).URL()+"/items",
		headers,
	)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := (schemacache.TableID{Database: "shop", Name: "items"}); source.table.ID != want {
		t.Fatalf("read table %v, want %v", source.table.ID, want)
	}
}

// repr-002: Content-Profile selects a second configured database for a write.
func TestContentProfileSelectsAConfiguredDatabase(t *testing.T) {
	t.Parallel()

	sink := &writer{}
	request, err := http.NewRequest(
		http.MethodPost,
		serveProfiles(t, &reader{}, sink).URL()+"/items",
		strings.NewReader(`{"sku":"W1"}`),
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Profile", "warehouse")
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = answer.Body.Close() })
	payload, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if answer.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", answer.StatusCode, http.StatusCreated, payload)
	}
	if want := (schemacache.TableID{Database: "warehouse", Name: "items"}); sink.table.ID != want {
		t.Fatalf("write table %v, want %v", sink.table.ID, want)
	}
}

// repr-002: Content-Profile outside db-schemas refuses in the PostgREST shape.
func TestContentProfileOutsideDbSchemasRefuses(t *testing.T) {
	t.Parallel()

	request, err := http.NewRequest(
		http.MethodPost,
		serveProfiles(t, &reader{}, &writer{}).URL()+"/items",
		strings.NewReader(`{"name":"x"}`),
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Profile", "tenant3")
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = answer.Body.Close() })
	payload, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	failure := apitest.AssertEnvelope(t, answer, payload, http.StatusNotAcceptable, "PGRST106")
	if want := "The schema must be one of the following: shop, warehouse"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}
