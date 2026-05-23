package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
)

func TestRunnerRunSubprocessJSONL(t *testing.T) {
	manifest := contracts.PluginManifest{
		ID:      "plugin_python_js_route_analyzer",
		Name:    "python-js-route-analyzer",
		Version: "0.1.0",
		Type:    contracts.PluginTypeAnalyzer,
		Runtime: "subprocess_jsonl",
		Risk:    contracts.RiskLow,
		Dependencies: []contracts.PluginDependency{
			{Name: "python3", Kind: "binary", Required: true},
		},
		Entrypoint: contracts.PluginEntrypoint{Kind: "subprocess", Path: filepath.Join("..", "..", "plugins", "python-js-route-analyzer", "analyzer.py")},
	}
	workspace := t.TempDir()
	jsRelPath := filepath.Join("artifacts", "http", "js_001.js")
	if err := os.MkdirAll(filepath.Join(workspace, "artifacts", "http"), 0o755); err != nil {
		t.Fatalf("mkdir js artifact dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, jsRelPath), []byte(`fetch('/api/v1/users', {method: 'POST'}); axios.get("/graphql"); const x = "/v2/status";`), 0o644); err != nil {
		t.Fatalf("write js artifact: %v", err)
	}

	recorder := command.NewRecorder(filepath.Join(workspace, "commands.log"))
	result, err := NewRunner(recorder).Run(context.Background(), manifest, RunRequest{
		ID:       "request_001",
		JobID:    "job_001",
		StepID:   "step_001",
		Target:   "http://localhost:8080/app/",
		WorkDir:  workspace,
		Timeout:  10 * time.Second,
		Recorder: recorder,
		Input:    map[string]any{"mode": "test"},
		Artifacts: []contracts.Artifact{{
			ID:       "artifact_js_asset_001",
			JobID:    "job_001",
			StepID:   "step_js_asset_collect_001",
			Type:     contracts.ArtifactText,
			Path:     jsRelPath,
			MIMEType: "application/javascript",
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v stderr=%s stdout=%s", err, result.Stderr, result.Stdout)
	}
	if result.Skipped {
		t.Fatalf("result unexpectedly skipped: %#v", result)
	}
	if result.Call.Status != contracts.StatusSucceeded || result.Call.Kind != contracts.ToolCallKindPlugin || result.Call.Command != "python3" {
		t.Fatalf("unexpected tool call: %#v", result.Call)
	}
	if len(result.Messages) != 1 || result.Messages[0].RequestID != "request_001" {
		t.Fatalf("unexpected messages: %#v stdout=%s", result.Messages, result.Stdout)
	}
	if len(result.Routes) != 3 {
		t.Fatalf("routes = %#v, want 3; stdout=%s", result.Routes, result.Stdout)
	}
	if result.Routes[0].Raw != "/api/v1/users" || result.Routes[0].Resolved != "http://localhost:8080/api/v1/users" || result.Routes[0].Method != "POST" {
		t.Fatalf("unexpected first route: %#v", result.Routes[0])
	}
	if result.Routes[1].Raw != "/graphql" || result.Routes[1].Method != "GET" {
		t.Fatalf("unexpected second route: %#v", result.Routes[1])
	}

	calls, err := recorder.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(calls) != 1 || calls[0].ToolName != "plugin.python-js-route-analyzer" || calls[0].Kind != contracts.ToolCallKindPlugin {
		t.Fatalf("unexpected recorded calls: %#v", calls)
	}
}

func TestRunnerRunSubprocessJSONLMissingPythonSkips(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	manifest := contracts.PluginManifest{
		ID:      "plugin_python_js_route_analyzer",
		Name:    "python-js-route-analyzer",
		Version: "0.1.0",
		Type:    contracts.PluginTypeAnalyzer,
		Runtime: "subprocess_jsonl",
		Risk:    contracts.RiskLow,
		Dependencies: []contracts.PluginDependency{
			{Name: "python3", Kind: "binary", Required: true},
		},
		Entrypoint: contracts.PluginEntrypoint{Kind: "subprocess", Path: "does-not-matter.py"},
	}
	recorder := command.NewRecorder(filepath.Join(t.TempDir(), "commands.log"))
	result, err := NewRunner(recorder).Run(context.Background(), manifest, RunRequest{ID: "request_001", JobID: "job_001", StepID: "step_001", Target: "http://localhost:8080", Recorder: recorder})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Skipped || result.Call.Status != contracts.StatusSkipped || !strings.Contains(result.Reason, "dependency python3 not found") {
		t.Fatalf("unexpected skipped result: %#v", result)
	}
}

func TestRunnerRunFFUFLowRate(t *testing.T) {
	fakeBin := t.TempDir()
	fakeFFUF := filepath.Join(fakeBin, "ffuf")
	if err := os.WriteFile(fakeFFUF, []byte("#!/bin/sh\nprintf '%s\n' '{\"results\":[{\"url\":\"http://localhost:8080/admin\",\"status\":200,\"length\":42}]}'\n"), 0o755); err != nil {
		t.Fatalf("write fake ffuf: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifest := contracts.PluginManifest{
		ID:               "plugin_ffuf_low_rate",
		Name:             "ffuf-low-rate",
		Version:          "0.1.0",
		Type:             contracts.PluginTypeTool,
		Runtime:          "external_command",
		Risk:             contracts.RiskMedium,
		RequiresApproval: true,
		Dependencies: []contracts.PluginDependency{
			{Name: "ffuf", Kind: "binary", Required: true},
		},
		Entrypoint: contracts.PluginEntrypoint{Kind: "command_builder", Path: filepath.Join("plugins", "ffuf-low-rate", "plugin.yaml")},
	}
	workspace := t.TempDir()
	recorder := command.NewRecorder(filepath.Join(workspace, "commands.log"))

	result, err := NewRunner(recorder).Run(context.Background(), manifest, RunRequest{
		ID:       "request_001",
		JobID:    "job_001",
		StepID:   "step_optional_ffuf_low_rate_001",
		Target:   "http://localhost:8080",
		WorkDir:  workspace,
		Timeout:  10 * time.Second,
		Recorder: recorder,
	})
	if err != nil {
		t.Fatalf("Run() error = %v stderr=%s stdout=%s", err, result.Stderr, result.Stdout)
	}
	if result.Skipped {
		t.Fatalf("result unexpectedly skipped: %#v", result)
	}
	if result.Call.Status != contracts.StatusSucceeded || result.Call.Kind != contracts.ToolCallKindPlugin || result.Call.Command != "ffuf" || result.Call.ToolName != "plugin.ffuf-low-rate" {
		t.Fatalf("unexpected tool call: %#v", result.Call)
	}
	if !containsArgSequence(result.Call.Args, "-rate", "2") || !containsArgSequence(result.Call.Args, "-of", "json") {
		t.Fatalf("ffuf args missing safe output/rate flags: %#v", result.Call.Args)
	}
	if _, err := os.Stat(filepath.Join(workspace, "artifacts", "tool-results", "ffuf-low-rate-wordlist.txt")); err != nil {
		t.Fatalf("ffuf wordlist not created: %v", err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Matched != "http://localhost:8080/admin" || result.Findings[0].Severity != "info" {
		t.Fatalf("unexpected ffuf findings: %#v stdout=%s", result.Findings, result.Stdout)
	}

	calls, err := recorder.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(calls) != 1 || calls[0].ToolName != "plugin.ffuf-low-rate" || calls[0].Kind != contracts.ToolCallKindPlugin || calls[0].Status != contracts.StatusSucceeded {
		t.Fatalf("unexpected recorded calls: %#v", calls)
	}
}

func TestResolveEntrypointPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.py")
	if err := os.WriteFile(path, []byte("print('ok')"), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	resolved, err := resolveEntrypointPath(path, "")
	if err != nil {
		t.Fatalf("resolve absolute: %v", err)
	}
	if resolved != path {
		t.Fatalf("resolved = %s, want %s", resolved, path)
	}

	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "plugins", "test"), 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	rel := filepath.Join("plugins", "test", "plugin.py")
	if err := os.WriteFile(filepath.Join(workDir, rel), []byte("print('ok')"), 0o644); err != nil {
		t.Fatalf("write relative plugin: %v", err)
	}
	resolved, err = resolveEntrypointPath(rel, workDir)
	if err != nil {
		t.Fatalf("resolve relative: %v", err)
	}
	if resolved != filepath.Join(workDir, rel) {
		t.Fatalf("resolved = %s, want %s", resolved, filepath.Join(workDir, rel))
	}
}

func containsArgSequence(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
