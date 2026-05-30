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
	Providers []ProviderProfile `json:"providers"`
	ActiveID  string            `json:"active_id"`
}

// ProviderStore persists provider configurations.
type ProviderStore interface {
	Load() (ProviderConfig, error)
	Save(ProviderConfig) error
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
		cfg, err := s.providers.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		var cfg ProviderConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := s.providers.Save(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, cfg)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
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
	if strings.Contains(strings.ToLower(req.Type), "anthropic") {
		provider = llm.NewAnthropic(cfg)
	} else {
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
			"latency_ms": latency, "error": err.Error(),
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
		"latency_ms": latency, "output": output,
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
			"name":        d.Name,
			"description": d.Description,
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
