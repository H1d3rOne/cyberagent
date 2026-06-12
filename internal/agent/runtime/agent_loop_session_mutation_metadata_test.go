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

func TestAgentLoopActionGateSessionMutationRequestHelpers(t *testing.T) {
	action := AgentLoopAction{Name: "workspace.files.write", Arguments: map[string]any{"path": "notes.md"}}
	authMetadata := AgentLoopAuthorizationRequestMutationMetadata("workspace.files.write", "workspace_write", "manual", "read_only")
	authReq := AgentLoopAuthorizationRequestMutationRequest(
		"session-1",
		"task-1",
		"run-1",
		"turn-1",
		"write notes",
		"en",
		"read_only",
		action,
		"workspace.files.write",
		"workspace_write",
		"manual",
		"needs approval",
		authMetadata,
	)
	action.Arguments["path"] = "mutated.md"
	authMetadata[AgentLoopActionNameMetadataKey] = "mutated"

	if authReq.Kind != SessionMutationRequestAuthorization ||
		authReq.Turn.SessionID != "session-1" ||
		authReq.Turn.TaskID != "task-1" ||
		authReq.Turn.RunID != "run-1" ||
		authReq.Turn.TurnID != "turn-1" ||
		authReq.Turn.Prompt != "write notes" ||
		authReq.Turn.Origin != string(SessionMutationRequestAuthorization) ||
		authReq.Turn.OperationLevel != "read_only" {
		t.Fatalf("authorization request turn/kind = %#v", authReq)
	}
	if authReq.Turn.Metadata[AgentLoopSessionMutationKindMetadataKey] != string(SessionMutationRequestAuthorization) ||
		authReq.Metadata[AgentLoopActionNameMetadataKey] != "workspace.files.write" {
		t.Fatalf("authorization metadata = turn %#v request %#v", authReq.Turn.Metadata, authReq.Metadata)
	}
	authPayload, ok := authReq.Payload.(AgentLoopAuthorizationRequestPayload)
	if !ok {
		t.Fatalf("authorization payload type = %T", authReq.Payload)
	}
	if authPayload.Action.Arguments["path"] != "notes.md" ||
		authPayload.ActionName != "workspace.files.write" ||
		authPayload.RequiredLevel != "workspace_write" ||
		authPayload.Policy != "manual" ||
		authPayload.Reason != "needs approval" {
		t.Fatalf("authorization payload = %#v", authPayload)
	}

	blockedMetadata := AgentLoopCapabilityBlockedMutationMetadata("scan.web.safe", "content_discovery", "use dir discovery", "safe_scan")
	blockedReq := AgentLoopCapabilityBlockedMutationRequest(
		"session-2",
		"task-2",
		"run-2",
		"turn-2",
		"run ffuf",
		"en",
		"safe_scan",
		"scan.web.safe",
		"content_discovery",
		"capability mismatch",
		"use dir discovery",
		blockedMetadata,
	)
	blockedMetadata[AgentLoopActionNameMetadataKey] = "mutated"

	if blockedReq.Kind != SessionMutationCapabilityBlocked ||
		blockedReq.Turn.Origin != string(SessionMutationCapabilityBlocked) ||
		blockedReq.Turn.Metadata[AgentLoopActionNameMetadataKey] != "scan.web.safe" ||
		blockedReq.Metadata[AgentLoopActionNameMetadataKey] != "scan.web.safe" {
		t.Fatalf("blocked request metadata/turn = %#v", blockedReq)
	}
	blockedPayload, ok := blockedReq.Payload.(AgentLoopCapabilityBlockedPayload)
	if !ok {
		t.Fatalf("blocked payload type = %T", blockedReq.Payload)
	}
	if blockedPayload.Language != "en" ||
		blockedPayload.ActionName != "scan.web.safe" ||
		blockedPayload.IntentID != "content_discovery" ||
		blockedPayload.Reason != "capability mismatch" ||
		blockedPayload.SuggestedPrompt != "use dir discovery" {
		t.Fatalf("blocked payload = %#v", blockedPayload)
	}
}

func TestAgentLoopOperatorSessionMutationRequestHelpers(t *testing.T) {
	controlMetadata := AgentLoopControlSessionMutationMetadata("pause", "safe_scan", true, "run-123", "job-456")
	controlReq := AgentLoopControlSessionMutationRequest(
		"session-control",
		"task-control",
		"turn-run-control",
		"turn-control",
		"pause after current step",
		"safe_scan",
		"pause",
		"operator pause",
		"run-123",
		"job-456",
		true,
		"operator-1",
		"zh-CN",
		controlMetadata,
	)
	controlMetadata[AgentLoopSessionMutationActionMetadataKey] = "mutated"

	if controlReq.Kind != SessionMutationControlSession ||
		controlReq.Turn.SessionID != "session-control" ||
		controlReq.Turn.TaskID != "task-control" ||
		controlReq.Turn.RunID != "turn-run-control" ||
		controlReq.Turn.TurnID != "turn-control" ||
		controlReq.Turn.Prompt != "pause after current step" ||
		controlReq.Turn.Origin != AgentLoopSessionMutationOperatorOrigin ||
		controlReq.Turn.OperationLevel != "safe_scan" {
		t.Fatalf("control request turn/kind = %#v", controlReq)
	}
	if controlReq.Turn.Metadata[AgentLoopSessionMutationKindMetadataKey] != string(SessionMutationControlSession) ||
		controlReq.Metadata[AgentLoopSessionMutationActionMetadataKey] != "pause" ||
		controlReq.Metadata[AgentLoopSessionMutationRunIDMetadataKey] != "run-123" ||
		controlReq.Metadata[AgentLoopSessionMutationJobIDMetadataKey] != "job-456" ||
		controlReq.Metadata[AgentLoopSessionMutationClearInputMetadataKey] != true {
		t.Fatalf("control metadata = turn %#v request %#v", controlReq.Turn.Metadata, controlReq.Metadata)
	}
	controlPayload, ok := controlReq.Payload.(AgentLoopControlSessionPayload)
	if !ok {
		t.Fatalf("control payload type = %T", controlReq.Payload)
	}
	if controlPayload.Action != "pause" ||
		controlPayload.Reason != "operator pause" ||
		controlPayload.RunID != "run-123" ||
		controlPayload.JobID != "job-456" ||
		!controlPayload.ClearInput ||
		controlPayload.Operator != "operator-1" ||
		controlPayload.Language != "zh-CN" {
		t.Fatalf("control payload = %#v", controlPayload)
	}

	resolveMetadata := AgentLoopResolveAuthorizationMutationMetadata("auth-1", "approve", "workspace_write", true)
	resolveReq := AgentLoopResolveAuthorizationMutationRequest(
		"session-resolve",
		"task-resolve",
		"turn-run-resolve",
		"turn-resolve",
		"workspace_write",
		"approve",
		"auth-1",
		"approved for scope",
		"operator-2",
		"en",
		true,
		resolveMetadata,
	)
	resolveMetadata[AgentLoopSessionMutationAuthorizationIDMetadataKey] = "mutated"

	if resolveReq.Kind != SessionMutationResolveAuthorization ||
		resolveReq.Turn.SessionID != "session-resolve" ||
		resolveReq.Turn.TaskID != "task-resolve" ||
		resolveReq.Turn.RunID != "turn-run-resolve" ||
		resolveReq.Turn.TurnID != "turn-resolve" ||
		resolveReq.Turn.Origin != AgentLoopSessionMutationOperatorOrigin ||
		resolveReq.Turn.OperationLevel != "workspace_write" {
		t.Fatalf("resolve request turn/kind = %#v", resolveReq)
	}
	if resolveReq.Turn.Metadata[AgentLoopSessionMutationKindMetadataKey] != string(SessionMutationResolveAuthorization) ||
		resolveReq.Metadata[AgentLoopSessionMutationAuthorizationIDMetadataKey] != "auth-1" ||
		resolveReq.Metadata[AgentLoopSessionMutationDecisionMetadataKey] != "approve" ||
		resolveReq.Metadata[AgentLoopSessionMutationOperationLevelProvidedMetadataKey] != true {
		t.Fatalf("resolve metadata = turn %#v request %#v", resolveReq.Turn.Metadata, resolveReq.Metadata)
	}
	resolvePayload, ok := resolveReq.Payload.(AgentLoopResolveAuthorizationPayload)
	if !ok {
		t.Fatalf("resolve payload type = %T", resolveReq.Payload)
	}
	if resolvePayload.Decision != "approve" ||
		resolvePayload.AuthorizationID != "auth-1" ||
		resolvePayload.Reason != "approved for scope" ||
		resolvePayload.Operator != "operator-2" ||
		resolvePayload.Language != "en" ||
		!resolvePayload.OperationLevelProvided {
		t.Fatalf("resolve payload = %#v", resolvePayload)
	}
}

func TestAgentLoopIngressSessionMutationRequestHelpers(t *testing.T) {
	appendTurn := AgentLoopAppendMessagesMutationTurn(
		"session-append",
		"task-append",
		"run-append",
		"turn-append",
		"summarize tools",
		"planner_loop",
		"safe_scan",
		1,
		true,
	)
	appendMetadata := AgentLoopAppendMessagesMutationMetadata(1, true, "")
	plan := &AgentLoopPlan{
		Phase:  "step_replan",
		Answer: "done",
		Actions: []AgentLoopAction{{
			Name:      "workspace.search",
			Arguments: map[string]any{"query": "TODO"},
		}},
	}
	messages := []AgentLoopMessage{{
		ID:      "msg-append",
		Role:    "tool",
		Content: "search result",
		ToolCalls: []AgentLoopToolCall{{
			ID:       "tool-append",
			Name:     "workspace.search",
			Result:   map[string]any{"query": "TODO"},
			Metadata: map[string]any{"phase": "tool"},
		}},
	}}
	appendReq := AgentLoopAppendMessagesMutationRequest(appendTurn, plan, messages, appendMetadata)
	appendTurn.Metadata[AgentLoopSessionMutationMessageCountMetadataKey] = 99
	appendMetadata[AgentLoopSessionMutationMessageCountMetadataKey] = 99
	plan.Actions[0].Arguments["query"] = "mutated"
	messages[0].ToolCalls[0].Result["query"] = "mutated"
	messages[0].ToolCalls[0].Metadata["phase"] = "mutated"

	if appendReq.Kind != SessionMutationAppendMessages ||
		appendReq.Turn.SessionID != "session-append" ||
		appendReq.Turn.TaskID != "task-append" ||
		appendReq.Turn.RunID != "run-append" ||
		appendReq.Turn.TurnID != "turn-append" ||
		appendReq.Turn.Prompt != "summarize tools" ||
		appendReq.Turn.Origin != "planner_loop" ||
		appendReq.Turn.OperationLevel != "safe_scan" {
		t.Fatalf("append request turn/kind = %#v", appendReq)
	}
	if appendReq.Turn.Metadata[AgentLoopSessionMutationMessageCountMetadataKey] != 1 ||
		appendReq.Turn.Metadata[AgentLoopSessionMutationPlannerApplyMetadataKey] != true ||
		appendReq.Turn.Metadata[AgentLoopSessionMutationOperationLevelMetadataKey] != "safe_scan" ||
		appendReq.Metadata[AgentLoopSessionMutationMessageCountMetadataKey] != 1 ||
		appendReq.Metadata[AgentLoopSessionMutationPlannerApplyMetadataKey] != true {
		t.Fatalf("append metadata = turn %#v request %#v", appendReq.Turn.Metadata, appendReq.Metadata)
	}
	appendPayload, ok := appendReq.Payload.(AgentLoopAppendMessagesPayload)
	if !ok {
		t.Fatalf("append payload type = %T", appendReq.Payload)
	}
	if appendPayload.Plan == nil ||
		appendPayload.Plan.Actions[0].Arguments["query"] != "TODO" ||
		appendPayload.Messages[0].ToolCalls[0].Result["query"] != "TODO" ||
		appendPayload.Messages[0].ToolCalls[0].Metadata["phase"] != "tool" {
		t.Fatalf("append payload not cloned = %#v", appendPayload)
	}

	recordMetadata := AgentLoopRecordUserMessageMutationMetadata("read_only")
	recordReq := AgentLoopRecordUserMessageMutationRequest(
		"session-record",
		"hello",
		"read_only",
		"operator-1",
		"zh-CN",
		recordMetadata,
	)
	recordMetadata[AgentLoopSessionMutationOperationLevelMetadataKey] = "mutated"

	if recordReq.Kind != SessionMutationRecordUserMessage ||
		recordReq.Turn.SessionID != "session-record" ||
		recordReq.Turn.Prompt != "hello" ||
		recordReq.Turn.Origin != AgentLoopSessionMutationUserOrigin ||
		recordReq.Turn.OperationLevel != "read_only" ||
		recordReq.Turn.Metadata[AgentLoopSessionMutationKindMetadataKey] != string(SessionMutationRecordUserMessage) ||
		recordReq.Metadata[AgentLoopSessionMutationOperationLevelMetadataKey] != "read_only" {
		t.Fatalf("record request metadata/turn = %#v", recordReq)
	}
	recordPayload, ok := recordReq.Payload.(AgentLoopRecordUserMessagePayload)
	if !ok {
		t.Fatalf("record payload type = %T", recordReq.Payload)
	}
	if recordPayload.Operator != "operator-1" || recordPayload.Language != "zh-CN" {
		t.Fatalf("record payload = %#v", recordPayload)
	}
}
