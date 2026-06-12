package daemon

import (
	"context"
	"errors"
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
)

type daemonAgentLLMStageRunner struct {
	manager *agentManager
}

type daemonLLMStageCallerFallbackReason string

const (
	daemonLLMStageCallerFallbackEngineNotAttached daemonLLMStageCallerFallbackReason = "runtime_engine_not_attached"
	daemonLLMStageCallerFallbackRunnerNotAttached daemonLLMStageCallerFallbackReason = "llm_stage_runner_not_attached"
)

const daemonLLMStageCallerFallbackOwnerLegacyCaller = "legacy_caller"

type daemonLLMStageCallerFallbackOwnershipEntry struct {
	reason daemonLLMStageCallerFallbackReason
	owner  string
}

type daemonLLMStageCallerFallbackBoundaryEntry struct {
	stage          runtime.LLMStage
	callerSurface  string
	legacyFallback string
	owner          string
	scope          string
}

type daemonLLMStageRunnerNotAttachedRetirementStage string

const (
	daemonLLMStageRunnerNotAttachedStageNoEngineFallbackRetained daemonLLMStageRunnerNotAttachedRetirementStage = "no_engine_llm_stage_legacy_fallback_retained"
	daemonLLMStageRunnerNotAttachedStageMissingRunnerBlocked     daemonLLMStageRunnerNotAttachedRetirementStage = "attached_engine_missing_llm_stage_runner_blocked"
	daemonLLMStageRunnerNotAttachedStageDetachedResultBlocked    daemonLLMStageRunnerNotAttachedRetirementStage = "detached_llm_stage_runner_result_blocked"
	daemonLLMStageRunnerNotAttachedStageNextBoundaryChosen       daemonLLMStageRunnerNotAttachedRetirementStage = "llm_turn_runner_compatibility_boundary_chosen"
)

const (
	daemonLLMStageRunnerNotAttachedDispositionLegacyFallback    = "legacy_fallback"
	daemonLLMStageRunnerNotAttachedDispositionBlockedError      = "blocked_error"
	daemonLLMStageRunnerNotAttachedDispositionAuditNextBoundary = "audit_next_boundary"
)

const daemonLLMStageNextFallbackRetirementBoundaryLLMTurnRunnerCompatibility = "llm_turn_runner_compatibility_trampoline_retirement_readiness"

type daemonLLMStageRunnerNotAttachedRetirementEntry struct {
	component    string
	stage        daemonLLMStageRunnerNotAttachedRetirementStage
	reason       daemonLLMStageCallerFallbackReason
	disposition  string
	evidence     string
	nextBoundary string
	nextStep     string
}

var daemonLLMStageCallerFallbackOwnershipTable = [...]daemonLLMStageCallerFallbackOwnershipEntry{
	{reason: daemonLLMStageCallerFallbackEngineNotAttached, owner: daemonLLMStageCallerFallbackOwnerLegacyCaller},
}

var daemonLLMStageCallerFallbackBoundaryTable = [...]daemonLLMStageCallerFallbackBoundaryEntry{
	{
		stage:          runtime.LLMStagePlannerChat,
		callerSurface:  "runRuntimeAgentLLMPlannerChat -> runRuntimeAgentLLMStage",
		legacyFallback: "handleAgentLLMChat",
		owner:          daemonLLMStageCallerFallbackOwnerLegacyCaller,
		scope:          "planner_chat compatibility fallback only when runtime engine is not attached",
	},
	{
		stage:          runtime.LLMStageToolSynthesis,
		callerSurface:  "runRuntimeAgentLLMToolSynthesis -> runRuntimeAgentLLMStage",
		legacyFallback: "synthesizeAgentToolResults",
		owner:          daemonLLMStageCallerFallbackOwnerLegacyCaller,
		scope:          "tool_synthesis compatibility fallback for planner_action_replan_cycle and LLM follow-up callers only when runtime engine is not attached",
	},
}

var daemonLLMStageRunnerNotAttachedRetirementTable = [...]daemonLLMStageRunnerNotAttachedRetirementEntry{
	{
		component:    "no_engine_llm_stage_legacy_fallback",
		stage:        daemonLLMStageRunnerNotAttachedStageNoEngineFallbackRetained,
		reason:       daemonLLMStageCallerFallbackEngineNotAttached,
		disposition:  daemonLLMStageRunnerNotAttachedDispositionLegacyFallback,
		evidence:     "no-engine LLM stage fallback remains scoped to the documented CYBERAGENT_RUNTIME legacy opt-out path, matching the N215/N216 no-engine fallback audits",
		nextBoundary: daemonLLMStageNextFallbackRetirementBoundaryLLMTurnRunnerCompatibility,
		nextStep:     "do not retire the no-engine LLM stage fallback until the explicit legacy runtime opt-out switch is removed",
	},
	{
		component:    "attached_engine_missing_llm_stage_runner",
		stage:        daemonLLMStageRunnerNotAttachedStageMissingRunnerBlocked,
		reason:       daemonLLMStageCallerFallbackRunnerNotAttached,
		disposition:  daemonLLMStageRunnerNotAttachedDispositionBlockedError,
		evidence:     "agentManager.AttachEngine attaches daemonAgentLLMStageRunner, so a nil LLMStageRunner on an attached engine represents an explicit detach/miswire and now returns runtime.ErrLLMStageRunnerNotAttached instead of planner_chat/tool_synthesis legacy fallback",
		nextBoundary: daemonLLMStageNextFallbackRetirementBoundaryLLMTurnRunnerCompatibility,
		nextStep:     "continue with deprecated LLMTurnRunner compatibility trampoline retirement readiness",
	},
	{
		component:    "detached_llm_stage_runner_result",
		stage:        daemonLLMStageRunnerNotAttachedStageDetachedResultBlocked,
		reason:       daemonLLMStageCallerFallbackRunnerNotAttached,
		disposition:  daemonLLMStageRunnerNotAttachedDispositionBlockedError,
		evidence:     "ErrLLMStageRunnerNotAttached returned by Engine.RunLLMStage is normalized to blocked/error and no longer falls through to planner_chat/tool_synthesis legacy fallback",
		nextBoundary: daemonLLMStageNextFallbackRetirementBoundaryLLMTurnRunnerCompatibility,
		nextStep:     "continue with deprecated LLMTurnRunner compatibility trampoline retirement readiness",
	},
	{
		component:    "llm_stage_next_fallback_boundary",
		stage:        daemonLLMStageRunnerNotAttachedStageNextBoundaryChosen,
		disposition:  daemonLLMStageRunnerNotAttachedDispositionAuditNextBoundary,
		evidence:     "LLMStage legacy fallback is now limited to no-engine legacy opt-out; attached-engine runner nil/ErrLLMStageRunnerNotAttached is blocked/error",
		nextBoundary: daemonLLMStageNextFallbackRetirementBoundaryLLMTurnRunnerCompatibility,
		nextStep:     "P0.5-N218 should decide whether the deprecated LLMTurnRunner compatibility trampoline can be retired or further narrowed now that AgentLoopRunner owns the LLM tool loop path",
	},
}

func daemonLLMStageCallerFallbackOwnerForReason(reason daemonLLMStageCallerFallbackReason) (string, bool) {
	for _, entry := range daemonLLMStageCallerFallbackOwnershipTable {
		if entry.reason == reason {
			return entry.owner, true
		}
	}
	return "", false
}

func daemonLLMStageCallerFallbackOwnedByLegacyCaller(reason daemonLLMStageCallerFallbackReason) bool {
	owner, ok := daemonLLMStageCallerFallbackOwnerForReason(reason)
	return ok && owner == daemonLLMStageCallerFallbackOwnerLegacyCaller
}

func daemonLLMStageCallerFallbackOwnershipError(reason daemonLLMStageCallerFallbackReason) error {
	return fmt.Errorf("daemon: llm stage fallback ownership missing for %s", reason)
}

func daemonLLMStageCallerPreDispatchFallbackReason(eng *runtime.Engine) (daemonLLMStageCallerFallbackReason, bool) {
	if eng == nil {
		return daemonLLMStageCallerFallbackEngineNotAttached, true
	}
	if eng.LLMStageRunner() == nil {
		return daemonLLMStageCallerFallbackRunnerNotAttached, true
	}
	return "", false
}

func daemonLLMStageCallerResultFallbackReason(result runtime.LLMStageResult, resultErr error) (daemonLLMStageCallerFallbackReason, bool) {
	if errors.Is(resultErr, runtime.ErrLLMStageRunnerNotAttached) ||
		errors.Is(result.Error, runtime.ErrLLMStageRunnerNotAttached) {
		return daemonLLMStageCallerFallbackRunnerNotAttached, true
	}
	return "", false
}

func daemonLLMStageCallerFallbackBoundaryForStage(stage runtime.LLMStage) (daemonLLMStageCallerFallbackBoundaryEntry, bool) {
	for _, entry := range daemonLLMStageCallerFallbackBoundaryTable {
		if entry.stage == stage {
			return entry, true
		}
	}
	return daemonLLMStageCallerFallbackBoundaryEntry{}, false
}

func daemonLLMStageCallerRunnerNotAttachedBlockedResult(stage runtime.LLMStage, turn runtime.TurnContext, result runtime.LLMStageResult, resultErr error) (runtime.LLMStageResult, bool) {
	blockedErr := error(nil)
	if errors.Is(resultErr, runtime.ErrLLMStageRunnerNotAttached) {
		blockedErr = resultErr
	} else if errors.Is(result.Error, runtime.ErrLLMStageRunnerNotAttached) {
		blockedErr = result.Error
	}
	if blockedErr == nil {
		return runtime.LLMStageResult{}, false
	}
	blocked := result
	blocked.Status = runtime.TurnResultBlocked
	if blocked.Reason == "" {
		blocked.Reason = blockedErr.Error()
	}
	blocked.Error = blockedErr
	data := runtime.CloneAgentLoopMapStringAny(blocked.Data)
	if data == nil {
		data = map[string]any{}
	}
	if stage != "" {
		data["stage"] = string(stage)
	}
	if turn.SessionID != "" {
		data["session_id"] = turn.SessionID
	}
	if turn.TaskID != "" {
		data["task_id"] = turn.TaskID
	}
	if turn.RunID != "" {
		data["run_id"] = turn.RunID
	}
	if turn.TurnID != "" {
		data["turn_id"] = turn.TurnID
	}
	data["error"] = blockedErr.Error()
	data["blocked_reason"] = string(daemonLLMStageCallerFallbackRunnerNotAttached)
	blocked.Data = data
	return blocked, true
}

func daemonLLMStageRunnerNotAttachedRetirementEntryForComponent(component string) (daemonLLMStageRunnerNotAttachedRetirementEntry, bool) {
	for _, entry := range daemonLLMStageRunnerNotAttachedRetirementTable {
		if entry.component == component {
			return entry, true
		}
	}
	return daemonLLMStageRunnerNotAttachedRetirementEntry{}, false
}

func daemonLLMStageRunnerNotAttachedRetirementEntriesForStage(stage daemonLLMStageRunnerNotAttachedRetirementStage) []daemonLLMStageRunnerNotAttachedRetirementEntry {
	var entries []daemonLLMStageRunnerNotAttachedRetirementEntry
	for _, entry := range daemonLLMStageRunnerNotAttachedRetirementTable {
		if entry.stage == stage {
			entries = append(entries, entry)
		}
	}
	return entries
}

func daemonLLMStageRunnerNotAttachedRetirementSelectedFallbackBoundary() (string, bool) {
	entries := daemonLLMStageRunnerNotAttachedRetirementEntriesForStage(daemonLLMStageRunnerNotAttachedStageNextBoundaryChosen)
	if len(entries) != 1 || entries[0].nextBoundary == "" {
		return "", false
	}
	return entries[0].nextBoundary, true
}

func daemonLLMStageRunnerNotAttachedRetirementAllowsLegacyFallback(reason daemonLLMStageCallerFallbackReason) bool {
	for _, entry := range daemonLLMStageRunnerNotAttachedRetirementTable {
		if entry.reason == reason &&
			entry.disposition == daemonLLMStageRunnerNotAttachedDispositionLegacyFallback {
			return true
		}
	}
	return false
}

func (r daemonAgentLLMStageRunner) RunLLMStage(ctx context.Context, req runtime.LLMStageRequest) (runtime.LLMStageResult, error) {
	if r.manager == nil {
		err := errors.New("daemon: llm stage runner has no manager")
		return daemonLLMStageErrorResult(req, err), err
	}

	switch req.Stage {
	case runtime.LLMStagePlannerChat:
		payload, ok := agentLoopLLMStagePayloadFromLLMStageRequest(req.Payload)
		if !ok {
			err := errors.New("daemon: invalid llm planner chat payload")
			return daemonLLMStageErrorResult(req, err), err
		}
		messages, toolCalls, plan, err := r.manager.handleAgentLLMChat(
			ctx,
			payload.Turn.SessionID,
			agentRuntimeTurnRefFromTurn(payload.Turn),
			payload.Language,
			sendMessageRequestFromAgentLoopLLMRequest(payload.LLMRequest),
			agentMessagesFromAgentLoopMessages(payload.History),
			agentMemorySummaryPtrFromAgentLoopMemorySummary(payload.MemorySummary),
		)
		status := runtime.TurnResultFinished
		reason := "legacy daemon LLM planner chat executed"
		if err != nil {
			status = runtime.TurnResultFailed
			reason = err.Error()
		}
		return runtime.LLMStageResult{
			Status:  status,
			Reason:  reason,
			Error:   err,
			Payload: agentLoopLLMStageResultPayloadFromLegacy(messages, toolCalls, plan),
			Data:    daemonLLMStageResultData(req.Stage, payload.Turn),
		}, err
	case runtime.LLMStageToolSynthesis:
		payload, ok := agentLoopLLMStagePayloadFromLLMStageRequest(req.Payload)
		if !ok {
			err := errors.New("daemon: invalid llm tool synthesis payload")
			return daemonLLMStageErrorResult(req, err), err
		}
		messages, toolCalls, plan, err := r.manager.synthesizeAgentToolResults(
			ctx,
			payload.Turn.SessionID,
			agentRuntimeTurnRefFromTurn(payload.Turn),
			payload.Language,
			sendMessageRequestFromAgentLoopLLMRequest(payload.LLMRequest),
			agentMessagesFromAgentLoopMessages(payload.History),
			agentMemorySummaryPtrFromAgentLoopMemorySummary(payload.MemorySummary),
			agentMessagesFromAgentLoopMessages(payload.ToolMessages),
		)
		status := runtime.TurnResultFinished
		reason := "legacy daemon LLM tool synthesis executed"
		if err != nil {
			status = runtime.TurnResultFailed
			reason = err.Error()
		}
		return runtime.LLMStageResult{
			Status:  status,
			Reason:  reason,
			Error:   err,
			Payload: agentLoopLLMStageResultPayloadFromLegacy(messages, toolCalls, plan),
			Data:    daemonLLMStageResultData(req.Stage, payload.Turn),
		}, err
	default:
		err := fmt.Errorf("daemon: unsupported llm stage %q", req.Stage)
		return daemonLLMStageErrorResult(req, err), err
	}
}

func daemonLLMStageErrorResult(req runtime.LLMStageRequest, err error) runtime.LLMStageResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return runtime.LLMStageResult{
		Status: runtime.TurnResultFailed,
		Reason: reason,
		Error:  err,
		Data:   daemonLLMStageResultData(req.Stage, req.Turn),
	}
}

func daemonLLMStageResultData(stage runtime.LLMStage, turn runtime.TurnContext) map[string]any {
	return map[string]any{
		"runner":     "daemon_legacy_llm_stage",
		"stage":      string(stage),
		"session_id": turn.SessionID,
		"task_id":    turn.TaskID,
		"run_id":     turn.RunID,
		"turn_id":    turn.TurnID,
	}
}

func (m *agentManager) runRuntimeAgentLLMPlannerChat(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, *runtime.AgentLoopPlan, error) {
	payload, err := m.runRuntimeAgentLLMStage(ctx, runtime.LLMStagePlannerChat, agentLLMStageDispatchInput{
		SessionID:      sessionID,
		TurnRef:        turnRef,
		Prompt:         prompt,
		Language:       language,
		OperationLevel: operationLevel,
		Request:        req,
		History:        history,
		MemorySummary:  memorySummary,
	}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
		messages, toolCalls, plan, err := m.handleAgentLLMChat(ctx, sessionID, turnRef, language, req, history, memorySummary)
		return messages, toolCalls, plan, err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return runtime.CloneAgentLoopMessages(payload.Messages), runtime.CloneAgentLoopToolCalls(payload.ToolCalls), runtime.CloneAgentLoopPlan(payload.Plan), nil
}

func (m *agentManager) runRuntimeAgentLLMToolSynthesis(ctx context.Context, sessionID string, turnRef agentRuntimeTurnRef, prompt, language, operationLevel string, req sendAgentMessageRequest, history []agentMessage, memorySummary *agentMemorySummary, toolMessages []runtime.AgentLoopMessage) ([]runtime.AgentLoopMessage, []runtime.AgentLoopToolCall, *runtime.AgentLoopPlan, error) {
	toolMessages = runtime.CloneAgentLoopMessages(toolMessages)
	payload, err := m.runRuntimeAgentLLMStage(ctx, runtime.LLMStageToolSynthesis, agentLLMStageDispatchInput{
		SessionID:      sessionID,
		TurnRef:        turnRef,
		Prompt:         prompt,
		Language:       language,
		OperationLevel: operationLevel,
		Request:        req,
		History:        history,
		MemorySummary:  memorySummary,
		ToolMessages:   toolMessages,
	}, func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error) {
		messages, toolCalls, plan, err := m.synthesizeAgentToolResults(ctx, sessionID, turnRef, language, req, history, memorySummary, agentMessagesFromAgentLoopMessages(toolMessages))
		return messages, toolCalls, plan, err
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return runtime.CloneAgentLoopMessages(payload.Messages), runtime.CloneAgentLoopToolCalls(payload.ToolCalls), runtime.CloneAgentLoopPlan(payload.Plan), nil
}

type agentLLMStageDispatchInput struct {
	SessionID      string
	TurnRef        agentRuntimeTurnRef
	Prompt         string
	Language       string
	OperationLevel string
	Request        sendAgentMessageRequest
	History        []agentMessage
	MemorySummary  *agentMemorySummary
	ToolMessages   []runtime.AgentLoopMessage
}

func (m *agentManager) runRuntimeAgentLLMStage(ctx context.Context, stage runtime.LLMStage, input agentLLMStageDispatchInput, fallback func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error)) (runtime.AgentLoopLLMStageResultPayload, error) {
	turn := agentLLMStageTurnContext(stage, input)
	eng := m.runtimeEngine()
	if reason, ok := daemonLLMStageCallerPreDispatchFallbackReason(eng); ok {
		if !daemonLLMStageRunnerNotAttachedRetirementAllowsLegacyFallback(reason) {
			blocked, _ := daemonLLMStageCallerRunnerNotAttachedBlockedResult(stage, turn, runtime.LLMStageResult{}, runtime.ErrLLMStageRunnerNotAttached)
			return runtime.AgentLoopLLMStageResultPayload{}, blocked.Error
		}
		if !daemonLLMStageCallerFallbackOwnedByLegacyCaller(reason) {
			return runtime.AgentLoopLLMStageResultPayload{}, daemonLLMStageCallerFallbackOwnershipError(reason)
		}
		return agentLoopLLMStageResultPayloadFromLegacyFallback(fallback)
	}

	result, err := eng.RunLLMStage(ctx, runtime.LLMStageRequest{
		Turn:     turn,
		Stage:    stage,
		Payload:  agentLLMStagePayloadFromDispatchInput(turn, input),
		Metadata: agentLLMStageMetadata(stage),
	})
	if blocked, ok := daemonLLMStageCallerRunnerNotAttachedBlockedResult(stage, turn, result, err); ok {
		return runtime.AgentLoopLLMStageResultPayload{}, blocked.Error
	}
	if reason, ok := daemonLLMStageCallerResultFallbackReason(result, err); ok {
		if !daemonLLMStageCallerFallbackOwnedByLegacyCaller(reason) {
			return runtime.AgentLoopLLMStageResultPayload{}, daemonLLMStageCallerFallbackOwnershipError(reason)
		}
		return agentLoopLLMStageResultPayloadFromLegacyFallback(fallback)
	}

	payload, ok := agentLoopLLMStageResultPayloadFromLLMStageResult(result.Payload)
	if ok {
		return payload, err
	}
	if err != nil {
		return runtime.AgentLoopLLMStageResultPayload{}, err
	}
	return runtime.AgentLoopLLMStageResultPayload{}, fmt.Errorf("daemon: llm %s runner returned invalid payload", stage)
}

func agentLLMStageTurnContext(stage runtime.LLMStage, input agentLLMStageDispatchInput) runtime.TurnContext {
	return runtime.TurnContext{
		SessionID:      input.SessionID,
		TaskID:         input.TurnRef.TaskID,
		RunID:          input.TurnRef.RunID,
		TurnID:         input.TurnRef.TurnID,
		Prompt:         input.Prompt,
		Origin:         input.Request.Origin,
		OperationLevel: input.OperationLevel,
		Metadata:       agentLLMStageMetadata(stage),
	}
}

func agentLLMStagePayloadFromDispatchInput(turn runtime.TurnContext, input agentLLMStageDispatchInput) runtime.AgentLoopLLMStagePayload {
	return runtime.AgentLoopLLMStagePayload{
		Turn:          runtime.CloneAgentLoopTurnContext(turn),
		Language:      input.Language,
		LLMRequest:    agentPlannerActionReplanCycleLLMRequestFromMessageRequest(input.Request),
		History:       agentLoopMessagesFromAgentMessages(input.History),
		MemorySummary: agentLoopMemorySummaryPtrFromAgentMemorySummary(input.MemorySummary),
		ToolMessages:  runtime.CloneAgentLoopMessages(input.ToolMessages),
	}
}

func agentLLMStageMetadata(stage runtime.LLMStage) map[string]any {
	return map[string]any{
		"runner": "llm_stage",
		"stage":  string(stage),
	}
}

func agentLoopLLMStageResultPayloadFromLegacyFallback(fallback func() ([]agentMessage, []agentToolCall, *agentPlannerResponse, error)) (runtime.AgentLoopLLMStageResultPayload, error) {
	if fallback == nil {
		return runtime.AgentLoopLLMStageResultPayload{}, errors.New("daemon: llm stage fallback is not configured")
	}
	messages, toolCalls, plan, err := fallback()
	return agentLoopLLMStageResultPayloadFromLegacy(messages, toolCalls, plan), err
}

func agentLoopLLMStagePayloadFromLLMStageRequest(payload any) (runtime.AgentLoopLLMStagePayload, bool) {
	switch value := payload.(type) {
	case runtime.AgentLoopLLMStagePayload:
		return value.Clone(), true
	case *runtime.AgentLoopLLMStagePayload:
		if value == nil {
			return runtime.AgentLoopLLMStagePayload{}, false
		}
		return value.Clone(), true
	default:
		return runtime.AgentLoopLLMStagePayload{}, false
	}
}

func agentLoopLLMStageResultPayloadFromLLMStageResult(payload any) (runtime.AgentLoopLLMStageResultPayload, bool) {
	switch value := payload.(type) {
	case runtime.AgentLoopLLMStageResultPayload:
		return value.Clone(), true
	case *runtime.AgentLoopLLMStageResultPayload:
		if value == nil {
			return runtime.AgentLoopLLMStageResultPayload{}, false
		}
		return value.Clone(), true
	default:
		return runtime.AgentLoopLLMStageResultPayload{}, false
	}
}

func agentLoopLLMStageResultPayloadFromLegacy(messages []agentMessage, toolCalls []agentToolCall, plan *agentPlannerResponse) runtime.AgentLoopLLMStageResultPayload {
	return runtime.AgentLoopLLMStageResultPayload{
		Messages:  agentLoopMessagesFromAgentMessages(messages),
		ToolCalls: agentLoopToolCallItemsFromAgentToolCalls(toolCalls),
		Plan:      agentLoopPlanPtrFromAgentPlannerResponse(plan),
	}
}

func agentLoopPlanPtrFromAgentPlannerResponse(plan *agentPlannerResponse) *runtime.AgentLoopPlan {
	if plan == nil {
		return nil
	}
	converted := agentPlannerActionReplanCyclePlanItemFromAgentPlannerResponse(*plan)
	return &converted
}

func agentPlannerResponsePtrFromAgentLoopPlan(plan *runtime.AgentLoopPlan) *agentPlannerResponse {
	if plan == nil {
		return nil
	}
	return agentPlannerResponseFromAgentLoopPlan(*plan)
}
