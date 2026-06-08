package runtime

import (
	"errors"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

const (
	// AgentLoopActionResultActionKey stores the action name in action execution
	// error fallback tool results.
	AgentLoopActionResultActionKey = "action"
	// AgentLoopActionResultDetailsKey stores the original service error details
	// in action execution error fallback tool results.
	AgentLoopActionResultDetailsKey = "details"
)

// AgentLoopActionExecutionErrorResult builds the tool result map used when one
// planner action execution fails. It preserves the current daemon fallback
// semantics: the action name remains authoritative at the top level, while
// ServiceError details are nested under details and expanded onto the result
// except for a conflicting action key.
func AgentLoopActionExecutionErrorResult(actionName string, err error) map[string]any {
	result := map[string]any{
		AgentLoopActionResultActionKey: actionName,
	}
	var serviceErr *services.ServiceError
	if errors.As(err, &serviceErr) && len(serviceErr.Details) > 0 {
		details := CloneAgentLoopMapStringAny(serviceErr.Details)
		result[AgentLoopActionResultDetailsKey] = details
		for key, value := range details {
			if key == AgentLoopActionResultActionKey {
				continue
			}
			result[key] = value
		}
	}
	return result
}
