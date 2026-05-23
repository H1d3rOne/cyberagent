package builtin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDiscoverContentSmall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("admin"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	result := DiscoverContentSmall(context.Background(), ContentDiscoveryInput{JobID: "job_001", StepID: "step_content_001", BaseURL: server.URL, Paths: []string{"/admin", "/missing"}, Timeout: time.Second, Allowed: func(string) bool { return true }})
	if len(result.Results) != 1 || result.Results[0].StatusCode != http.StatusOK {
		t.Fatalf("unexpected results: %#v", result.Results)
	}
	if len(result.Evidence) != 1 {
		t.Fatalf("evidence count = %d, want 1", len(result.Evidence))
	}
}

func TestDiscoverContentSmallHonorsAllowed(t *testing.T) {
	result := DiscoverContentSmall(context.Background(), ContentDiscoveryInput{JobID: "job_001", StepID: "step_content_001", BaseURL: "http://example.test", Paths: []string{"/admin"}, Timeout: time.Second, Allowed: func(string) bool { return false }})
	if len(result.Results) != 0 || len(result.Skipped) != 1 {
		t.Fatalf("unexpected result/skipped: %#v %#v", result.Results, result.Skipped)
	}
}
