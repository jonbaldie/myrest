package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// writeScalarRPC must keep the GET headers but write no payload for HEAD,
// which is the same rule that writeRead applies on the tabular read path.
func TestWriteScalarRPCSkipsBodyForHead(t *testing.T) {
	t.Parallel()

	repr := representation{kind: representationJSONArray, contentType: "application/json"}

	get := httptest.NewRecorder()
	writeScalarRPC(get, httptest.NewRequest(http.MethodGet, "/rpc/add_them", nil), repr, int64(3))
	if get.Body.String() != "3\n" {
		t.Fatalf("GET body = %q, want %q", get.Body.String(), "3\n")
	}

	head := httptest.NewRecorder()
	writeScalarRPC(head, httptest.NewRequest(http.MethodHead, "/rpc/add_them", nil), repr, int64(3))
	if head.Body.Len() != 0 {
		t.Fatalf("HEAD body = %q, want empty", head.Body.String())
	}
	if head.Code != get.Code {
		t.Fatalf("HEAD status = %d, want %d", head.Code, get.Code)
	}
	if head.Header().Get("Content-Type") != get.Header().Get("Content-Type") {
		t.Fatalf("HEAD Content-Type = %q, want %q",
			head.Header().Get("Content-Type"), get.Header().Get("Content-Type"))
	}
}
