// Package search exposes a minimal pluggable web-search backend used by the
// agent's `web.search` tool. Keep providers small and stateless; a single
// Search call must be safe to run concurrently.
package search

import (
	"context"
	"errors"
	"strings"
)

// ErrNotConfigured is returned when no provider is configured or its API key
// is missing. The agent surfaces this verbatim so the LLM can ask the operator
// to configure Settings → Web Search.
var ErrNotConfigured = errors.New("web search backend not configured")

type Query struct {
	Query      string
	MaxResults int
	Locale     string
}

type Item struct {
	Title   string  `json:"title,omitempty"`
	URL     string  `json:"url,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
	Source  string  `json:"source,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

type Result struct {
	Provider string `json:"provider"`
	Query    string `json:"query"`
	Items    []Item `json:"items"`
}

type Provider interface {
	Name() string
	Search(ctx context.Context, q Query) (Result, error)
}

// Config mirrors the subset of web.WebSearchConfig the search package needs.
// Defined here so this package can stay free of higher-level dependencies.
type Config struct {
	Provider   string
	APIKey     string
	BaseURL    string
	MaxResults int
}

func NewFromConfig(cfg Config) (Provider, error) {
	name := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if name == "" || name == "disabled" || name == "none" {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrNotConfigured
	}
	switch name {
	case "tavily":
		return newTavily(cfg), nil
	default:
		return nil, errors.New("unsupported web search provider: " + name)
	}
}
