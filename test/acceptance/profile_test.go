package acceptance_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// repr-002: Accept-Profile selects a second configured database for a read.
func TestAcceptProfileSelectsAConfiguredDatabaseOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture", "myrest_hidden")

	headers := http.Header{}
	headers.Set("Accept-Profile", "myrest_hidden")
	response, body := apitest.Do(t, http.MethodGet, service.URL()+"/outside_items", headers)

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	if want := `[{"id":1,"name":"hidden"}]`; string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// repr-002: a profile outside db-schemas refuses in the PostgREST shape.
func TestAcceptProfileOutsideDbSchemasRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture", "myrest_hidden")

	headers := http.Header{}
	headers.Set("Accept-Profile", "tenant3")
	response, body := apitest.Do(t, http.MethodGet, service.URL()+"/items", headers)

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST106")
	if want := "The schema must be one of the following: myrest_fixture, myrest_hidden"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}

// The default database, when the client sends no profile header, is the first
// of db-schemas.
func TestReadWithoutProfileUsesTheDefaultDatabaseOverMySQL(t *testing.T) {
	response, body := get(t, serve(t, "myrest_fixture", "myrest_hidden"), "/outside_items")

	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}

// repr-002: Content-Profile selects a second configured database for a write,
// and a later Accept-Profile read of that database sees the row.
func TestContentProfileSelectsAConfiguredDatabaseOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture", "myrest_hidden")

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/outside_items",
		strings.NewReader(`{"name":"profile-write"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Profile", "myrest_hidden")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}

	headers := http.Header{}
	headers.Set("Accept-Profile", "myrest_hidden")
	response, body = apitest.Do(
		t,
		http.MethodGet,
		service.URL()+"/outside_items?select=name&name=eq.profile-write",
		headers,
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", response.StatusCode, body)
	}
	if want := `[{"name":"profile-write"}]`; string(body) != want+"\n" {
		t.Fatalf("read-back body = %s, want %s", body, want)
	}
}

// repr-002: Content-Profile outside db-schemas refuses in the PostgREST shape.
func TestContentProfileOutsideDbSchemasRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture", "myrest_hidden")

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/items",
		strings.NewReader(`{"name":"x"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Profile", "tenant3")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	failure := apitest.AssertEnvelope(t, response, body, http.StatusNotAcceptable, "PGRST106")
	if want := "The schema must be one of the following: myrest_fixture, myrest_hidden"; failure.Message != want {
		t.Fatalf("message = %q, want %q", failure.Message, want)
	}
}
