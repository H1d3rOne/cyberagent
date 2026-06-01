package planner

import (
	"encoding/json"
	"strings"
)

// RecommendationOption is a structured next-step option emitted by the planner.
type RecommendationOption struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Detail  string `json:"detail,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Command string `json:"command,omitempty"`
}

// ContextRef points planner output at run evidence, workspace files, or other
// daemon-visible context records.
type ContextRef struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	RefID   string `json:"ref_id"`
	Label   string `json:"label"`
	To      string `json:"to,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// NextAction describes the next operator-visible step after planner output.
type NextAction struct {
	Type            string `json:"type"`
	Reason          string `json:"reason,omitempty"`
	SuggestedPrompt string `json:"suggested_prompt,omitempty"`
	AutoContinue    bool   `json:"auto_continue,omitempty"`
}

func (a *NextAction) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type            string `json:"type"`
		Reason          string `json:"reason"`
		SuggestedPrompt string `json:"suggested_prompt"`
		SuggestPrompt   string `json:"suggest_prompt"`
		Prompt          string `json:"prompt"`
		AutoContinue    bool   `json:"auto_continue"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Type = raw.Type
	a.Reason = raw.Reason
	a.SuggestedPrompt = firstNonEmpty(raw.SuggestedPrompt, raw.SuggestPrompt, raw.Prompt)
	a.AutoContinue = raw.AutoContinue
	return nil
}

// Response is the LLM planner JSON envelope used by the daemon planner loop.
type Response struct {
	Phase                 string                 `json:"phase,omitempty"`
	Answer                string                 `json:"answer"`
	SuggestedPrompt       string                 `json:"suggested_prompt,omitempty"`
	SuggestPrompt         string                 `json:"suggest_prompt,omitempty"`
	SuggestedPrompts      []string               `json:"suggested_prompts,omitempty"`
	RecommendationOptions []RecommendationOption `json:"recommendation_options,omitempty"`
	Task                  *Task                  `json:"task,omitempty"`
	Todos                 []Todo                 `json:"todos,omitempty"`
	ContextRefs           []ContextRef           `json:"context_refs,omitempty"`
	NextAction            *NextAction            `json:"next_action,omitempty"`
	Reasoning             string                 `json:"reasoning,omitempty"`
	Actions               []Action               `json:"actions,omitempty"`
	ToolCalls             []Action               `json:"tool_calls,omitempty"`
}

// Action is a planner-emitted tool request. Its custom JSON decoder accepts
// common model variants: arguments, args, input, parameters, and OpenAI-style
// function envelopes.
type Action struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

func (a *Action) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name       string         `json:"name"`
		Arguments  any            `json:"arguments"`
		Args       any            `json:"args"`
		Input      any            `json:"input"`
		Parameters any            `json:"parameters"`
		Function   map[string]any `json:"function"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Name = strings.TrimSpace(raw.Name)
	args := raw.Arguments
	if args == nil {
		args = raw.Args
	}
	if args == nil {
		args = raw.Input
	}
	if args == nil {
		args = raw.Parameters
	}
	if len(raw.Function) > 0 {
		if a.Name == "" {
			a.Name = stringFromAny(raw.Function["name"])
		}
		if args == nil {
			args = raw.Function["arguments"]
		}
	}
	a.Arguments = NormalizeActionArguments(args)
	return nil
}

// Task is the planner's task summary.
type Task struct {
	Goal        string `json:"goal,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Status      string `json:"status,omitempty"`
	ActiveRunID string `json:"active_run_id,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

// Todo is one planner checklist item.
type Todo struct {
	Title         string   `json:"title"`
	Detail        string   `json:"detail,omitempty"`
	Status        string   `json:"status,omitempty"`
	Priority      int      `json:"priority,omitempty"`
	ContextRefIDs []string `json:"context_ref_ids,omitempty"`
	BlockedReason string   `json:"blocked_reason,omitempty"`
}

func NormalizeActionArguments(value any) map[string]any {
	switch v := value.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[strings.TrimSpace(key)] = val
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, val := range v {
			out[strings.TrimSpace(key)] = val
		}
		return out
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return map[string]any{}
		}
		var parsed map[string]any
		if strings.HasPrefix(trimmed, "{") && json.Unmarshal([]byte(trimmed), &parsed) == nil {
			return NormalizeActionArguments(parsed)
		}
		return map[string]any{"input": trimmed}
	default:
		return map[string]any{"input": v}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}
