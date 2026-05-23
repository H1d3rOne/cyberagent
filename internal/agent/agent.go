package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/agent/prompt"
	"github.com/H1d3rOne/cyberagent/internal/agent/tools"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/policy"
	"github.com/H1d3rOne/cyberagent/internal/core/ratelimit"
	"github.com/H1d3rOne/cyberagent/internal/core/redactor"
)

type Config struct {
	Provider     llm.Provider
	Target       string
	Scope        []string
	Instructions string
	MaxTurns     int
	NoConfirm    bool
	OutDir       string
	OnEvent      func(Event)
	OnConfirm    func(toolName string, input json.RawMessage) bool
}

type EventType string

const (
	EventThinking   EventType = "thinking"
	EventTextDelta  EventType = "text_delta"
	EventToolCall   EventType = "tool_call"
	EventToolResult EventType = "tool_result"
	EventFinding    EventType = "finding"
	EventDone       EventType = "done"
	EventError      EventType = "error"
	EventProgress   EventType = "progress"
)

type Event struct {
	Type      EventType
	Message   string
	ToolName  string
	ToolInput json.RawMessage
	Finding   *contracts.Finding
	Turn      int
	MaxTurns  int
	Elapsed   time.Duration
}

type Result struct {
	Findings   []contracts.Finding
	Summary    string
	Turns      int
	Duration   time.Duration
	TokensIn   int
	TokensOut  int
	EstCostUSD float64
}

type Agent struct {
	config     Config
	registry   *tools.Registry
	scopeGuard policy.ScopeGuard
	scope      contracts.Scope
	limiter    *ratelimit.Limiter
	redactor   *redactor.Redactor
	history    []llm.Message
	findings   []contracts.Finding
	mu         sync.Mutex
}

func New(cfg Config) *Agent {
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 50
	}
	scope := buildScope(cfg.Scope)
	return &Agent{
		config:     cfg,
		registry:   tools.NewRegistry(),
		scopeGuard: policy.NewScopeGuard(),
		scope:      scope,
		limiter:    ratelimit.New(ratelimit.Config{GlobalRPS: 5, GlobalBurst: 3, PerHostRPS: 3, PerHostBurst: 2, MaxConcurrencyAll: 5}),
		redactor:   redactor.New(),
	}
}

func (a *Agent) Registry() *tools.Registry {
	return a.registry
}

func (a *Agent) Run(ctx context.Context) (*Result, error) {
	start := time.Now()
	var totalIn, totalOut int

	systemPrompt := prompt.BuildSystem(prompt.BuilderConfig{
		Target:       a.config.Target,
		Scope:        a.config.Scope,
		Instructions: a.config.Instructions,
		ToolNames:    a.toolNames(),
	})

	a.history = []llm.Message{{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{llm.TextBlock{Text: fmt.Sprintf("Begin security assessment of: %s", a.config.Target)}},
	}}

	for turn := 0; turn < a.config.MaxTurns; turn++ {
		a.emit(Event{Type: EventProgress, Turn: turn + 1, MaxTurns: a.config.MaxTurns, Elapsed: time.Since(start)})
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
			a.autoSaveSession()
			return nil, fmt.Errorf("llm chat turn %d: %w", turn, err)
		}

		totalIn += resp.Usage.InputTokens
		totalOut += resp.Usage.OutputTokens

		a.history = append(a.history, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})

		toolCalls := a.extractToolCalls(resp.Content)
		if len(toolCalls) == 0 {
			summary := a.extractText(resp.Content)
			a.emit(Event{Type: EventDone, Message: summary})
			return &Result{
				Findings:   a.findings,
				Summary:    summary,
				Turns:      turn + 1,
				Duration:   time.Since(start),
				TokensIn:   totalIn,
				TokensOut:  totalOut,
				EstCostUSD: estimateCost(a.config.Provider.Name(), totalIn, totalOut),
			}, nil
		}

		results := a.executeTools(ctx, toolCalls)
		a.history = append(a.history, llm.Message{Role: llm.RoleUser, Content: results})
	}

	return &Result{
		Findings:   a.findings,
		Summary:    "Assessment stopped: maximum turns reached.",
		Turns:      a.config.MaxTurns,
		Duration:   time.Since(start),
		TokensIn:   totalIn,
		TokensOut:  totalOut,
		EstCostUSD: estimateCost(a.config.Provider.Name(), totalIn, totalOut),
	}, nil
}

func (a *Agent) executeTools(ctx context.Context, calls []llm.ToolUseBlock) []llm.ContentBlock {
	if len(calls) == 1 {
		return []llm.ContentBlock{a.executeSingleTool(ctx, calls[0])}
	}

	results := make([]llm.ContentBlock, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c llm.ToolUseBlock) {
			defer wg.Done()
			results[idx] = a.executeSingleTool(ctx, c)
		}(i, call)
	}
	wg.Wait()
	return results
}

func (a *Agent) executeSingleTool(ctx context.Context, call llm.ToolUseBlock) llm.ContentBlock {
	a.emit(Event{Type: EventToolCall, ToolName: call.Name, ToolInput: call.Input})

	if call.Name == "add_finding" {
		return a.handleFinding(call)
	}

	if call.Name == "generate_report" {
		return a.handleGenerateReport(call)
	}

	tool, ok := a.registry.Get(call.Name)
	if !ok {
		return llm.ToolResultBlock{
			ToolUseID: call.ID,
			Content:   fmt.Sprintf("error: unknown tool %q", call.Name),
			IsError:   true,
		}
	}

	if isHighRisk(call.Name) && !a.config.NoConfirm {
		if a.config.OnConfirm != nil && !a.config.OnConfirm(call.Name, call.Input) {
			return llm.ToolResultBlock{
				ToolUseID: call.ID,
				Content:   "error: user denied execution of high-risk tool",
				IsError:   true,
			}
		}
	}

	if targetURL := extractTargetURL(call.Input); targetURL != "" {
		decision := a.scopeGuard.CheckURL("agent", "agent", call.Name, targetURL, a.scope)
		if decision.Decision == contracts.PolicyDeny {
			a.emit(Event{Type: EventError, ToolName: call.Name, Message: fmt.Sprintf("scope denied: %s", targetURL)})
			return llm.ToolResultBlock{
				ToolUseID: call.ID,
				Content:   fmt.Sprintf("error: target %q is outside authorized scope", targetURL),
				IsError:   true,
			}
		}
		if host := extractHost(targetURL); host != "" {
			if err := a.limiter.Wait(ctx, host); err != nil {
				return llm.ToolResultBlock{
					ToolUseID: call.ID,
					Content:   fmt.Sprintf("error: rate limit: %s", err.Error()),
					IsError:   true,
				}
			}
			a.limiter.Release()
		}
	}

	toolCtx, toolCancel := context.WithTimeout(ctx, toolTimeout(call.Name))
	defer toolCancel()

	output, err := tool.Execute(toolCtx, call.Input)
	if err != nil {
		a.emit(Event{Type: EventError, Message: err.Error(), ToolName: call.Name})
		return llm.ToolResultBlock{
			ToolUseID: call.ID,
			Content:   fmt.Sprintf("error: %s", err.Error()),
			IsError:   true,
		}
	}

	output = a.redactor.Redact(output)
	output = limitOutput(output)

	a.emit(Event{Type: EventToolResult, ToolName: call.Name, Message: truncateEvent(output, 200)})
	return llm.ToolResultBlock{
		ToolUseID: call.ID,
		Content:   output,
	}
}

func (a *Agent) handleFinding(call llm.ToolUseBlock) llm.ToolResultBlock {
	fi, err := tools.ParseFindingInput(call.Input)
	if err != nil {
		return llm.ToolResultBlock{ToolUseID: call.ID, Content: fmt.Sprintf("error: %s", err.Error()), IsError: true}
	}
	a.mu.Lock()
	for _, existing := range a.findings {
		if existing.Title == fi.Title && contains(existing.AffectedTargets, a.config.Target) {
			a.mu.Unlock()
			return llm.ToolResultBlock{ToolUseID: call.ID, Content: fmt.Sprintf("Finding already recorded: %s", fi.Title)}
		}
	}
	id := fmt.Sprintf("finding_%d", len(a.findings)+1)
	finding := fi.ToFinding(id, a.config.Target)
	a.findings = append(a.findings, finding)
	a.mu.Unlock()
	a.emit(Event{Type: EventFinding, Finding: &finding, Message: finding.Title})
	return llm.ToolResultBlock{ToolUseID: call.ID, Content: fmt.Sprintf("Finding recorded: %s (severity: %s)", finding.Title, finding.Severity)}
}

func (a *Agent) handleGenerateReport(call llm.ToolUseBlock) llm.ToolResultBlock {
	var params struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(call.Input, &params); err != nil {
		return llm.ToolResultBlock{ToolUseID: call.ID, Content: fmt.Sprintf("error: %s", err.Error()), IsError: true}
	}

	a.mu.Lock()
	findings := make([]contracts.Finding, len(a.findings))
	copy(findings, a.findings)
	a.mu.Unlock()

	result := &Result{
		Findings: findings,
		Summary:  params.Summary,
		Turns:    len(a.history) / 2,
	}

	dir := a.config.OutDir
	if dir == "" {
		dir = "."
	}

	report, err := GenerateReport(dir, result, a.config.Target)
	if err != nil {
		return llm.ToolResultBlock{ToolUseID: call.ID, Content: fmt.Sprintf("Report generation failed: %s", err.Error()), IsError: true}
	}

	return llm.ToolResultBlock{
		ToolUseID: call.ID,
		Content:   fmt.Sprintf("Report written. Findings: %d. Files: %s, %s", len(findings), report.MDPath, report.JSONPath),
	}
}

func (a *Agent) extractToolCalls(blocks []llm.ContentBlock) []llm.ToolUseBlock {
	var calls []llm.ToolUseBlock
	for _, b := range blocks {
		if tu, ok := b.(llm.ToolUseBlock); ok {
			calls = append(calls, tu)
		}
	}
	return calls
}

func (a *Agent) extractText(blocks []llm.ContentBlock) string {
	for _, b := range blocks {
		if tb, ok := b.(llm.TextBlock); ok {
			return tb.Text
		}
	}
	return ""
}

func (a *Agent) toolNames() []string {
	defs := a.registry.Definitions()
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func (a *Agent) emit(e Event) {
	if a.config.OnEvent != nil {
		a.config.OnEvent(e)
	}
}

func (a *Agent) chatWithStream(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	sp, ok := a.config.Provider.(llm.StreamProvider)
	if !ok {
		return a.config.Provider.Chat(ctx, req)
	}

	ch, err := sp.StreamChat(ctx, req)
	if err != nil {
		return a.config.Provider.Chat(ctx, req)
	}

	var blocks []llm.ContentBlock
	var textBuf string
	var stopReason llm.StopReason
	var usage llm.Usage

	for event := range ch {
		switch event.Type {
		case "text_delta":
			textBuf += event.TextDelta
			a.emit(Event{Type: EventTextDelta, Message: event.TextDelta})
		case "tool_use":
			if textBuf != "" {
				blocks = append(blocks, llm.TextBlock{Text: textBuf})
				textBuf = ""
			}
			blocks = append(blocks, event.Block)
		case "done":
			if textBuf != "" {
				blocks = append(blocks, llm.TextBlock{Text: textBuf})
				textBuf = ""
			}
			stopReason = event.StopReason
			usage = event.Usage
		case "error":
			if event.Error != nil {
				return nil, event.Error
			}
		}
	}

	return &llm.ChatResponse{
		Content:    blocks,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

func truncateEvent(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func buildScope(hosts []string) contracts.Scope {
	if len(hosts) == 0 {
		return contracts.Scope{}
	}
	domains := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if u, err := url.Parse(h); err == nil && u.Hostname() != "" {
			domains = append(domains, u.Hostname())
		} else {
			domains = append(domains, h)
		}
	}
	return contracts.NewScope("agent", contracts.ScopeRules{
		Domains: domains,
		Schemes: []string{"http", "https"},
	})
}

func extractTargetURL(input json.RawMessage) string {
	var params struct {
		URL  string `json:"url"`
		Host string `json:"host"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ""
	}
	if params.URL != "" {
		return params.URL
	}
	if params.Host != "" {
		return "http://" + params.Host
	}
	return ""
}

func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

var highRiskTools = map[string]bool{
	"discover_paths": true,
	"check_ports":    true,
	"run_nuclei":     true,
}

func isHighRisk(name string) bool {
	return highRiskTools[name]
}

var toolTimeouts = map[string]time.Duration{
	"run_nuclei":     180 * time.Second,
	"discover_paths": 120 * time.Second,
	"check_ports":    60 * time.Second,
}

const defaultToolTimeout = 60 * time.Second

func toolTimeout(name string) time.Duration {
	if t, ok := toolTimeouts[name]; ok {
		return t
	}
	return defaultToolTimeout
}

const maxToolOutputBytes = 32 * 1024

func limitOutput(s string) string {
	if len(s) <= maxToolOutputBytes {
		return s
	}
	return s[:maxToolOutputBytes] + fmt.Sprintf("\n...[output truncated from %d bytes to %d bytes]", len(s), maxToolOutputBytes)
}

func (a *Agent) autoSaveSession() {
	dir := a.config.OutDir
	if dir == "" {
		dir = "."
	}
	if path, err := a.SaveSession(dir); err == nil {
		a.emit(Event{Type: EventError, Message: fmt.Sprintf("session auto-saved to %s", path)})
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func estimateCost(provider string, inputTokens, outputTokens int) float64 {
	var inRate, outRate float64
	switch provider {
	case "anthropic":
		inRate = 3.0 / 1_000_000
		outRate = 15.0 / 1_000_000
	case "openai":
		inRate = 3.0 / 1_000_000
		outRate = 15.0 / 1_000_000
	default:
		return 0
	}
	return float64(inputTokens)*inRate + float64(outputTokens)*outRate
}
