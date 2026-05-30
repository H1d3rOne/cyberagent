package web

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent"
	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	agenttools "github.com/H1d3rOne/cyberagent/internal/agent/tools"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type ScanStatus string

const (
	ScanStatusRunning   ScanStatus = "running"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusFailed    ScanStatus = "failed"
	ScanStatusStopped   ScanStatus = "stopped"
)

type StartScanRequest struct {
	Target       string `json:"target"`
	Scope        string `json:"scope"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	Instructions string `json:"instructions"`
	MaxTurns     int    `json:"max_turns"`
	MultiAgent   bool   `json:"multi_agent"`
	NoConfirm    bool   `json:"no_confirm"`
	Interactive  bool   `json:"interactive"`
}

type ScanListItem struct {
	ID        string     `json:"id"`
	Target    string     `json:"target"`
	Status    ScanStatus `json:"status"`
	StartedAt time.Time  `json:"started_at"`
	Duration  string     `json:"duration"`
	Findings  int        `json:"findings"`
	Turns     int        `json:"turns"`
	MaxTurns  int        `json:"max_turns"`
	Provider  string     `json:"provider"`
}

type ScanDetailResponse struct {
	ScanListItem
	TokensIn    int                 `json:"tokens_in"`
	TokensOut   int                 `json:"tokens_out"`
	EstCost     float64             `json:"est_cost"`
	FindingsD   []contracts.Finding `json:"findings_detail"`
	Summary     string              `json:"summary"`
	Model       string              `json:"model"`
	Events      []UIEvent           `json:"events"`
	Interactive bool                `json:"interactive"`
	Waiting     bool                `json:"waiting"`
}

type ManagedScan struct {
	mu          sync.RWMutex
	ID          string
	Config      StartScanRequest
	cancel      context.CancelFunc
	Status      ScanStatus
	Result      *agent.Result
	events      []UIEvent
	startedAt   time.Time
	turns       int
	maxTurns    int
	tokensIn    int
	tokensOut   int
	findings    []contracts.Finding
	clients     map[chan UIEvent]struct{}
	interactive *agent.WebInteractiveAgent
	waiting     bool
}

func (ms *ManagedScan) addEvent(e UIEvent) {
	ms.mu.Lock()
	ms.events = append(ms.events, e)
	if len(ms.events) > 1000 {
		ms.events = ms.events[len(ms.events)-1000:]
	}
	clients := make([]chan UIEvent, 0, len(ms.clients))
	for ch := range ms.clients {
		clients = append(clients, ch)
	}
	ms.mu.Unlock()

	for _, ch := range clients {
		select {
		case ch <- e:
		default:
		}
	}
}

func (ms *ManagedScan) subscribe() chan UIEvent {
	ch := make(chan UIEvent, 128)
	ms.mu.Lock()
	ms.clients[ch] = struct{}{}
	ms.mu.Unlock()
	return ch
}

func (ms *ManagedScan) unsubscribe(ch chan UIEvent) {
	ms.mu.Lock()
	delete(ms.clients, ch)
	ms.mu.Unlock()
}

func (ms *ManagedScan) toListItem() ScanListItem {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	dur := time.Since(ms.startedAt)
	if ms.Status != ScanStatusRunning {
		if ms.Result != nil {
			dur = ms.Result.Duration
		}
	}
	return ScanListItem{
		ID:        ms.ID,
		Target:    ms.Config.Target,
		Status:    ms.Status,
		StartedAt: ms.startedAt,
		Duration:  dur.Round(time.Second).String(),
		Findings:  len(ms.findings),
		Turns:     ms.turns,
		MaxTurns:  ms.maxTurns,
		Provider:  ms.Config.Provider,
	}
}

func (ms *ManagedScan) toDetail() ScanDetailResponse {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	item := ms.toListItem()
	detail := ScanDetailResponse{
		ScanListItem: item,
		TokensIn:     ms.tokensIn,
		TokensOut:    ms.tokensOut,
		FindingsD:    ms.findings,
		Model:        ms.Config.Model,
		Events:       ms.events,
		Interactive:  ms.Config.Interactive,
		Waiting:      ms.waiting,
	}
	if ms.Result != nil {
		detail.EstCost = ms.Result.EstCostUSD
		detail.Summary = ms.Result.Summary
	}
	return detail
}

type ScanManager struct {
	mu            sync.RWMutex
	scans         map[string]*ManagedScan
	buildProvider func(name string, cfg llm.ProviderConfig) llm.Provider
}

func NewScanManager(buildProvider func(string, llm.ProviderConfig) llm.Provider) *ScanManager {
	return &ScanManager{
		scans:         make(map[string]*ManagedScan),
		buildProvider: buildProvider,
	}
}

func (sm *ScanManager) List() []ScanListItem {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	items := make([]ScanListItem, 0, len(sm.scans))
	for _, ms := range sm.scans {
		items = append(items, ms.toListItem())
	}
	return items
}

func (sm *ScanManager) Get(id string) (*ManagedScan, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	ms, ok := sm.scans[id]
	return ms, ok
}

func (sm *ScanManager) Start(req StartScanRequest) (string, error) {
	if req.Target == "" {
		return "", fmt.Errorf("target is required")
	}
	if req.MaxTurns == 0 {
		req.MaxTurns = 50
	}
	if req.Provider == "" {
		req.Provider = "anthropic"
	}

	id := fmt.Sprintf("scan_%d", time.Now().UnixNano())

	scopeList := []string{req.Target}
	if req.Scope != "" {
		scopeList = strings.Split(req.Scope, ",")
	}

	provider := sm.buildProvider(req.Provider, llm.ProviderConfig{
		APIKey:  req.APIKey,
		BaseURL: req.BaseURL,
		Model:   req.Model,
	})

	ms := &ManagedScan{
		ID:        id,
		Config:    req,
		Status:    ScanStatusRunning,
		startedAt: time.Now(),
		maxTurns:  req.MaxTurns,
		clients:   make(map[chan UIEvent]struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	ms.cancel = cancel

	sm.mu.Lock()
	sm.scans[id] = ms
	sm.mu.Unlock()

	onEvent := func(e agent.Event) {
		uiEvent := UIEvent{
			Type:      string(e.Type),
			Message:   e.Message,
			Tool:      e.ToolName,
			Turn:      e.Turn,
			MaxTurns:  e.MaxTurns,
			Timestamp: time.Now().UnixMilli(),
			Phase:     string(e.Phase),
		}
		if e.Finding != nil {
			uiEvent.Severity = string(e.Finding.Severity)
		}

		ms.mu.Lock()
		switch e.Type {
		case agent.EventProgress:
			ms.turns = e.Turn
		case agent.EventFinding:
			if e.Finding != nil {
				ms.findings = append(ms.findings, *e.Finding)
			}
		}
		ms.mu.Unlock()

		ms.addEvent(uiEvent)
	}

	if req.Interactive {
		go sm.runInteractive(ms, provider, scopeList, req, onEvent)
	} else if req.MultiAgent {
		go sm.runMultiAgent(ctx, ms, provider, scopeList, req, onEvent)
	} else {
		go sm.runSingleAgent(ctx, ms, provider, scopeList, req, onEvent)
	}

	return id, nil
}

func (sm *ScanManager) runSingleAgent(ctx context.Context, ms *ManagedScan, provider llm.Provider, scope []string, req StartScanRequest, onEvent func(agent.Event)) {
	a := agent.New(agent.Config{
		Provider:     provider,
		Target:       req.Target,
		Scope:        scope,
		Instructions: req.Instructions,
		MaxTurns:     req.MaxTurns,
		NoConfirm:    req.NoConfirm,
		OnEvent:      onEvent,
	})

	result, err := a.Run(ctx)
	ms.mu.Lock()
	if err != nil {
		if ctx.Err() != nil {
			ms.Status = ScanStatusStopped
		} else {
			ms.Status = ScanStatusFailed
		}
	} else {
		ms.Status = ScanStatusCompleted
		ms.Result = result
		ms.tokensIn = result.TokensIn
		ms.tokensOut = result.TokensOut
		ms.findings = result.Findings
	}
	ms.mu.Unlock()

	ms.addEvent(UIEvent{Type: "done", Message: "Scan finished", Timestamp: time.Now().UnixMilli()})
}

func (sm *ScanManager) runMultiAgent(ctx context.Context, ms *ManagedScan, provider llm.Provider, scope []string, req StartScanRequest, onEvent func(agent.Event)) {
	sm.runSingleAgent(ctx, ms, provider, scope, req, onEvent)
}

func (sm *ScanManager) runInteractive(ms *ManagedScan, provider llm.Provider, scope []string, req StartScanRequest, onEvent func(agent.Event)) {
	ia := agent.NewWebInteractive(agent.Config{
		Provider:     provider,
		Target:       req.Target,
		Scope:        scope,
		Instructions: req.Instructions,
		MaxTurns:     req.MaxTurns,
		NoConfirm:    req.NoConfirm,
		OnEvent: func(e agent.Event) {
			onEvent(e)
			if e.Type == agent.EventDone {
				ms.mu.Lock()
				ms.waiting = true
				ms.mu.Unlock()
			}
		},
	})

	ms.mu.Lock()
	ms.interactive = ia
	ms.mu.Unlock()

	ia.Start()

	result := <-ia.Done()

	ms.mu.Lock()
	ms.Status = ScanStatusCompleted
	ms.Result = result
	ms.tokensIn = result.TokensIn
	ms.tokensOut = result.TokensOut
	ms.findings = result.Findings
	ms.waiting = false
	ms.mu.Unlock()

	ms.addEvent(UIEvent{Type: "done", Message: "Interactive session ended", Timestamp: time.Now().UnixMilli()})
}

func (sm *ScanManager) Stop(id string) error {
	sm.mu.RLock()
	ms, ok := sm.scans[id]
	sm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("scan %q not found", id)
	}
	if ms.Status != ScanStatusRunning {
		return fmt.Errorf("scan %q is not running", id)
	}
	if ms.interactive != nil {
		ms.interactive.Stop()
	} else {
		ms.cancel()
	}
	return nil
}

func (sm *ScanManager) SendMessage(id, message string) error {
	sm.mu.RLock()
	ms, ok := sm.scans[id]
	sm.mu.RUnlock()
	if !ok {
		return fmt.Errorf("scan %q not found", id)
	}
	if ms.interactive == nil {
		return fmt.Errorf("scan %q is not in interactive mode", id)
	}
	if ms.Status != ScanStatusRunning {
		return fmt.Errorf("scan %q is not running", id)
	}

	ms.addEvent(UIEvent{
		Type:      "user_message",
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
	})

	ms.mu.Lock()
	ms.waiting = false
	ms.mu.Unlock()

	ms.interactive.SendMessage(message)
	return nil
}

func (sm *ScanManager) Delete(id string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	ms, ok := sm.scans[id]
	if !ok {
		return fmt.Errorf("scan %q not found", id)
	}
	if ms.Status == ScanStatusRunning {
		ms.cancel()
	}
	delete(sm.scans, id)
	return nil
}

func (sm *ScanManager) ToolDefinitions() []llm.ToolDef {
	r := agenttools.NewRegistry()
	return r.Definitions()
}
