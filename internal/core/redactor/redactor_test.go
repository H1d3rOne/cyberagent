package redactor

import (
	"testing"
)

func TestRedactBearerToken(t *testing.T) {
	r := New()
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig"
	got := r.Redact(input)
	want := "Authorization: Bearer [REDACTED]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactAPIKey(t *testing.T) {
	r := New()
	input := `api_key=sk-1234567890abcdef`
	got := r.Redact(input)
	if got == input {
		t.Errorf("expected api_key to be redacted, got %q", got)
	}
	if got != "api_key=[REDACTED]" {
		t.Errorf("got %q, want %q", got, "api_key=[REDACTED]")
	}
}

func TestRedactPassword(t *testing.T) {
	r := New()
	input := `password="hunter2"`
	got := r.Redact(input)
	want := `password="[REDACTED]"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactHeaders(t *testing.T) {
	r := New()
	headers := map[string][]string{
		"Authorization": {"Bearer secret-token-value"},
		"Cookie":        {"session=abc123def456"},
		"Content-Type":  {"application/json"},
	}
	redacted := r.RedactHeaders(headers)
	if redacted["Content-Type"][0] != "application/json" {
		t.Errorf("Content-Type should not be redacted")
	}
	if redacted["Authorization"][0] == "Bearer secret-token-value" {
		t.Errorf("Authorization should be redacted")
	}
	if redacted["Cookie"][0] == "session=abc123def456" {
		t.Errorf("Cookie should be redacted")
	}
}

func TestRedactSessionID(t *testing.T) {
	r := New()
	input := "session_id=abcdef123456"
	got := r.Redact(input)
	want := "session_id=[REDACTED]"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMaskValueShort(t *testing.T) {
	got := maskValue("short")
	if got != "[REDACTED]" {
		t.Errorf("short values should be fully redacted, got %q", got)
	}
}

func TestMaskValueLong(t *testing.T) {
	got := maskValue("a-long-token-value")
	if got != "a-lo...[REDACTED]" {
		t.Errorf("got %q, want %q", got, "a-lo...[REDACTED]")
	}
}
