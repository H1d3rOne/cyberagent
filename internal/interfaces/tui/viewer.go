package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
)

// RunViewer opens the read-only TUI viewer for an existing run directory.
func RunViewer(runDir string) error {
	data, err := LoadRun(runDir)
	if err != nil {
		return err
	}
	program := tea.NewProgram(newModel(data))
	_, err = program.Run()
	return err
}

// RunData is the read-only run snapshot consumed by the TUI.
type RunData struct {
	RunDir    string
	Job       contracts.Job
	Plan      contracts.Plan
	Events    []contracts.Event
	Approvals []contracts.HITLRequest
	Evidence  []contracts.Evidence
	Findings  []contracts.Finding
	Report    string
}

// LoadRun loads known run files for display.
func LoadRun(runDir string) (RunData, error) {
	var data RunData
	data.RunDir = runDir
	if err := readJSON(filepath.Join(runDir, "job.json"), &data.Job); err != nil {
		return RunData{}, err
	}
	if err := readJSON(filepath.Join(runDir, "plan.json"), &data.Plan); err != nil {
		return RunData{}, err
	}
	if err := readJSON(filepath.Join(runDir, "findings.json"), &data.Findings); err != nil {
		return RunData{}, err
	}
	if err := readOptionalJSON(filepath.Join(runDir, "approvals.json"), &data.Approvals); err != nil {
		return RunData{}, err
	}
	var evidenceIndex struct {
		Evidence []contracts.Evidence `json:"evidence"`
	}
	if err := readJSON(filepath.Join(runDir, "evidence", "index.json"), &evidenceIndex); err != nil {
		return RunData{}, err
	}
	data.Evidence = evidenceIndex.Evidence
	eventStore := events.NewStore(filepath.Join(runDir, "events.jsonl"))
	events, err := eventStore.ReadAll()
	if err != nil {
		return RunData{}, err
	}
	data.Events = events
	report, err := os.ReadFile(filepath.Join(runDir, "reports", "report.md"))
	if err == nil {
		data.Report = string(report)
	}
	return data, nil
}

func readJSON(path string, out any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func readOptionalJSON(path string, out any) error {
	if err := readJSON(path, out); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

type model struct {
	data     RunData
	tab      int
	scroll   int
	width    int
	height   int
	sections []string
}

func newModel(data RunData) model {
	return model{data: data, sections: []string{"Plan", "Events", "Approvals", "Evidence", "Findings", "Report"}}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "right", "tab", "l":
			m.tab = (m.tab + 1) % len(m.sections)
			m.scroll = 0
		case "left", "shift+tab", "h":
			m.tab--
			if m.tab < 0 {
				m.tab = len(m.sections) - 1
			}
			m.scroll = 0
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(m.sections) {
				m.tab = idx
				m.scroll = 0
			}
		case "j", "down":
			m.scroll++
		case "k", "up":
			if m.scroll > 0 {
				m.scroll--
			}
		case "g", "home":
			m.scroll = 0
		case "G", "end":
			m.scroll = 9999
		}
	}
	return m, nil
}

func (m model) View() string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", headerStyle.Render("CyberAgent Run Viewer"))
	fmt.Fprintf(&b, "job: %s | status: %s | run: %s\n\n", m.data.Job.ID, m.data.Job.Status, m.data.RunDir)
	for i, section := range m.sections {
		label := fmt.Sprintf("[%d] %s", i+1, section)
		if i == m.tab {
			b.WriteString(activeStyle.Render(label))
		} else {
			b.WriteString(muted.Render(label))
		}
		if i < len(m.sections)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n")
	b.WriteString(muted.Render(fmt.Sprintf("Use ←/→ or 1-%d to switch, j/k to scroll, q to quit.", len(m.sections))))
	b.WriteString("\n\n")

	var content string
	switch m.sections[m.tab] {
	case "Plan":
		content = renderPlan(m.data.Plan)
	case "Events":
		content = renderEvents(m.data.Events)
	case "Approvals":
		content = renderApprovals(m.data.Approvals)
	case "Evidence":
		content = renderEvidence(m.data.Evidence)
	case "Findings":
		content = renderFindings(m.data.Findings)
	case "Report":
		content = renderReport(m.data.Report)
	}

	lines := strings.Split(content, "\n")
	viewHeight := m.height - 6
	if viewHeight < 5 {
		viewHeight = 20
	}
	if m.scroll > len(lines)-viewHeight {
		m.scroll = len(lines) - viewHeight
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
	end := m.scroll + viewHeight
	if end > len(lines) {
		end = len(lines)
	}
	b.WriteString(strings.Join(lines[m.scroll:end], "\n"))
	b.WriteString("\n")
	return b.String()
}

func renderPlan(plan contracts.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan: %s | planner: %s | steps: %d\n\n", plan.ID, plan.Planner, len(plan.Steps))
	for _, step := range plan.Steps {
		fmt.Fprintf(&b, "- %s\n", step)
	}
	return b.String()
}

func renderApprovals(items []contracts.HITLRequest) string {
	if len(items) == 0 {
		return "No approval requests.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Approvals are read-only in the TUI. Use `cyberagent approvals approve|deny` from CLI to resolve.\n\n")
	for _, item := range items {
		fmt.Fprintf(&b, "- [%s] %s\n", item.Status, item.ID)
		fmt.Fprintf(&b, "  step: %s\n", emptyDash(item.StepID))
		fmt.Fprintf(&b, "  action: %s\n", emptyDash(item.RequestedAction))
		fmt.Fprintf(&b, "  policy: %s\n", emptyDash(item.PolicyDecisionID))
		fmt.Fprintf(&b, "  reason: %s\n", emptyDash(item.Reason))
		if len(item.Options) > 0 {
			fmt.Fprintf(&b, "  options: %s\n", strings.Join(item.Options, ", "))
		}
		if item.ResolvedAt != nil {
			fmt.Fprintf(&b, "  resolved_at: %s\n", item.ResolvedAt.Format("2006-01-02 15:04:05 MST"))
		}
		if len(item.Resolution) > 0 {
			fmt.Fprintf(&b, "  resolution: %v\n", item.Resolution)
		}
	}
	return b.String()
}

func renderEvents(items []contracts.Event) string {
	var b strings.Builder
	for _, event := range tailEvents(items, 500) {
		step := event.StepID
		if step == "" {
			step = "-"
		}
		fmt.Fprintf(&b, "%s %-22s %-7s %s %s\n", event.Timestamp.Format("15:04:05"), event.Type, event.Level, step, event.Message)
	}
	return b.String()
}

func renderEvidence(items []contracts.Evidence) string {
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "- %s [%s] %s -> %s\n", item.ID, item.Type, item.Title, item.TargetRef)
	}
	return b.String()
}

func renderFindings(items []contracts.Finding) string {
	if len(items) == 0 {
		return "No findings.\n"
	}
	var b strings.Builder
	for _, item := range items {
		fmt.Fprintf(&b, "- [%s/%s] %s\n  evidence: %s\n", item.Severity, item.Confidence, item.Title, strings.Join(item.EvidenceIDs, ", "))
	}
	return b.String()
}

func renderReport(report string) string {
	if report == "" {
		return "report.md not found or empty.\n"
	}
	return report + "\n"
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func tailEvents(events []contracts.Event, max int) []contracts.Event {
	if len(events) <= max {
		return events
	}
	return events[len(events)-max:]
}
