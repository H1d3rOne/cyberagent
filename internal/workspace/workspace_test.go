package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceRejectsAdjacentPrefixTraversal(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, DefaultMaxSize)
	ws, err := manager.Create("session")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if !strings.HasSuffix(filepath.Clean(ws.Path), filepath.Join("session", "workspace")) {
		t.Fatalf("unexpected workspace path: %s", ws.Path)
	}

	escapeRel := "../workspace_evil/owned.txt"
	if err := manager.WriteFile("session", escapeRel, []byte("escape")); err == nil {
		t.Fatalf("WriteFile should reject adjacent-prefix traversal path")
	}
	if _, err := os.Stat(filepath.Join(root, "session", "workspace_evil", "owned.txt")); !os.IsNotExist(err) {
		t.Fatalf("escaped file should not exist; stat err=%v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "session", "workspace_evil"), 0o755); err != nil {
		t.Fatalf("create adjacent directory: %v", err)
	}
	if _, err := manager.ListFiles("session", "../workspace_evil"); err == nil {
		t.Fatalf("ListFiles should reject adjacent-prefix traversal path")
	}
	if _, err := manager.ReadFile("session", "../workspace_evil/owned.txt"); err == nil {
		t.Fatalf("ReadFile should reject adjacent-prefix traversal path")
	}
	if err := manager.DeleteFile("session", "../workspace_evil/owned.txt"); err == nil {
		t.Fatalf("DeleteFile should reject adjacent-prefix traversal path")
	}
}

func TestWorkspaceAllowsNestedRelativePaths(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, DefaultMaxSize)
	if _, err := manager.Create("session"); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if err := manager.WriteFile("session", "notes/nested.md", []byte("hello")); err != nil {
		t.Fatalf("WriteFile nested path: %v", err)
	}
	data, err := manager.ReadFile("session", "notes/../notes/nested.md")
	if err != nil {
		t.Fatalf("ReadFile normalized nested path: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file content: %q", string(data))
	}
	files, err := manager.ListFiles("session", "notes")
	if err != nil {
		t.Fatalf("ListFiles nested path: %v", err)
	}
	if len(files) != 1 || files[0].Path != filepath.Join("notes", "nested.md") {
		t.Fatalf("unexpected file list: %#v", files)
	}
}

func TestWorkspaceMoveFile(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, DefaultMaxSize)
	if _, err := manager.Create("session"); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if err := manager.WriteFile("session", "notes.md", []byte("hello")); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}
	if err := manager.MoveFile("session", "notes.md", "archive/notes-renamed.md"); err != nil {
		t.Fatalf("MoveFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "session", "workspace", "notes.md")); !os.IsNotExist(err) {
		t.Fatalf("source file should be moved away; stat err=%v", err)
	}
	data, err := manager.ReadFile("session", "archive/notes-renamed.md")
	if err != nil {
		t.Fatalf("ReadFile moved file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected moved file content: %q", string(data))
	}
}

func TestWorkspaceCopyFileAndMakeDir(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, DefaultMaxSize)
	if _, err := manager.Create("session"); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	if err := manager.MakeDir("session", "archive"); err != nil {
		t.Fatalf("MakeDir: %v", err)
	}
	if err := manager.WriteFile("session", "notes.md", []byte("copy me")); err != nil {
		t.Fatalf("WriteFile source: %v", err)
	}
	if err := manager.CopyFile("session", "notes.md", "archive/notes-copy.md"); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	source, err := manager.ReadFile("session", "notes.md")
	if err != nil {
		t.Fatalf("ReadFile source after copy: %v", err)
	}
	copied, err := manager.ReadFile("session", "archive/notes-copy.md")
	if err != nil {
		t.Fatalf("ReadFile copied file: %v", err)
	}
	if string(source) != "copy me" || string(copied) != "copy me" {
		t.Fatalf("unexpected copy contents: source=%q copied=%q", string(source), string(copied))
	}
	if err := manager.CopyFile("session", "notes.md", "../workspace_evil/copy.md"); err == nil {
		t.Fatalf("CopyFile should reject traversal destination")
	}
	if err := manager.MakeDir("session", "../workspace_evil"); err == nil {
		t.Fatalf("MakeDir should reject traversal destination")
	}
}
