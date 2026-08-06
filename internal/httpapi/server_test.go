package httpapi_test

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/jonbaldie/myrest/internal/httpapi"
)

func TestStartedServiceAnswersHTTPWithoutParityClaim(t *testing.T) {
	t.Parallel()

	service, err := httpapi.Start()
	if err != nil {
		t.Fatalf("start service: %v", err)
	}
	t.Cleanup(func() {
		if stopErr := service.Close(); stopErr != nil {
			t.Errorf("close service: %v", stopErr)
		}
	})

	response, err := http.Get(service.URL() + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	contentType := response.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", contentType, "application/json")
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var payload struct {
		Service string `json:"service"`
		Parity  string `json:"parity"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode body %q: %v", body, err)
	}
	if payload.Service != "myrest" {
		t.Fatalf("service = %q, want %q", payload.Service, "myrest")
	}
	if payload.Parity != "none" {
		t.Fatalf("parity = %q, want %q (no parity behaviour yet)", payload.Parity, "none")
	}
}

func TestListenRejectsBoundAddress(t *testing.T) {
	t.Parallel()

	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen blocker: %v", err)
	}
	t.Cleanup(func() { _ = blocker.Close() })

	_, err = httpapi.Listen(blocker.Addr().String())
	if err == nil {
		t.Fatal("Listen succeeded on an already-bound address")
	}
}

func TestCloseStopsAcceptingRequests(t *testing.T) {
	t.Parallel()

	service, err := httpapi.Start()
	if err != nil {
		t.Fatalf("start service: %v", err)
	}
	baseURL := service.URL()
	if err := service.Close(); err != nil {
		t.Fatalf("close service: %v", err)
	}

	_, err = http.Get(baseURL + "/")
	if err == nil {
		t.Fatal("GET succeeded after Close")
	}
}
