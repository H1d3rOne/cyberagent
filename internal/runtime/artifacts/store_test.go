package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestStoreSaveBytes(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	data := []byte("hello artifact")

	artifact, err := store.SaveBytes("artifact_001", "job_001", "step_001", contracts.ArtifactText, "artifacts/http/body.txt", data, "text/plain")
	if err != nil {
		t.Fatalf("SaveBytes() error = %v", err)
	}
	if artifact.ID != "artifact_001" || artifact.JobID != "job_001" || artifact.StepID != "step_001" {
		t.Fatalf("unexpected artifact identity: %#v", artifact)
	}
	if artifact.Type != contracts.ArtifactText || artifact.Path != "artifacts/http/body.txt" || artifact.SizeBytes != int64(len(data)) {
		t.Fatalf("unexpected artifact metadata: %#v", artifact)
	}
	sum := sha256.Sum256(data)
	if artifact.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 = %s, want %s", artifact.SHA256, hex.EncodeToString(sum[:]))
	}
	body, err := os.ReadFile(filepath.Join(workspace, artifact.Path))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if string(body) != string(data) {
		t.Fatalf("artifact body = %q, want %q", body, data)
	}
}

func TestStoreSaveJSONAndWriteJSON(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)

	artifact, err := store.SaveJSON("artifact_json_001", "job_001", "step_001", "artifacts/tool-results/result.json", map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("SaveJSON() error = %v", err)
	}
	if artifact.Type != contracts.ArtifactJSON || artifact.MIMEType != "application/json" {
		t.Fatalf("unexpected JSON artifact metadata: %#v", artifact)
	}
	var decoded map[string]bool
	readJSONFile(t, filepath.Join(workspace, artifact.Path), &decoded)
	if !decoded["ok"] {
		t.Fatalf("unexpected decoded artifact: %#v", decoded)
	}

	if err := store.WriteJSON("job.json", map[string]string{"id": "job_001"}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	var job map[string]string
	readJSONFile(t, filepath.Join(workspace, "job.json"), &job)
	if job["id"] != "job_001" {
		t.Fatalf("unexpected job json: %#v", job)
	}
}

func TestStoreWriteIndexCompletesMetadata(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	data := []byte("<!doctype html><title>ok</title>")
	artifact, err := store.SaveBytes("artifact_html_001", "job_001", "step_001", contracts.ArtifactHTML, "artifacts/http/homepage.html", data, "")
	if err != nil {
		t.Fatalf("SaveBytes() error = %v", err)
	}
	artifact.SHA256 = ""
	artifact.SizeBytes = 0
	artifact.MIMEType = ""
	artifact.CreatedAt = time.Time{}

	completed, err := store.WriteIndex([]contracts.Artifact{artifact})
	if err != nil {
		t.Fatalf("WriteIndex() error = %v", err)
	}
	if len(completed) != 1 {
		t.Fatalf("completed artifacts = %d, want 1", len(completed))
	}
	sum := sha256.Sum256(data)
	if completed[0].SHA256 != hex.EncodeToString(sum[:]) || completed[0].SizeBytes != int64(len(data)) || completed[0].MIMEType == "" || completed[0].CreatedAt.IsZero() {
		t.Fatalf("metadata not completed: %#v", completed[0])
	}

	var index Index
	readJSONFile(t, filepath.Join(workspace, "artifacts", "index.json"), &index)
	if index.SchemaVersion != "0.1.1" || index.Count != 1 || len(index.Artifacts) != 1 {
		t.Fatalf("unexpected artifact index: %#v", index)
	}
	if index.Artifacts[0].ID != "artifact_html_001" || index.Artifacts[0].SHA256 != completed[0].SHA256 {
		t.Fatalf("unexpected indexed artifact: %#v", index.Artifacts[0])
	}
}

func TestStoreRejectsUnsafePaths(t *testing.T) {
	store := NewStore(t.TempDir())
	unsafe := []string{"", "../escape.txt", "artifacts/../escape.txt", filepath.Join(string(filepath.Separator), "tmp", "escape.txt")}
	for _, path := range unsafe {
		t.Run(path, func(t *testing.T) {
			_, err := store.SaveBytes("artifact_001", "job_001", "step_001", contracts.ArtifactText, path, []byte("x"), "text/plain")
			if err == nil {
				t.Fatalf("expected SaveBytes to reject %q", path)
			}
			if !strings.Contains(err.Error(), "invalid artifact path") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	if err := store.WriteJSON("../job.json", map[string]string{"id": "bad"}); err == nil {
		t.Fatalf("expected WriteJSON to reject unsafe path")
	}
}

func readJSONFile(t *testing.T, path string, out any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}
