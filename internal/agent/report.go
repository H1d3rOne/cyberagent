package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type ReportOutput struct {
	MDPath   string
	JSONPath string
}

func GenerateReport(workspace string, result *Result, target string) (*ReportOutput, error) {
	reportDir := filepath.Join(workspace, "reports")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return nil, fmt.Errorf("create report dir: %w", err)
	}

	md := renderAgentMarkdown(result, target)
	mdPath := filepath.Join(reportDir, "agent_report.md")
	if err := os.WriteFile(mdPath, []byte(md), 0o644); err != nil {
		return nil, fmt.Errorf("write markdown report: %w", err)
	}

	jsonReport := map[string]interface{}{
		"target":      target,
		"turns":       result.Turns,
		"duration_ms": result.Duration.Milliseconds(),
		"summary":     result.Summary,
		"findings":    result.Findings,
		"generated":   time.Now().UTC().Format(time.RFC3339),
	}
	jsonBytes, err := json.MarshalIndent(jsonReport, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal json report: %w", err)
	}
	jsonPath := filepath.Join(reportDir, "agent_report.json")
	if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
		return nil, fmt.Errorf("write json report: %w", err)
	}

	return &ReportOutput{MDPath: mdPath, JSONPath: jsonPath}, nil
}

func renderAgentMarkdown(result *Result, target string) string {
	var b strings.Builder
	b.WriteString("# CyberAgent Security Assessment Report\n\n")
	b.WriteString(fmt.Sprintf("**Target:** %s\n", target))
	b.WriteString(fmt.Sprintf("**Duration:** %s\n", result.Duration.Round(time.Millisecond)))
	b.WriteString(fmt.Sprintf("**Agent Turns:** %d\n\n", result.Turns))

	b.WriteString("## Summary\n\n")
	b.WriteString(result.Summary)
	b.WriteString("\n\n")

	if len(result.Findings) == 0 {
		b.WriteString("## Findings\n\nNo security issues identified.\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("## Findings (%d)\n\n", len(result.Findings)))

	grouped := groupBySeverity(result.Findings)
	for _, sev := range []contracts.Severity{"critical", "high", "medium", "low", "info"} {
		items := grouped[sev]
		if len(items) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("### %s (%d)\n\n", strings.ToUpper(string(sev)), len(items)))
		for _, f := range items {
			b.WriteString(fmt.Sprintf("#### %s\n\n", f.Title))
			b.WriteString(fmt.Sprintf("%s\n\n", f.Description))
			if f.Remediation != "" {
				b.WriteString(fmt.Sprintf("**Remediation:** %s\n\n", f.Remediation))
			}
			b.WriteString("---\n\n")
		}
	}
	return b.String()
}

func groupBySeverity(findings []contracts.Finding) map[contracts.Severity][]contracts.Finding {
	m := make(map[contracts.Severity][]contracts.Finding)
	for _, f := range findings {
		m[f.Severity] = append(m[f.Severity], f)
	}
	return m
}
