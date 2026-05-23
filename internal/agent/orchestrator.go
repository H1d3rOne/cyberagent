package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type OrchestratorConfig struct {
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

type OrchestratorResult struct {
	ReconResult *Result
	DeepResult  *Result
	Combined    *Result
}

type Orchestrator struct {
	config OrchestratorConfig
}

func NewOrchestrator(cfg OrchestratorConfig) *Orchestrator {
	return &Orchestrator{config: cfg}
}

func (o *Orchestrator) Run(ctx context.Context) (*OrchestratorResult, error) {
	start := time.Now()

	reconTurns := o.config.MaxTurns / 3
	if reconTurns < 10 {
		reconTurns = 10
	}
	deepTurns := o.config.MaxTurns - reconTurns

	o.emitProgress("Starting reconnaissance agent...")

	reconAgent := New(Config{
		Provider:     o.config.Provider,
		Target:       o.config.Target,
		Scope:        o.config.Scope,
		Instructions: o.buildReconInstructions(),
		MaxTurns:     reconTurns,
		NoConfirm:    o.config.NoConfirm,
		OutDir:       o.config.OutDir,
		OnEvent:      o.wrapEvent("recon"),
		OnConfirm:    o.config.OnConfirm,
	})

	reconResult, err := reconAgent.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("recon agent: %w", err)
	}

	o.emitProgress(fmt.Sprintf("Recon complete: %d findings, %d turns. Starting deep analysis...", len(reconResult.Findings), reconResult.Turns))

	deepInstructions := o.buildDeepInstructions(reconResult)

	deepAgent := New(Config{
		Provider:     o.config.Provider,
		Target:       o.config.Target,
		Scope:        o.config.Scope,
		Instructions: deepInstructions,
		MaxTurns:     deepTurns,
		NoConfirm:    o.config.NoConfirm,
		OutDir:       o.config.OutDir,
		OnEvent:      o.wrapEvent("deep"),
		OnConfirm:    o.config.OnConfirm,
	})

	deepResult, err := deepAgent.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("deep agent: %w", err)
	}

	combined := o.mergeResults(reconResult, deepResult, time.Since(start))

	o.emitProgress(fmt.Sprintf("Multi-agent scan complete: %d total findings", len(combined.Findings)))

	return &OrchestratorResult{
		ReconResult: reconResult,
		DeepResult:  deepResult,
		Combined:    combined,
	}, nil
}

func (o *Orchestrator) buildReconInstructions() string {
	base := `You are the RECONNAISSANCE agent. Your job is to map the attack surface quickly and broadly.

Focus on:
- Probing target reachability and technology fingerprinting
- Fetching robots.txt and sitemap.xml
- Extracting HTML structure (links, forms, scripts)
- Discovering paths with wordlist
- Checking DNS records
- Inspecting TLS configuration
- Identifying open ports

Do NOT spend time on deep analysis. Record preliminary findings (info/low severity) and move on.
When you have mapped the surface sufficiently, stop and summarize what you found.`

	if o.config.Instructions != "" {
		base += "\n\nAdditional context: " + o.config.Instructions
	}
	return base
}

func (o *Orchestrator) buildDeepInstructions(reconResult *Result) string {
	var sb strings.Builder
	sb.WriteString(`You are the DEEP ANALYSIS agent. A reconnaissance agent has already mapped the attack surface.

Your job is to perform focused, in-depth security analysis based on the recon findings below.

Focus on:
- Verifying and upgrading preliminary findings to confirmed vulnerabilities
- Checking security headers and cookies in detail
- Analyzing JavaScript for sensitive routes and data exposure
- Testing specific endpoints identified during recon
- Looking for authentication/authorization issues
- Checking for information disclosure

`)

	sb.WriteString("## Recon Summary\n")
	sb.WriteString(reconResult.Summary)
	sb.WriteString("\n\n")

	if len(reconResult.Findings) > 0 {
		sb.WriteString("## Preliminary Findings from Recon\n")
		for _, f := range reconResult.Findings {
			sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", f.Severity, f.Title, f.Description))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Investigate these findings deeper. Confirm or dismiss them with evidence. Look for additional vulnerabilities the recon agent may have missed.\n")

	if o.config.Instructions != "" {
		sb.WriteString("\nAdditional context: " + o.config.Instructions)
	}

	return sb.String()
}

func (o *Orchestrator) mergeResults(recon, deep *Result, totalDuration time.Duration) *Result {
	seen := map[string]bool{}
	var findings []contracts.Finding

	for _, f := range deep.Findings {
		key := f.Title + "|" + strings.Join(f.AffectedTargets, ",")
		if !seen[key] {
			seen[key] = true
			findings = append(findings, f)
		}
	}
	for _, f := range recon.Findings {
		key := f.Title + "|" + strings.Join(f.AffectedTargets, ",")
		if !seen[key] {
			seen[key] = true
			findings = append(findings, f)
		}
	}

	return &Result{
		Findings:   findings,
		Summary:    fmt.Sprintf("Recon: %s\n\nDeep Analysis: %s", recon.Summary, deep.Summary),
		Turns:      recon.Turns + deep.Turns,
		Duration:   totalDuration,
		TokensIn:   recon.TokensIn + deep.TokensIn,
		TokensOut:  recon.TokensOut + deep.TokensOut,
		EstCostUSD: recon.EstCostUSD + deep.EstCostUSD,
	}
}

func (o *Orchestrator) wrapEvent(phase string) func(Event) {
	return func(e Event) {
		e.Message = fmt.Sprintf("[%s] %s", phase, e.Message)
		if o.config.OnEvent != nil {
			o.config.OnEvent(e)
		}
	}
}

func (o *Orchestrator) emitProgress(msg string) {
	if o.config.OnEvent != nil {
		o.config.OnEvent(Event{Type: EventProgress, Message: msg})
	}
}
