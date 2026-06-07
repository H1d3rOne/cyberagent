package runtime

import "testing"

func TestAgentLoopLLMRequestCloneIsolatesAuthProfiles(t *testing.T) {
	enabled := true
	req := AgentLoopLLMRequest{
		Origin:               "user",
		OperationLevel:       "safe_scan",
		Provider:             "openai",
		Model:                "gpt-cycle",
		APIKey:               "api-key",
		BaseURL:              "https://llm.example",
		Instructions:         "planner instructions",
		DefaultAuthProfileID: "auth-llm",
		AuthProfiles: []AgentLoopAuthProfile{{
			ID:      "auth-llm",
			Type:    "bearer",
			Token:   "secret-token",
			Enabled: &enabled,
		}},
	}

	cloned := req.Clone()
	req.AuthProfiles[0].Token = "mutated-token"
	*req.AuthProfiles[0].Enabled = false

	if cloned.Origin != "user" || cloned.OperationLevel != "safe_scan" || cloned.Provider != "openai" || cloned.Model != "gpt-cycle" || cloned.APIKey != "api-key" || cloned.BaseURL != "https://llm.example" || cloned.Instructions != "planner instructions" || cloned.DefaultAuthProfileID != "auth-llm" {
		t.Fatalf("clone changed scalar fields: %#v", cloned)
	}
	if len(cloned.AuthProfiles) != 1 || cloned.AuthProfiles[0].Token != "secret-token" {
		t.Fatalf("clone changed auth profiles: %#v", cloned.AuthProfiles)
	}
	if cloned.AuthProfiles[0].Enabled == nil || !*cloned.AuthProfiles[0].Enabled {
		t.Fatalf("clone shares or lost Enabled value: %#v", cloned.AuthProfiles[0].Enabled)
	}
	if cloned.AuthProfiles[0].Enabled == req.AuthProfiles[0].Enabled {
		t.Fatalf("clone reused Enabled pointer")
	}
}
