package runtime

import "strings"

const (
	// AgentLoopActionBatchDefaultMaxActions is the default upper bound applied to
	// one planner action batch.
	AgentLoopActionBatchDefaultMaxActions = 6
	// AgentLoopActionQueueIDMetadataKey stores the queue identifier shared by all
	// tool calls produced from one planner action batch.
	AgentLoopActionQueueIDMetadataKey = "action_queue_id"
	// AgentLoopActionIndexMetadataKey stores the 1-based action position inside a batch.
	AgentLoopActionIndexMetadataKey = "action_index"
	// AgentLoopActionCountMetadataKey stores the effective action count after truncation.
	AgentLoopActionCountMetadataKey = "action_count"
	// AgentLoopActionNameMetadataKey stores the normalized action name for a tool call.
	AgentLoopActionNameMetadataKey = "action_name"
	// AgentLoopActionBatchTruncatedMetadataKey records whether a batch was bounded.
	AgentLoopActionBatchTruncatedMetadataKey = "truncated"
)

// AgentLoopActionQueueIDGenerator produces a queue ID for non-empty action batches.
type AgentLoopActionQueueIDGenerator func() string

// AgentLoopPreparedActionBatch is the side-effect-free setup for bounded
// planner action execution. It clones the action slice, applies the batch limit,
// and seeds the runtime-neutral batch result metadata that daemon adapters will
// keep appending to as actions execute.
type AgentLoopPreparedActionBatch struct {
	QueueID string
	Actions []AgentLoopAction
	Result  AgentLoopActionBatchResult
}

// Clone returns a copy with mutable actions/result fields isolated.
func (batch AgentLoopPreparedActionBatch) Clone() AgentLoopPreparedActionBatch {
	cloned := batch
	cloned.Actions = CloneAgentLoopActions(batch.Actions)
	cloned.Result = batch.Result.Clone()
	return cloned
}

// CloneAgentLoopPreparedActionBatch clones a prepared action batch.
func CloneAgentLoopPreparedActionBatch(batch AgentLoopPreparedActionBatch) AgentLoopPreparedActionBatch {
	return batch.Clone()
}

// PrepareAgentLoopActionBatch clones and bounds a planner action batch while
// preserving current daemon semantics: ActionCount is the effective count after
// truncation, and empty batches carry no queue metadata.
func PrepareAgentLoopActionBatch(actions []AgentLoopAction, queueID string, maxActions int) AgentLoopPreparedActionBatch {
	clonedActions := CloneAgentLoopActions(actions)
	result := AgentLoopActionBatchResult{ActionCount: len(clonedActions)}
	if len(clonedActions) == 0 {
		return AgentLoopPreparedActionBatch{QueueID: queueID, Actions: clonedActions, Result: result}
	}
	if maxActions > 0 && len(clonedActions) > maxActions {
		clonedActions = clonedActions[:maxActions]
		result.Truncated = true
		result.ActionCount = len(clonedActions)
	}
	result.Metadata = AgentLoopActionBatchMetadata(queueID, len(clonedActions), result.Truncated)
	return AgentLoopPreparedActionBatch{QueueID: queueID, Actions: clonedActions, Result: result}
}

// PrepareAgentLoopActionBatchWithGeneratedQueueID prepares a planner action
// batch using runtime-owned defaults. Queue IDs are generated only for non-empty
// action batches.
func PrepareAgentLoopActionBatchWithGeneratedQueueID(actions []AgentLoopAction, newQueueID AgentLoopActionQueueIDGenerator) AgentLoopPreparedActionBatch {
	queueID := ""
	if len(actions) > 0 && newQueueID != nil {
		queueID = newQueueID()
	}
	return PrepareAgentLoopActionBatch(actions, queueID, AgentLoopActionBatchDefaultMaxActions)
}

// AgentLoopActionQueueIDFromJobID converts a run/job ID into the queue ID shape
// used by planner action batches.
func AgentLoopActionQueueIDFromJobID(jobID string) string {
	return strings.Replace(jobID, "job_", "queue_", 1)
}

// AgentLoopActionBatchMetadata builds the batch-level queue metadata shared by
// action batch results.
func AgentLoopActionBatchMetadata(queueID string, count int, truncated bool) map[string]any {
	return map[string]any{
		AgentLoopActionQueueIDMetadataKey:        queueID,
		AgentLoopActionCountMetadataKey:          count,
		AgentLoopActionBatchTruncatedMetadataKey: truncated,
	}
}

// AgentLoopActionQueueMetadata builds the per-action queue metadata stamped on
// top-level tool calls and message-embedded tool calls.
func AgentLoopActionQueueMetadata(queueID string, index, count int, actionName string) map[string]any {
	return map[string]any{
		AgentLoopActionQueueIDMetadataKey: queueID,
		AgentLoopActionIndexMetadataKey:   index,
		AgentLoopActionCountMetadataKey:   count,
		AgentLoopActionNameMetadataKey:    actionName,
	}
}

// AppendAgentLoopActionBatchActionResultWithQueueMetadata stamps runtime-owned
// action queue metadata onto top-level tool calls and message-embedded tool
// calls before appending one action result to a batch accumulator.
func AppendAgentLoopActionBatchActionResultWithQueueMetadata(result *AgentLoopActionBatchResult, preparedBatch AgentLoopPreparedActionBatch, actionIndex int, actionName string, messages []AgentLoopMessage, toolCalls []AgentLoopToolCall) {
	actionMetadata := AgentLoopActionQueueMetadata(preparedBatch.QueueID, actionIndex, len(preparedBatch.Actions), actionName)
	toolCalls = AgentLoopToolCallsWithMetadata(toolCalls, actionMetadata)
	messages = AgentLoopMessagesWithToolMetadata(messages, actionMetadata)
	AppendAgentLoopActionBatchActionResult(result, actionIndex, messages, toolCalls)
}

// MergeAgentLoopMetadata clones and merges metadata maps from left to right.
// Later maps win on key conflicts; nil maps are skipped. A nil result means no
// metadata was supplied.
func MergeAgentLoopMetadata(metadataMaps ...map[string]any) map[string]any {
	var out map[string]any
	for _, metadata := range metadataMaps {
		if len(metadata) == 0 {
			continue
		}
		if out == nil {
			out = make(map[string]any, len(metadata))
		}
		for key, value := range metadata {
			out[key] = value
		}
	}
	return out
}

// AgentLoopToolCallWithMetadata clones a tool call and merges additional
// metadata onto the clone.
func AgentLoopToolCallWithMetadata(call AgentLoopToolCall, metadata map[string]any) AgentLoopToolCall {
	out := call.Clone()
	out.Metadata = MergeAgentLoopMetadata(out.Metadata, metadata)
	return out
}

// AgentLoopToolCallsWithMetadata clones tool calls and merges metadata onto
// each cloned call.
func AgentLoopToolCallsWithMetadata(calls []AgentLoopToolCall, metadata map[string]any) []AgentLoopToolCall {
	if calls == nil {
		return nil
	}
	out := CloneAgentLoopToolCalls(calls)
	for i := range out {
		out[i] = AgentLoopToolCallWithMetadata(out[i], metadata)
	}
	return out
}

// AgentLoopMessagesWithToolMetadata clones messages and merges metadata onto
// every embedded tool call.
func AgentLoopMessagesWithToolMetadata(messages []AgentLoopMessage, metadata map[string]any) []AgentLoopMessage {
	if messages == nil {
		return nil
	}
	out := CloneAgentLoopMessages(messages)
	for i := range out {
		for j := range out[i].ToolCalls {
			out[i].ToolCalls[j] = AgentLoopToolCallWithMetadata(out[i].ToolCalls[j], metadata)
		}
	}
	return out
}
