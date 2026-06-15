package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDaemonAuditSinkDisabledDoesNotCreateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "events.jsonl")
	sink, err := newDaemonAuditSink(AuditConfig{Enabled: false, Path: path})
	if err != nil {
		t.Fatalf("new audit sink: %v", err)
	}
	if err := sink.Record(context.Background(), daemonAuditEvent{Type: "test.event"}); err != nil {
		t.Fatalf("record disabled audit event: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("close disabled audit sink: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled audit should not create file, stat err=%v", err)
	}
}

func TestDaemonAuditSinkWritesJSONLAndRedactsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "events.jsonl")
	sink, err := newDaemonAuditSink(AuditConfig{Enabled: true, Path: path})
	if err != nil {
		t.Fatalf("new audit sink: %v", err)
	}
	details := map[string]any{
		"safe":          "visible",
		"token":         "secret-token",
		"api_key":       "secret-api-key",
		"authorization": "Bearer secret-authorization",
		"nested": map[string]any{
			"password": "secret-password",
			"scope":    "local/default",
		},
		"items": []any{
			map[string]any{"private_key": "secret-private-key", "name": "provider"},
		},
	}
	if err := sink.Record(context.Background(), daemonAuditEvent{Type: "provider.secret.updated", Outcome: "ok", Details: details}); err != nil {
		t.Fatalf("record audit event: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("close audit sink: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	for _, leaked := range []string{"secret-token", "secret-api-key", "secret-authorization", "secret-password", "secret-private-key"} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("audit log leaked %q: %s", leaked, string(data))
		}
	}
	if !strings.Contains(string(data), auditRedactedValue) || !strings.Contains(string(data), "visible") {
		t.Fatalf("audit log did not preserve/redact expected fields: %s", string(data))
	}

	var event daemonAuditEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode audit event: %v\n%s", err, string(data))
	}
	if event.EventID == "" || event.Time == "" || event.Type != "provider.secret.updated" || event.Outcome != "ok" {
		t.Fatalf("audit event envelope incomplete: %#v", event)
	}
	if got := event.Details["token"]; got != auditRedactedValue {
		t.Fatalf("token detail = %#v, want redacted", got)
	}
}

func TestNewServerWritesStartupAuditWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit", "events.jsonl")
	tokenFile := filepath.Join(dir, "token")
	adminBootstrapTokenFile := filepath.Join(dir, "admin", "bootstrap-token")
	if err := os.MkdirAll(filepath.Dir(adminBootstrapTokenFile), 0o700); err != nil {
		t.Fatalf("create admin dir: %v", err)
	}
	if err := os.WriteFile(adminBootstrapTokenFile, []byte("bootstrap-secret\n"), 0o600); err != nil {
		t.Fatalf("write admin bootstrap token: %v", err)
	}
	server, err := NewServer(Config{
		RunsDir:   filepath.Join(dir, "runs"),
		TokenFile: tokenFile,
		Audit: AuditConfig{
			Enabled: true,
			Path:    auditPath,
		},
		Admin: AdminConfig{
			BootstrapTokenFile:     adminBootstrapTokenFile,
			MembershipAPIEnabled:   true,
			MembershipWriteEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	trimmedToken := strings.TrimSpace(string(token))
	if trimmedToken == "" {
		t.Fatalf("expected generated token")
	}
	if strings.Contains(string(data), trimmedToken) || strings.Contains(string(data), tokenFile) || strings.Contains(string(data), "bootstrap-secret") || strings.Contains(string(data), adminBootstrapTokenFile) {
		t.Fatalf("startup audit leaked token material/path: %s", string(data))
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() {
		t.Fatalf("expected one startup audit event, data=%q", string(data))
	}
	var event daemonAuditEvent
	if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
		t.Fatalf("decode startup audit event: %v", err)
	}
	if event.Type != "daemon.startup" || event.Outcome != "ready" {
		t.Fatalf("unexpected startup audit event: %#v", event)
	}
	if got := event.Details["admin_bootstrap_configured"]; got != true {
		t.Fatalf("admin_bootstrap_configured = %#v, want true", got)
	}
	if got := event.Details["admin_bootstrap_ready"]; got != true {
		t.Fatalf("admin_bootstrap_ready = %#v, want true", got)
	}
	if got := event.Details["admin_membership_write"]; got != true {
		t.Fatalf("admin_membership_write = %#v, want true", got)
	}
	if scanner.Scan() {
		t.Fatalf("expected only one startup audit event, extra=%q", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan audit log: %v", err)
	}
}
