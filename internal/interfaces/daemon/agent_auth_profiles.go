package daemon

import (
	"context"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

func (m *agentManager) resolveStoredHTTPAuthProfiles(ctx context.Context, profiles []agentHTTPAuthProfile, defaultID string) ([]agentHTTPAuthProfile, string) {
	store := m.providerStore()
	if store == nil {
		return cloneAgentHTTPAuthProfiles(profiles), defaultID
	}
	cfg, err := loadProviderConfigForAgentAuthProfiles(ctx, store)
	if err != nil || len(cfg.AuthProfiles) == 0 {
		return cloneAgentHTTPAuthProfiles(profiles), defaultID
	}
	storedProfiles := agentHTTPAuthProfilesFromStored(cfg.AuthProfiles)
	if len(profiles) == 0 {
		return storedProfiles, firstNonEmpty(defaultID, cfg.DefaultAuthProfileID)
	}

	storedByID := make(map[string]agentHTTPAuthProfile, len(storedProfiles))
	for _, profile := range storedProfiles {
		if id := strings.ToLower(strings.TrimSpace(profile.ID)); id != "" {
			storedByID[id] = profile
		}
	}
	out := cloneAgentHTTPAuthProfiles(profiles)
	for i := range out {
		profile := &out[i]
		stored, ok := storedByID[strings.ToLower(strings.TrimSpace(profile.ID))]
		if !ok || !sameAgentHTTPAuthProfileSecretScope(stored, *profile) {
			continue
		}
		if strings.TrimSpace(profile.Token) == "" {
			profile.Token = stored.Token
		}
		if strings.TrimSpace(profile.Password) == "" {
			profile.Password = stored.Password
		}
		if strings.TrimSpace(profile.HeaderValue) == "" {
			profile.HeaderValue = stored.HeaderValue
		}
		if strings.TrimSpace(profile.Cookie) == "" {
			profile.Cookie = stored.Cookie
		}
	}
	return out, defaultID
}

func loadProviderConfigForAgentAuthProfiles(ctx context.Context, store web.ProviderStore) (web.ProviderConfig, error) {
	if scoped, ok := store.(web.ContextProviderStore); ok {
		return scoped.LoadContext(ctx)
	}
	return store.Load()
}

func agentHTTPAuthProfilesFromStored(profiles []web.APIAuthProfile) []agentHTTPAuthProfile {
	if len(profiles) == 0 {
		return nil
	}
	out := make([]agentHTTPAuthProfile, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, agentHTTPAuthProfileFromStored(profile))
	}
	return out
}

func agentHTTPAuthProfileFromStored(profile web.APIAuthProfile) agentHTTPAuthProfile {
	return agentHTTPAuthProfile{
		ID:          profile.ID,
		Name:        profile.Name,
		Type:        profile.Type,
		BaseURL:     profile.BaseURL,
		Token:       profile.Token,
		Username:    profile.Username,
		Password:    profile.Password,
		HeaderName:  profile.HeaderName,
		HeaderValue: profile.HeaderValue,
		Cookie:      profile.Cookie,
		Enabled:     cloneBoolPtr(profile.Enabled),
	}
}

func cloneAgentHTTPAuthProfiles(profiles []agentHTTPAuthProfile) []agentHTTPAuthProfile {
	if profiles == nil {
		return nil
	}
	out := make([]agentHTTPAuthProfile, len(profiles))
	for i, profile := range profiles {
		out[i] = profile
		out[i].Enabled = cloneBoolPtr(profile.Enabled)
	}
	return out
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func sameAgentHTTPAuthProfileSecretScope(existing agentHTTPAuthProfile, incoming agentHTTPAuthProfile) bool {
	if !agentHTTPAuthProfileSecretConfigured(existing) {
		return false
	}
	return normalizeAgentHTTPAuthProfileType(existing.Type) == normalizeAgentHTTPAuthProfileType(incoming.Type)
}

func agentHTTPAuthProfileSecretConfigured(profile agentHTTPAuthProfile) bool {
	switch normalizeAgentHTTPAuthProfileType(profile.Type) {
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
