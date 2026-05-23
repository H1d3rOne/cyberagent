package runs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manager creates and opens run workspaces.
type Manager struct {
	Root string
}

// NewManager returns a run manager rooted at root. Empty root defaults to runs.
func NewManager(root string) Manager {
	if strings.TrimSpace(root) == "" {
		root = "runs"
	}
	return Manager{Root: root}
}

// Create creates a workspace for jobID. If jobID is empty, a new ID is generated.
func (m Manager) Create(jobID string) (Layout, error) {
	if strings.TrimSpace(jobID) == "" {
		jobID = NewJobID(time.Now().UTC())
	}
	if err := validateJobID(jobID); err != nil {
		return Layout{}, err
	}
	layout := NewLayout(m.Root, jobID)
	for _, dir := range layout.Dirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Layout{}, fmt.Errorf("create run dir %s: %w", dir, err)
		}
	}
	for _, file := range []string{layout.EventsPath, layout.CommandsPath, layout.EvidenceJSONL} {
		if err := touch(file); err != nil {
			return Layout{}, err
		}
	}
	return layout, nil
}

// Open returns the layout for an existing workspace after a basic existence check.
func (m Manager) Open(jobID string) (Layout, error) {
	if err := validateJobID(jobID); err != nil {
		return Layout{}, err
	}
	layout := NewLayout(m.Root, jobID)
	info, err := os.Stat(layout.Path)
	if err != nil {
		return Layout{}, fmt.Errorf("open run workspace: %w", err)
	}
	if !info.IsDir() {
		return Layout{}, fmt.Errorf("run workspace is not a directory: %s", layout.Path)
	}
	return layout, nil
}

// NewJobID creates a readable unique job ID.
func NewJobID(t time.Time) string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("job_%s", t.Format("20060102_150405"))
	}
	return fmt.Sprintf("job_%s_%s", t.Format("20060102_150405"), hex.EncodeToString(buf))
}

func validateJobID(jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("job ID is empty")
	}
	if strings.Contains(jobID, string(os.PathSeparator)) || filepath.Base(jobID) != jobID || strings.Contains(jobID, "..") {
		return fmt.Errorf("invalid job ID %q", jobID)
	}
	return nil
}

func touch(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("touch %s: %w", path, err)
	}
	return file.Close()
}
