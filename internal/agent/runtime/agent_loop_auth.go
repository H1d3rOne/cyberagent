package runtime

// AgentLoopAuthorization is the runtime-neutral authorization context used by
// planner loop action execution and LLM synthesis stages.
type AgentLoopAuthorization struct {
	AuthProfiles         []AgentLoopAuthProfile
	DefaultAuthProfileID string
}

// AgentLoopAuthProfile is a runtime-neutral HTTP auth profile descriptor. It
// intentionally carries the same secret-bearing fields as the daemon legacy
// auth profile and must be cloned at DTO boundaries.
type AgentLoopAuthProfile struct {
	ID          string
	Name        string
	Type        string
	BaseURL     string
	Token       string
	Username    string
	Password    string
	HeaderName  string
	HeaderValue string
	Cookie      string
	Enabled     *bool
}

// Clone returns a copy with profile slices and pointer fields isolated.
func (auth AgentLoopAuthorization) Clone() AgentLoopAuthorization {
	cloned := auth
	cloned.AuthProfiles = CloneAgentLoopAuthProfiles(auth.AuthProfiles)
	return cloned
}

// Clone returns a copy with pointer fields isolated.
func (profile AgentLoopAuthProfile) Clone() AgentLoopAuthProfile {
	cloned := profile
	cloned.Enabled = CloneAgentLoopBoolPtr(profile.Enabled)
	return cloned
}

// CloneAgentLoopAuthProfiles clones auth profile slices and their pointer fields.
func CloneAgentLoopAuthProfiles(profiles []AgentLoopAuthProfile) []AgentLoopAuthProfile {
	if profiles == nil {
		return nil
	}
	out := make([]AgentLoopAuthProfile, len(profiles))
	for i, profile := range profiles {
		out[i] = profile.Clone()
	}
	return out
}

// CloneAgentLoopAuthorization clones an optional authorization context.
func CloneAgentLoopAuthorization(auth *AgentLoopAuthorization) *AgentLoopAuthorization {
	if auth == nil {
		return nil
	}
	cloned := auth.Clone()
	return &cloned
}

// CloneAgentLoopBoolPtr clones an optional bool pointer used by runtime loop DTOs.
func CloneAgentLoopBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
