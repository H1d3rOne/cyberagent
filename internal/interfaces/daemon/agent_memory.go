package daemon

import (
	"fmt"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

func agentLLMHistory(history []agentMessage, memory *agentMemorySummary, language string) []llm.Message {
	start := len(history) - 20
	if start < 0 {
		start = 0
	}

	var memoryMessage *llm.Message
	if memoryPrompt := agentMemorySummaryPrompt(language, memory); memoryPrompt != "" {
		message := llm.Message{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{llm.TextBlock{Text: memoryPrompt}},
		}
		memoryMessage = &message
	}

	messages := make([]llm.Message, 0, len(history[start:])+1)
	seenUser := false
	for _, message := range history[start:] {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len(content) > 4000 {
			content = content[:4000] + "\n…"
		}

		role := llm.RoleAssistant
		switch message.Role {
		case "user":
			role = llm.RoleUser
			seenUser = true
		case "agent":
			if !seenUser {
				continue
			}
		case "tool":
			if !seenUser {
				continue
			}
			content = "Runtime/tool observation:\n" + content
		default:
			continue
		}

		if len(messages) > 0 && messages[len(messages)-1].Role == role {
			previous := messages[len(messages)-1].Content
			if len(previous) > 0 {
				if text, ok := previous[len(previous)-1].(llm.TextBlock); ok {
					text.Text = strings.TrimSpace(text.Text) + "\n\n" + content
					messages[len(messages)-1].Content[len(previous)-1] = text
					continue
				}
			}
		}
		messages = append(messages, llm.Message{Role: role, Content: []llm.ContentBlock{llm.TextBlock{Text: content}}})
	}

	if len(messages) > 17 {
		messages = messages[len(messages)-16:]
	}
	for len(messages) > 0 && messages[0].Role != llm.RoleUser {
		messages = messages[1:]
	}
	if memoryMessage != nil {
		messages = append([]llm.Message{*memoryMessage}, messages...)
	}
	return messages
}

func agentLLMToolSynthesisHistory(history []agentMessage, memory *agentMemorySummary, toolMessages []agentMessage, language string) []llm.Message {
	messages := agentLLMHistory(history, memory, language)
	observation := agentToolObservationPrompt(language, toolMessages)
	if strings.TrimSpace(observation) == "" {
		return messages
	}
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{llm.TextBlock{Text: observation}},
	})
	if len(messages) > 18 {
		messages = trimAgentLLMMessagesPreservingFirst(messages, 18, agentMemorySummaryPrompt(language, memory) != "")
	}
	for len(messages) > 0 && messages[0].Role != llm.RoleUser {
		messages = messages[1:]
	}
	return messages
}

func trimAgentLLMMessagesPreservingFirst(messages []llm.Message, max int, preserveFirst bool) []llm.Message {
	if max <= 0 || len(messages) <= max {
		return messages
	}
	if !preserveFirst || len(messages) == 0 {
		return messages[len(messages)-max:]
	}
	if max == 1 {
		return []llm.Message{messages[0]}
	}
	tail := messages[1:]
	tailMax := max - 1
	if len(tail) > tailMax {
		tail = tail[len(tail)-tailMax:]
	}
	trimmed := make([]llm.Message, 0, 1+len(tail))
	trimmed = append(trimmed, messages[0])
	trimmed = append(trimmed, tail...)
	return trimmed
}

func compactAgentSessionMessagesLocked(session *agentSession, max int) *agentMemoryCompaction {
	if session == nil {
		return nil
	}
	if max <= 0 {
		max = agentSessionMaxMessages
	}
	retain := max
	trigger := "message_window"
	triggerThreshold := max
	if agentSessionMemoryCompactionTurns > 0 {
		trigger = "turn_count"
		triggerThreshold = agentSessionMemoryCompactionTurns
	}
	if len(session.Messages) <= retain {
		return nil
	}
	compactCount := len(session.Messages) - retain
	if compactCount <= 0 {
		return nil
	}
	previousCount := 0
	if session.MemorySummary != nil {
		previousCount = session.MemorySummary.MessageCount
	}
	compacted := cloneAgentMessages(session.Messages[:compactCount])
	session.MemorySummary = buildAgentMemorySummary(session, compacted, session.MemorySummary)
	session.Messages = cloneAgentMessages(session.Messages[compactCount:])
	compaction := &agentMemoryCompaction{
		Summary:          cloneAgentMemorySummary(session.MemorySummary),
		CompactedCount:   len(compacted),
		RetainedCount:    len(session.Messages),
		PreviousCount:    previousCount,
		Trigger:          trigger,
		TriggerThreshold: triggerThreshold,
		OccurredAt:       time.Now().UTC(),
	}
	if len(compacted) > 0 {
		last := compacted[len(compacted)-1]
		compaction.LastCompactedID = last.ID
		if !last.Timestamp.IsZero() {
			ts := last.Timestamp
			compaction.LastCompactedAt = &ts
		}
	}
	return compaction
}

func buildAgentMemorySummary(session *agentSession, compacted []agentMessage, previous *agentMemorySummary) *agentMemorySummary {
	if len(compacted) == 0 && previous == nil {
		return nil
	}
	parts := make([]string, 0, 12)
	if previous != nil && strings.TrimSpace(previous.Summary) != "" {
		parts = append(parts, "Previous memory: "+agentLimitBlock(strings.TrimSpace(previous.Summary), 1800))
	}
	if session != nil && session.ActiveTask != nil {
		taskBits := []string{
			"goal=" + emptyString(session.ActiveTask.Goal, "-"),
			"kind=" + emptyString(session.ActiveTask.Kind, "-"),
			"status=" + emptyString(session.ActiveTask.Status, "-"),
		}
		if session.ActiveTask.ActiveRunID != "" {
			taskBits = append(taskBits, "active_run="+session.ActiveTask.ActiveRunID)
		}
		if session.ActiveTask.Summary != "" {
			taskBits = append(taskBits, "summary="+agentLimitBlock(session.ActiveTask.Summary, 320))
		}
		parts = append(parts, "Active task: "+strings.Join(taskBits, " | "))
	}
	if session != nil && len(session.Todos) > 0 {
		todoParts := make([]string, 0, minInt(len(session.Todos), 8))
		for idx, todo := range session.Todos {
			if idx >= 8 {
				break
			}
			todoParts = append(todoParts, fmt.Sprintf("%s:%s", emptyString(todo.Title, todo.ID), emptyString(todo.Status, "-")))
		}
		if len(todoParts) > 0 {
			parts = append(parts, "Todos: "+strings.Join(todoParts, "; "))
		}
	}
	if session != nil && len(session.ContextRefs) > 0 {
		refParts := make([]string, 0, minInt(len(session.ContextRefs), 8))
		for idx, ref := range session.ContextRefs {
			if idx >= 8 {
				break
			}
			refText := firstNonEmpty(ref.Label, ref.RefID, ref.ID)
			if ref.Summary != "" {
				refText += " (" + agentLimitBlock(ref.Summary, 180) + ")"
			}
			refParts = append(refParts, refText)
		}
		if len(refParts) > 0 {
			parts = append(parts, "Context refs: "+strings.Join(refParts, "; "))
		}
	}

	messageParts := make([]string, 0, minInt(len(compacted), 32))
	compactedWindow := compacted
	if len(compacted) > 32 {
		// Preserve the beginning and the tail of a compacted slice. The first
		// few messages often carry target/scope/constraints, while the tail
		// carries the latest state before compaction.
		compactedWindow = append(cloneAgentMessages(compacted[:8]), compacted[len(compacted)-24:]...)
	}
	for _, message := range compactedWindow {
		summary := agentMemoryMessageLine(message)
		if summary != "" {
			messageParts = append(messageParts, summary)
		}
	}
	if len(messageParts) > 0 {
		parts = append(parts, "Compacted conversation:\n"+strings.Join(messageParts, "\n"))
	}

	summary := agentLimitBlock(strings.TrimSpace(strings.Join(parts, "\n\n")), 5000)
	if summary == "" && previous != nil {
		summary = previous.Summary
	}
	var lastID string
	var lastAt *time.Time
	if len(compacted) > 0 {
		last := compacted[len(compacted)-1]
		lastID = last.ID
		if !last.Timestamp.IsZero() {
			ts := last.Timestamp
			lastAt = &ts
		}
	} else if previous != nil {
		lastID = previous.LastMessageID
		if previous.LastMessageAt != nil {
			ts := *previous.LastMessageAt
			lastAt = &ts
		}
	}
	messageCount := len(compacted)
	if previous != nil {
		messageCount += previous.MessageCount
	}
	return &agentMemorySummary{
		Summary:       summary,
		MessageCount:  messageCount,
		LastMessageID: lastID,
		LastMessageAt: lastAt,
		UpdatedAt:     time.Now().UTC(),
	}
}

func agentMemoryMessageLine(message agentMessage) string {
	role := emptyString(message.Role, "message")
	content := strings.TrimSpace(message.Content)
	if content != "" {
		content = strings.ReplaceAll(content, "\r\n", "\n")
		content = strings.ReplaceAll(content, "\r", "\n")
		content = strings.Join(strings.Fields(content), " ")
		content = agentLimitBlock(content, 420)
	}
	toolBits := make([]string, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		name := emptyString(call.Name, "tool")
		status := string(call.Status)
		if status == "" {
			status = "-"
		}
		toolBits = append(toolBits, name+":"+status)
	}
	prefix := "- " + role
	if !message.Timestamp.IsZero() {
		prefix += "@" + message.Timestamp.Format(time.RFC3339)
	}
	switch {
	case content != "" && len(toolBits) > 0:
		return prefix + ": " + content + " [tools: " + strings.Join(toolBits, ", ") + "]"
	case content != "":
		return prefix + ": " + content
	case len(toolBits) > 0:
		return prefix + ": [tools: " + strings.Join(toolBits, ", ") + "]"
	default:
		return ""
	}
}

func agentMemorySummaryPrompt(language string, memory *agentMemorySummary) string {
	if memory == nil || strings.TrimSpace(memory.Summary) == "" {
		return ""
	}
	return agentText(
		language,
		fmt.Sprintf("以下是当前 Agent 会话早期内容的压缩记忆。它来自已从短期聊天窗口裁剪的 %d 条消息；请把它当作已有上下文，不要要求用户重复这些信息。\n\n%s", memory.MessageCount, agentLimitBlock(memory.Summary, 5000)),
		fmt.Sprintf("The following is compressed memory from the earlier part of this Agent session. It comes from %d message(s) that were trimmed out of the short-term chat window; treat it as existing context and do not ask the operator to repeat it.\n\n%s", memory.MessageCount, agentLimitBlock(memory.Summary, 5000)),
	)
}
