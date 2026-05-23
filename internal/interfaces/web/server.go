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
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/events", s.handleSSE)
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

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, dashboardHTML)
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>CyberAgent Dashboard</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', monospace; background: #0d1117; color: #c9d1d9; padding: 20px; }
h1 { color: #58a6ff; margin-bottom: 20px; }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin-bottom: 20px; }
.card { background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 16px; }
.card h2 { color: #8b949e; font-size: 12px; text-transform: uppercase; margin-bottom: 8px; }
.card .value { font-size: 24px; font-weight: bold; color: #f0f6fc; }
.status-running { color: #3fb950; }
.status-completed { color: #58a6ff; }
.status-starting { color: #d29922; }
.findings { margin-bottom: 20px; }
.finding { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 12px; margin-bottom: 8px; }
.finding .title { font-weight: bold; }
.sev-critical { border-left: 3px solid #f85149; }
.sev-high { border-left: 3px solid #da3633; }
.sev-medium { border-left: 3px solid #d29922; }
.sev-low { border-left: 3px solid #58a6ff; }
.sev-info { border-left: 3px solid #8b949e; }
.events { background: #161b22; border: 1px solid #30363d; border-radius: 8px; padding: 16px; max-height: 400px; overflow-y: auto; }
.event { font-size: 13px; padding: 4px 0; border-bottom: 1px solid #21262d; }
.event .time { color: #8b949e; margin-right: 8px; }
.event .tool { color: #d2a8ff; }
.event .finding-ev { color: #3fb950; }
</style>
</head>
<body>
<h1>CyberAgent Dashboard</h1>
<div class="grid">
  <div class="card"><h2>Status</h2><div class="value" id="status">starting</div></div>
  <div class="card"><h2>Target</h2><div class="value" id="target">-</div></div>
  <div class="card"><h2>Progress</h2><div class="value" id="progress">0 / 0</div></div>
  <div class="card"><h2>Elapsed</h2><div class="value" id="elapsed">0s</div></div>
  <div class="card"><h2>Tokens</h2><div class="value" id="tokens">0 / 0</div></div>
  <div class="card"><h2>Findings</h2><div class="value" id="finding-count">0</div></div>
</div>
<div class="findings" id="findings"></div>
<h2 style="color:#8b949e;margin-bottom:10px;">Event Log</h2>
<div class="events" id="events"></div>
<script>
async function loadState() {
  const r = await fetch('/api/state');
  const s = await r.json();
  document.getElementById('status').textContent = s.status;
  document.getElementById('status').className = 'value status-' + s.status;
  document.getElementById('target').textContent = s.target;
  document.getElementById('progress').textContent = s.turns + ' / ' + s.max_turns;
  document.getElementById('elapsed').textContent = s.elapsed;
  document.getElementById('tokens').textContent = s.tokens_in + ' in / ' + s.tokens_out + ' out';
  document.getElementById('finding-count').textContent = (s.findings||[]).length;
  renderFindings(s.findings||[]);
}
function renderFindings(findings) {
  const el = document.getElementById('findings');
  el.innerHTML = findings.map(f =>
    '<div class="finding sev-'+f.severity+'"><span class="title">['+f.severity+'] '+f.title+'</span><br><small>'+f.description+'</small></div>'
  ).join('');
}
function addEvent(e) {
  const el = document.getElementById('events');
  const t = new Date(e.timestamp).toLocaleTimeString();
  let cls = '', msg = e.message || '';
  if (e.type === 'tool_call') { cls = 'tool'; msg = 'tool: ' + e.tool; }
  else if (e.type === 'finding') { cls = 'finding-ev'; }
  el.innerHTML = '<div class="event"><span class="time">'+t+'</span><span class="'+cls+'">'+msg+'</span></div>' + el.innerHTML;
  if (el.children.length > 200) el.removeChild(el.lastChild);
  if (e.type === 'progress') {
    document.getElementById('progress').textContent = e.turn + ' / ' + e.max_turns;
  }
}
loadState();
const es = new EventSource('/api/events');
es.onmessage = function(ev) {
  const e = JSON.parse(ev.data);
  addEvent(e);
  if (e.type === 'finding') loadState();
  if (e.type === 'done') { loadState(); document.getElementById('status').textContent = 'completed'; }
};
setInterval(loadState, 5000);
</script>
</body>
</html>`
