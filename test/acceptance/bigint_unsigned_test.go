package acceptance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Issue #287: BIGINT UNSIGNED above max int64 must stay a JSON number.
func TestBigintUnsignedAboveMaxInt64StaysANumberOverMySQL(t *testing.T) {
	service := serve(t, "myrest_fixture")

	request, err := http.NewRequest(
		http.MethodPost,
		service.URL()+"/colors",
		strings.NewReader(`{"id":9223372036854775808,"name":"replay-287"}`),
	)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Prefer", "return=representation")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /colors: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
	}
	if !strings.Contains(string(body), `"id":9223372036854775808`) {
		t.Fatalf("representation = %s, want id as number", body)
	}
	if strings.Contains(string(body), `"id":"9223372036854775808"`) {
		t.Fatalf("representation = %s, want id as number", body)
	}
	t.Cleanup(func() {
		req, err := http.NewRequest(http.MethodDelete, service.URL()+"/colors?id=eq.9223372036854775808", nil)
		if err == nil {
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = res.Body.Close()
			}
		}
		// A large explicit id raises AUTO_INCREMENT past max int64.
		// LastInsertId cannot hold that value, so reset the counter.
		_ = harness.Exec("ALTER TABLE myrest_fixture.colors AUTO_INCREMENT = 1")
	})

	filteredResp, filteredBody := get(t, service, "/colors?select=id&name=eq.replay-287")
	if filteredResp.StatusCode != http.StatusOK {
		t.Fatalf("filtered status = %d, want %d; body = %s", filteredResp.StatusCode, http.StatusOK, filteredBody)
	}
	var filtered []map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(filteredBody)))
	decoder.UseNumber()
	if err := decoder.Decode(&filtered); err != nil {
		t.Fatalf("decode filtered body %s: %v", filteredBody, err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered rows = %d, want 1; body = %s", len(filtered), filteredBody)
	}
	if id, ok := filtered[0]["id"].(json.Number); !ok || id.String() != "9223372036854775808" {
		t.Fatalf("filtered id = %#v, want 9223372036854775808", filtered[0]["id"])
	}

	orderResp, orderBody := get(t, service, "/colors?select=id&order=id.desc&limit=1")
	if orderResp.StatusCode != http.StatusOK {
		t.Fatalf("ordered status = %d, want %d; body = %s", orderResp.StatusCode, http.StatusOK, orderBody)
	}
	var ordered []map[string]any
	orderDecoder := json.NewDecoder(strings.NewReader(string(orderBody)))
	orderDecoder.UseNumber()
	if err := orderDecoder.Decode(&ordered); err != nil {
		t.Fatalf("decode ordered body %s: %v", orderBody, err)
	}
	if len(ordered) != 1 {
		t.Fatalf("ordered rows = %d, want 1; body = %s", len(ordered), orderBody)
	}
	if id, ok := ordered[0]["id"].(json.Number); !ok || id.String() != "9223372036854775808" {
		t.Fatalf("ordered id = %#v, want 9223372036854775808", ordered[0]["id"])
	}
}
