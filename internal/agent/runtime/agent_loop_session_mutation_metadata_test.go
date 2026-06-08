package runtime

import "testing"

func TestAgentLoopSessionMutationMetadataHelpers(t *testing.T) {
	base := AgentLoopSessionMutationMetadata(SessionMutationRequestAuthorization, map[string]any{"extra": "value"})
	if base[AgentLoopSessionMutationMutatorMetadataKey] != AgentLoopSessionMutationMutatorMetadataValue ||
		base[AgentLoopSessionMutationKindMetadataKey] != string(SessionMutationRequestAuthorization) ||
		base["extra"] != "value" {
		t.Fatalf("base metadata = %#v", base)
	}

	auth := AgentLoopAuthorizationRequestMutationMetadata("workspace.files.write", "workspace_write", "manual", "safe_scan")
	if auth[AgentLoopSessionMutationMutatorMetadataKey] != AgentLoopSessionMutationMutatorMetadataValue ||
		auth[AgentLoopSessionMutationKindMetadataKey] != string(SessionMutationRequestAuthorization) ||
		auth[AgentLoopActionNameMetadataKey] != "workspace.files.write" ||
		auth[AgentLoopSessionMutationRequiredLevelMetadataKey] != "workspace_write" ||
		auth[AgentLoopSessionMutationPolicyMetadataKey] != "manual" ||
		auth[AgentLoopSessionMutationOperationLevelMetadataKey] != "safe_scan" {
		t.Fatalf("authorization metadata = %#v", auth)
	}

	blocked := AgentLoopCapabilityBlockedMutationMetadata("scan.web.safe", "content_discovery", "Use scan.web.safe", "safe_scan")
	if blocked[AgentLoopSessionMutationKindMetadataKey] != string(SessionMutationCapabilityBlocked) ||
		blocked[AgentLoopActionNameMetadataKey] != "scan.web.safe" ||
		blocked[AgentLoopSessionMutationIntentIDMetadataKey] != "content_discovery" ||
		blocked[AgentLoopSessionMutationSuggestedPromptMetadataKey] != "Use scan.web.safe" ||
		blocked[AgentLoopSessionMutationOperationLevelMetadataKey] != "safe_scan" {
		t.Fatalf("capability metadata = %#v", blocked)
	}

	auth[AgentLoopActionNameMetadataKey] = "mutated"
	if blocked[AgentLoopActionNameMetadataKey] != "scan.web.safe" {
		t.Fatalf("metadata helpers shared maps: auth=%#v blocked=%#v", auth, blocked)
	}
}
