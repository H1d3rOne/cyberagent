package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type Server struct {
	mu        sync.RWMutex
	events    []UIEvent
	findings  []contracts.Finding
	status    string
	target    string
	startTime time.Time
	turns     int
	maxTurns  int
	tokensIn  int
	tokensOut int
	clients   map[chan UIEvent]struct{}
}

type UIEvent struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	Tool      string `json:"tool,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Turn      int    `json:"turn,omitempty"`
	MaxTurns  int    `json:"max_turns,omitempty"`
	Timestamp int64  `json:"timestamp"`
	Phase     string `json:"phase,omitempty"`
}

type DashboardState struct {
	Status    string              `json:"status"`
	Target    string              `json:"target"`
	Elapsed   string              `json:"elapsed"`
	Turns     int                 `json:"turns"`
	MaxTurns  int                 `json:"max_turns"`
	TokensIn  int                 `json:"tokens_in"`
	TokensOut int                 `json:"tokens_out"`
	Findings  []contracts.Finding `json:"findings"`
	Events    []UIEvent           `json:"events"`
}

func NewServer(target string, maxTurns int) *Server {
	return &Server{
		target:    target,
		maxTurns:  maxTurns,
		status:    "starting",
		startTime: time.Now(),
		clients:   make(map[chan UIEvent]struct{}),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/events", s.handleSSE)
	mux.Handle("/", FrontendHandler())
	return mux
}

func (s *Server) OnEvent(e agent.Event) {
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

	s.mu.Lock()
	s.events = append(s.events, uiEvent)
	if len(s.events) > 500 {
		s.events = s.events[len(s.events)-500:]
	}

	switch e.Type {
	case agent.EventProgress:
		s.turns = e.Turn
		s.status = "running"
	case agent.EventFinding:
		if e.Finding != nil {
			s.findings = append(s.findings, *e.Finding)
		}
	case agent.EventDone:
		s.status = "completed"
	}
	s.mu.Unlock()

	s.broadcast(uiEvent)
}

func (s *Server) SetTokens(in, out int) {
	s.mu.Lock()
	s.tokensIn = in
	s.tokensOut = out
	s.mu.Unlock()
}

func (s *Server) broadcast(e UIEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.clients {
		select {
		case ch <- e:
		default:
		}
	}
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	state := DashboardState{
		Status:    s.status,
		Target:    s.target,
		Elapsed:   time.Since(s.startTime).Round(time.Second).String(),
		Turns:     s.turns,
		MaxTurns:  s.maxTurns,
		TokensIn:  s.tokensIn,
		TokensOut: s.tokensOut,
		Findings:  s.findings,
		Events:    s.events,
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan UIEvent, 64)
	s.mu.Lock()
	s.clients[ch] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
	}()

	for {
		select {
		case event := <-ch:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
