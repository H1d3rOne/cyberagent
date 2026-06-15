package daemon

import (
	"context"
	"fmt"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

type agentLLMProviderAttempt struct {
	Request     sendAgentMessageRequest
	Provider    llm.Provider
	ProfileID   string
	ProfileName string
}

type agentLLMFallbackRecord struct {
	Provider    string `json:"provider"`
	Model       string `json:"model,omitempty"`
	ProfileID   string `json:"profile_id,omitempty"`
	ProfileName string `json:"profile_name,omitempty"`
	Error       string `json:"error"`
}

func (m *agentManager) AttachProviderStore(store web.ProviderStore) {
	m.mu.Lock()
	m.providers = store
	m.mu.Unlock()
}

func (m *agentManager) providerStore() web.ProviderStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.providers
}

func (m *agentManager) agentLLMProviderAttempts(req sendAgentMessageRequest) []agentLLMProviderAttempt {
	seen := map[string]bool{}
	attempts := make([]agentLLMProviderAttempt, 0, 3)
	appendAttempt := func(candidate sendAgentMessageRequest, profileID, profileName string) {
		if !agentLLMConfigured(candidate) {
			return
		}
		key := agentLLMProviderAttemptKey(candidate)
		if seen[key] {
			return
		}
		seen[key] = true
		provider := buildAgentLLMProvider(candidate.Provider, llm.ProviderConfig{
			APIKey:  strings.TrimSpace(candidate.APIKey),
			BaseURL: strings.TrimSpace(candidate.BaseURL),
			Model:   strings.TrimSpace(candidate.Model),
		})
		attempts = append(attempts, agentLLMProviderAttempt{
			Request:     candidate,
			Provider:    provider,
			ProfileID:   strings.TrimSpace(profileID),
			ProfileName: strings.TrimSpace(profileName),
		})
	}

	store := m.providerStore()
	var profiles []web.ProviderProfile
	if store != nil {
		if cfg, err := store.Load(); err == nil && len(cfg.Providers) > 0 {
			profiles = orderedAgentProviderProfiles(cfg)
		}
	}
	if agentLLMShouldUseRequestAttempt(req, len(profiles) > 0) {
		appendAttempt(req, "", "")
	}
	for _, profile := range profiles {
		candidate := sendAgentMessageRequest{
			Message:              req.Message,
			Operator:             req.Operator,
			Language:             req.Language,
			OperationLevel:       req.OperationLevel,
			Origin:               req.Origin,
			Provider:             profile.Type,
			Model:                profile.Model,
			APIKey:               profile.APIKey,
			BaseURL:              profile.BaseURL,
			Instructions:         firstNonEmpty(profile.Instructions, req.Instructions),
			AuthProfiles:         req.AuthProfiles,
			DefaultAuthProfileID: req.DefaultAuthProfileID,
		}
		appendAttempt(candidate, profile.ID, profile.Name)
	}
	return attempts
}

func agentLLMShouldUseRequestAttempt(req sendAgentMessageRequest, hasStoredProfiles bool) bool {
	if !agentLLMConfigured(req) {
		return false
	}
	if !hasStoredProfiles {
		return true
	}
	if normalizeAgentProvider(req.Provider) == "ollama" {
		return true
	}
	if strings.TrimSpace(req.APIKey) != "" {
		return true
	}
	// Browser-visible provider profiles intentionally redact API keys. When a
	// dev frontend sends only model/base_url back to the daemon, prefer the
	// server-side provider store so the redacted/stale request candidate does
	// not fail first with 401 before the saved secret can be used.
	if strings.TrimSpace(req.BaseURL) != "" {
		return false
	}
	return true
}

func orderedAgentProviderProfiles(cfg web.ProviderConfig) []web.ProviderProfile {
	profiles := append([]web.ProviderProfile(nil), cfg.Providers...)
	activeID := strings.TrimSpace(cfg.ActiveID)
	if activeID == "" || len(profiles) < 2 {
		return profiles
	}
	ordered := make([]web.ProviderProfile, 0, len(profiles))
	for _, profile := range profiles {
		if strings.TrimSpace(profile.ID) == activeID {
			ordered = append(ordered, profile)
			break
		}
	}
	for _, profile := range profiles {
		if strings.TrimSpace(profile.ID) != activeID {
			ordered = append(ordered, profile)
		}
	}
	if len(ordered) == 0 {
		return profiles
	}
	return ordered
}

func agentLLMProviderAttemptKey(req sendAgentMessageRequest) string {
	return strings.Join([]string{
		normalizeAgentProvider(req.Provider),
		strings.TrimSpace(req.Model),
		strings.TrimSpace(req.BaseURL),
		strings.TrimSpace(req.APIKey),
	}, "\x00")
}

func agentLLMAttemptDetail(attempts []agentLLMProviderAttempt) string {
	if len(attempts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		label := agentLLMAttemptProviderName(attempt)
		if model := strings.TrimSpace(attempt.Request.Model); model != "" {
			label += "/" + model
		}
		if attempt.ProfileName != "" {
			label += " (" + attempt.ProfileName + ")"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " → ")
}

func (m *agentManager) chatAgentLLMWithFallback(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, phase string, attempts []agentLLMProviderAttempt, buildRequest func(agentLLMProviderAttempt) llm.ChatRequest) (*llm.ChatResponse, bool, agentLLMProviderAttempt, []agentLLMFallbackRecord, error) {
	if len(attempts) == 0 {
		return nil, false, agentLLMProviderAttempt{}, nil, fmt.Errorf("AI provider is not configured")
	}
	var fallbackLog []agentLLMFallbackRecord
	var lastErr error
	for idx, attempt := range attempts {
		if attempt.Provider == nil {
			continue
		}
		resp, streamed, err := m.chatAgentLLM(ctx, sessionID, turnRef, phase, attempt.Provider, buildRequest(attempt))
		if err == nil {
			return resp, streamed, attempt, fallbackLog, nil
		}
		lastErr = err
		fallbackLog = append(fallbackLog, agentLLMFallbackRecord{
			Provider:    agentLLMAttemptProviderName(attempt),
			Model:       strings.TrimSpace(attempt.Request.Model),
			ProfileID:   attempt.ProfileID,
			ProfileName: attempt.ProfileName,
			Error:       err.Error(),
		})
		if streamed || idx == len(attempts)-1 || !agentLLMShouldFailOver(err) {
			return nil, streamed, attempt, fallbackLog, err
		}
	}
	if lastErr != nil {
		return nil, false, attempts[len(attempts)-1], fallbackLog, lastErr
	}
	return nil, false, agentLLMProviderAttempt{}, fallbackLog, fmt.Errorf("AI provider is not configured")
}

func agentLLMShouldFailOver(err error) bool {
	if llm.IsRetryableError(err) {
		return true
	}
	return agentLLMIsProviderAuthOrCredentialError(err)
}

func agentLLMIsProviderAuthOrCredentialError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"401",
		"403",
		"unauthorized",
		"forbidden",
		"invalid api key",
		"invalid_api_key",
		"incorrect api key",
		"authentication",
		"permission denied",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func agentLLMAttemptProviderName(attempt agentLLMProviderAttempt) string {
	if attempt.Provider != nil {
		return attempt.Provider.Name()
	}
	return normalizeAgentProvider(attempt.Request.Provider)
}

func agentToolCallWithLLMFallbackMetadata(tool agentToolCall, attempt agentLLMProviderAttempt, fallbackLog []agentLLMFallbackRecord, streamed bool, turnID string) agentToolCall {
	metadata := map[string]interface{}{
		"provider": agentLLMAttemptProviderName(attempt),
		"model":    strings.TrimSpace(attempt.Request.Model),
	}
	if attempt.ProfileID != "" {
		metadata["provider_profile_id"] = attempt.ProfileID
	}
	if attempt.ProfileName != "" {
		metadata["provider_profile_name"] = attempt.ProfileName
	}
	if len(fallbackLog) > 0 {
		metadata["provider_fallback"] = true
		metadata["provider_fallback_count"] = len(fallbackLog)
		metadata["provider_attempts"] = fallbackLog
	}
	if streamed {
		metadata["runtime_streamed"] = true
		if strings.TrimSpace(turnID) != "" {
			metadata["runtime_turn_id"] = turnID
		}
	}
	return withAgentToolMetadata(tool, metadata)
}

func agentLLMToolResult(attempt agentLLMProviderAttempt, response *llm.ChatResponse, fallbackLog []agentLLMFallbackRecord, phase string) map[string]any {
	var usage llm.Usage
	if response != nil {
		usage = response.Usage
	}
	result := map[string]any{
		"provider":      agentLLMAttemptProviderName(attempt),
		"model":         strings.TrimSpace(attempt.Request.Model),
		"input_tokens":  usage.InputTokens,
		"output_tokens": usage.OutputTokens,
	}
	if phase != "" {
		result["phase"] = phase
	}
	if attempt.ProfileID != "" {
		result["provider_profile_id"] = attempt.ProfileID
	}
	if attempt.ProfileName != "" {
		result["provider_profile_name"] = attempt.ProfileName
	}
	if len(fallbackLog) > 0 {
		result["provider_fallback"] = true
		result["provider_fallback_count"] = len(fallbackLog)
		result["provider_attempts"] = fallbackLog
	}
	return result
}
