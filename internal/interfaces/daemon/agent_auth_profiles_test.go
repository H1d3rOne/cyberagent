package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

func testBoolPtr(value bool) *bool {
	return &value
}

func TestResolveStoredHTTPAuthProfilesUsesStoredProfilesWhenRequestOmitsProfiles(t *testing.T) {
	store := newProviderStore(filepath.Join(t.TempDir(), "providers.json"))
	if err := store.Save(web.ProviderConfig{
		AuthProfiles: []web.APIAuthProfile{{
			ID:      "stored-api",
			Name:    "Stored API",
			Type:    "bearer",
			BaseURL: "https://api.example.test",
			Token:   "stored-token",
			Enabled: testBoolPtr(true),
		}},
		DefaultAuthProfileID: "stored-api",
	}); err != nil {
		t.Fatal(err)
	}
	manager := &agentManager{}
	manager.AttachProviderStore(store)

	profiles, defaultID := manager.resolveStoredHTTPAuthProfiles(context.Background(), nil, "")

	if defaultID != "stored-api" {
		t.Fatalf("expected stored default auth profile id, got %q", defaultID)
	}
	if len(profiles) != 1 || profiles[0].ID != "stored-api" || profiles[0].Token != "stored-token" {
		t.Fatalf("expected stored auth profile secret to be loaded, got %#v", profiles)
	}
	if profiles[0].Enabled == nil || !*profiles[0].Enabled {
		t.Fatalf("expected enabled flag clone, got %#v", profiles[0].Enabled)
	}
}

func TestResolveStoredHTTPAuthProfilesMergesRedactedRequestByIDAndType(t *testing.T) {
	store := newProviderStore(filepath.Join(t.TempDir(), "providers.json"))
	if err := store.Save(web.ProviderConfig{
		AuthProfiles: []web.APIAuthProfile{{
			ID:          "api-key",
			Type:        "api_key",
			BaseURL:     "https://api.example.test",
			HeaderName:  "X-API-Key",
			HeaderValue: "stored-header-secret",
			Enabled:     testBoolPtr(true),
		}},
		DefaultAuthProfileID: "api-key",
	}); err != nil {
		t.Fatal(err)
	}
	manager := &agentManager{}
	manager.AttachProviderStore(store)

	profiles, defaultID := manager.resolveStoredHTTPAuthProfiles(context.Background(), []agentHTTPAuthProfile{{
		ID:         "api-key",
		Type:       "api_key",
		BaseURL:    "https://api.example.test",
		HeaderName: "X-API-Key",
		Enabled:    testBoolPtr(true),
	}}, "api-key")

	if defaultID != "api-key" {
		t.Fatalf("expected request default auth profile id, got %q", defaultID)
	}
	if len(profiles) != 1 || profiles[0].HeaderValue != "stored-header-secret" {
		t.Fatalf("expected stored header secret to merge into redacted request, got %#v", profiles)
	}
}

func TestResolveStoredHTTPAuthProfilesDoesNotMergeAcrossChangedType(t *testing.T) {
	store := newProviderStore(filepath.Join(t.TempDir(), "providers.json"))
	if err := store.Save(web.ProviderConfig{
		AuthProfiles: []web.APIAuthProfile{{
			ID:          "auth-1",
			Type:        "api_key",
			HeaderValue: "stored-header-secret",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	manager := &agentManager{}
	manager.AttachProviderStore(store)

	profiles, _ := manager.resolveStoredHTTPAuthProfiles(context.Background(), []agentHTTPAuthProfile{{
		ID:   "auth-1",
		Type: "bearer",
	}}, "")

	if len(profiles) != 1 {
		t.Fatalf("expected one request profile, got %#v", profiles)
	}
	if profiles[0].Token != "" || profiles[0].HeaderValue != "" {
		t.Fatalf("expected no stored secret to cross auth profile type, got %#v", profiles[0])
	}
}
