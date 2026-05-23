package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
)

// RunRequest describes one plugin execution request.
type RunRequest struct {
	ID             string
	JobID          string
	StepID         string
	Target         string
	Scope          contracts.Scope
	WorkDir        string
	Timeout        time.Duration
	Recorder       *command.Recorder
	Input          map[string]any
	Artifacts      []contracts.Artifact
	Evidence       []contracts.Evidence
	MaxStdoutBytes int64
	MaxStderrBytes int64
}

// RunResult captures plugin execution output and status.
type RunResult struct {
	Call     contracts.ToolCall
	Stdout   []byte
	Stderr   []byte
	Skipped  bool
	Reason   string
	Messages []SubprocessMessage
	Routes   []PluginRoute
	Findings []PluginFinding
}

// PluginFinding is a minimal normalized finding parsed from plugin output.
type PluginFinding struct {
	TemplateID string         `json:"template_id,omitempty"`
	Name       string         `json:"name,omitempty"`
	Severity   string         `json:"severity,omitempty"`
	Matched    string         `json:"matched,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

// Runner executes supported MVP plugin manifests through the command boundary.
type Runner struct {
	Executor command.Executor
}

const (
	defaultPluginMaxStdoutBytes int64 = 1024 * 1024
	defaultPluginMaxStderrBytes int64 = 256 * 1024
)

// NewRunner creates a plugin runner.
func NewRunner(recorder *command.Recorder) Runner {
	return Runner{Executor: command.NewExecutor(recorder)}
}

// Run executes one supported plugin. Missing optional dependencies return a skipped result.
func (r Runner) Run(ctx context.Context, manifest contracts.PluginManifest, req RunRequest) (RunResult, error) {
	switch manifest.Runtime {
	case "external_command":
		return r.runExternalCommand(ctx, manifest, req)
	case "subprocess_jsonl":
		return r.runSubprocessJSONL(ctx, manifest, req)
	default:
		return RunResult{}, fmt.Errorf("unsupported plugin runtime %q", manifest.Runtime)
	}
}

func (r Runner) runExternalCommand(ctx context.Context, manifest contracts.PluginManifest, req RunRequest) (RunResult, error) {
	switch manifest.Name {
	case "nuclei-safe":
		return r.runNucleiSafe(ctx, manifest, req)
	case "ffuf-low-rate":
		return r.runFFUFLowRate(ctx, manifest, req)
	default:
		return RunResult{}, fmt.Errorf("unsupported plugin %q", manifest.Name)
	}
}

func (r Runner) runNucleiSafe(ctx context.Context, manifest contracts.PluginManifest, req RunRequest) (RunResult, error) {
	if _, err := exec.LookPath("nuclei"); err != nil {
		return r.dependencyMissingPluginResult(req, manifest, "dependency nuclei not found in PATH")
	}

	args := []string{
		"-target", req.Target,
		"-severity", "info,low",
		"-jsonl",
		"-silent",
		"-no-color",
	}
	result, err := r.Executor.Run(ctx, command.ExecRequest{
		ID:             req.ID,
		JobID:          req.JobID,
		StepID:         req.StepID,
		ToolName:       "plugin." + manifest.Name,
		ToolVersion:    manifest.Version,
		Command:        "nuclei",
		Args:           args,
		CWD:            req.WorkDir,
		Timeout:        req.Timeout,
		Kind:           contracts.ToolCallKindPlugin,
		MaxStdoutBytes: pluginMaxStdoutBytes(req),
		MaxStderrBytes: pluginMaxStderrBytes(req),
		NoRecord:       true,
	})
	if err != nil {
		return r.skippedFailedPluginResult(req, manifest, result, err)
	}
	findings, err := parseNucleiJSONL(result.Stdout)
	if err != nil {
		return r.invalidOutputPluginResult(req, manifest, result, err)
	}
	call := annotatePluginCall(result.Call, manifest)
	if err := r.recordPluginCall(req, manifest, call); err != nil {
		return RunResult{}, err
	}
	return RunResult{Call: call, Stdout: result.Stdout, Stderr: result.Stderr, Findings: findings}, err
}

func (r Runner) runFFUFLowRate(ctx context.Context, manifest contracts.PluginManifest, req RunRequest) (RunResult, error) {
	if _, err := exec.LookPath("ffuf"); err != nil {
		return r.dependencyMissingPluginResult(req, manifest, "dependency ffuf not found in PATH")
	}
	wordlistPath, err := ensureFFUFWordlist(req.WorkDir)
	if err != nil {
		return RunResult{}, err
	}
	fuzzURL := strings.TrimRight(req.Target, "/") + "/FUZZ"
	args := []string{
		"-u", fuzzURL,
		"-w", wordlistPath,
		"-rate", "2",
		"-of", "json",
		"-s",
	}
	result, err := r.Executor.Run(ctx, command.ExecRequest{
		ID:             req.ID,
		JobID:          req.JobID,
		StepID:         req.StepID,
		ToolName:       "plugin." + manifest.Name,
		ToolVersion:    manifest.Version,
		Command:        "ffuf",
		Args:           args,
		CWD:            req.WorkDir,
		Timeout:        req.Timeout,
		Kind:           contracts.ToolCallKindPlugin,
		MaxStdoutBytes: pluginMaxStdoutBytes(req),
		MaxStderrBytes: pluginMaxStderrBytes(req),
		NoRecord:       true,
	})
	if err != nil {
		return r.skippedFailedPluginResult(req, manifest, result, err)
	}
	findings, err := parseFFUFJSON(result.Stdout)
	if err != nil {
		return r.invalidOutputPluginResult(req, manifest, result, err)
	}
	call := annotatePluginCall(result.Call, manifest)
	if err := r.recordPluginCall(req, manifest, call); err != nil {
		return RunResult{}, err
	}
	return RunResult{Call: call, Stdout: result.Stdout, Stderr: result.Stderr, Findings: findings}, err
}

func (r Runner) runSubprocessJSONL(ctx context.Context, manifest contracts.PluginManifest, req RunRequest) (RunResult, error) {
	if manifest.Entrypoint.Kind != "subprocess" {
		return RunResult{}, fmt.Errorf("unsupported subprocess entrypoint kind %q", manifest.Entrypoint.Kind)
	}
	commandName := "python3"
	if hasBinaryDependency(manifest, "python") {
		commandName = "python"
	}
	if _, err := exec.LookPath(commandName); err != nil {
		return r.dependencyMissingPluginResult(req, manifest, fmt.Sprintf("dependency %s not found in PATH", commandName))
	}
	entrypoint, err := resolveEntrypointPath(manifest.Entrypoint.Path, req.WorkDir)
	if err != nil {
		return RunResult{}, err
	}
	stdin, err := EncodeSubprocessRequest(SubprocessRequest{ID: req.ID, JobID: req.JobID, StepID: req.StepID, Target: req.Target, Scope: req.Scope, Input: req.Input, Artifacts: req.Artifacts, Evidence: req.Evidence})
	if err != nil {
		return RunResult{}, err
	}
	result, err := r.Executor.Run(ctx, command.ExecRequest{
		ID:             req.ID,
		JobID:          req.JobID,
		StepID:         req.StepID,
		ToolName:       "plugin." + manifest.Name,
		ToolVersion:    manifest.Version,
		Command:        commandName,
		Args:           []string{entrypoint},
		CWD:            req.WorkDir,
		Timeout:        req.Timeout,
		Kind:           contracts.ToolCallKindPlugin,
		Stdin:          stdin,
		MaxStdoutBytes: pluginMaxStdoutBytes(req),
		MaxStderrBytes: pluginMaxStderrBytes(req),
		NoRecord:       true,
	})
	if err != nil {
		return r.skippedFailedPluginResult(req, manifest, result, err)
	}
	messages, err := DecodeSubprocessMessages(result.Stdout)
	if err != nil {
		return r.invalidOutputPluginResult(req, manifest, result, err)
	}
	if len(messages) == 0 {
		return r.invalidOutputPluginResult(req, manifest, result, fmt.Errorf("subprocess output is empty"))
	}
	routes := []PluginRoute{}
	findings := []PluginFinding{}
	for _, message := range messages {
		routes = append(routes, message.Routes...)
		findings = append(findings, message.Findings...)
	}
	call := annotatePluginCall(result.Call, manifest)
	if err := r.recordPluginCall(req, manifest, call); err != nil {
		return RunResult{}, err
	}
	return RunResult{Call: call, Stdout: result.Stdout, Stderr: result.Stderr, Messages: messages, Routes: routes, Findings: findings}, nil
}

func skippedPluginCall(req RunRequest, manifest contracts.PluginManifest, reason string) contracts.ToolCall {
	call := contracts.ToolCall{
		ID:          req.ID,
		JobID:       req.JobID,
		StepID:      req.StepID,
		ToolName:    "plugin." + manifest.Name,
		ToolVersion: manifest.Version,
		RunnerID:    "local",
		Kind:        contracts.ToolCallKindPlugin,
		Command:     manifest.Name,
		Args:        []string{"--target", req.Target},
		CWD:         req.WorkDir,
		EnvPolicy:   "inherit_minimal",
		StartedAt:   contracts.Now(),
		Status:      contracts.StatusSkipped,
		Error:       reason,
		Metadata: contracts.Metadata{
			"plugin_id":   manifest.ID,
			"runtime":     manifest.Runtime,
			"risk":        manifest.Risk,
			"error_class": "dependency_missing",
		},
	}
	finished := contracts.Now()
	call.FinishedAt = &finished
	exitCode := 0
	call.ExitCode = &exitCode
	return call
}

func (r Runner) dependencyMissingPluginResult(req RunRequest, manifest contracts.PluginManifest, reason string) (RunResult, error) {
	call := skippedPluginCall(req, manifest, reason)
	if err := r.recordPluginCall(req, manifest, call); err != nil {
		return RunResult{}, err
	}
	return RunResult{Call: call, Skipped: true, Reason: call.Error}, nil
}

func (r Runner) skippedFailedPluginResult(req RunRequest, manifest contracts.PluginManifest, result command.ExecResult, err error) (RunResult, error) {
	call := annotatePluginCall(result.Call, manifest)
	if call.Error == "" && err != nil {
		call.Error = err.Error()
	}
	if call.Metadata == nil {
		call.Metadata = contracts.Metadata{}
	}
	if _, ok := call.Metadata["error_class"]; !ok {
		call.Metadata["error_class"] = "execution_failed"
	}
	if recordErr := r.recordPluginCall(req, manifest, call); recordErr != nil {
		return RunResult{}, recordErr
	}
	reason := call.Error
	if reason == "" && err != nil {
		reason = err.Error()
	}
	return RunResult{Call: call, Stdout: result.Stdout, Stderr: result.Stderr, Skipped: true, Reason: reason}, nil
}

func (r Runner) invalidOutputPluginResult(req RunRequest, manifest contracts.PluginManifest, result command.ExecResult, parseErr error) (RunResult, error) {
	err := fmt.Errorf("invalid_output: %w", parseErr)
	call := markPluginCallFailed(result.Call, manifest, err, "invalid_output")
	if recordErr := r.recordPluginCall(req, manifest, call); recordErr != nil {
		return RunResult{}, recordErr
	}
	return RunResult{Call: call, Stdout: result.Stdout, Stderr: result.Stderr, Skipped: true, Reason: call.Error}, nil
}

func (r Runner) recordPluginCall(req RunRequest, manifest contracts.PluginManifest, call contracts.ToolCall) error {
	recorder := req.Recorder
	if recorder == nil {
		recorder = r.Executor.Recorder
	}
	if recorder == nil {
		return nil
	}
	return recorder.Append(annotatePluginCall(call, manifest))
}

func annotatePluginCall(call contracts.ToolCall, manifest contracts.PluginManifest) contracts.ToolCall {
	if call.Metadata == nil {
		call.Metadata = contracts.Metadata{}
	}
	call.Metadata["plugin_id"] = manifest.ID
	call.Metadata["runtime"] = manifest.Runtime
	call.Metadata["risk"] = manifest.Risk
	return call
}

func markPluginCallFailed(call contracts.ToolCall, manifest contracts.PluginManifest, err error, class string) contracts.ToolCall {
	call = annotatePluginCall(call, manifest)
	call.Status = contracts.StatusFailed
	exitCode := 1
	call.ExitCode = &exitCode
	if call.FinishedAt == nil {
		finished := contracts.Now()
		call.FinishedAt = &finished
	}
	if err != nil {
		call.Error = err.Error()
	}
	if call.Metadata == nil {
		call.Metadata = contracts.Metadata{}
	}
	call.Metadata["error_class"] = class
	return call
}

func pluginMaxStdoutBytes(req RunRequest) int64 {
	if req.MaxStdoutBytes > 0 {
		return req.MaxStdoutBytes
	}
	return defaultPluginMaxStdoutBytes
}

func pluginMaxStderrBytes(req RunRequest) int64 {
	if req.MaxStderrBytes > 0 {
		return req.MaxStderrBytes
	}
	return defaultPluginMaxStderrBytes
}

func parseNucleiJSONL(stdout []byte) ([]PluginFinding, error) {
	var findings []PluginFinding
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			return nil, fmt.Errorf("decode nuclei JSONL: %w", err)
		}
		finding := PluginFinding{Data: raw}
		if v, ok := raw["template-id"].(string); ok {
			finding.TemplateID = v
		}
		if v, ok := raw["template_id"].(string); ok && finding.TemplateID == "" {
			finding.TemplateID = v
		}
		if info, ok := raw["info"].(map[string]any); ok {
			if v, ok := info["name"].(string); ok {
				finding.Name = v
			}
			if v, ok := info["severity"].(string); ok {
				finding.Severity = strings.ToLower(v)
			}
		}
		if v, ok := raw["matched-at"].(string); ok {
			finding.Matched = v
		}
		if v, ok := raw["matched"].(string); ok && finding.Matched == "" {
			finding.Matched = v
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func parseFFUFJSON(stdout []byte) ([]PluginFinding, error) {
	if len(bytes.TrimSpace(stdout)) == 0 {
		return nil, fmt.Errorf("ffuf JSON output is empty")
	}
	var raw struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout), &raw); err != nil {
		return nil, fmt.Errorf("decode ffuf JSON: %w", err)
	}
	findings := make([]PluginFinding, 0, len(raw.Results))
	for _, result := range raw.Results {
		matched, _ := result["url"].(string)
		if matched == "" {
			continue
		}
		status := ""
		if v, ok := result["status"]; ok {
			status = fmt.Sprintf("%v", v)
		}
		name := "ffuf discovered content"
		if status != "" {
			name += " (" + status + ")"
		}
		findings = append(findings, PluginFinding{
			Name:     name,
			Severity: "info",
			Matched:  matched,
			Data:     result,
		})
	}
	return findings, nil
}

func ensureFFUFWordlist(workDir string) (string, error) {
	if strings.TrimSpace(workDir) == "" {
		return "", fmt.Errorf("workdir is required for ffuf-low-rate")
	}
	path := filepath.Join(workDir, "artifacts", "tool-results", "ffuf-low-rate-wordlist.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create ffuf wordlist dir: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	words := []byte("robots.txt\nsitemap.xml\nadmin\nlogin\napi\n")
	if err := os.WriteFile(path, words, 0o644); err != nil {
		return "", fmt.Errorf("write ffuf wordlist: %w", err)
	}
	return path, nil
}

func hasBinaryDependency(manifest contracts.PluginManifest, name string) bool {
	for _, dep := range manifest.Dependencies {
		if dep.Kind == "binary" && dep.Name == name {
			return true
		}
	}
	return false
}

func resolveEntrypointPath(path string, workDir string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("plugin entrypoint path is required")
	}
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("plugin entrypoint %s: %w", path, err)
		}
		return path, nil
	}
	candidates := []string{path}
	if workDir != "" {
		candidates = append(candidates, filepath.Join(workDir, path))
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			absolute, err := filepath.Abs(candidate)
			if err != nil {
				return "", fmt.Errorf("resolve plugin entrypoint %s: %w", candidate, err)
			}
			return absolute, nil
		}
	}
	return "", fmt.Errorf("plugin entrypoint not found: %s", path)
}
