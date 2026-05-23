package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// Store writes raw artifacts below one run workspace.
type Store struct {
	workspace string
}

// NewStore creates an artifact store rooted at a run workspace path.
func NewStore(workspace string) Store {
	return Store{workspace: workspace}
}

// SaveBytes writes bytes to a relative artifact path and returns an Artifact contract.
func (s Store) SaveBytes(id, jobID, stepID string, typ contracts.ArtifactType, relPath string, data []byte, mimeType string) (contracts.Artifact, error) {
	if strings.TrimSpace(relPath) == "" || filepath.IsAbs(relPath) || strings.Contains(relPath, "..") {
		return contracts.Artifact{}, fmt.Errorf("invalid artifact path %q", relPath)
	}
	full := filepath.Join(s.workspace, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return contracts.Artifact{}, fmt.Errorf("create artifact dir: %w", err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return contracts.Artifact{}, fmt.Errorf("write artifact %s: %w", relPath, err)
	}
	sum := sha256.Sum256(data)
	return contracts.Artifact{
		ID:        id,
		JobID:     jobID,
		StepID:    stepID,
		Type:      typ,
		Path:      relPath,
		SHA256:    hex.EncodeToString(sum[:]),
		SizeBytes: int64(len(data)),
		MIMEType:  mimeType,
		CreatedAt: contracts.Now(),
		Metadata:  contracts.Metadata{},
	}, nil
}

// SaveJSON marshals value and writes it as a JSON artifact.
func (s Store) SaveJSON(id, jobID, stepID string, relPath string, value any) (contracts.Artifact, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return contracts.Artifact{}, fmt.Errorf("marshal JSON artifact: %w", err)
	}
	return s.SaveBytes(id, jobID, stepID, contracts.ArtifactJSON, relPath, encoded, "application/json")
}

// WriteJSON writes a non-artifact JSON file below workspace.
func (s Store) WriteJSON(relPath string, value any) error {
	if strings.TrimSpace(relPath) == "" || filepath.IsAbs(relPath) || strings.Contains(relPath, "..") {
		return fmt.Errorf("invalid workspace path %q", relPath)
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON file: %w", err)
	}
	full := filepath.Join(s.workspace, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create JSON file dir: %w", err)
	}
	return os.WriteFile(full, encoded, 0o644)
}
