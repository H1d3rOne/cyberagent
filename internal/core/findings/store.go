package findings

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// EvidenceIndex stores evidence and artifacts for report/replay.
type EvidenceIndex struct {
	Artifacts []contracts.Artifact `json:"artifacts"`
	Evidence  []contracts.Evidence `json:"evidence"`
}

// Store persists evidence and finding files in one run workspace.
type Store struct {
	workspace string
}

// NewStore creates a findings store rooted at a run workspace path.
func NewStore(workspace string) Store {
	return Store{workspace: workspace}
}

// WriteEvidence writes evidence/index.json and evidence/evidence.jsonl.
func (s Store) WriteEvidence(index EvidenceIndex) error {
	if err := writeJSON(filepath.Join(s.workspace, "evidence", "index.json"), index); err != nil {
		return err
	}
	path := filepath.Join(s.workspace, "evidence", "evidence.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create evidence dir: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create evidence jsonl: %w", err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, evidence := range index.Evidence {
		encoded, err := json.Marshal(evidence)
		if err != nil {
			return fmt.Errorf("marshal evidence: %w", err)
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			return fmt.Errorf("write evidence jsonl: %w", err)
		}
	}
	return writer.Flush()
}

// WriteFindings writes findings.json.
func (s Store) WriteFindings(findings []contracts.Finding) error {
	return writeJSON(filepath.Join(s.workspace, "findings.json"), findings)
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return os.WriteFile(path, encoded, 0o644)
}
