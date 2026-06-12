package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type memoryProviderStore struct {
	mu  sync.Mutex
	cfg ProviderConfig
}

func (s *memoryProviderStore) Load() (ProviderConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneProviderConfig(s.cfg), nil
}

func (s *memoryProviderStore) Save(cfg ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cloneProviderConfig(cfg)
	return nil
}

func (s *memoryProviderStore) snapshot() ProviderConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneProviderConfig(s.cfg)
}

func cloneProviderConfig(cfg ProviderConfig) ProviderConfig {
	clone := cfg
	if cfg.Providers != nil {
		clone.Providers = append([]ProviderProfile(nil), cfg.Providers...)
	}
	if cfg.WebSearch != nil {
		webSearch := *cfg.WebSearch
		clone.WebSearch = &webSearch
	}
	if cfg.AuthProfiles != nil {
		clone.AuthProfiles = cloneAPIAuthProfiles(cfg.AuthProfiles)
	}
	return clone
}

func boolPtr(value bool) *bool {
	return &value
}

func TestProviderProfilesGETRedactsSecretsAndReportsStatus(t *testing.T) {
	store := &memoryProviderStore{cfg: ProviderConfig{
		Providers: []ProviderProfile{{
			ID:           "p1",
			Name:         "Primary",
			Type:         "openai",
			Model:        "gpt-4.1",
			APIKey:       "sk-provider-secret",
			BaseURL:      "https://llm.example.test/v1",
			Instructions: "be concise",
			MaxTurns:     42,
		}},
		ActiveID: "p1",
		WebSearch: &WebSearchConfig{
			Provider:   "tavily",
			APIKey:     "tvly-search-secret",
			BaseURL:    "https://search.example.test",
			MaxResults: 4,
		},
		AuthProfiles: []APIAuthProfile{{
			ID:          "auth-1",
			Name:        "Primary API",
			Type:        "api_key",
			BaseURL:     "https://api.example.test",
			HeaderName:  "X-API-Key",
			HeaderValue: "api-auth-secret",
			Enabled:     boolPtr(true),
		}},
		DefaultAuthProfileID: "auth-1",
	}}
	server := NewAPIServer(nil, store)

	req := httptest.NewRequest(http.MethodGet, "/api/provider-profiles", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "sk-provider-secret") || strings.Contains(body, "tvly-search-secret") || strings.Contains(body, "api-auth-secret") {
		t.Fatalf("response leaked secret: %s", body)
	}
	for _, rawSecretField := range []string{`"api_key":`, `"token":`, `"password":`, `"header_value":`, `"cookie":`} {
		if strings.Contains(body, rawSecretField) {
			t.Fatalf("response should not include raw secret field %s: %s", rawSecretField, body)
		}
	}

	var got ProviderConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ActiveID != "p1" || len(got.Providers) != 1 {
		t.Fatalf("unexpected provider response: %+v", got)
	}
	if !got.Providers[0].APIKeyConfigured {
		t.Fatalf("expected provider api_key_configured=true: %+v", got.Providers[0])
	}
	if got.WebSearch == nil || !got.WebSearch.APIKeyConfigured {
		t.Fatalf("expected web_search api_key_configured=true: %+v", got.WebSearch)
	}
	if got.DefaultAuthProfileID != "auth-1" || len(got.AuthProfiles) != 1 || !got.AuthProfiles[0].SecretConfigured || !got.AuthProfiles[0].HeaderValueConfigured {
		t.Fatalf("expected auth profile configured status in response: %+v", got)
	}
}

func TestProviderProfilesPUTPreservesExistingSecretsWhenIncomingEmpty(t *testing.T) {
	store := &memoryProviderStore{cfg: ProviderConfig{
		Providers: []ProviderProfile{{
			ID:      "p1",
			Name:    "Primary",
			Type:    "openai",
			Model:   "gpt-4o",
			APIKey:  "sk-existing-provider",
			BaseURL: "https://old-llm.example.test/v1",
		}},
		ActiveID: "p1",
		WebSearch: &WebSearchConfig{
			Provider:   "tavily",
			APIKey:     "tvly-existing-search",
			BaseURL:    "https://old-search.example.test",
			MaxResults: 5,
		},
		AuthProfiles: []APIAuthProfile{{
			ID:          "auth-1",
			Name:        "API Auth",
			Type:        "api_key",
			BaseURL:     "https://api.example.test",
			HeaderName:  "X-API-Key",
			HeaderValue: "existing-header-secret",
			Enabled:     boolPtr(true),
		}},
		DefaultAuthProfileID: "auth-1",
	}}
	server := NewAPIServer(nil, store)
	body := []byte(`{
		"providers": [{
			"id": "p1",
			"name": "Primary updated",
			"type": "openai",
			"model": "gpt-4.1",
			"api_key": "",
			"base_url": "https://new-llm.example.test/v1",
			"max_turns": 12
		}],
		"active_id": "p1",
		"web_search": {
			"provider": "tavily",
			"api_key": "",
			"base_url": "https://new-search.example.test",
			"max_results": 3
		},
		"auth_profiles": [{
			"id": "auth-1",
			"name": "API Auth updated",
			"type": "api_key",
			"base_url": "https://api.example.test",
			"header_name": "X-API-Key",
			"header_value": "",
			"enabled": true
		}],
		"default_auth_profile_id": "auth-1"
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/provider-profiles", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := store.snapshot()
	if len(got.Providers) != 1 {
		t.Fatalf("expected one provider, got %+v", got.Providers)
	}
	if got.Providers[0].APIKey != "sk-existing-provider" {
		t.Fatalf("expected provider secret to be preserved, got %+v", got.Providers[0])
	}
	if got.Providers[0].BaseURL != "https://new-llm.example.test/v1" || got.Providers[0].Model != "gpt-4.1" {
		t.Fatalf("expected non-secret provider fields to update, got %+v", got.Providers[0])
	}
	if got.WebSearch == nil || got.WebSearch.APIKey != "tvly-existing-search" {
		t.Fatalf("expected web search secret to be preserved, got %+v", got.WebSearch)
	}
	if got.WebSearch.BaseURL != "https://new-search.example.test" || got.WebSearch.MaxResults != 3 {
		t.Fatalf("expected non-secret web search fields to update, got %+v", got.WebSearch)
	}
	if len(got.AuthProfiles) != 1 || got.AuthProfiles[0].HeaderValue != "existing-header-secret" {
		t.Fatalf("expected auth profile secret to be preserved, got %+v", got.AuthProfiles)
	}
	if got.AuthProfiles[0].Name != "API Auth updated" {
		t.Fatalf("expected non-secret auth profile fields to update, got %+v", got.AuthProfiles[0])
	}

	responseBody := rec.Body.String()
	if strings.Contains(responseBody, "sk-existing-provider") || strings.Contains(responseBody, "tvly-existing-search") || strings.Contains(responseBody, "existing-header-secret") {
		t.Fatalf("response leaked preserved secret: %s", responseBody)
	}
	for _, rawSecretField := range []string{`"api_key":`, `"token":`, `"password":`, `"header_value":`, `"cookie":`} {
		if strings.Contains(responseBody, rawSecretField) {
			t.Fatalf("response should not include raw secret field %s: %s", rawSecretField, responseBody)
		}
	}
	var resp ProviderConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Providers) != 1 || !resp.Providers[0].APIKeyConfigured || resp.WebSearch == nil || !resp.WebSearch.APIKeyConfigured {
		t.Fatalf("expected configured status in response: %+v", resp)
	}
	if len(resp.AuthProfiles) != 1 || !resp.AuthProfiles[0].SecretConfigured || !resp.AuthProfiles[0].HeaderValueConfigured {
		t.Fatalf("expected auth profile configured status in response: %+v", resp)
	}
}

func TestProviderProfilesPUTDoesNotCarrySecretAcrossChangedProviderScope(t *testing.T) {
	store := &memoryProviderStore{cfg: ProviderConfig{
		Providers: []ProviderProfile{{
			ID:     "p1",
			Type:   "openai",
			APIKey: "sk-existing-provider",
		}},
		WebSearch: &WebSearchConfig{
			Provider: "tavily",
			APIKey:   "tvly-existing-search",
		},
		AuthProfiles: []APIAuthProfile{{
			ID:          "auth-1",
			Type:        "api_key",
			HeaderValue: "existing-header-secret",
		}},
	}}
	server := NewAPIServer(nil, store)
	body := []byte(`{
		"providers": [{
			"id": "p1",
			"type": "anthropic",
			"api_key": ""
		}],
		"web_search": {
			"provider": "brave",
			"api_key": ""
		},
		"auth_profiles": [{
			"id": "auth-1",
			"type": "bearer",
			"token": ""
		}]
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/provider-profiles", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := store.snapshot()
	if len(got.Providers) != 1 || got.Providers[0].APIKey != "" {
		t.Fatalf("expected provider secret not to cross changed type, got %+v", got.Providers)
	}
	if got.WebSearch == nil || got.WebSearch.APIKey != "" {
		t.Fatalf("expected web search secret not to cross changed provider, got %+v", got.WebSearch)
	}
	if len(got.AuthProfiles) != 1 || got.AuthProfiles[0].Token != "" || got.AuthProfiles[0].HeaderValue != "" {
		t.Fatalf("expected auth profile secret not to cross changed type, got %+v", got.AuthProfiles)
	}
}

func TestProviderProfilesPUTPartialAuthUpdatePreservesProviders(t *testing.T) {
	store := &memoryProviderStore{cfg: ProviderConfig{
		Providers: []ProviderProfile{{
			ID:     "p1",
			Type:   "openai",
			APIKey: "sk-existing-provider",
		}},
		ActiveID: "p1",
		WebSearch: &WebSearchConfig{
			Provider: "tavily",
			APIKey:   "tvly-existing-search",
		},
	}}
	server := NewAPIServer(nil, store)
	body := []byte(`{
		"auth_profiles": [{
			"id": "auth-1",
			"name": "API Auth",
			"type": "bearer",
			"token": "bearer-secret",
			"enabled": true
		}],
		"default_auth_profile_id": "auth-1"
	}`)

	req := httptest.NewRequest(http.MethodPut, "/api/provider-profiles", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := store.snapshot()
	if len(got.Providers) != 1 || got.Providers[0].APIKey != "sk-existing-provider" || got.ActiveID != "p1" {
		t.Fatalf("partial auth update should preserve providers, got %+v", got)
	}
	if got.WebSearch == nil || got.WebSearch.APIKey != "tvly-existing-search" {
		t.Fatalf("partial auth update should preserve web search, got %+v", got.WebSearch)
	}
	if len(got.AuthProfiles) != 1 || got.AuthProfiles[0].Token != "bearer-secret" || got.DefaultAuthProfileID != "auth-1" {
		t.Fatalf("partial auth update did not save auth profile, got %+v", got)
	}
	if strings.Contains(rec.Body.String(), "bearer-secret") {
		t.Fatalf("response leaked auth profile secret: %s", rec.Body.String())
	}
}
