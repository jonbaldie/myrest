package httpapi

import "testing"

func TestReportedBaseURLUsesOpenAPIServerProxyURI(t *testing.T) {
	t.Parallel()

	got := reportedBaseURL("https://api.example/v1/", "http://127.0.0.1:3000")
	if got != "https://api.example/v1" {
		t.Fatalf("reportedBaseURL = %q, want https://api.example/v1", got)
	}
}

func TestReportedBaseURLFallsBackToListenURL(t *testing.T) {
	t.Parallel()

	got := reportedBaseURL("", "http://127.0.0.1:3000/")
	if got != "http://127.0.0.1:3000" {
		t.Fatalf("reportedBaseURL = %q, want http://127.0.0.1:3000", got)
	}
}
