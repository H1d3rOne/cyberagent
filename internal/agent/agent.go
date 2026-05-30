package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/agent/plan"
	"github.com/H1d3rOne/cyberagent/internal/agent/prompt"
	"github.com/H1d3rOne/cyberagent/internal/agent/tools"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/policy"
	"github.com/H1d3rOne/cyberagent/internal/core/ratelimit"
	"github.com/H1d3rOne/cyberagent/internal/core/redactor"
)

type Config struct {
	Provider       llm.Provider
	Target         string
	Scope          []string
	Instructions   string
	MaxTurns       int
	NoConfirm      bool
	OutDir         string
	OperationLevel string
	OnEvent        func(Event)
	OnConfirm      func(toolName string, input json.RawMessage) bool
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
	EventPlanUpdate EventType = "plan_update"
)

type AgentPhase string

const (
	PhasePlanning    AgentPhase = "planning"
	PhaseExecuting   AgentPhase = "executing"
	PhaseReplanning  AgentPhase = "replanning"
	PhaseFinalAnswer AgentPhase = "final_answer"
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
	Phase     AgentPhase
	Plan      *plan.Plan
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
	config       Config
	registry     *tools.Registry
	scopeGuard   policy.ScopeGuard
	scope        contracts.Scope
	limiter      *ratelimit.Limiter
	redactor     *redactor.Redactor
	history      []llm.Message
	findings     []contracts.Finding
	mu           sync.Mutex
	currentPhase AgentPhase
	totalIn      int
	totalOut     int
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

	pl := a.buildPlan(ctx, systemPrompt)
	a.emitPlan(pl)

	turnsLeft := a.config.MaxTurns
	lastSummary := ""
	for {
		todo := pl.NextPending()
		if todo == nil || turnsLeft <= 0 {
			break
		}
		todo.Status = plan.TodoInProgress
		used, summary, _, err := a.runTodoLoop(ctx, systemPrompt, todo, turnsLeft, start)
		if err != nil {
			a.autoSaveSession()
			return nil, err
		}
		turnsLeft -= used
		if summary != "" {
			lastSummary = summary
		}
		pl.Mark(todo.ID, plan.TodoDone)
		a.emitPlan(pl)
	}

	if lastSummary == "" {
		lastSummary = "Assessment stopped: maximum turns reached."
	}
	a.currentPhase = PhaseFinalAnswer
	a.emit(Event{Type: EventDone, Message: lastSummary, Phase: PhaseFinalAnswer})
	return a.result(lastSummary, a.config.MaxTurns-turnsLeft, start), nil
}

// runTodoLoop runs the ReAct inner loop for a single todo. Returns turns used,
// the final text (if the model stopped without tool calls), whether the model
// signalled overall completion (no tool calls), and any error.
func (a *Agent) runTodoLoop(ctx context.Context, systemPrompt string, todo *plan.Todo, budget int, start time.Time) (int, string, bool, error) {
	if todo.Title != "" {
		a.history = append(a.history, llm.Message{
			Role:    llm.RoleUser,
			Content: []llm.ContentBlock{llm.TextBlock{Text: "## STEP: " + todo.Title}},
		})
	}
	for used := 0; used < budget; used++ {
		a.currentPhase = PhaseExecuting
		a.emit(Event{Type: EventProgress, Turn: a.config.MaxTurns - budget + used + 1, MaxTurns: a.config.MaxTurns, Elapsed: time.Since(start), Phase: PhaseExecuting})
		a.history = compressHistory(a.history)

		resp, err := a.chatWithStream(ctx, llm.ChatRequest{
			System:      systemPrompt,
			Messages:    a.history,
			Tools:       a.registry.Definitions(),
			MaxTokens:   4096,
			Temperature: 0.1,
		})
		if err != nil {
			return used, "", false, fmt.Errorf("llm chat: %w", err)
		}
		a.addUsage(resp.Usage)
		a.history = append(a.history, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})

		toolCalls := a.extractToolCalls(resp.Content)
		if len(toolCalls) == 0 {
			return used + 1, a.extractText(resp.Content), true, nil
		}
		results := a.executeTools(ctx, toolCalls)
		a.history = append(a.history, llm.Message{Role: llm.RoleUser, Content: results})
	}
	return budget, "", false, nil
}

func (a *Agent) result(summary string, turns int, start time.Time) *Result {
	return &Result{
		Findings:   a.findings,
		Summary:    summary,
		Turns:      turns,
		Duration:   time.Since(start),
		TokensIn:   a.totalIn,
		TokensOut:  a.totalOut,
		EstCostUSD: estimateCost(a.config.Provider.Name(), a.totalIn, a.totalOut),
	}
}

func (a *Agent) addUsage(u llm.Usage) {
	a.totalIn += u.InputTokens
	a.totalOut += u.OutputTokens
}

func (a *Agent) emitPlan(pl *plan.Plan) {
	done, total := pl.Coverage()
	a.emit(Event{Type: EventPlanUpdate, Plan: pl, Message: fmt.Sprintf("coverage %d/%d", done, total), Phase: a.currentPhase})
}

// buildPlan asks the LLM for a structured assessment plan. On any failure it
// falls back to a single todo wrapping the whole assessment, so the engine
// degrades to the original single-loop behaviour.
func (a *Agent) buildPlan(ctx context.Context, systemPrompt string) *plan.Plan {
	fallback := &plan.Plan{
		Goal:    "Security assessment of " + a.config.Target,
		Todos:   []plan.Todo{{ID: "t1", Title: "", Status: plan.TodoPending}},
		Version: 1,
	}
	a.currentPhase = PhasePlanning
	resp, err := a.config.Provider.Chat(ctx, llm.ChatRequest{
		System: systemPrompt,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock{Text: fmt.Sprintf(
			"Output ONLY a JSON object planning the assessment of %s. Schema: {\"goal\":string,\"todos\":[{\"title\":string,\"stage\":string}]}. No prose, no tool calls.",
			a.config.Target)}}}},
		MaxTokens:   1024,
		Temperature: 0.1,
	})
	if err != nil {
		return fallback
	}
	a.addUsage(resp.Usage)
	var parsed struct {
		Goal  string `json:"goal"`
		Todos []struct {
			Title string `json:"title"`
			Stage string `json:"stage"`
		} `json:"todos"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(a.extractText(resp.Content))), &parsed); err != nil || len(parsed.Todos) == 0 {
		return fallback
	}
	pl := &plan.Plan{Goal: firstNonEmpty(parsed.Goal, fallback.Goal), Version: 1}
	for i, t := range parsed.Todos {
		title := strings.TrimSpace(t.Title)
		if title == "" {
			continue
		}
		pl.Todos = append(pl.Todos, plan.Todo{ID: fmt.Sprintf("t%d", i+1), Title: title, Stage: t.Stage, Status: plan.TodoPending})
	}
	if len(pl.Todos) == 0 {
		return fallback
	}
	return pl
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return s
	}
	return s[start : end+1]
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

	if a.config.OperationLevel != "" && !plan.LevelAtLeast(a.config.OperationLevel, toolRequiredLevel(call.Name)) {
		a.emit(Event{Type: EventError, ToolName: call.Name, Message: "operation level denied"})
		return llm.ToolResultBlock{
			ToolUseID: call.ID,
			Content:   fmt.Sprintf("error: tool %q requires a higher operation level than %q", call.Name, a.config.OperationLevel),
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

func toolRequiredLevel(name string) string {
	if highRiskTools[name] {
		return plan.LevelSafeScan
	}
	return plan.LevelReadOnly
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

func (a *Agent) inferPhase(turn int, history []llm.Message) AgentPhase {
	if turn == 0 {
		return PhasePlanning
	}
	// Check last assistant message for phase indicators
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleAssistant {
			text := ""
			for _, b := range history[i].Content {
				if tb, ok := b.(llm.TextBlock); ok {
					text += tb.Text
				}
			}
			lower := strings.ToLower(text)
			if strings.Contains(lower, "## plan") || strings.Contains(lower, "assessment plan") {
				return PhasePlanning
			}
			if strings.Contains(lower, "## stepreplan") || strings.Contains(lower, "plan unchanged") || strings.Contains(lower, "plan revision") {
				return PhaseReplanning
			}
			if strings.Contains(lower, "## final_answer") || strings.Contains(lower, "final assessment") {
				return PhaseFinalAnswer
			}
			break
		}
	}
	if a.currentPhase == PhasePlanning {
		return PhaseExecuting
	}
	return a.currentPhase
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
