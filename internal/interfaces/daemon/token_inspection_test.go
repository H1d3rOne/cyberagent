package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInspectTokenFileReportsSafeDiagnostics(t *testing.T) {
	dir := t.TempDir()
	missing := InspectTokenFile(filepath.Join(dir, "missing-token"))
	if missing.Exists || missing.Readable || missing.Length != 0 || missing.Error != "" {
		t.Fatalf("missing token inspection = %#v", missing)
	}

	emptyPath := filepath.Join(dir, "empty-token")
	if err := os.WriteFile(emptyPath, []byte("\n"), 0o600); err != nil {
		t.Fatalf("write empty token: %v", err)
	}
	empty := InspectTokenFile(emptyPath)
	if !empty.Exists || !empty.Readable || !empty.Empty || empty.Length != 0 || empty.Error != "" {
		t.Fatalf("empty token inspection = %#v", empty)
	}

	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("super-secret-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	got := InspectTokenFile(tokenPath)
	if !got.Exists || !got.Readable || got.Empty || got.Length != len("super-secret-token") || got.Error != "" {
		t.Fatalf("token inspection = %#v", got)
	}
	if strings.Contains(got.Error, "super-secret-token") || strings.Contains(got.Mode, "super-secret-token") {
		t.Fatalf("inspection leaked token: %#v", got)
	}
	if runtime.GOOS != "windows" && (!got.SecurePermissions || got.Mode != "0600") {
		t.Fatalf("secure token permissions not detected: %#v", got)
	}
}

func TestInspectTokenFileFlagsLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX token mode checks do not apply on windows")
	}
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("write token: %v", err)
	}
	got := InspectTokenFile(path)
	if got.SecurePermissions || got.Mode != "0644" {
		t.Fatalf("loose token permissions not flagged: %#v", got)
	}
}

func TestRedactBearerTokenForLogDoesNotLeakSecret(t *testing.T) {
	secret := "  token-value-to-hide  "
	got := RedactBearerTokenForLog(secret)
	if strings.Contains(got, "token-value-to-hide") || !strings.Contains(got, "len=19") {
		t.Fatalf("unexpected redaction = %q", got)
	}
	if got := RedactBearerTokenForLog(" \n"); got != "<redacted:empty>" {
		t.Fatalf("empty redaction = %q", got)
	}
}
