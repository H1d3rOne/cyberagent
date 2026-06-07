package runtime

// AgentLoopLLMRequest is the runtime-neutral LLM configuration passed through
// planner/replan loop synthesis boundaries. Prompt, language, and operator stay
// on the turn/cycle context instead of this provider request DTO.
type AgentLoopLLMRequest struct {
	Origin               string
	OperationLevel       string
	Provider             string
	Model                string
	APIKey               string
	BaseURL              string
	Instructions         string
	AuthProfiles         []AgentLoopAuthProfile
	DefaultAuthProfileID string
}

// Clone returns a copy with auth profile slices and pointer fields isolated.
func (req AgentLoopLLMRequest) Clone() AgentLoopLLMRequest {
	cloned := req
	cloned.AuthProfiles = CloneAgentLoopAuthProfiles(req.AuthProfiles)
	return cloned
}

// CloneAgentLoopLLMRequest clones an optional LLM request.
func CloneAgentLoopLLMRequest(req *AgentLoopLLMRequest) *AgentLoopLLMRequest {
	if req == nil {
		return nil
	}
	cloned := req.Clone()
	return &cloned
}
