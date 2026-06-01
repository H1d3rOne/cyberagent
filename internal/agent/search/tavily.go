package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const tavilyDefaultEndpoint = "https://api.tavily.com/search"

type tavilyProvider struct {
	apiKey     string
	endpoint   string
	maxResults int
	httpClient *http.Client
}

func newTavily(cfg Config) *tavilyProvider {
	endpoint := strings.TrimSpace(cfg.BaseURL)
	if endpoint == "" {
		endpoint = tavilyDefaultEndpoint
	}
	max := cfg.MaxResults
	if max <= 0 {
		max = 5
	}
	return &tavilyProvider{
		apiKey:     strings.TrimSpace(cfg.APIKey),
		endpoint:   endpoint,
		maxResults: max,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *tavilyProvider) Name() string { return "tavily" }

type tavilyRequest struct {
	APIKey      string `json:"api_key"`
	Query       string `json:"query"`
	MaxResults  int    `json:"max_results,omitempty"`
	SearchDepth string `json:"search_depth,omitempty"`
}

type tavilyResponse struct {
	Query   string         `json:"query"`
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

func (p *tavilyProvider) Search(ctx context.Context, q Query) (Result, error) {
	query := strings.TrimSpace(q.Query)
	if query == "" {
		return Result{}, errors.New("empty query")
	}
	max := q.MaxResults
	if max <= 0 {
		max = p.maxResults
	}
	if max > 10 {
		max = 10
	}
	body, err := json.Marshal(tavilyRequest{
		APIKey:      p.apiKey,
		Query:       query,
		MaxResults:  max,
		SearchDepth: "basic",
	})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode/100 != 2 {
		snippet := strings.TrimSpace(string(raw))
		if len(snippet) > 240 {
			snippet = snippet[:240]
		}
		return Result{}, fmt.Errorf("tavily search failed: %s: %s", resp.Status, snippet)
	}
	var parsed tavilyResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Result{}, fmt.Errorf("tavily decode: %w", err)
	}
	items := make([]Item, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		items = append(items, Item{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: strings.TrimSpace(r.Content),
			Source:  "tavily",
			Score:   r.Score,
		})
	}
	return Result{Provider: "tavily", Query: query, Items: items}, nil
}
