package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultWorkspaceRoot = ".cyberagent/workspaces"
	DefaultMaxSize       = 100 * 1024 * 1024 // 100MB
	DefaultMaxFileSize   = 10 * 1024 * 1024   // 10MB
)

// Manager manages agent workspaces.
type Manager struct {
	mu       sync.Mutex
	root     string
	maxSize  int64
	maxFiles int
}

// NewManager creates a new workspace manager.
func NewManager(root string, maxSize int64) *Manager {
	if root == "" {
		root = DefaultWorkspaceRoot
	}
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	return &Manager{
		root:    root,
		maxSize: maxSize,
	}
}

// Workspace represents an agent workspace.
type Workspace struct {
	SessionID string    `json:"session_id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
}

// FileInfo represents a file in the workspace.
type FileInfo struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"is_dir"`
	ModTime  time.Time `json:"mod_time"`
	Mode     string    `json:"mode"`
}

// Create creates a new workspace for a session.
func (m *Manager) Create(sessionID string) (*Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}

	// Sanitize session ID
	safeID := sanitizePath(sessionID)
	wsPath := filepath.Join(m.root, safeID, "workspace")

	// Create directory
	if err := os.MkdirAll(wsPath, 0755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	// Create state file
	statePath := filepath.Join(m.root, safeID, "state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		if err := os.WriteFile(statePath, []byte("{}"), 0644); err != nil {
			return nil, fmt.Errorf("create state file: %w", err)
		}
	}

	// Create notes file
	notesPath := filepath.Join(m.root, safeID, "notes.md")
	if _, err := os.Stat(notesPath); os.IsNotExist(err) {
		if err := os.WriteFile(notesPath, []byte("# Agent Notes\n"), 0644); err != nil {
			return nil, fmt.Errorf("create notes file: %w", err)
		}
	}

	return &Workspace{
		SessionID: sessionID,
		Path:      wsPath,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Get returns the workspace for a session.
func (m *Manager) Get(sessionID string) (*Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sessionID == "" {
		return nil, fmt.Errorf("session ID is required")
	}

	safeID := sanitizePath(sessionID)
	wsPath := filepath.Join(m.root, safeID, "workspace")

	// Check if directory exists
	info, err := os.Stat(wsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workspace not found for session %s", sessionID)
		}
		return nil, fmt.Errorf("check workspace: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("workspace path is not a directory")
	}

	return &Workspace{
		SessionID: sessionID,
		Path:      wsPath,
		CreatedAt: info.ModTime(),
	}, nil
}

// ListFiles lists files in the workspace.
func (m *Manager) ListFiles(sessionID, relPath string) ([]FileInfo, error) {
	ws, err := m.Get(sessionID)
	if err != nil {
		return nil, err
	}

	// Resolve path
	absPath := filepath.Join(ws.Path, relPath)
	if !strings.HasPrefix(absPath, ws.Path) {
		return nil, fmt.Errorf("path traversal not allowed")
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("check path: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory")
	}

	// List files
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(relPath, entry.Name()),
			Size:    info.Size(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime(),
			Mode:    info.Mode().String(),
		})
	}

	return files, nil
}

// ReadFile reads a file from the workspace.
func (m *Manager) ReadFile(sessionID, relPath string) ([]byte, error) {
	ws, err := m.Get(sessionID)
	if err != nil {
		return nil, err
	}

	// Resolve path
	absPath := filepath.Join(ws.Path, relPath)
	if !strings.HasPrefix(absPath, ws.Path) {
		return nil, fmt.Errorf("path traversal not allowed")
	}

	// Check file size
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("check file: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory")
	}

	if info.Size() > DefaultMaxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), DefaultMaxFileSize)
	}

	// Read file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return data, nil
}

// WriteFile writes a file to the workspace.
func (m *Manager) WriteFile(sessionID, relPath string, data []byte) error {
	ws, err := m.Get(sessionID)
	if err != nil {
		return err
	}

	// Resolve path
	absPath := filepath.Join(ws.Path, relPath)
	if !strings.HasPrefix(absPath, ws.Path) {
		return fmt.Errorf("path traversal not allowed")
	}

	// Check data size
	if int64(len(data)) > DefaultMaxFileSize {
		return fmt.Errorf("data too large: %d bytes (max %d)", len(data), DefaultMaxFileSize)
	}

	// Create directory if needed
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(absPath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// DeleteFile deletes a file from the workspace.
func (m *Manager) DeleteFile(sessionID, relPath string) error {
	ws, err := m.Get(sessionID)
	if err != nil {
		return err
	}

	// Resolve path
	absPath := filepath.Join(ws.Path, relPath)
	if !strings.HasPrefix(absPath, ws.Path) {
		return fmt.Errorf("path traversal not allowed")
	}

	// Check if file exists
	_, err = os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("check file: %w", err)
	}

	// Delete file
	if err := os.Remove(absPath); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}

	return nil
}

// sanitizePath sanitizes a path component.
func sanitizePath(path string) string {
	// Replace path separators and special characters
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "\\", "_")
	path = strings.ReplaceAll(path, "..", "_")
	path = strings.ReplaceAll(path, " ", "_")
	return path
}
