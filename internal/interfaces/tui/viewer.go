package tui

import (
	"encoding/json"
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
	RunDir   string
	Job      contracts.Job
	Plan     contracts.Plan
	Events   []contracts.Event
	Evidence []contracts.Evidence
	Findings []contracts.Finding
	Report   string
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

type model struct {
	data     RunData
	tab      int
	scroll   int
	width    int
	height   int
	sections []string
}

func newModel(data RunData) model {
	return model{data: data, sections: []string{"Plan", "Events", "Evidence", "Findings", "Report"}}
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
		case "1", "2", "3", "4", "5":
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
	b.WriteString(muted.Render("Use ←/→ or 1-5 to switch, j/k to scroll, q to quit."))
	b.WriteString("\n\n")

	var content string
	switch m.sections[m.tab] {
	case "Plan":
		content = renderPlan(m.data.Plan)
	case "Events":
		content = renderEvents(m.data.Events)
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

func tailEvents(events []contracts.Event, max int) []contracts.Event {
	if len(events) <= max {
		return events
	}
	return events[len(events)-max:]
}
