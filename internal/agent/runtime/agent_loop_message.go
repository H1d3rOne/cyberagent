package runtime

import "time"

// AgentLoopMessage is the runtime-neutral representation of a message carried
// by agent loop response/history state. Daemon-specific session structs convert
// to and from this DTO at legacy boundaries.
type AgentLoopMessage struct {
	ID                    string
	Role                  string
	Content               string
	Timestamp             time.Time
	Phase                 string
	ToolCalls             []AgentLoopToolCall
	Links                 []AgentLoopLink
	Reasoning             string
	SuggestedPrompts      []string
	RecommendationOptions []AgentLoopRecommendationOption
}

// AgentLoopLink points a message at a related run, artifact, file, or route.
type AgentLoopLink struct {
	To    string
	Label string
	Kind  string
}

// Clone returns a deep-enough copy for runtime loop DTO boundaries. Tool calls
// and slices are cloned, while map values inside tool calls remain top-level
// shallow clones through AgentLoopToolCall.Clone.
func (message AgentLoopMessage) Clone() AgentLoopMessage {
	cloned := message
	cloned.ToolCalls = CloneAgentLoopToolCalls(message.ToolCalls)
	cloned.Links = CloneAgentLoopLinks(message.Links)
	cloned.SuggestedPrompts = append([]string(nil), message.SuggestedPrompts...)
	cloned.RecommendationOptions = CloneAgentLoopRecommendationOptions(message.RecommendationOptions)
	return cloned
}

// CloneAgentLoopMessages clones message slices and their mutable nested fields.
func CloneAgentLoopMessages(messages []AgentLoopMessage) []AgentLoopMessage {
	if messages == nil {
		return nil
	}
	out := make([]AgentLoopMessage, len(messages))
	for i, message := range messages {
		out[i] = message.Clone()
	}
	return out
}

// CloneAgentLoopLinks clones message link slices.
func CloneAgentLoopLinks(links []AgentLoopLink) []AgentLoopLink {
	if links == nil {
		return nil
	}
	out := make([]AgentLoopLink, len(links))
	copy(out, links)
	return out
}
