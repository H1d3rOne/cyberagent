package runs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagerCreateWorkspace(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root)
	layout, err := manager.Create("job_test_001")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if layout.ArtifactsIndex != filepath.Join(root, "job_test_001", "artifacts", "index.json") {
		t.Fatalf("unexpected artifact index path: %s", layout.ArtifactsIndex)
	}

	for _, dir := range layout.Dirs() {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("expected dir %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", dir)
		}
	}

	for _, file := range []string{layout.EventsPath, layout.CommandsPath, layout.EvidenceJSONL, layout.ApprovalsPath} {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatalf("expected file %s: %v", file, err)
		}
		if info.IsDir() {
			t.Fatalf("expected %s to be a file", file)
		}
	}

	opened, err := manager.Open("job_test_001")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if opened.Path != filepath.Join(root, "job_test_001") {
		t.Fatalf("unexpected opened path: %s", opened.Path)
	}
}

func TestManagerRejectsInvalidJobID(t *testing.T) {
	manager := NewManager(t.TempDir())
	if _, err := manager.Create("../bad"); err == nil {
		t.Fatalf("expected invalid job ID error")
	}
}
