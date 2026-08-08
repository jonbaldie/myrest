package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

// write-004: a PUT by primary key with resolution preference succeeds.
func TestPutUpsertByPrimaryKeyOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	// Insert a new primary key with merge-duplicates.
	request, err := http.NewRequest(
		http.MethodPut,
		service.URL()+"/items?id=eq.100",
		strings.NewReader(`{"id":100,"name":"upsert-new"}`),
	)
	if err != nil {
		t.Fatalf("new PUT: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "resolution=merge-duplicates")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT insert: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("PUT insert status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}

	// Update the same primary key with merge-duplicates.
	request, err = http.NewRequest(
		http.MethodPut,
		service.URL()+"/items?id=eq.100",
		strings.NewReader(`{"id":100,"name":"upsert-merged"}`),
	)
	if err != nil {
		t.Fatalf("new PUT merge: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "resolution=merge-duplicates")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT merge: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT merge status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}

	_, body = get(t, service, "/items?select=name&id=eq.100")
	if string(body) != `[{"name":"upsert-merged"}]`+"\n" {
		t.Fatalf("after merge body = %s", body)
	}

	// ignore-duplicates leaves the existing row alone.
	request, err = http.NewRequest(
		http.MethodPut,
		service.URL()+"/items?id=eq.100",
		strings.NewReader(`{"id":100,"name":"should-not-win"}`),
	)
	if err != nil {
		t.Fatalf("new PUT ignore: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "resolution=ignore-duplicates")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT ignore: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("PUT ignore status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
	}
	_, body = get(t, service, "/items?select=name&id=eq.100")
	if string(body) != `[{"name":"upsert-merged"}]`+"\n" {
		t.Fatalf("after ignore body = %s", body)
	}
}

// A PUT that does not target the primary key refuses stably.
func TestPutWithoutPrimaryKeyTargetRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	request, err := http.NewRequest(
		http.MethodPut,
		service.URL()+"/items?name=eq.alpha",
		strings.NewReader(`{"id":1,"name":"alpha"}`),
	)
	if err != nil {
		t.Fatalf("new PUT: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "resolution=merge-duplicates")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST105")
}

// write-006: a write through a writable view succeeds; a write through a
// non-updatable view refuses stably.
func TestWriteThroughViewOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := apitest.PostJSON(
		t, service.URL()+"/items_view", `{"name":"via-view"}`,
	)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("writable view POST status = %d, want %d; body = %s",
			response.StatusCode, http.StatusCreated, body)
	}

	_, body = get(t, service, "/items_view?select=name&name=eq.via-view")
	if string(body) != `[{"name":"via-view"}]`+"\n" {
		t.Fatalf("read-back through view = %s", body)
	}

	response, body = apitest.PostJSON(
		t, service.URL()+"/items_stats", `{"total":1}`,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// A write without the matching grant is denied.
func TestWriteWithoutGrantDeniedOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	// secrets has no INSERT for the anonymous role.
	response, body := apitest.PostJSON(t, service.URL()+"/secrets", `{"payload":"nope"}`)
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")

	// orders is readable and insertable, but has no UPDATE grant.
	request, err := http.NewRequest(
		http.MethodPatch,
		service.URL()+"/orders?id=eq.1",
		strings.NewReader(`{"item_id":2}`),
	)
	if err != nil {
		t.Fatalf("new PATCH: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")

	// secrets has no INSERT for the anonymous role: PUT is denied the same way.
	request, err = http.NewRequest(
		http.MethodPut,
		service.URL()+"/secrets?id=eq.1",
		strings.NewReader(`{"id":1,"payload":"nope"}`),
	)
	if err != nil {
		t.Fatalf("new PUT: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "resolution=merge-duplicates")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusNotFound, "PGRST205")
}

// write-007: return=minimal and return=headers-only behave as the parity target.
func TestPreferReturnMinimalAndHeadersOnlyOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/items",
		strings.NewReader(`{"name":"minimal-row"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=minimal")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST minimal: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated || len(body) != 0 {
		t.Fatalf("minimal status=%d body=%s", response.StatusCode, body)
	}
	if got := response.Header.Get("Preference-Applied"); got != "return=minimal" {
		t.Fatalf("Preference-Applied = %q", got)
	}

	request, err = http.NewRequest(
		http.MethodPost,
		service.URL()+"/items",
		strings.NewReader(`{"name":"headers-row"}`),
	)
	if err != nil {
		t.Fatalf("new POST headers: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=headers-only")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST headers-only: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated || len(body) != 0 {
		t.Fatalf("headers-only status=%d body=%s", response.StatusCode, body)
	}
	location := response.Header.Get("Location")
	if !strings.HasPrefix(location, "/items?id=eq.") {
		t.Fatalf("Location = %q", location)
	}
	if got := response.Header.Get("Preference-Applied"); got != "return=headers-only" {
		t.Fatalf("Preference-Applied = %q", got)
	}
}

// write-008 and smoke-003: return=representation returns an honest body.
func TestPreferReturnRepresentationOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/items",
		strings.NewReader(`{"name":"represented"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
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
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), `"name":"represented"`) {
		t.Fatalf("body = %s", body)
	}
	if got := response.Header.Get("Preference-Applied"); got != "return=representation" {
		t.Fatalf("Preference-Applied = %q", got)
	}

	// PATCH representation re-reads by primary key after the update.
	request, err = http.NewRequest(
		http.MethodPatch,
		service.URL()+"/items?name=eq.represented",
		strings.NewReader(`{"name":"represented-2"}`),
	)
	if err != nil {
		t.Fatalf("new PATCH: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status = %d; body = %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), `"name":"represented-2"`) {
		t.Fatalf("PATCH body = %s", body)
	}
}

// write-011: return=representation with a nested select over a cache
// relationship nests the related rows.
func TestPreferReturnRepresentationWithEmbedOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/orders?select=id,items(id,name)",
		strings.NewReader(`{"item_id":1}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
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
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), `"items":{"id":1,"name":"alpha"}`) {
		t.Fatalf("body = %s, want nested items", body)
	}
}

// Nested filter and order on a write representation follow embed read rules.
func TestPreferReturnRepresentationEmbedNestedFilterOrderOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	// Own item and orders so package-shared fixture writes cannot change the nest.
	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/items?select=id",
		strings.NewReader(`{"name":"embed-order-owner"}`),
	)
	if err != nil {
		t.Fatalf("new item POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("item POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read item body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("item status = %d; body = %s", response.StatusCode, body)
	}
	itemID := jsonNumberField(t, body, "id")

	var orderIDs []string
	for range 3 {
		request, err = http.NewRequest(
			http.MethodPost,
			service.URL()+"/orders?select=id",
			strings.NewReader(`{"item_id":`+itemID+`}`),
		)
		if err != nil {
			t.Fatalf("new order POST: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Prefer", "return=representation")
		response, err = http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("order POST: %v", err)
		}
		t.Cleanup(func() { _ = response.Body.Close() })
		body, err = io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("read order body: %v", err)
		}
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("order status = %d; body = %s", response.StatusCode, body)
		}
		orderIDs = append(orderIDs, jsonNumberField(t, body, "id"))
	}

	request, err = http.NewRequest(
		http.MethodPatch,
		service.URL()+"/items?id=eq."+itemID+
			"&select=id,orders(id)&orders.order=id.desc&orders.limit=1&orders.id=gt."+orderIDs[0],
		strings.NewReader(`{"name":"embed-order-owner"}`),
	)
	if err != nil {
		t.Fatalf("new PATCH: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	// id=gt.first keeps the second and third orders; desc + limit 1 keeps the third.
	want := `[{"id":` + itemID + `,"orders":[{"id":` + orderIDs[2] + `}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// write-012: return=representation with a nested select and no cache
// relationship refuses stably.
func TestPreferReturnRepresentationEmbedWithoutRelationshipRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/items?select=id,profiles(id)",
		strings.NewReader(`{"name":"no-embed"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST200")

	response, body = get(t, service, "/items?select=name&name=eq.no-embed")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read-back status = %d; body = %s", response.StatusCode, body)
	}
	if string(body) != "[]\n" {
		t.Fatalf("read-back body = %s, want no inserted row", body)
	}
}

// write-009: return=representation refuses when an honest body is not available.
func TestPreferReturnRepresentationWithoutPrimaryKeyRefusesOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/loose_notes",
		strings.NewReader(`{"body":"note"}`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
}

// write-010: missing=default, max-affected, and handling preferences.
func TestPreferMissingMaxAffectedAndHandlingOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	// missing=default: bulk POST where only some rows name tone. Without the
	// Prefer, the omitted tone becomes NULL and NOT NULL fails; with it, SQL
	// DEFAULT fills plain.
	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/colors",
		strings.NewReader(`[{"name":"crimson"},{"name":"azure","tone":"bright"}]`),
	)
	if err != nil {
		t.Fatalf("new POST: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "missing=default, return=representation")
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
		t.Fatalf("status = %d; body = %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), `"tone":"plain"`) || !strings.Contains(string(body), `"tone":"bright"`) {
		t.Fatalf("body = %s, want plain default and bright", body)
	}

	// Seed rows for max-affected.
	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"max-a"}`)
	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"max-b"}`)
	_, _ = apitest.PostJSON(t, service.URL()+"/items", `{"name":"max-c"}`)

	headers := http.Header{}
	headers.Set("Prefer", "handling=strict, max-affected=1")
	response, body = apitest.Do(
		t,
		http.MethodDelete,
		service.URL()+"/items?name=like.max-*",
		headers,
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST124")

	// Rows must still exist after the refused delete.
	_, body = get(t, service, "/items?select=name&name=like.max-*&order=name.asc")
	if !strings.Contains(string(body), "max-a") || !strings.Contains(string(body), "max-c") {
		t.Fatalf("rows changed after max-affected refuse: %s", body)
	}

	// handling=strict rejects unknown tokens.
	request, err = http.NewRequest(
		http.MethodPost,
		service.URL()+"/items",
		strings.NewReader(`{"name":"nope"}`),
	)
	if err != nil {
		t.Fatalf("new POST strict: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "handling=strict, not-a-real-prefer")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST strict: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST122")
}

func jsonNumberField(t *testing.T, body []byte, field string) string {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("decode body: %v; body = %s", err, body)
	}
	if len(rows) != 1 {
		t.Fatalf("body rows = %d; body = %s", len(rows), body)
	}
	value, ok := rows[0][field]
	if !ok {
		t.Fatalf("missing %s in %s", field, body)
	}
	switch typed := value.(type) {
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return fmt.Sprint(typed)
	}
}
