package acceptance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/apitest"
)

// Issue #197: JSON numbers must keep precision across write bodies and POST /rpc.
func TestWriteAndRPCJSONNumberPrecision(t *testing.T) {
	service := serve(t, "myrest_fixture")
	t.Cleanup(func() {
		req, _ := http.NewRequest(http.MethodDelete, service.URL()+"/items?id=gte.9007199254740990", nil)
		res, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = res.Body.Close()
		}
		req2, _ := http.NewRequest(http.MethodDelete, service.URL()+"/measurements?id=gte.1", nil)
		res2, err := http.DefaultClient.Do(req2)
		if err == nil {
			_ = res2.Body.Close()
		}
	})

	t.Run("POST items with large integer ID", func(t *testing.T) {
		request, err := http.NewRequest(
			http.MethodPost,
			service.URL()+"/items",
			strings.NewReader(`{"id":9007199254740993,"name":"big"}`),
		)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Prefer", "return=representation")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("POST /items: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
		}

		location := response.Header.Get("Location")
		if want := "/items?id=eq.9007199254740993"; location != want {
			t.Fatalf("Location = %q, want %q", location, want)
		}

		var created []map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.UseNumber()
		if err := decoder.Decode(&created); err != nil {
			t.Fatalf("decode response body %s: %v", body, err)
		}
		if len(created) != 1 {
			t.Fatalf("len(created) = %d, want 1", len(created))
		}
		if id, ok := created[0]["id"].(json.Number); !ok || id.String() != "9007199254740993" {
			t.Fatalf("representation id = %#v, want 9007199254740993", created[0]["id"])
		}

		// Read back using the ID sent by the client.
		readResp, readBody := get(t, service, "/items?id=eq.9007199254740993")
		if readResp.StatusCode != http.StatusOK {
			t.Fatalf("read-back status = %d; body = %s", readResp.StatusCode, readBody)
		}
		var rows []map[string]any
		readDecoder := json.NewDecoder(strings.NewReader(string(readBody)))
		readDecoder.UseNumber()
		if err := readDecoder.Decode(&rows); err != nil {
			t.Fatalf("decode read-back body %s: %v", readBody, err)
		}
		if len(rows) != 1 {
			t.Fatalf("read-back rows = %d, want 1; body = %s", len(rows), readBody)
		}
	})

	t.Run("PUT items with large integer ID", func(t *testing.T) {
		request, err := http.NewRequest(
			http.MethodPut,
			service.URL()+"/items?id=eq.9007199254740995",
			strings.NewReader(`{"id":9007199254740995,"name":"put-big"}`),
		)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("PUT /items: %v", err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want 201 or 204", response.StatusCode)
		}

		readResp, readBody := get(t, service, "/items?id=eq.9007199254740995")
		if readResp.StatusCode != http.StatusOK {
			t.Fatalf("read-back status = %d; body = %s", readResp.StatusCode, readBody)
		}
		var rows []map[string]any
		readDecoder := json.NewDecoder(strings.NewReader(string(readBody)))
		readDecoder.UseNumber()
		if err := readDecoder.Decode(&rows); err != nil {
			t.Fatalf("decode read-back body %s: %v", readBody, err)
		}
		if len(rows) != 1 {
			t.Fatalf("read-back rows = %d, want 1; body = %s", len(rows), readBody)
		}
		if id, ok := rows[0]["id"].(json.Number); !ok || id.String() != "9007199254740995" {
			t.Fatalf("read-back id = %#v, want 9007199254740995", rows[0]["id"])
		}
	})

	t.Run("PATCH items with large integer ID", func(t *testing.T) {
		postResp, _ := apitest.PostJSON(t, service.URL()+"/items", `{"name":"patch-target"}`)
		if postResp.StatusCode != http.StatusCreated {
			t.Fatalf("setup post status = %d", postResp.StatusCode)
		}

		response, body := writeJSON(
			t,
			http.MethodPatch,
			service.URL()+"/items?name=eq.patch-target",
			`{"id":9007199254740996}`,
		)
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
		}

		readResp, readBody := get(t, service, "/items?id=eq.9007199254740996")
		if readResp.StatusCode != http.StatusOK {
			t.Fatalf("read-back status = %d; body = %s", readResp.StatusCode, readBody)
		}
		var rows []map[string]any
		readDecoder := json.NewDecoder(strings.NewReader(string(readBody)))
		readDecoder.UseNumber()
		if err := readDecoder.Decode(&rows); err != nil {
			t.Fatalf("decode read-back body %s: %v", readBody, err)
		}
		if len(rows) != 1 {
			t.Fatalf("read-back rows = %d, want 1; body = %s", len(rows), readBody)
		}
		if id, ok := rows[0]["id"].(json.Number); !ok || id.String() != "9007199254740996" {
			t.Fatalf("read-back id = %#v, want 9007199254740996", rows[0]["id"])
		}
	})

	t.Run("POST /rpc/add_them with large integer", func(t *testing.T) {
		request, err := http.NewRequest(
			http.MethodPost,
			service.URL()+"/rpc/add_them",
			strings.NewReader(`{"a":9007199254740993,"b":0}`),
		)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("POST /rpc/add_them: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
		}
		if strings.TrimSpace(string(body)) != "9007199254740993" {
			t.Fatalf("body = %q, want 9007199254740993", strings.TrimSpace(string(body)))
		}
	})

	t.Run("POST /rpc/add_them with MaxInt64", func(t *testing.T) {
		request, err := http.NewRequest(
			http.MethodPost,
			service.URL()+"/rpc/add_them",
			strings.NewReader(`{"a":9223372036854775807,"b":0}`),
		)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("POST /rpc/add_them: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
		}
		if strings.TrimSpace(string(body)) != "9223372036854775807" {
			t.Fatalf("body = %q, want 9223372036854775807", strings.TrimSpace(string(body)))
		}
	})

	t.Run("POST /rpc/echo_name with large integer into VARCHAR", func(t *testing.T) {
		request, err := http.NewRequest(
			http.MethodPost,
			service.URL()+"/rpc/echo_name",
			strings.NewReader(`{"src":9007199254740993}`),
		)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("POST /rpc/echo_name: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, body)
		}
		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("unmarshal echo_name result %s: %v", body, err)
		}
		if result["dst"] != "9007199254740993" {
			t.Fatalf("dst = %#v, want 9007199254740993", result["dst"])
		}
	})

	t.Run("GET /rpc/add_them query arguments stay unchanged", func(t *testing.T) {
		resp, body := get(t, service, "/rpc/add_them?a=9007199254740993&b=0")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", resp.StatusCode, http.StatusOK, body)
		}
		if strings.TrimSpace(string(body)) != "9007199254740993" {
			t.Fatalf("body = %q, want 9007199254740993", strings.TrimSpace(string(body)))
		}
	})

	t.Run("PATCH /profiles nested JSON large integer", func(t *testing.T) {
		response, body := writeJSON(
			t,
			http.MethodPatch,
			service.URL()+"/profiles?id=eq.1",
			`{"meta":{"n":9007199254740993}}`,
		)
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusNoContent, body)
		}

		updated, readBody := get(t, service, "/profiles?select=meta&id=eq.1")
		if updated.StatusCode != http.StatusOK {
			t.Fatalf("read-back status = %d; body = %s", updated.StatusCode, readBody)
		}
		var rows []map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(readBody)))
		decoder.UseNumber()
		if err := decoder.Decode(&rows); err != nil {
			t.Fatalf("decode read-back body %s: %v", readBody, err)
		}
		meta, ok := rows[0]["meta"].(map[string]any)
		if !ok {
			t.Fatalf("meta = %#v, want map", rows[0]["meta"])
		}
		if n, ok := meta["n"].(json.Number); !ok || n.String() != "9007199254740993" {
			t.Fatalf("meta[n] = %#v, want 9007199254740993", meta["n"])
		}
	})

	t.Run("POST /measurements with DECIMAL(30,10)", func(t *testing.T) {
		request, err := http.NewRequest(
			http.MethodPost,
			service.URL()+"/measurements",
			strings.NewReader(`{"amount":12345678901234567890.1234567891}`),
		)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Prefer", "return=representation")

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("POST /measurements: %v", err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusCreated, body)
		}

		var created []map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.UseNumber()
		if err := decoder.Decode(&created); err != nil {
			t.Fatalf("decode response body %s: %v", body, err)
		}
		if len(created) != 1 {
			t.Fatalf("len(created) = %d, want 1", len(created))
		}
		if amount, ok := created[0]["amount"].(json.Number); !ok || amount.String() != "12345678901234567890.1234567891" {
			t.Fatalf("amount = %#v, want 12345678901234567890.1234567891", created[0]["amount"])
		}

		// Read back
		readResp, readBody := get(t, service, "/measurements?select=amount&limit=1")
		if readResp.StatusCode != http.StatusOK {
			t.Fatalf("read-back status = %d; body = %s", readResp.StatusCode, readBody)
		}
		var rows []map[string]any
		readDecoder := json.NewDecoder(strings.NewReader(string(readBody)))
		readDecoder.UseNumber()
		if err := readDecoder.Decode(&rows); err != nil {
			t.Fatalf("decode read-back body %s: %v", readBody, err)
		}
		if len(rows) != 1 {
			t.Fatalf("read-back rows = %d, want 1; body = %s", len(rows), readBody)
		}
		if amount, ok := rows[0]["amount"].(json.Number); !ok || amount.String() != "12345678901234567890.1234567891" {
			t.Fatalf("read-back amount = %#v, want 12345678901234567890.1234567891", rows[0]["amount"])
		}
	})
}
