package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/agent/prompt"
	"github.com/H1d3rOne/cyberagent/internal/agent/tools"
)

type WebInteractiveAgent struct {
	agent        *Agent
	inputCh      chan string
	systemPrompt string
	turn         int
	totalIn      int
	totalOut     int
	start        time.Time
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan *Result
	waiting      bool
}

func NewWebInteractive(cfg Config) *WebInteractiveAgent {
	a := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())

	systemPrompt := prompt.BuildSystem(prompt.BuilderConfig{
		Target:       cfg.Target,
		Scope:        cfg.Scope,
		Instructions: cfg.Instructions,
		ToolNames:    a.toolNames(),
	})

	return &WebInteractiveAgent{
		agent:        a,
		inputCh:      make(chan string, 1),
		systemPrompt: systemPrompt,
		start:        time.Now(),
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan *Result, 1),
	}
}

func (w *WebInteractiveAgent) Registry() *tools.Registry {
	return w.agent.registry
}

func (w *WebInteractiveAgent) IsWaiting() bool {
	return w.waiting
}

func (w *WebInteractiveAgent) Start() {
	go w.loop()
}

func (w *WebInteractiveAgent) SendMessage(msg string) {
	select {
	case w.inputCh <- msg:
	default:
	}
}

func (w *WebInteractiveAgent) Stop() {
	w.cancel()
}

func (w *WebInteractiveAgent) Done() <-chan *Result {
	return w.done
}

func (w *WebInteractiveAgent) loop() {
	a := w.agent

	if len(a.history) == 0 {
		a.history = []llm.Message{{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{llm.TextBlock{Text: fmt.Sprintf("Begin security assessment of: %s", a.config.Target)}},
		}}
	}

	for w.turn < a.config.MaxTurns {
		select {
		case <-w.ctx.Done():
			w.finish("Session cancelled.")
			return
		default:
		}

		a.emit(Event{Type: EventProgress, Turn: w.turn + 1, MaxTurns: a.config.MaxTurns, Elapsed: time.Since(w.start)})
		a.history = compressHistory(a.history)

		chatReq := llm.ChatRequest{
			System:      w.systemPrompt,
			Messages:    a.history,
			Tools:       a.registry.Definitions(),
			MaxTokens:   4096,
			Temperature: 0.1,
		}

		resp, err := a.chatWithStream(w.ctx, chatReq)
		if err != nil {
			if w.ctx.Err() != nil {
				w.finish("Session cancelled.")
				return
			}
			a.emit(Event{Type: EventError, Message: err.Error()})
			w.finish(fmt.Sprintf("Error: %s", err.Error()))
			return
		}

		w.totalIn += resp.Usage.InputTokens
		w.totalOut += resp.Usage.OutputTokens
		a.history = append(a.history, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		w.turn++

		toolCalls := a.extractToolCalls(resp.Content)
		if len(toolCalls) > 0 {
			results := a.executeTools(w.ctx, toolCalls)
			a.history = append(a.history, llm.Message{Role: llm.RoleUser, Content: results})
			continue
		}

		text := a.extractText(resp.Content)
		a.emit(Event{Type: EventDone, Message: text})

		w.waiting = true
		select {
		case userMsg := <-w.inputCh:
			w.waiting = false
			if userMsg == "" || userMsg == "exit" || userMsg == "quit" {
				w.finish(text)
				return
			}
			a.history = append(a.history, llm.Message{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{llm.TextBlock{Text: userMsg}},
			})
		case <-w.ctx.Done():
			w.waiting = false
			w.finish(text)
			return
		}
	}

	w.finish("Maximum turns reached.")
}

func (w *WebInteractiveAgent) finish(summary string) {
	w.done <- &Result{
		Findings:   w.agent.findings,
		Summary:    summary,
		Turns:      w.turn,
		Duration:   time.Since(w.start),
		TokensIn:   w.totalIn,
		TokensOut:  w.totalOut,
		EstCostUSD: estimateCost(w.agent.config.Provider.Name(), w.totalIn, w.totalOut),
	}
}
