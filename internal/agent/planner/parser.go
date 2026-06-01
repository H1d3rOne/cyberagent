package planner

import (
	"encoding/json"
	"fmt"
	"strings"
)

// HasActions reports whether a parsed planner response has executable actions
// after daemon-specific normalization/deduplication.
type HasActions func(*Response) bool

// ParseResponse extracts and decodes the first usable planner JSON response
// from raw model text, OpenAI-compatible envelopes, or JSON-encoded strings.
func ParseResponse(raw string, hasActions HasActions) (*Response, error) {
	candidates := extractJSONCandidates(raw)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("planner response does not contain a JSON object")
	}

	var lastErr error
	for _, candidate := range candidates {
		if content, ok := unwrapOpenAIChatCompletionContent(candidate); ok {
			plan, err := ParseResponse(content, hasActions)
			if err == nil {
				return plan, nil
			}
			trimmed := strings.TrimSpace(content)
			if trimmed != "" {
				return &Response{Answer: trimmed}, nil
			}
			lastErr = err
			continue
		}

		var encodedContent string
		if err := json.Unmarshal([]byte(candidate), &encodedContent); err == nil {
			encodedContent = strings.TrimSpace(encodedContent)
			if encodedContent == "" {
				lastErr = fmt.Errorf("planner response string is empty")
				continue
			}
			plan, err := ParseResponse(encodedContent, hasActions)
			if err == nil {
				return plan, nil
			}
			return &Response{Answer: encodedContent}, nil
		}

		var plan Response
		if err := json.Unmarshal([]byte(candidate), &plan); err != nil {
			lastErr = fmt.Errorf("decode planner response: %w", err)
			continue
		}
		NormalizeSuggestedPrompt(&plan)
		if isResponseEmpty(&plan, hasActions) {
			lastErr = fmt.Errorf("planner response is empty")
			continue
		}
		return &plan, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("planner response is empty")
}

func isResponseEmpty(plan *Response, hasActions HasActions) bool {
	if plan == nil {
		return true
	}
	if strings.TrimSpace(plan.Answer) != "" || strings.TrimSpace(PrimarySuggestedPrompt(plan)) != "" {
		return false
	}
	if plan.Task != nil || len(plan.Todos) > 0 || plan.NextAction != nil || len(plan.ContextRefs) > 0 {
		return false
	}
	if hasActions != nil {
		return !hasActions(plan)
	}
	return !hasRawActions(plan)
}

func hasRawActions(plan *Response) bool {
	if plan == nil {
		return false
	}
	for _, action := range append(append([]Action{}, plan.Actions...), plan.ToolCalls...) {
		if strings.TrimSpace(action.Name) != "" || strings.TrimSpace(stringFromAny(action.Arguments["name"])) != "" {
			return true
		}
	}
	return false
}

func NormalizeSuggestedPrompt(plan *Response) {
	if plan == nil {
		return
	}
	suggested := PrimarySuggestedPrompt(plan)
	if suggested == "" {
		return
	}
	if plan.NextAction == nil {
		plan.NextAction = &NextAction{
			Type:            "continue",
			Reason:          "model suggested a follow-up prompt",
			SuggestedPrompt: suggested,
			AutoContinue:    false,
		}
		return
	}
	if strings.TrimSpace(plan.NextAction.SuggestedPrompt) == "" {
		plan.NextAction.SuggestedPrompt = suggested
	}
}

func PrimarySuggestedPrompt(plan *Response) string {
	if plan == nil {
		return ""
	}
	if plan.NextAction != nil {
		if value := strings.TrimSpace(plan.NextAction.SuggestedPrompt); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(plan.SuggestedPrompt); value != "" {
		return value
	}
	if value := strings.TrimSpace(plan.SuggestPrompt); value != "" {
		return value
	}
	for _, value := range plan.SuggestedPrompts {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 3 {
			if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
				raw = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
			}
		}
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return ""
	}
	return strings.TrimSpace(raw[start : end+1])
}

func extractJSONCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	candidates := []string{}
	if candidate := extractJSONObject(raw); candidate != "" {
		candidates = append(candidates, candidate)
	}
	for _, candidate := range extractBalancedJSONObjects(raw) {
		candidates = append(candidates, candidate)
	}
	seen := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func extractBalancedJSONObjects(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	objects := []string{}
	depth := 0
	start := -1
	inString := false
	escaped := false
	for idx, r := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = idx
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
			if depth == 0 && start >= 0 {
				objects = append(objects, strings.TrimSpace(raw[start:idx+1]))
				start = -1
			}
		}
	}
	for left, right := 0, len(objects)-1; left < right; left, right = left+1, right-1 {
		objects[left], objects[right] = objects[right], objects[left]
	}
	return objects
}

func unwrapOpenAIChatCompletionContent(candidate string) (string, bool) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(candidate), &envelope); err != nil {
		return "", false
	}
	choices, _ := envelope["choices"].([]any)
	if len(choices) == 0 {
		return "", false
	}
	rootSuggestion := openAIEnvelopeSuggestedPrompt(envelope)
	for _, item := range choices {
		choice, _ := item.(map[string]any)
		if len(choice) == 0 {
			continue
		}
		message, _ := choice["message"].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		suggestion := firstNonEmpty(
			openAIEnvelopeSuggestedPrompt(message),
			openAIEnvelopeSuggestedPrompt(delta),
			openAIEnvelopeSuggestedPrompt(choice),
			rootSuggestion,
		)
		for _, value := range []any{message["content"], delta["content"], choice["text"]} {
			if content := openAIEnvelopeContentText(value); content != "" {
				return mergeSuggestedPromptContent(content, suggestion), true
			}
		}
	}
	if rootSuggestion != "" {
		return mergeSuggestedPromptContent("", rootSuggestion), true
	}
	return "", true
}

func openAIEnvelopeContentText(value any) string {
	switch content := value.(type) {
	case string:
		return strings.TrimSpace(content)
	case []any:
		parts := make([]string, 0, len(content))
		for _, item := range content {
			switch part := item.(type) {
			case string:
				if text := strings.TrimSpace(part); text != "" {
					parts = append(parts, text)
				}
			case map[string]any:
				if text := firstNonEmpty(stringFromAny(part["text"]), stringFromAny(part["content"])); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

func openAIEnvelopeSuggestedPrompt(records ...map[string]any) string {
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		if value := firstNonEmpty(stringFromAny(record["suggested_prompt"]), stringFromAny(record["suggest_prompt"]), stringFromAny(record["prompt"])); value != "" {
			return value
		}
		for _, key := range []string{"suggested_prompts", "suggestions"} {
			items, _ := record[key].([]any)
			for _, item := range items {
				if value := stringFromAny(item); strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return ""
}

func mergeSuggestedPromptContent(content, suggest string) string {
	content = strings.TrimSpace(content)
	suggest = strings.TrimSpace(suggest)
	if suggest == "" {
		return content
	}
	payload := map[string]any{}
	if content != "" && json.Unmarshal([]byte(content), &payload) == nil && len(payload) > 0 {
		if strings.TrimSpace(stringFromAny(payload["suggested_prompt"])) == "" && strings.TrimSpace(stringFromAny(payload["suggest_prompt"])) == "" {
			payload["suggest_prompt"] = suggest
		}
		encoded, err := json.Marshal(payload)
		if err == nil {
			return string(encoded)
		}
		return content
	}
	payload["answer"] = content
	payload["suggest_prompt"] = suggest
	encoded, err := json.Marshal(payload)
	if err != nil {
		return content
	}
	return string(encoded)
}
