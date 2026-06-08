package runtime

import "testing"

func TestAgentLoopAuthorizationCloneIsolatesAuthProfiles(t *testing.T) {
	enabled := true
	auth := AgentLoopAuthorization{
		AuthProfiles: []AgentLoopAuthProfile{{
			ID:          "auth-1",
			Name:        "Primary auth",
			Type:        "bearer",
			BaseURL:     "https://sandbox.example",
			Token:       "secret-token",
			Username:    "agent-user",
			Password:    "agent-pass",
			HeaderName:  "X-Agent-Auth",
			HeaderValue: "header-secret",
			Cookie:      "session=secret",
			Enabled:     &enabled,
		}},
		DefaultAuthProfileID: "auth-1",
	}

	cloned := auth.Clone()
	auth.AuthProfiles[0].Token = "mutated-token"
	*auth.AuthProfiles[0].Enabled = false

	if cloned.DefaultAuthProfileID != "auth-1" || len(cloned.AuthProfiles) != 1 {
		t.Fatalf("clone lost auth profile metadata: %#v", cloned)
	}
	profile := cloned.AuthProfiles[0]
	if profile.ID != "auth-1" || profile.Name != "Primary auth" || profile.Type != "bearer" || profile.BaseURL != "https://sandbox.example" || profile.Token != "secret-token" || profile.Username != "agent-user" || profile.Password != "agent-pass" || profile.HeaderName != "X-Agent-Auth" || profile.HeaderValue != "header-secret" || profile.Cookie != "session=secret" {
		t.Fatalf("clone changed auth profile fields: %#v", profile)
	}
	if profile.Enabled == nil || !*profile.Enabled {
		t.Fatalf("clone shares or lost Enabled value: %#v", profile.Enabled)
	}
	if profile.Enabled == auth.AuthProfiles[0].Enabled {
		t.Fatalf("clone reused Enabled pointer")
	}
}

func TestAgentLoopAuthorizationActionMetadata(t *testing.T) {
	metadata := AgentLoopAuthorizationActionMetadata("auth-1", "approved", "daemon.health")
	if metadata[AgentLoopAuthorizationIDMetadataKey] != "auth-1" {
		t.Fatalf("authorization_id = %#v, want auth-1", metadata[AgentLoopAuthorizationIDMetadataKey])
	}
	if metadata[AgentLoopAuthorizationStatusMetadataKey] != "approved" {
		t.Fatalf("authorization_status = %#v, want approved", metadata[AgentLoopAuthorizationStatusMetadataKey])
	}
	if metadata[AgentLoopActionNameMetadataKey] != "daemon.health" {
		t.Fatalf("action_name = %#v, want daemon.health", metadata[AgentLoopActionNameMetadataKey])
	}
}
