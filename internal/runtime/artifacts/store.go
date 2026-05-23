package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

const indexSchemaVersion = "0.1.1"

// Index is the machine-readable artifact inventory for one run workspace.
type Index struct {
	SchemaVersion string               `json:"schema_version"`
	GeneratedAt   time.Time            `json:"generated_at"`
	Count         int                  `json:"count"`
	Artifacts     []contracts.Artifact `json:"artifacts"`
}

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
	full, err := s.resolveWorkspacePath(relPath)
	if err != nil {
		return contracts.Artifact{}, fmt.Errorf("invalid artifact path %q", relPath)
	}
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

// WriteIndex completes artifact metadata from disk and writes artifacts/index.json.
func (s Store) WriteIndex(items []contracts.Artifact) ([]contracts.Artifact, error) {
	completed := make([]contracts.Artifact, 0, len(items))
	for _, item := range items {
		artifact, err := s.CompleteMetadata(item)
		if err != nil {
			return nil, err
		}
		completed = append(completed, artifact)
	}
	index := Index{
		SchemaVersion: indexSchemaVersion,
		GeneratedAt:   contracts.Now(),
		Count:         len(completed),
		Artifacts:     completed,
	}
	if err := s.WriteJSON("artifacts/index.json", index); err != nil {
		return nil, fmt.Errorf("write artifact index: %w", err)
	}
	return completed, nil
}

// CompleteMetadata fills hash, size, MIME, and created_at from the artifact file when absent.
func (s Store) CompleteMetadata(artifact contracts.Artifact) (contracts.Artifact, error) {
	sha, size, detectedMIME, modTime, err := s.fileMetadata(artifact.Path)
	if err != nil {
		return contracts.Artifact{}, fmt.Errorf("artifact %s metadata: %w", artifact.ID, err)
	}
	if strings.TrimSpace(artifact.SHA256) == "" {
		artifact.SHA256 = sha
	}
	if artifact.SizeBytes == 0 {
		artifact.SizeBytes = size
	}
	if strings.TrimSpace(artifact.MIMEType) == "" {
		artifact.MIMEType = detectedMIME
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = modTime
	}
	return artifact, nil
}

// WriteJSON writes a non-artifact JSON file below workspace.
func (s Store) WriteJSON(relPath string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON file: %w", err)
	}
	full, err := s.resolveWorkspacePath(relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create JSON file dir: %w", err)
	}
	return os.WriteFile(full, encoded, 0o644)
}

func (s Store) fileMetadata(relPath string) (string, int64, string, time.Time, error) {
	full, err := s.resolveWorkspacePath(relPath)
	if err != nil {
		return "", 0, "", time.Time{}, err
	}
	file, err := os.Open(full)
	if err != nil {
		return "", 0, "", time.Time{}, fmt.Errorf("open artifact file %s: %w", relPath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", 0, "", time.Time{}, fmt.Errorf("stat artifact file %s: %w", relPath, err)
	}
	sniff := make([]byte, 512)
	n, readErr := file.Read(sniff)
	if readErr != nil && readErr != io.EOF {
		return "", 0, "", time.Time{}, fmt.Errorf("read artifact file %s: %w", relPath, readErr)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", 0, "", time.Time{}, fmt.Errorf("seek artifact file %s: %w", relPath, err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", 0, "", time.Time{}, fmt.Errorf("hash artifact file %s: %w", relPath, err)
	}
	mimeType := detectMIME(relPath, sniff[:n])
	return hex.EncodeToString(hash.Sum(nil)), info.Size(), mimeType, info.ModTime().UTC(), nil
}

func (s Store) resolveWorkspacePath(relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" || filepath.IsAbs(relPath) || strings.Contains(relPath, "..") {
		return "", fmt.Errorf("invalid workspace path %q", relPath)
	}
	return filepath.Join(s.workspace, relPath), nil
}

func detectMIME(path string, sample []byte) string {
	if byExt := strings.TrimSpace(mime.TypeByExtension(filepath.Ext(path))); byExt != "" {
		return byExt
	}
	if len(sample) > 0 {
		return http.DetectContentType(sample)
	}
	return "application/octet-stream"
}
