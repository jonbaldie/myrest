package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// embed-001 / smoke-005: GET with a nested select over a declared FK succeeds.
func TestEmbedManyToOneOverDeclaredForeignKey(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/orders?select=id,items(id,name)&id=eq.1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"items":{"id":1,"name":"alpha"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-001: one-to-many embed nests an array.
func TestEmbedOneToManyOverDeclaredForeignKey(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,name,orders(id)&id=eq.1&orders.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"name":"alpha","orders":[{"id":1},{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-002: nested filter, order, and limit succeed.
func TestEmbedNestedFilterOrderAndLimit(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,orders(id)&id=eq.1&orders.order=id.desc&orders.limit=1&orders.id=gt.1",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"orders":[{"id":2}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// Many-to-many embed through a declared join table succeeds.
func TestEmbedManyToManyThroughJoinTable(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,tags(id,name)&id=eq.1&tags.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"tags":[{"id":1,"name":"hot"},{"id":2,"name":"cold"}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// Disambiguation selects one relationship when more than one applies.
func TestEmbedDisambiguationSelectsOneRelationship(t *testing.T) {
	service := serve(t, "myrest_fixture")

	response, body := get(t, service, "/deliveries?select=id,addresses(label)")
	failure := apitest.AssertEnvelope(t, response, body, http.StatusMultipleChoices, "PGRST201")
	if !strings.Contains(failure.Message, "more than one relationship") {
		t.Fatalf("message = %q", failure.Message)
	}

	response, body = get(
		t,
		service,
		"/deliveries?select=id,from_addr:addresses!deliveries_from(label)",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":1,"from_addr":{"label":"from-here"}}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// embed-001: nested embed over declared FKs succeeds.
func TestNestedEmbedOverDeclaredForeignKeys(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,orders(id,items(name))&id=eq.2&orders.order=id.asc",
	)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
	}
	want := `[{"id":2,"orders":[{"id":3,"items":{"name":"beta"}}]}]`
	if string(body) != want+"\n" {
		t.Fatalf("body = %s, want %s", body, want)
	}
}

// View-chain embed with no declared FK refuses like any missing path.
func TestEmbedThroughViewWithoutForeignKeyRefuses(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,items_view(id)",
	)
	apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "PGRST200")
}

// embed-004: a computed relationship embed refuses stably.
func TestComputedRelationshipEmbedRefuses(t *testing.T) {
	response, body := get(
		t,
		serve(t, "myrest_fixture"),
		"/items?select=id,item_count(*)",
	)
	failure := apitest.AssertEnvelope(t, response, body, http.StatusBadRequest, "MYREST001")
	if !strings.Contains(strings.ToLower(failure.Message), "computed relationship") {
		t.Fatalf("message = %q, want a computed relationship refusal", failure.Message)
	}
}
