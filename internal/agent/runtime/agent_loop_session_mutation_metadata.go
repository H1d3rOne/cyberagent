package runtime

const (
	// AgentLoopSessionMutationMutatorMetadataKey marks a request as routed through
	// the engine session mutation adapter.
	AgentLoopSessionMutationMutatorMetadataKey = "mutator"
	// AgentLoopSessionMutationKindMetadataKey stores the session mutation kind.
	AgentLoopSessionMutationKindMetadataKey = "kind"
	// AgentLoopSessionMutationMutatorMetadataValue is the stable adapter label for
	// session mutation requests.
	AgentLoopSessionMutationMutatorMetadataValue = "session_mutation"
	// AgentLoopSessionMutationRequiredLevelMetadataKey stores the required
	// operation level for authorization requests.
	AgentLoopSessionMutationRequiredLevelMetadataKey = "required_level"
	// AgentLoopSessionMutationPolicyMetadataKey stores the authorization policy.
	AgentLoopSessionMutationPolicyMetadataKey = "policy"
	// AgentLoopSessionMutationOperationLevelMetadataKey stores the active operation level.
	AgentLoopSessionMutationOperationLevelMetadataKey = "operation_level"
	// AgentLoopSessionMutationIntentIDMetadataKey stores the blocked intent ID.
	AgentLoopSessionMutationIntentIDMetadataKey = "intent_id"
	// AgentLoopSessionMutationSuggestedPromptMetadataKey stores a safer follow-up prompt.
	AgentLoopSessionMutationSuggestedPromptMetadataKey = "suggested_prompt"
)

// AgentLoopSessionMutationMetadata builds shared metadata for runtime session
// mutation requests. Extra maps are merged after the base mutator/kind fields.
func AgentLoopSessionMutationMetadata(kind SessionMutationKind, metadataMaps ...map[string]any) map[string]any {
	base := map[string]any{
		AgentLoopSessionMutationMutatorMetadataKey: AgentLoopSessionMutationMutatorMetadataValue,
		AgentLoopSessionMutationKindMetadataKey:    string(kind),
	}
	return MergeAgentLoopMetadata(append([]map[string]any{base}, metadataMaps...)...)
}

// AgentLoopAuthorizationRequestMutationMetadata builds request metadata for a
// request_authorization session mutation.
func AgentLoopAuthorizationRequestMutationMetadata(actionName, requiredLevel, policy, operationLevel string) map[string]any {
	return AgentLoopSessionMutationMetadata(SessionMutationRequestAuthorization, map[string]any{
		AgentLoopActionNameMetadataKey:                    actionName,
		AgentLoopSessionMutationRequiredLevelMetadataKey:  requiredLevel,
		AgentLoopSessionMutationPolicyMetadataKey:         policy,
		AgentLoopSessionMutationOperationLevelMetadataKey: operationLevel,
	})
}

// AgentLoopCapabilityBlockedMutationMetadata builds request metadata for a
// capability_blocked session mutation.
func AgentLoopCapabilityBlockedMutationMetadata(actionName, intentID, suggestedPrompt, operationLevel string) map[string]any {
	return AgentLoopSessionMutationMetadata(SessionMutationCapabilityBlocked, map[string]any{
		AgentLoopActionNameMetadataKey:                     actionName,
		AgentLoopSessionMutationIntentIDMetadataKey:        intentID,
		AgentLoopSessionMutationSuggestedPromptMetadataKey: suggestedPrompt,
		AgentLoopSessionMutationOperationLevelMetadataKey:  operationLevel,
	})
}
