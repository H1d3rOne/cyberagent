package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type NucleiOptions struct {
	Target   string
	Tags     []string
	Severity []string
	Timeout  time.Duration
}

type NucleiResult struct {
	Findings []NucleiFinding
	RawOut   string
	Error    string
}

type NucleiFinding struct {
	TemplateID       string   `json:"template-id"`
	Name             string   `json:"name"`
	Severity         string   `json:"severity"`
	Type             string   `json:"type"`
	Host             string   `json:"host"`
	Matched          string   `json:"matched-at"`
	ExtractedResults []string `json:"extracted-results,omitempty"`
}

func RunNuclei(ctx context.Context, opts NucleiOptions) NucleiResult {
	if _, err := exec.LookPath("nuclei"); err != nil {
		return NucleiResult{Error: "nuclei binary not found in PATH"}
	}

	if opts.Timeout == 0 {
		opts.Timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	args := []string{
		"-target", opts.Target,
		"-jsonl",
		"-silent",
		"-no-color",
	}

	if len(opts.Tags) > 0 {
		args = append(args, "-tags", strings.Join(opts.Tags, ","))
	}
	if len(opts.Severity) > 0 {
		args = append(args, "-severity", strings.Join(opts.Severity, ","))
	}

	cmd := exec.CommandContext(ctx, "nuclei", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NucleiResult{Error: "nuclei timed out", RawOut: stdout.String()}
		}
		if stderr.Len() > 0 {
			return NucleiResult{Error: fmt.Sprintf("nuclei error: %s", stderr.String()), RawOut: stdout.String()}
		}
	}

	var findings []NucleiFinding
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var f NucleiFinding
		if err := json.Unmarshal([]byte(line), &f); err == nil && f.TemplateID != "" {
			findings = append(findings, f)
		}
	}

	return NucleiResult{
		Findings: findings,
		RawOut:   stdout.String(),
	}
}
