package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	})
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

	replayOut, err := executeCLI(NewRootCommand("CyberAgent", "vtest", "github.com/H1d3rOne/cyberagent"), "replay", runDir)
	if err != nil {
		t.Fatalf("replay command error = %v", err)
	}
	if !strings.Contains(replayOut, "job.created") || !strings.Contains(replayOut, "job.finished") {
		t.Fatalf("unexpected replay output: %q", replayOut)
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
