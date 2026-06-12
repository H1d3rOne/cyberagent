package runtime

// AgentLoopSessionMutationTurn builds the turn boundary for a session mutation
// request. Metadata is cloned so callers can safely reuse request-level maps.
func AgentLoopSessionMutationTurn(sessionID, taskID, runID, turnID, prompt string, kind SessionMutationKind, operationLevel string, metadata map[string]any) TurnContext {
	return AgentLoopSessionMutationTurnWithOrigin(sessionID, taskID, runID, turnID, prompt, string(kind), kind, operationLevel, metadata)
}

// AgentLoopSessionMutationTurnWithOrigin builds the turn boundary for a
// session mutation request with an explicit origin. Empty origin falls back to
// the mutation kind.
func AgentLoopSessionMutationTurnWithOrigin(sessionID, taskID, runID, turnID, prompt, origin string, kind SessionMutationKind, operationLevel string, metadata map[string]any) TurnContext {
	if origin == "" {
		origin = string(kind)
	}
	return TurnContext{
		SessionID:      sessionID,
		TaskID:         taskID,
		RunID:          runID,
		TurnID:         turnID,
		Prompt:         prompt,
		Origin:         origin,
		OperationLevel: operationLevel,
		Metadata:       CloneAgentLoopMapStringAny(metadata),
	}
}

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
	// AgentLoopSessionMutationActionMetadataKey stores an operator control action.
	AgentLoopSessionMutationActionMetadataKey = "action"
	// AgentLoopSessionMutationRunIDMetadataKey stores a control request run ID.
	AgentLoopSessionMutationRunIDMetadataKey = "run_id"
	// AgentLoopSessionMutationJobIDMetadataKey stores a control request job ID.
	AgentLoopSessionMutationJobIDMetadataKey = "job_id"
	// AgentLoopSessionMutationClearInputMetadataKey records whether queued input
	// should be cleared by a control request.
	AgentLoopSessionMutationClearInputMetadataKey = "clear_input"
	// AgentLoopSessionMutationAuthorizationIDMetadataKey stores the authorization
	// request ID being resolved.
	AgentLoopSessionMutationAuthorizationIDMetadataKey = AgentLoopAuthorizationIDMetadataKey
	// AgentLoopSessionMutationDecisionMetadataKey stores an authorization decision.
	AgentLoopSessionMutationDecisionMetadataKey = "decision"
	// AgentLoopSessionMutationOperationLevelProvidedMetadataKey records whether an
	// authorization resolution explicitly supplied an operation level.
	AgentLoopSessionMutationOperationLevelProvidedMetadataKey = "operation_level_provided"
	// AgentLoopSessionMutationMessageCountMetadataKey stores the number of
	// messages appended by an append_messages mutation.
	AgentLoopSessionMutationMessageCountMetadataKey = "message_count"
	// AgentLoopSessionMutationPlannerApplyMetadataKey records whether a planner
	// state update accompanies appended messages.
	AgentLoopSessionMutationPlannerApplyMetadataKey = "planner_apply"
	// AgentLoopSessionMutationUserOrigin is the stable origin for user prompt
	// recording mutations.
	AgentLoopSessionMutationUserOrigin = "user"
	// AgentLoopSessionMutationOperatorOrigin is the stable origin for
	// operator-driven external session mutations.
	AgentLoopSessionMutationOperatorOrigin = "operator"
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

// AgentLoopControlSessionMutationMetadata builds request metadata for a
// control_session mutation.
func AgentLoopControlSessionMutationMetadata(action, operationLevel string, clearInput bool, runID, jobID string) map[string]any {
	return AgentLoopSessionMutationMetadata(SessionMutationControlSession, map[string]any{
		AgentLoopSessionMutationActionMetadataKey:         action,
		AgentLoopSessionMutationOperationLevelMetadataKey: operationLevel,
		AgentLoopSessionMutationClearInputMetadataKey:     clearInput,
		AgentLoopSessionMutationRunIDMetadataKey:          runID,
		AgentLoopSessionMutationJobIDMetadataKey:          jobID,
	})
}

// AgentLoopResolveAuthorizationMutationMetadata builds request metadata for a
// resolve_authorization mutation.
func AgentLoopResolveAuthorizationMutationMetadata(authorizationID, decision, operationLevel string, operationLevelProvided bool) map[string]any {
	return AgentLoopSessionMutationMetadata(SessionMutationResolveAuthorization, map[string]any{
		AgentLoopSessionMutationAuthorizationIDMetadataKey:        authorizationID,
		AgentLoopSessionMutationDecisionMetadataKey:               decision,
		AgentLoopSessionMutationOperationLevelMetadataKey:         operationLevel,
		AgentLoopSessionMutationOperationLevelProvidedMetadataKey: operationLevelProvided,
	})
}

// AgentLoopAppendMessagesMutationMetadata builds request metadata for an
// append_messages mutation.
func AgentLoopAppendMessagesMutationMetadata(messageCount int, plannerApply bool, operationLevel string) map[string]any {
	metadata := map[string]any{
		AgentLoopSessionMutationMessageCountMetadataKey: messageCount,
		AgentLoopSessionMutationPlannerApplyMetadataKey: plannerApply,
	}
	if operationLevel != "" {
		metadata[AgentLoopSessionMutationOperationLevelMetadataKey] = operationLevel
	}
	return AgentLoopSessionMutationMetadata(SessionMutationAppendMessages, metadata)
}

// AgentLoopRecordUserMessageMutationMetadata builds request metadata for a
// record_user_message mutation.
func AgentLoopRecordUserMessageMutationMetadata(operationLevel string) map[string]any {
	return AgentLoopSessionMutationMetadata(SessionMutationRecordUserMessage, map[string]any{
		AgentLoopSessionMutationOperationLevelMetadataKey: operationLevel,
	})
}

// AgentLoopAppendMessagesMutationTurn builds the turn boundary for an
// append_messages mutation using the caller-supplied origin.
func AgentLoopAppendMessagesMutationTurn(sessionID, taskID, runID, turnID, prompt, origin, operationLevel string, messageCount int, plannerApply bool) TurnContext {
	return TurnContext{
		SessionID:      sessionID,
		TaskID:         taskID,
		RunID:          runID,
		TurnID:         turnID,
		Prompt:         prompt,
		Origin:         origin,
		OperationLevel: operationLevel,
		Metadata:       AgentLoopAppendMessagesMutationMetadata(messageCount, plannerApply, operationLevel),
	}
}

// AgentLoopAuthorizationRequestMutationRequest builds the runtime-neutral
// request_authorization session mutation request used by action gates.
func AgentLoopAuthorizationRequestMutationRequest(sessionID, taskID, runID, turnID, operatorPrompt, language, operationLevel string, action AgentLoopAction, actionName, requiredLevel, policy, reason string, metadata map[string]any) SessionMutationRequest {
	return SessionMutationRequest{
		Turn: AgentLoopSessionMutationTurn(
			sessionID,
			taskID,
			runID,
			turnID,
			operatorPrompt,
			SessionMutationRequestAuthorization,
			operationLevel,
			AgentLoopSessionMutationMetadata(SessionMutationRequestAuthorization),
		),
		Kind: SessionMutationRequestAuthorization,
		Payload: AgentLoopAuthorizationRequestPayload{
			Language:      language,
			Action:        action.Clone(),
			ActionName:    actionName,
			RequiredLevel: requiredLevel,
			Policy:        policy,
			Reason:        reason,
		},
		Metadata: CloneAgentLoopMapStringAny(metadata),
	}
}

// AgentLoopCapabilityBlockedMutationRequest builds the runtime-neutral
// capability_blocked session mutation request used by action gates.
func AgentLoopCapabilityBlockedMutationRequest(sessionID, taskID, runID, turnID, operatorPrompt, language, operationLevel, actionName, intentID, reason, suggestedPrompt string, metadata map[string]any) SessionMutationRequest {
	return SessionMutationRequest{
		Turn: AgentLoopSessionMutationTurn(
			sessionID,
			taskID,
			runID,
			turnID,
			operatorPrompt,
			SessionMutationCapabilityBlocked,
			operationLevel,
			metadata,
		),
		Kind: SessionMutationCapabilityBlocked,
		Payload: AgentLoopCapabilityBlockedPayload{
			Language:        language,
			ActionName:      actionName,
			IntentID:        intentID,
			Reason:          reason,
			SuggestedPrompt: suggestedPrompt,
		},
		Metadata: CloneAgentLoopMapStringAny(metadata),
	}
}

// AgentLoopControlSessionMutationRequest builds the runtime-neutral
// control_session mutation request used by external operator controls.
func AgentLoopControlSessionMutationRequest(sessionID, taskID, turnRunID, turnID, prompt, operationLevel, action, reason, controlRunID, jobID string, clearInput bool, operator, language string, metadata map[string]any) SessionMutationRequest {
	return SessionMutationRequest{
		Turn: AgentLoopSessionMutationTurnWithOrigin(
			sessionID,
			taskID,
			turnRunID,
			turnID,
			prompt,
			AgentLoopSessionMutationOperatorOrigin,
			SessionMutationControlSession,
			operationLevel,
			AgentLoopSessionMutationMetadata(SessionMutationControlSession),
		),
		Kind: SessionMutationControlSession,
		Payload: AgentLoopControlSessionPayload{
			Action:     action,
			Reason:     reason,
			RunID:      controlRunID,
			JobID:      jobID,
			ClearInput: clearInput,
			Operator:   operator,
			Language:   language,
		},
		Metadata: CloneAgentLoopMapStringAny(metadata),
	}
}

// AgentLoopResolveAuthorizationMutationRequest builds the runtime-neutral
// resolve_authorization mutation request used by external operator decisions.
func AgentLoopResolveAuthorizationMutationRequest(sessionID, taskID, turnRunID, turnID, operationLevel, decision, authorizationID, reason, operator, language string, operationLevelProvided bool, metadata map[string]any) SessionMutationRequest {
	return SessionMutationRequest{
		Turn: AgentLoopSessionMutationTurnWithOrigin(
			sessionID,
			taskID,
			turnRunID,
			turnID,
			"",
			AgentLoopSessionMutationOperatorOrigin,
			SessionMutationResolveAuthorization,
			operationLevel,
			AgentLoopSessionMutationMetadata(SessionMutationResolveAuthorization),
		),
		Kind: SessionMutationResolveAuthorization,
		Payload: AgentLoopResolveAuthorizationPayload{
			Decision:               decision,
			AuthorizationID:        authorizationID,
			Reason:                 reason,
			Operator:               operator,
			Language:               language,
			OperationLevelProvided: operationLevelProvided,
		},
		Metadata: CloneAgentLoopMapStringAny(metadata),
	}
}

// AgentLoopAppendMessagesMutationRequest builds the runtime-neutral
// append_messages mutation request. Turn and payload mutable fields are cloned.
func AgentLoopAppendMessagesMutationRequest(turn TurnContext, plan *AgentLoopPlan, messages []AgentLoopMessage, metadata map[string]any) SessionMutationRequest {
	return SessionMutationRequest{
		Turn: CloneAgentLoopTurnContext(turn),
		Kind: SessionMutationAppendMessages,
		Payload: AgentLoopAppendMessagesPayload{
			Plan:     CloneAgentLoopPlan(plan),
			Messages: CloneAgentLoopMessages(messages),
		},
		Metadata: CloneAgentLoopMapStringAny(metadata),
	}
}

// AgentLoopRecordUserMessageMutationRequest builds the runtime-neutral
// record_user_message mutation request used by user prompt ingress.
func AgentLoopRecordUserMessageMutationRequest(sessionID, prompt, operationLevel, operator, language string, metadata map[string]any) SessionMutationRequest {
	return SessionMutationRequest{
		Turn: AgentLoopSessionMutationTurnWithOrigin(
			sessionID,
			"",
			"",
			"",
			prompt,
			AgentLoopSessionMutationUserOrigin,
			SessionMutationRecordUserMessage,
			operationLevel,
			AgentLoopSessionMutationMetadata(SessionMutationRecordUserMessage),
		),
		Kind: SessionMutationRecordUserMessage,
		Payload: AgentLoopRecordUserMessagePayload{
			Operator: operator,
			Language: language,
		},
		Metadata: CloneAgentLoopMapStringAny(metadata),
	}
}
