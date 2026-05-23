package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/agent/prompt"
)

func (a *Agent) RunInteractive(ctx context.Context, input io.Reader, output io.Writer) (*Result, error) {
	start := time.Now()
	scanner := bufio.NewScanner(input)

	systemPrompt := prompt.BuildSystem(prompt.BuilderConfig{
		Target:       a.config.Target,
		Scope:        a.config.Scope,
		Instructions: a.config.Instructions,
		ToolNames:    a.toolNames(),
	})

	if len(a.history) == 0 {
		a.history = []llm.Message{{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{llm.TextBlock{Text: fmt.Sprintf("Begin security assessment of: %s", a.config.Target)}},
		}}
	}

	turn := 0
	for turn < a.config.MaxTurns {
		a.history = compressHistory(a.history)

		chatReq := llm.ChatRequest{
			System:      systemPrompt,
			Messages:    a.history,
			Tools:       a.registry.Definitions(),
			MaxTokens:   4096,
			Temperature: 0.1,
		}

		resp, err := a.chatWithStream(ctx, chatReq)
		if err != nil {
			return nil, fmt.Errorf("llm chat turn %d: %w", turn, err)
		}

		a.history = append(a.history, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		turn++

		toolCalls := a.extractToolCalls(resp.Content)
		if len(toolCalls) > 0 {
			results := a.executeTools(ctx, toolCalls)
			a.history = append(a.history, llm.Message{Role: llm.RoleUser, Content: results})
			continue
		}

		text := a.extractText(resp.Content)
		a.emit(Event{Type: EventDone, Message: text})

		fmt.Fprintf(output, "\n[agent] %s\n", text)
		fmt.Fprintf(output, "\n> ")

		if !scanner.Scan() {
			break
		}
		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" || userInput == "exit" || userInput == "quit" {
			break
		}

		a.history = append(a.history, llm.Message{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{llm.TextBlock{Text: userInput}},
		})
	}

	summary := "Interactive session ended."
	if len(a.history) > 0 {
		if last := a.extractText(a.history[len(a.history)-1].Content); last != "" {
			summary = last
		}
	}

	return &Result{
		Findings: a.findings,
		Summary:  summary,
		Turns:    turn,
		Duration: time.Since(start),
	}, nil
}
