package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

// ProviderProfile is a persisted AI provider configuration.
type ProviderProfile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Model        string `json:"model"`
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	Instructions string `json:"instructions"`
	MaxTurns     int    `json:"max_turns"`
}

// ProviderConfig is the persisted set of providers plus the active selection.
type ProviderConfig struct {
	Providers            []ProviderProfile `json:"providers"`
	ActiveID             string            `json:"active_id"`
	WebSearch            *WebSearchConfig  `json:"web_search,omitempty"`
	AuthProfiles         []APIAuthProfile  `json:"auth_profiles,omitempty"`
	DefaultAuthProfileID string            `json:"default_auth_profile_id,omitempty"`
}

// ProviderProfileResponse is the browser-facing provider profile. Secrets stay
// server-side; the UI only receives whether a secret is configured.
type ProviderProfileResponse struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Model            string `json:"model"`
	BaseURL          string `json:"base_url"`
	Instructions     string `json:"instructions"`
	MaxTurns         int    `json:"max_turns"`
	APIKeyConfigured bool   `json:"api_key_configured"`
}

// ProviderConfigResponse is the browser-facing provider configuration.
type ProviderConfigResponse struct {
	Providers            []ProviderProfileResponse `json:"providers"`
	ActiveID             string                    `json:"active_id"`
	WebSearch            *WebSearchConfigResponse  `json:"web_search,omitempty"`
	AuthProfiles         []APIAuthProfileResponse  `json:"auth_profiles,omitempty"`
	DefaultAuthProfileID string                    `json:"default_auth_profile_id,omitempty"`
}

// WebSearchConfig is the persisted web-search backend used by the agent's
// `web.search` tool. Provider names are lowercase ("tavily" today; future:
// "brave", "serpapi"). Empty Provider or APIKey means the tool is disabled.
type WebSearchConfig struct {
	Provider   string `json:"provider,omitempty"`
	APIKey     string `json:"api_key,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

// WebSearchConfigResponse is the browser-facing web-search configuration.
type WebSearchConfigResponse struct {
	Provider         string `json:"provider,omitempty"`
	BaseURL          string `json:"base_url,omitempty"`
	MaxResults       int    `json:"max_results,omitempty"`
	APIKeyConfigured bool   `json:"api_key_configured"`
}

// APIAuthProfile is a persisted HTTP/API authorization profile for agent
// tools such as http.request. Secret fields stay server-side for browser
// responses.
type APIAuthProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	Token       string `json:"token,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderName  string `json:"header_name,omitempty"`
	HeaderValue string `json:"header_value,omitempty"`
	Cookie      string `json:"cookie,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

// APIAuthProfileResponse is the browser-facing auth profile. It intentionally
// omits raw token/password/header/cookie values and only reports configured
// state.
type APIAuthProfileResponse struct {
	ID                    string `json:"id"`
	Name                  string `json:"name,omitempty"`
	Type                  string `json:"type,omitempty"`
	BaseURL               string `json:"base_url,omitempty"`
	Username              string `json:"username,omitempty"`
	HeaderName            string `json:"header_name,omitempty"`
	Enabled               bool   `json:"enabled"`
	SecretConfigured      bool   `json:"secret_configured"`
	TokenConfigured       bool   `json:"token_configured"`
	PasswordConfigured    bool   `json:"password_configured"`
	HeaderValueConfigured bool   `json:"header_value_configured"`
	CookieConfigured      bool   `json:"cookie_configured"`
}

// ProviderStore persists provider configurations.
type ProviderStore interface {
	Load() (ProviderConfig, error)
	Save(ProviderConfig) error
}

// ContextProviderStore is an optional extension for daemon-backed stores that
// need to observe request-scoped tenant/workspace context without changing the
// stable ProviderStore contract used by the local dashboard.
type ContextProviderStore interface {
	ProviderStore
	LoadContext(context.Context) (ProviderConfig, error)
	SaveContext(context.Context, ProviderConfig) error
}

type APIServer struct {
	manager   *ScanManager
	providers ProviderStore
	mux       *http.ServeMux
}

func NewAPIServer(manager *ScanManager, providers ProviderStore) *APIServer {
	s := &APIServer{manager: manager, providers: providers, mux: http.NewServeMux()}
	s.registerRoutes()
	return s
}

func (s *APIServer) Handler() http.Handler {
	return corsMiddleware(s.mux)
}

func (s *APIServer) registerRoutes() {
	s.mux.HandleFunc("/api/scans", s.handleScans)
	s.mux.HandleFunc("/api/scans/", s.handleScanByID)
	s.mux.HandleFunc("/api/providers", s.handleProviders)
	s.mux.HandleFunc("/api/provider-profiles", s.handleProviderProfiles)
	s.mux.HandleFunc("/api/provider-profiles/test", s.handleProviderTest)
	s.mux.HandleFunc("/api/tools", s.handleTools)
}

func (s *APIServer) handleScans(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listScans(w, r)
	case http.MethodPost:
		s.startScan(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *APIServer) listScans(w http.ResponseWriter, _ *http.Request) {
	items := s.manager.List()
	writeJSON(w, http.StatusOK, items)
}

func (s *APIServer) startScan(w http.ResponseWriter, r *http.Request) {
	var req StartScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	id, err := s.manager.Start(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *APIServer) handleScanByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/scans/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]

	if id == "" {
		http.Error(w, "scan id required", http.StatusBadRequest)
		return
	}

	if len(parts) == 2 {
		switch parts[1] {
		case "stop":
			s.stopScan(w, r, id)
		case "events":
			s.streamEvents(w, r, id)
		case "message":
			s.sendMessage(w, r, id)
		default:
			http.NotFound(w, r)
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getScan(w, r, id)
	case http.MethodDelete:
		s.deleteScan(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *APIServer) getScan(w http.ResponseWriter, _ *http.Request, id string) {
	ms, ok := s.manager.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "scan not found"})
		return
	}
	writeJSON(w, http.StatusOK, ms.toDetail())
}

func (s *APIServer) stopScan(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.manager.Stop(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *APIServer) deleteScan(w http.ResponseWriter, _ *http.Request, id string) {
	if err := s.manager.Delete(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *APIServer) sendMessage(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message is required"})
		return
	}
	if err := s.manager.SendMessage(id, body.Message); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func (s *APIServer) streamEvents(w http.ResponseWriter, r *http.Request, id string) {
	ms, ok := s.manager.Get(id)
	if !ok {
		http.Error(w, "scan not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := ms.subscribe()
	defer ms.unsubscribe(ch)

	for {
		select {
		case event := <-ch:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *APIServer) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	providers := []map[string]string{
		{"id": "anthropic", "name": "Anthropic (Claude)", "default_model": "claude-sonnet-4-20250514"},
		{"id": "openai", "name": "OpenAI (GPT)", "default_model": "gpt-4o"},
		{"id": "ollama", "name": "Ollama (Local)", "default_model": "llama3"},
		{"id": "anthropic-compatible", "name": "Anthropic Compatible", "default_model": ""},
		{"id": "openai-compatible", "name": "OpenAI Compatible", "default_model": ""},
	}
	writeJSON(w, http.StatusOK, providers)
}

func (s *APIServer) handleProviderProfiles(w http.ResponseWriter, r *http.Request) {
	if s.providers == nil {
		http.Error(w, "provider store unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		cfg, err := loadProviderConfig(r.Context(), s.providers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, providerConfigResponse(cfg))
	case http.MethodPut:
		existing, err := loadProviderConfig(r.Context(), s.providers)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg, err := decodeProviderConfigUpdate(r, existing)
		if err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := saveProviderConfig(r.Context(), s.providers, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, providerConfigResponse(cfg))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func decodeProviderConfigUpdate(r *http.Request, existing ProviderConfig) (ProviderConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return ProviderConfig{}, err
	}
	cfg := cloneProviderConfigForUpdate(existing)
	if value, ok := raw["providers"]; ok {
		var providers []ProviderProfile
		if err := json.Unmarshal(value, &providers); err != nil {
			return ProviderConfig{}, err
		}
		cfg.Providers = mergeProviderSecrets(existing.Providers, providers)
	}
	if value, ok := raw["active_id"]; ok {
		if err := json.Unmarshal(value, &cfg.ActiveID); err != nil {
			return ProviderConfig{}, err
		}
	}
	if value, ok := raw["web_search"]; ok {
		if isJSONNull(value) {
			cfg.WebSearch = nil
		} else {
			var webSearch WebSearchConfig
			if err := json.Unmarshal(value, &webSearch); err != nil {
				return ProviderConfig{}, err
			}
			cfg.WebSearch = mergeWebSearchSecret(existing.WebSearch, &webSearch)
		}
	}
	if value, ok := raw["auth_profiles"]; ok {
		var profiles []APIAuthProfile
		if err := json.Unmarshal(value, &profiles); err != nil {
			return ProviderConfig{}, err
		}
		cfg.AuthProfiles = mergeAPIAuthProfileSecrets(existing.AuthProfiles, profiles)
	}
	if value, ok := raw["default_auth_profile_id"]; ok {
		if err := json.Unmarshal(value, &cfg.DefaultAuthProfileID); err != nil {
			return ProviderConfig{}, err
		}
	}
	return cfg, nil
}

func isJSONNull(value json.RawMessage) bool {
	return strings.EqualFold(strings.TrimSpace(string(value)), "null")
}

func cloneProviderConfigForUpdate(cfg ProviderConfig) ProviderConfig {
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

func loadProviderConfig(ctx context.Context, store ProviderStore) (ProviderConfig, error) {
	if scoped, ok := store.(ContextProviderStore); ok {
		return scoped.LoadContext(ctx)
	}
	return store.Load()
}

func saveProviderConfig(ctx context.Context, store ProviderStore, cfg ProviderConfig) error {
	if scoped, ok := store.(ContextProviderStore); ok {
		return scoped.SaveContext(ctx, cfg)
	}
	return store.Save(cfg)
}

func providerConfigResponse(cfg ProviderConfig) ProviderConfigResponse {
	providers := make([]ProviderProfileResponse, 0, len(cfg.Providers))
	for _, profile := range cfg.Providers {
		providers = append(providers, ProviderProfileResponse{
			ID:               profile.ID,
			Name:             profile.Name,
			Type:             profile.Type,
			Model:            profile.Model,
			BaseURL:          profile.BaseURL,
			Instructions:     profile.Instructions,
			MaxTurns:         profile.MaxTurns,
			APIKeyConfigured: strings.TrimSpace(profile.APIKey) != "",
		})
	}
	resp := ProviderConfigResponse{
		Providers:            providers,
		ActiveID:             cfg.ActiveID,
		AuthProfiles:         apiAuthProfileResponses(cfg.AuthProfiles),
		DefaultAuthProfileID: cfg.DefaultAuthProfileID,
	}
	if cfg.WebSearch != nil {
		resp.WebSearch = &WebSearchConfigResponse{
			Provider:         cfg.WebSearch.Provider,
			BaseURL:          cfg.WebSearch.BaseURL,
			MaxResults:       cfg.WebSearch.MaxResults,
			APIKeyConfigured: strings.TrimSpace(cfg.WebSearch.APIKey) != "",
		}
	}
	return resp
}

func mergeProviderConfigSecrets(ctx context.Context, store ProviderStore, incoming ProviderConfig) (ProviderConfig, error) {
	existing, err := loadProviderConfig(ctx, store)
	if err != nil {
		return ProviderConfig{}, err
	}
	incoming.Providers = mergeProviderSecrets(existing.Providers, incoming.Providers)
	incoming.AuthProfiles = mergeAPIAuthProfileSecrets(existing.AuthProfiles, incoming.AuthProfiles)
	incoming.WebSearch = mergeWebSearchSecret(existing.WebSearch, incoming.WebSearch)
	return incoming, nil
}

func mergeProviderSecrets(existingProfiles, incomingProfiles []ProviderProfile) []ProviderProfile {
	if incomingProfiles == nil {
		return []ProviderProfile{}
	}
	existingByID := make(map[string]ProviderProfile, len(existingProfiles))
	for _, profile := range existingProfiles {
		if strings.TrimSpace(profile.ID) == "" {
			continue
		}
		existingByID[profile.ID] = profile
	}
	out := append([]ProviderProfile(nil), incomingProfiles...)
	for i := range out {
		profile := &out[i]
		if strings.TrimSpace(profile.APIKey) != "" {
			continue
		}
		existingProfile, ok := existingByID[profile.ID]
		if !ok || !sameProviderSecretScope(existingProfile, *profile) {
			continue
		}
		profile.APIKey = existingProfile.APIKey
	}
	return out
}

func mergeWebSearchSecret(existing, incoming *WebSearchConfig) *WebSearchConfig {
	if incoming == nil {
		return nil
	}
	webSearch := *incoming
	if strings.TrimSpace(webSearch.APIKey) == "" && existing != nil && sameWebSearchSecretScope(*existing, webSearch) {
		webSearch.APIKey = existing.APIKey
	}
	return &webSearch
}

func mergeAPIAuthProfileSecrets(existingProfiles, incomingProfiles []APIAuthProfile) []APIAuthProfile {
	if incomingProfiles == nil {
		return nil
	}
	existingByID := make(map[string]APIAuthProfile, len(existingProfiles))
	for _, profile := range existingProfiles {
		if strings.TrimSpace(profile.ID) == "" {
			continue
		}
		existingByID[profile.ID] = profile
	}
	out := cloneAPIAuthProfiles(incomingProfiles)
	for i := range out {
		profile := &out[i]
		existingProfile, ok := existingByID[profile.ID]
		if !ok || !sameAPIAuthProfileSecretScope(existingProfile, *profile) {
			continue
		}
		if strings.TrimSpace(profile.Token) == "" {
			profile.Token = existingProfile.Token
		}
		if strings.TrimSpace(profile.Password) == "" {
			profile.Password = existingProfile.Password
		}
		if strings.TrimSpace(profile.HeaderValue) == "" {
			profile.HeaderValue = existingProfile.HeaderValue
		}
		if strings.TrimSpace(profile.Cookie) == "" {
			profile.Cookie = existingProfile.Cookie
		}
	}
	return out
}

func sameProviderSecretScope(existing ProviderProfile, incoming ProviderProfile) bool {
	if strings.TrimSpace(existing.APIKey) == "" {
		return false
	}
	existingType := strings.TrimSpace(strings.ToLower(existing.Type))
	incomingType := strings.TrimSpace(strings.ToLower(incoming.Type))
	return existingType == "" || incomingType == "" || existingType == incomingType
}

func sameWebSearchSecretScope(existing WebSearchConfig, incoming WebSearchConfig) bool {
	if strings.TrimSpace(existing.APIKey) == "" {
		return false
	}
	existingProvider := strings.TrimSpace(strings.ToLower(existing.Provider))
	incomingProvider := strings.TrimSpace(strings.ToLower(incoming.Provider))
	return existingProvider != "" && existingProvider == incomingProvider
}

func apiAuthProfileResponses(profiles []APIAuthProfile) []APIAuthProfileResponse {
	if len(profiles) == 0 {
		return nil
	}
	out := make([]APIAuthProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		enabled := true
		if profile.Enabled != nil {
			enabled = *profile.Enabled
		}
		tokenConfigured := strings.TrimSpace(profile.Token) != ""
		passwordConfigured := strings.TrimSpace(profile.Password) != ""
		headerValueConfigured := strings.TrimSpace(profile.HeaderValue) != ""
		cookieConfigured := strings.TrimSpace(profile.Cookie) != ""
		out = append(out, APIAuthProfileResponse{
			ID:                    profile.ID,
			Name:                  profile.Name,
			Type:                  profile.Type,
			BaseURL:               profile.BaseURL,
			Username:              profile.Username,
			HeaderName:            profile.HeaderName,
			Enabled:               enabled,
			SecretConfigured:      apiAuthProfileSecretConfigured(profile),
			TokenConfigured:       tokenConfigured,
			PasswordConfigured:    passwordConfigured,
			HeaderValueConfigured: headerValueConfigured,
			CookieConfigured:      cookieConfigured,
		})
	}
	return out
}

func apiAuthProfileSecretConfigured(profile APIAuthProfile) bool {
	switch normalizeAPIAuthProfileType(profile.Type) {
	case "basic":
		return strings.TrimSpace(profile.Password) != ""
	case "api_key":
		return strings.TrimSpace(profile.HeaderValue) != "" || strings.TrimSpace(profile.Token) != ""
	case "cookie":
		return strings.TrimSpace(profile.Cookie) != "" || strings.TrimSpace(profile.HeaderValue) != ""
	case "custom":
		return strings.TrimSpace(profile.HeaderValue) != ""
	default:
		return strings.TrimSpace(profile.Token) != "" || strings.TrimSpace(profile.HeaderValue) != ""
	}
}

func sameAPIAuthProfileSecretScope(existing APIAuthProfile, incoming APIAuthProfile) bool {
	if !apiAuthProfileSecretConfigured(existing) {
		return false
	}
	existingType := normalizeAPIAuthProfileType(existing.Type)
	incomingType := normalizeAPIAuthProfileType(incoming.Type)
	return existingType == incomingType
}

func normalizeAPIAuthProfileType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "basic":
		return "basic"
	case "api_key", "apikey", "api-key":
		return "api_key"
	case "cookie":
		return "cookie"
	case "custom", "header":
		return "custom"
	default:
		return "bearer"
	}
}

func cloneAPIAuthProfiles(profiles []APIAuthProfile) []APIAuthProfile {
	if profiles == nil {
		return nil
	}
	out := make([]APIAuthProfile, len(profiles))
	for i, profile := range profiles {
		out[i] = profile
		if profile.Enabled != nil {
			enabled := *profile.Enabled
			out[i].Enabled = &enabled
		}
	}
	return out
}

func (s *APIServer) handleProviderTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Type    string `json:"type"`
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	cfg := llm.ProviderConfig{APIKey: req.APIKey, BaseURL: req.BaseURL, Model: req.Model}
	var provider llm.Provider
	requestURL := ""
	if strings.Contains(strings.ToLower(req.Type), "anthropic") {
		requestURL = llm.ResolveAnthropicEndpoint(req.BaseURL)
		provider = llm.NewAnthropic(cfg)
	} else {
		requestURL = llm.ResolveOpenAIEndpoint(req.BaseURL)
		provider = llm.NewOpenAI(cfg)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := provider.Chat(ctx, llm.ChatRequest{
		Model:     req.Model,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: "ping"}}}},
		MaxTokens: 1,
	})
	latency := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": false, "provider": provider.Name(), "model": req.Model,
			"latency_ms": latency, "error": err.Error(), "request_url": requestURL,
		})
		return
	}
	output := ""
	if resp != nil {
		for _, block := range resp.Content {
			if tb, ok := block.(llm.TextBlock); ok {
				output += tb.Text
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "provider": provider.Name(), "model": req.Model,
		"latency_ms": latency, "output": output, "request_url": requestURL,
	})
}

func (s *APIServer) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defs := s.manager.ToolDefinitions()
	tools := make([]map[string]interface{}, 0, len(defs))
	for _, d := range defs {
		tools = append(tools, map[string]interface{}{
			"name":         d.Name,
			"description":  d.Description,
			"input_schema": d.InputSchema,
		})
	}
	writeJSON(w, http.StatusOK, tools)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
