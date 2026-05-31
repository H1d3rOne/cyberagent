package llm

import (
	"bytes"
	"net/url"
	"strings"
)

func diagnosticURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	if parsed.User != nil {
		parsed.User = url.User("[redacted]")
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key := range query {
			query.Set(key, "[redacted]")
		}
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func responseSnippet(body []byte, limit int) string {
	text := strings.TrimSpace(string(bytes.TrimSpace(body)))
	if text == "" {
		return "<empty response body>"
	}
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "...[truncated]"
}
