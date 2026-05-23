package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	coreruntime "github.com/H1d3rOne/cyberagent/internal/core/runtime"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
	"github.com/H1d3rOne/cyberagent/internal/runtime/state"
	"github.com/spf13/cobra"
)

func TestVersionCommand(t *testing.T) {
	out, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "version")
	if err != nil {
		t.Fatalf("version command error = %v", err)
	}
	if !strings.Contains(out, "CyberAgent vtest") || !strings.Contains(out, "github.com/H1d3rOne/cyberagent") {
		t.Fatalf("unexpected version output: %q", out)
	}
}

func TestDoctorCommand(t *testing.T) {
	out, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "doctor")
	if err != nil {
		t.Fatalf("doctor command error = %v", err)
	}
	if !strings.Contains(out, "CyberAgent doctor") || !strings.Contains(out, "platform:") || !strings.Contains(out, "workspace") {
		t.Fatalf("unexpected doctor output: %q", out)
	}
}

func TestPluginsCommands(t *testing.T) {
	withTempWorkingDir(t, func(root string) {
		if err := os.MkdirAll(filepath.Join(root, "schemas", "plugins"), 0o755); err != nil {
			t.Fatalf("mkdir schemas/plugins: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "schemas", "plugins", "alpha.input.schema.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
			t.Fatalf("write input schema: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "schemas", "plugins", "alpha.output.schema.json"), []byte(`{"type":"object"}`), 0o644); err != nil {
			t.Fatalf("write output schema: %v", err)
		}
		writeTestPlugin(t, root, "alpha", `id: plugin_alpha
name: alpha
version: 0.1.0
type: analyzer
runtime: subprocess_jsonl
risk: low
description: Alpha test plugin.
dependencies:
  - name: go
    kind: binary
    required: true
input_schema: schemas/plugins/alpha.input.schema.json
output_schema: schemas/plugins/alpha.output.schema.json
entrypoint:
  kind: subprocess
  path: plugins/alpha/analyzer.py
`)

		listOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "plugins", "list")
		if err != nil {
			t.Fatalf("plugins list error = %v", err)
		}
		if !strings.Contains(listOut, "NAME") || !strings.Contains(listOut, "alpha") || !strings.Contains(listOut, "Alpha test plugin") {
			t.Fatalf("unexpected plugins list output: %q", listOut)
		}

		doctorOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "plugins", "doctor")
		if err != nil {
			t.Fatalf("plugins doctor error = %v", err)
		}
		for _, want := range []string{"alpha (analyzer): ok", "go", "schema input_schema", "schema output_schema", "valid JSON schema"} {
			if !strings.Contains(doctorOut, want) {
				t.Fatalf("plugins doctor output missing %q: %q", want, doctorOut)
			}
		}

		doctorJSONOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "plugins", "doctor", "--json")
		if err != nil {
			t.Fatalf("plugins doctor --json error = %v", err)
		}
		var doctorJSON pluginsDoctorOutput
		if err := json.Unmarshal([]byte(doctorJSONOut), &doctorJSON); err != nil {
			t.Fatalf("decode plugins doctor --json output: %v\noutput=%s", err, doctorJSONOut)
		}
		if doctorJSON.SchemaVersion != "0.1.1" || !doctorJSON.OK || doctorJSON.Count != 1 || len(doctorJSON.Plugins) != 1 {
			t.Fatalf("unexpected plugins doctor --json output: %#v", doctorJSON)
		}
		health := doctorJSON.Plugins[0]
		if health.Plugin.Name != "alpha" || !health.OK {
			t.Fatalf("unexpected plugin health in JSON: %#v", health)
		}
		if len(health.Dependencies) != 1 || health.Dependencies[0].Name != "go" || !health.Dependencies[0].OK {
			t.Fatalf("unexpected dependency JSON: %#v", health.Dependencies)
		}
		if len(health.Schemas) != 2 || !health.Schemas[0].OK || !health.Schemas[1].OK {
			t.Fatalf("unexpected schema JSON: %#v", health.Schemas)
		}
	})
}

func TestApprovalsCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
	}))
	defer server.Close()

	runDir := filepath.Join(t.TempDir(), "run_approvals")
	_, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "scan", "web", "--target", server.URL, "--scope", server.URL, "--out", runDir, "--profile", "safe", "--use-plugin", "ffuf-low-rate")
	if err != nil {
		t.Fatalf("scan web with ffuf-low-rate error = %v", err)
	}

	listOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "approvals", "list", runDir)
	if err != nil {
		t.Fatalf("approvals list error = %v", err)
	}
	for _, want := range []string{"ID", "approval_step_optional_ffuf_low_rate_001", "pending", "step_optional_ffuf_low_rate_001"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("approvals list output missing %q: %q", want, listOut)
		}
	}

	approveOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "approvals", "approve", runDir, "step_optional_ffuf_low_rate_001")
	if err != nil {
		t.Fatalf("approvals approve error = %v", err)
	}
	for _, want := range []string{"approval approve", "approval_step_optional_ffuf_low_rate_001", "status: approved"} {
		if !strings.Contains(approveOut, want) {
			t.Fatalf("approvals approve output missing %q: %q", want, approveOut)
		}
	}

	var approvals []contracts.HITLRequest
	readJSON(t, filepath.Join(runDir, "approvals.json"), &approvals)
	if len(approvals) != 1 || approvals[0].Status != contracts.HITLApproved || approvals[0].ResolvedAt == nil {
		t.Fatalf("unexpected approvals.json after approve: %#v", approvals)
	}

	stateStore, err := state.Open(filepath.Join(runDir, "state", "bolt.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	persisted, err := stateStore.LoadHITLRequest("approval_step_optional_ffuf_low_rate_001")
	stateStore.Close()
	if err != nil {
		t.Fatalf("LoadHITLRequest() error = %v", err)
	}
	if persisted.Status != contracts.HITLApproved {
		t.Fatalf("unexpected persisted approval: %#v", persisted)
	}

	replayOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "replay", runDir)
	if err != nil {
		t.Fatalf("replay after approval error = %v", err)
	}
	if !strings.Contains(replayOut, "approval.resolved") {
		t.Fatalf("replay output missing approval.resolved: %q", replayOut)
	}

	calls, err := command.NewRecorder(filepath.Join(runDir, "commands.log")).ReadAll()
	if err != nil {
		t.Fatalf("read commands.log: %v", err)
	}
	if !hasToolCall(calls, "cyberagent.approvals") {
		t.Fatalf("commands.log missing approval audit call: %#v", calls)
	}

	_, err = executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "approvals", "deny", runDir, "step_optional_ffuf_low_rate_001")
	if err == nil || !strings.Contains(err.Error(), "not pending") {
		t.Fatalf("expected deny approved request to fail with not pending, got err=%v", err)
	}
}

func TestReplayResumeApprovedFFUFLowRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
		case "/admin":
			_, _ = w.Write([]byte("admin"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runDir := filepath.Join(t.TempDir(), "run_resume_ffuf")
	_, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "scan", "web", "--target", server.URL, "--scope", server.URL, "--out", runDir, "--profile", "safe", "--use-plugin", "ffuf-low-rate")
	if err != nil {
		t.Fatalf("scan web with ffuf-low-rate error = %v", err)
	}
	if _, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "approvals", "approve", runDir, "step_optional_ffuf_low_rate_001"); err != nil {
		t.Fatalf("approvals approve error = %v", err)
	}

	fakeBin := t.TempDir()
	fakeFFUF := filepath.Join(fakeBin, "ffuf")
	fakeOutput := `{"results":[{"url":"` + server.URL + `/admin","status":200,"length":5}]}`
	if err := os.WriteFile(fakeFFUF, []byte("#!/bin/sh\nprintf '%s\\n' '"+fakeOutput+"'\n"), 0o755); err != nil {
		t.Fatalf("write fake ffuf: %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	replayOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "replay", runDir, "--resume-approved")
	if err != nil {
		t.Fatalf("replay --resume-approved error = %v\noutput=%s", err, replayOut)
	}
	for _, want := range []string{"resume-approved completed", "resumed: [step_optional_ffuf_low_rate_001]", "step.finished", "tool.finished"} {
		if !strings.Contains(replayOut, want) {
			t.Fatalf("replay --resume-approved output missing %q: %q", want, replayOut)
		}
	}

	mustExist(t, filepath.Join(runDir, "artifacts", "tool-results", "ffuf-low-rate.jsonl"))
	mustExist(t, filepath.Join(runDir, "artifacts", "tool-results", "ffuf-low-rate-wordlist.txt"))
	var artifactIndex struct {
		Artifacts []contracts.Artifact `json:"artifacts"`
	}
	readJSON(t, filepath.Join(runDir, "artifacts", "index.json"), &artifactIndex)
	var foundFFUFArtifact bool
	for _, artifact := range artifactIndex.Artifacts {
		if artifact.ID == "artifact_ffuf_low_rate_stdout_001" && artifact.SHA256 != "" && artifact.SizeBytes > 0 && artifact.MIMEType != "" {
			foundFFUFArtifact = true
			break
		}
	}
	if !foundFFUFArtifact {
		t.Fatalf("artifacts/index.json missing completed ffuf stdout artifact metadata: %#v", artifactIndex.Artifacts)
	}

	var persistedFindings []contracts.Finding
	readJSON(t, filepath.Join(runDir, "findings.json"), &persistedFindings)
	var foundFFUFFinding bool
	for _, finding := range persistedFindings {
		if finding.Category == "plugin/ffuf-low-rate" && len(finding.AffectedTargets) == 1 && finding.AffectedTargets[0] == server.URL+"/admin" {
			foundFFUFFinding = true
			break
		}
	}
	if !foundFFUFFinding {
		t.Fatalf("findings.json missing ffuf-low-rate finding: %#v", persistedFindings)
	}

	calls, err := command.NewRecorder(filepath.Join(runDir, "commands.log")).ReadAll()
	if err != nil {
		t.Fatalf("read commands.log: %v", err)
	}
	if !hasToolCallStatus(calls, "plugin.ffuf-low-rate", contracts.StatusSucceeded) {
		t.Fatalf("commands.log missing succeeded plugin.ffuf-low-rate call: %#v", calls)
	}

	stateStore, err := state.Open(filepath.Join(runDir, "state", "bolt.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	step, err := stateStore.LoadStep("step_optional_ffuf_low_rate_001")
	stateStore.Close()
	if err != nil {
		t.Fatalf("LoadStep(ffuf) error = %v", err)
	}
	if step.Status != contracts.StatusSucceeded || step.Error != "" {
		t.Fatalf("unexpected resumed ffuf step state: %#v", step)
	}

	reportMD, err := os.ReadFile(filepath.Join(runDir, "reports", "report.md"))
	if err != nil {
		t.Fatalf("read report.md: %v", err)
	}
	if !strings.Contains(string(reportMD), "plugin/ffuf-low-rate") || strings.Contains(string(reportMD), "step_optional_ffuf_low_rate_001,") {
		t.Fatalf("report.md not regenerated as expected: %s", reportMD)
	}
}

func TestScanReportAndReplayCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><html><head><script src="/app.js"></script></head><body><a href="/admin">Admin</a></body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch('/api/users')`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runDir := filepath.Join(t.TempDir(), "run_cli")
	scanOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "scan", "web", "--target", server.URL, "--scope", server.URL, "--out", runDir, "--profile", "safe", "--no-external-tools")
	if err != nil {
		t.Fatalf("scan web error = %v", err)
	}
	if !strings.Contains(scanOut, "CyberAgent scan completed") || !strings.Contains(scanOut, "workspace: "+runDir) || !strings.Contains(scanOut, "report:") {
		t.Fatalf("unexpected scan output: %q", scanOut)
	}

	reportOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "report", runDir)
	if err != nil {
		t.Fatalf("report command error = %v", err)
	}
	if !strings.Contains(reportOut, "# CyberAgent Report") || !strings.Contains(reportOut, server.URL) {
		t.Fatalf("unexpected report output: %q", reportOut)
	}

	reportMDOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "report", runDir, "--format", "md")
	if err != nil {
		t.Fatalf("report --format md command error = %v", err)
	}
	if reportMDOut != reportOut {
		t.Fatalf("report --format md output differs from default")
	}

	reportJSONOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "report", runDir, "--format", "json")
	if err != nil {
		t.Fatalf("report --format json command error = %v", err)
	}
	var reportJSON struct {
		Job      contracts.Job       `json:"job"`
		Target   contracts.Target    `json:"target"`
		Findings []contracts.Finding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(reportJSONOut), &reportJSON); err != nil {
		t.Fatalf("decode report --format json output: %v\noutput=%s", err, reportJSONOut)
	}
	if reportJSON.Job.Status != contracts.StatusSucceeded || reportJSON.Target.Value != server.URL {
		t.Fatalf("unexpected report JSON output: %#v", reportJSON)
	}

	_, err = executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "report", runDir, "--format", "xml")
	if err == nil || !strings.Contains(err.Error(), "invalid report format") {
		t.Fatalf("expected invalid report format error, got %v", err)
	}

	replayOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "replay", runDir)
	if err != nil {
		t.Fatalf("replay command error = %v", err)
	}
	if !strings.Contains(replayOut, "job.created") || !strings.Contains(replayOut, "job.finished") {
		t.Fatalf("unexpected replay output: %q", replayOut)
	}
}

func TestScanWebWritesBuildMetadataFromCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><body>ok</body></html>`))
	}))
	defer server.Close()

	runDir := filepath.Join(t.TempDir(), "run_cli_metadata")
	cmd := NewRootCommandWithBuildInfo("CyberAgent", coreruntime.BuildInfo{
		Version: "vcli-test",
		Commit:  "deadbeef",
		Date:    "2026-05-24T01:02:03Z",
		Module:  "github.com/H1d3rOne/cyberagent",
	})
	if _, err := executeCLI(cmd, "scan", "web", "--target", server.URL, "--scope", server.URL, "--out", runDir, "--profile", "safe", "--use-plugin", "nuclei-safe"); err != nil {
		t.Fatalf("scan web error = %v", err)
	}

	var metadata coreruntime.RunMetadataSnapshot
	readJSON(t, filepath.Join(runDir, "metadata.json"), &metadata)
	if metadata.Binary.Version != "vcli-test" || metadata.Binary.Commit != "deadbeef" || metadata.Binary.Date != "2026-05-24T01:02:03Z" {
		t.Fatalf("unexpected metadata binary info: %#v", metadata.Binary)
	}
	if metadata.Command.Name != "scan web" || !containsString(metadata.Command.Args, "--use-plugin") || !containsString(metadata.Command.Args, "nuclei-safe") {
		t.Fatalf("unexpected metadata command args: %#v", metadata.Command)
	}

	var effective coreruntime.EffectiveConfigSnapshot
	readJSON(t, filepath.Join(runDir, "effective_config.json"), &effective)
	if effective.Profile != "safe" || len(effective.EnabledPlugins) != 1 || effective.EnabledPlugins[0] != "nuclei-safe" {
		t.Fatalf("unexpected effective config: %#v", effective)
	}

	var plugins coreruntime.PluginSnapshot
	readJSON(t, filepath.Join(runDir, "plugins.snapshot.json"), &plugins)
	if !containsString(plugins.Enabled, "nuclei-safe") || !containsString(plugins.Disabled, "ffuf-low-rate") {
		t.Fatalf("unexpected plugin snapshot: %#v", plugins)
	}
}

func TestScanWebRequiresScope(t *testing.T) {
	_, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "scan", "web", "--target", "http://127.0.0.1:1")
	if err == nil {
		t.Fatalf("expected scan web without scope to fail")
	}
	if !strings.Contains(err.Error(), "scope is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitCommand(t *testing.T) {
	withTempWorkingDir(t, func(root string) {
		out, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "init")
		if err != nil {
			t.Fatalf("init command error = %v", err)
		}
		if !strings.Contains(out, "Created .cyberagent.yaml") {
			t.Fatalf("unexpected init output: %q", out)
		}
		if _, err := os.Stat(filepath.Join(root, ".cyberagent.yaml")); err != nil {
			t.Fatalf("config file not created: %v", err)
		}
	})
}

func TestInitCommandAlreadyExists(t *testing.T) {
	withTempWorkingDir(t, func(root string) {
		if err := os.WriteFile(filepath.Join(root, ".cyberagent.yaml"), []byte("existing"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "init")
		if err == nil {
			t.Fatal("expected error when config already exists")
		}
		if !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestVerboseFlag(t *testing.T) {
	out, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "--verbose", "version")
	if err != nil {
		t.Fatalf("verbose version error = %v", err)
	}
	if !strings.Contains(out, "CyberAgent vtest") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestQuietFlag(t *testing.T) {
	out, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "--quiet", "version")
	if err != nil {
		t.Fatalf("quiet version error = %v", err)
	}
	if !strings.Contains(out, "CyberAgent vtest") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func executeCLI(cmd *cobra.Command, args ...string) (string, error) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if errOut.Len() > 0 {
		out.Write(errOut.Bytes())
	}
	return out.String(), err
}

func withTempWorkingDir(t *testing.T, fn func(root string)) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()
	fn(root)
}

func writeTestPlugin(t *testing.T, root, name, manifest string) {
	t.Helper()
	dir := filepath.Join(root, "plugins", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write plugin manifest: %v", err)
	}
}

func hasToolCall(calls []contracts.ToolCall, toolName string) bool {
	for _, call := range calls {
		if call.ToolName == toolName {
			return true
		}
	}
	return false
}

func hasToolCallStatus(calls []contracts.ToolCall, toolName string, status contracts.Status) bool {
	for _, call := range calls {
		if call.ToolName == toolName && call.Status == status {
			return true
		}
	}
	return false
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
