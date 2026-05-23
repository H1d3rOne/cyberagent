package agent

import (
	"fmt"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
)

const (
	maxContextTokens        = 100000
	compressThresholdTokens = 80000
	maxToolResultLength     = 2000
	tokensPerChar           = 0.25
)

func estimateTokens(history []llm.Message) int {
	total := 0
	for _, msg := range history {
		for _, b := range msg.Content {
			switch block := b.(type) {
			case llm.TextBlock:
				total += estimateStringTokens(block.Text)
			case llm.ToolUseBlock:
				total += estimateStringTokens(block.Name) + estimateStringTokens(string(block.Input))
			case llm.ToolResultBlock:
				total += estimateStringTokens(block.Content)
			}
		}
	}
	return total
}

func estimateStringTokens(s string) int {
	t := int(float64(len(s)) * tokensPerChar)
	if t < 1 && len(s) > 0 {
		return 1
	}
	return t
}

func compressHistory(history []llm.Message) []llm.Message {
	tokens := estimateTokens(history)
	if tokens <= compressThresholdTokens {
		return history
	}

	keepRecent := 10
	if keepRecent > len(history) {
		keepRecent = len(history)
	}

	compressed := make([]llm.Message, 0, len(history))
	cutoff := len(history) - keepRecent

	for i, msg := range history {
		if i >= cutoff {
			compressed = append(compressed, msg)
			continue
		}
		compressed = append(compressed, compressMessage(msg))
	}

	for estimateTokens(compressed) > maxContextTokens && len(compressed) > keepRecent+1 {
		compressed = append(compressed[:1], compressed[2:]...)
	}

	return compressed
}

func compressMessage(msg llm.Message) llm.Message {
	blocks := make([]llm.ContentBlock, 0, len(msg.Content))
	for _, b := range msg.Content {
		switch block := b.(type) {
		case llm.ToolResultBlock:
			if len(block.Content) > maxToolResultLength {
				block.Content = block.Content[:maxToolResultLength] + fmt.Sprintf("...[truncated from %d chars]", len(block.Content))
			}
			blocks = append(blocks, block)
		default:
			blocks = append(blocks, b)
		}
	}
	return llm.Message{Role: msg.Role, Content: blocks}
}
