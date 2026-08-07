package acceptance_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// write-001: a POST of one object and a POST of a JSON array both insert rows.
func TestPostInsertsSingleAndBulkOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := apitest.PostJSON(t, service.URL()+"/items", `{"name":"gamma"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("single POST status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if len(body) != 0 {
		t.Fatalf("single POST body = %s, want empty", body)
	}

	response, body = apitest.PostJSON(
		t, service.URL()+"/items", `[{"name":"delta"},{"name":"epsilon"}]`,
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("bulk POST status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}

	response, body = get(t, service, "/items?select=name&name=in.(gamma,delta,epsilon)&order=name.asc")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", response.StatusCode, body)
	}
	want := `[{"name":"delta"},{"name":"epsilon"},{"name":"gamma"}]`
	if string(body) != want+"\n" {
		t.Fatalf("read-back body = %s, want %s", body, want)
	}
}

// write-002: a PATCH with a filter updates only the matching rows.
func TestPatchUpdatesMatchingRowsOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"patch-me"}`)

	request, err := http.NewRequest(
		http.MethodPatch,
		service.URL()+"/items?name=eq.patch-me",
		strings.NewReader(`{"name":"patched"}`),
	)
	if err != nil {
		t.Fatalf("new PATCH: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}

	_, body = get(t, service, "/items?select=name&name=eq.patched")
	if string(body) != `[{"name":"patched"}]`+"\n" {
		t.Fatalf("after PATCH body = %s", body)
	}
	_, body = get(t, service, "/items?select=name&name=eq.patch-me")
	if string(body) != `[]`+"\n" {
		t.Fatalf("unmatched row changed: %s", body)
	}
}

// write-003: a DELETE with a filter removes only the matching rows.
func TestDeleteRemovesMatchingRowsOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"delete-me"}`)
	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"keep-me"}`)

	response, body := apitest.Do(
		t, http.MethodDelete, service.URL()+"/items?name=eq.delete-me", nil,
	)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}

	_, body = get(t, service, "/items?select=name&name=eq.delete-me")
	if string(body) != `[]`+"\n" {
		t.Fatalf("deleted row still present: %s", body)
	}
	_, body = get(t, service, "/items?select=name&name=eq.keep-me")
	if string(body) != `[{"name":"keep-me"}]`+"\n" {
		t.Fatalf("kept row missing: %s", body)
	}
}

// write-005: a PATCH or DELETE with no filter and no Prefer: all-rows refuses.
func TestUnboundedWriteRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	request, err := http.NewRequest(
		http.MethodPatch,
		service.URL()+"/items",
		strings.NewReader(`{"name":"nope"}`),
	)
	if err != nil {
		t.Fatalf("new PATCH: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")

	response, body = apitest.Do(t, http.MethodDelete, service.URL()+"/items", nil)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST100")
}

// A write without the matching grant is denied.
func TestWriteWithoutGrantDeniedOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	// secrets has no INSERT for the anonymous role.
	response, body := apitest.PostJSON(t, service.URL()+"/secrets", `{"payload":"nope"}`)
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}
