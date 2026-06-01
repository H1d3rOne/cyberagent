package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

type scheduleAction string

const (
	// Deprecated: schedules are normalized to agent_prompt during validation.
	scheduleActionAgentPrompt scheduleAction = "agent_prompt"
	// Deprecated: legacy scan_web entries are migrated into Prompt text.
	scheduleActionScanWeb scheduleAction = "scan_web"
)

type scheduleSpec struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Cron      string         `json:"cron"`
	Action    scheduleAction `json:"action"`
	Enabled   bool           `json:"enabled"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`

	SessionID string `json:"session_id,omitempty"`
	Operator  string `json:"operator,omitempty"`
	Language  string `json:"language,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	OpsLevel  string `json:"ops_level,omitempty"`

	Target  string `json:"target,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Profile string `json:"profile,omitempty"`

	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastStatus    string     `json:"last_status,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
	LastJobID     string     `json:"last_job_id,omitempty"`
	LastMessageID string     `json:"last_message_id,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
	InFlight      bool       `json:"in_flight,omitempty"`
}

type scheduleRunner struct {
	SendAgentPrompt func(ctx context.Context, s scheduleSpec) (msgID string, err error)
	// Deprecated: scan_web schedules are migrated into SendAgentPrompt.
	StartScanWeb func(ctx context.Context, s scheduleSpec) (jobID string, err error)
}

type scheduleManager struct {
	mu       sync.Mutex
	storeDir string
	parser   cron.Parser
	cron     *cron.Cron
	entries  map[string]cron.EntryID
	items    map[string]*scheduleSpec
	runner   scheduleRunner
	started  bool
}

func newScheduleManager(runsDir string) *scheduleManager {
	if strings.TrimSpace(runsDir) == "" {
		runsDir = DefaultRunsDir
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	m := &scheduleManager{
		storeDir: filepath.Join(runsDir, ".agent", "schedules"),
		parser:   parser,
		cron:     cron.New(cron.WithLocation(time.Local), cron.WithParser(parser)),
		entries:  map[string]cron.EntryID{},
		items:    map[string]*scheduleSpec{},
	}
	_ = m.loadLocked()
	return m
}

func (m *scheduleManager) BindRunner(runner scheduleRunner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runner = runner
}

func (m *scheduleManager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	for id, spec := range m.items {
		if spec.Enabled {
			_ = m.scheduleEntryLocked(id)
		}
	}
	m.mu.Unlock()
	m.cron.Start()
	m.refreshNextRunAll()
}

func (m *scheduleManager) Stop(ctx context.Context) {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	m.started = false
	m.mu.Unlock()
	stopCtx := m.cron.Stop()
	if ctx == nil {
		<-stopCtx.Done()
		return
	}
	select {
	case <-stopCtx.Done():
	case <-ctx.Done():
	}
}

func (m *scheduleManager) loadLocked() error {
	if strings.TrimSpace(m.storeDir) == "" {
		return nil
	}
	entries, err := os.ReadDir(m.storeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(m.storeDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var spec scheduleSpec
		if err := json.Unmarshal(data, &spec); err != nil || strings.TrimSpace(spec.ID) == "" {
			continue
		}
		if err := m.validateLocked(&spec); err != nil {
			continue
		}
		spec.InFlight = false
		clone := spec
		m.items[spec.ID] = &clone
		_ = m.persistLocked(&clone)
	}
	return nil
}

func (m *scheduleManager) persistLocked(spec *scheduleSpec) error {
	if spec == nil || strings.TrimSpace(spec.ID) == "" {
		return nil
	}
	if err := os.MkdirAll(m.storeDir, 0o755); err != nil {
		return services.NewError(services.CodeInternal, "create schedule store", map[string]any{"path": m.storeDir}, err)
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return services.NewError(services.CodeInternal, "encode schedule", map[string]any{"id": spec.ID}, err)
	}
	path := filepath.Join(m.storeDir, spec.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return services.NewError(services.CodeInternal, "write schedule", map[string]any{"id": spec.ID}, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return services.NewError(services.CodeInternal, "commit schedule", map[string]any{"id": spec.ID}, err)
	}
	return nil
}

func (m *scheduleManager) deleteFileLocked(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	_ = os.Remove(filepath.Join(m.storeDir, id+".json"))
}

func (m *scheduleManager) validateLocked(spec *scheduleSpec) error {
	if spec == nil {
		return services.NewError(services.CodeBadRequest, "schedule body required", nil, nil)
	}
	spec.Cron = strings.TrimSpace(spec.Cron)
	if spec.Cron == "" {
		return services.NewError(services.CodeBadRequest, "cron required", map[string]any{"field": "cron"}, nil)
	}
	if _, err := m.parser.Parse(spec.Cron); err != nil {
		return services.NewError(services.CodeBadRequest, "invalid cron expression", map[string]any{"field": "cron", "value": spec.Cron}, err)
	}
	normalizeSchedulePromptSpec(spec)
	switch spec.Action {
	case scheduleActionAgentPrompt:
		if strings.TrimSpace(spec.Prompt) == "" {
			return services.NewError(services.CodeBadRequest, "prompt required for schedule", map[string]any{"field": "prompt"}, nil)
		}
		if strings.TrimSpace(spec.Operator) == "" {
			spec.Operator = "scheduler"
		}
		if strings.TrimSpace(spec.Language) == "" {
			spec.Language = "zh"
		}
	default:
		return services.NewError(services.CodeBadRequest, "unsupported action", map[string]any{"field": "action", "value": string(spec.Action)}, nil)
	}
	if strings.TrimSpace(spec.Name) == "" {
		spec.Name = string(spec.Action) + " · " + spec.Cron
	}
	return nil
}

func normalizeSchedulePromptSpec(spec *scheduleSpec) {
	if spec == nil {
		return
	}
	spec.Action = scheduleAction(strings.ToLower(strings.TrimSpace(string(spec.Action))))
	switch spec.Action {
	case "", scheduleActionAgentPrompt:
		spec.Action = scheduleActionAgentPrompt
	case scheduleActionScanWeb:
		if strings.TrimSpace(spec.Prompt) == "" {
			spec.Prompt = schedulePromptFromLegacyScanWeb(*spec)
		}
		spec.Action = scheduleActionAgentPrompt
	default:
		if strings.TrimSpace(spec.Prompt) != "" {
			spec.Action = scheduleActionAgentPrompt
		}
	}
	if strings.TrimSpace(spec.OpsLevel) == "" {
		spec.OpsLevel = "safe_scan"
	} else {
		spec.OpsLevel = normalizeAgentOperationLevel(spec.OpsLevel)
	}
}

func schedulePromptFromLegacyScanWeb(spec scheduleSpec) string {
	target := strings.TrimSpace(spec.Target)
	if target == "" {
		return ""
	}
	prompt := "扫描 " + target
	if scope := strings.TrimSpace(spec.Scope); scope != "" {
		prompt += " scope=" + scope
	}
	if profile := strings.TrimSpace(spec.Profile); profile != "" {
		prompt += " profile=" + profile
	}
	return prompt
}

func (m *scheduleManager) Create(spec scheduleSpec) (scheduleSpec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	spec.ID = strings.Replace(runs.NewJobID(now), "job_", "sched_", 1)
	spec.CreatedAt = now
	spec.UpdatedAt = now
	spec.LastRunAt = nil
	spec.LastStatus = ""
	spec.LastError = ""
	spec.InFlight = false
	if err := m.validateLocked(&spec); err != nil {
		return scheduleSpec{}, err
	}
	clone := spec
	m.items[spec.ID] = &clone
	if err := m.persistLocked(&clone); err != nil {
		delete(m.items, spec.ID)
		return scheduleSpec{}, err
	}
	if clone.Enabled && m.started {
		_ = m.scheduleEntryLocked(spec.ID)
	}
	out := clone
	out.NextRunAt = m.nextRunForLocked(spec.ID)
	return out, nil
}

func (m *scheduleManager) Update(id string, patch scheduleSpec) (scheduleSpec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[id]
	if !ok {
		return scheduleSpec{}, scheduleNotFound(id)
	}
	merged := *current
	if patch.Name != "" {
		merged.Name = patch.Name
	}
	if patch.Cron != "" {
		merged.Cron = patch.Cron
	}
	if patch.Action != "" {
		merged.Action = patch.Action
	}
	merged.SessionID = patch.SessionID
	merged.Operator = patch.Operator
	merged.Language = patch.Language
	merged.Prompt = patch.Prompt
	merged.OpsLevel = patch.OpsLevel
	merged.Target = patch.Target
	merged.Scope = patch.Scope
	merged.Profile = patch.Profile
	merged.Enabled = patch.Enabled
	merged.UpdatedAt = time.Now().UTC()
	if err := m.validateLocked(&merged); err != nil {
		return scheduleSpec{}, err
	}
	m.unscheduleEntryLocked(id)
	clone := merged
	m.items[id] = &clone
	if err := m.persistLocked(&clone); err != nil {
		return scheduleSpec{}, err
	}
	if clone.Enabled && m.started {
		_ = m.scheduleEntryLocked(id)
	}
	out := clone
	out.NextRunAt = m.nextRunForLocked(id)
	return out, nil
}

func (m *scheduleManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[id]; !ok {
		return scheduleNotFound(id)
	}
	m.unscheduleEntryLocked(id)
	delete(m.items, id)
	m.deleteFileLocked(id)
	return nil
}

func (m *scheduleManager) Get(id string) (scheduleSpec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[id]
	if !ok {
		return scheduleSpec{}, scheduleNotFound(id)
	}
	out := *current
	out.NextRunAt = m.nextRunForLocked(id)
	return out, nil
}

func (m *scheduleManager) List() []scheduleSpec {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]scheduleSpec, 0, len(m.items))
	for id, spec := range m.items {
		out := *spec
		out.NextRunAt = m.nextRunForLocked(id)
		result = append(result, out)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (m *scheduleManager) SetEnabled(id string, enabled bool) (scheduleSpec, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[id]
	if !ok {
		return scheduleSpec{}, scheduleNotFound(id)
	}
	if current.Enabled == enabled {
		out := *current
		out.NextRunAt = m.nextRunForLocked(id)
		return out, nil
	}
	current.Enabled = enabled
	current.UpdatedAt = time.Now().UTC()
	if err := m.persistLocked(current); err != nil {
		return scheduleSpec{}, err
	}
	if enabled && m.started {
		_ = m.scheduleEntryLocked(id)
	} else {
		m.unscheduleEntryLocked(id)
	}
	out := *current
	out.NextRunAt = m.nextRunForLocked(id)
	return out, nil
}

func (m *scheduleManager) RunNow(ctx context.Context, id string) (scheduleSpec, error) {
	m.mu.Lock()
	current, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return scheduleSpec{}, scheduleNotFound(id)
	}
	clone := *current
	m.mu.Unlock()
	m.fire(ctx, clone.ID)
	return m.Get(id)
}

func (m *scheduleManager) scheduleEntryLocked(id string) error {
	spec, ok := m.items[id]
	if !ok {
		return scheduleNotFound(id)
	}
	if existing, has := m.entries[id]; has {
		m.cron.Remove(existing)
		delete(m.entries, id)
	}
	entryID, err := m.cron.AddFunc(spec.Cron, func() {
		m.fire(context.Background(), id)
	})
	if err != nil {
		return services.NewError(services.CodeBadRequest, "register cron entry", map[string]any{"id": id, "cron": spec.Cron}, err)
	}
	m.entries[id] = entryID
	return nil
}

func (m *scheduleManager) unscheduleEntryLocked(id string) {
	if entryID, has := m.entries[id]; has {
		m.cron.Remove(entryID)
		delete(m.entries, id)
	}
}

func (m *scheduleManager) nextRunForLocked(id string) *time.Time {
	entryID, has := m.entries[id]
	if !has {
		return nil
	}
	entry := m.cron.Entry(entryID)
	if entry.ID == 0 || entry.Next.IsZero() {
		return nil
	}
	t := entry.Next
	return &t
}

func (m *scheduleManager) refreshNextRunAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.items {
		_ = m.nextRunForLocked(id)
	}
}

func (m *scheduleManager) fire(ctx context.Context, id string) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()

	m.mu.Lock()
	spec, ok := m.items[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	if spec.InFlight {
		spec.LastRunAt = &now
		spec.LastStatus = "skipped"
		spec.LastError = ""
		_ = m.persistLocked(spec)
		m.mu.Unlock()
		return
	}
	spec.InFlight = true
	clone := *spec
	runner := m.runner
	m.mu.Unlock()

	var (
		jobID  string
		msgID  string
		runErr error
	)
	switch clone.Action {
	case scheduleActionAgentPrompt:
		if runner.SendAgentPrompt == nil {
			runErr = errors.New("agent prompt runner not bound")
			break
		}
		msgID, runErr = runner.SendAgentPrompt(ctx, clone)
	default:
		runErr = fmt.Errorf("unsupported action %q", clone.Action)
	}

	finishedAt := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[id]
	if !ok {
		return
	}
	current.InFlight = false
	current.LastRunAt = &finishedAt
	if runErr != nil {
		current.LastStatus = "failed"
		current.LastError = runErr.Error()
	} else {
		current.LastStatus = "success"
		current.LastError = ""
	}
	current.LastJobID = jobID
	current.LastMessageID = msgID
	_ = m.persistLocked(current)
}

func scheduleNotFound(id string) error {
	return services.NewError(services.CodeNotFound, "schedule not found", map[string]any{"id": id}, nil)
}
