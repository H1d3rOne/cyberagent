package builtin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	result, err := HTTPProbe(context.Background(), HTTPProbeOptions{URL: server.URL, Methods: []string{http.MethodHead}, Timeout: time.Second})
	if err != nil {
		t.Fatalf("HTTPProbe() error = %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusOK)
	}
	if result.Method != http.MethodHead {
		t.Fatalf("method = %s, want HEAD", result.Method)
	}
	if result.Headers["X-Test"][0] != "ok" {
		t.Fatalf("missing X-Test header: %#v", result.Headers)
	}
}

func TestHTTPFetchLimitsBodyAndHashes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()

	result, err := HTTPFetch(context.Background(), HTTPFetchOptions{URL: server.URL, Timeout: time.Second, MaxBytes: 3})
	if err != nil {
		t.Fatalf("HTTPFetch() error = %v", err)
	}
	if string(result.Body) != "abc" {
		t.Fatalf("body = %q, want abc", string(result.Body))
	}
	if !result.Truncated {
		t.Fatalf("expected truncated body")
	}
	if result.BodySize != 3 || result.BodySHA256 == "" {
		t.Fatalf("unexpected body metadata: size=%d sha=%q", result.BodySize, result.BodySHA256)
	}
}

func TestHTTPFetchTracksRedirects(t *testing.T) {
	var finalURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("final"))
	}))
	defer server.Close()
	finalURL = server.URL + "/final"

	result, err := HTTPFetch(context.Background(), HTTPFetchOptions{URL: server.URL, Timeout: time.Second, MaxBytes: 100})
	if err != nil {
		t.Fatalf("HTTPFetch() error = %v", err)
	}
	if result.FinalURL != finalURL {
		t.Fatalf("final URL = %s, want %s", result.FinalURL, finalURL)
	}
	if len(result.Redirects) != 1 || result.Redirects[0] != finalURL {
		t.Fatalf("unexpected redirects: %#v", result.Redirects)
	}
}
