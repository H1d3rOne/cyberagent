package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const auditRedactedValue = "[redacted]"

type daemonAuditSink interface {
	Record(context.Context, daemonAuditEvent) error
	Close() error
}

type daemonAuditEvent struct {
	EventID     string         `json:"event_id"`
	Time        string         `json:"time"`
	Type        string         `json:"type"`
	Outcome     string         `json:"outcome,omitempty"`
	PrincipalID string         `json:"principal_id,omitempty"`
	TenantID    string         `json:"tenant_id,omitempty"`
	WorkspaceID string         `json:"workspace_id,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
}

type noopDaemonAuditSink struct{}

func (noopDaemonAuditSink) Record(context.Context, daemonAuditEvent) error { return nil }
func (noopDaemonAuditSink) Close() error                                   { return nil }

type fileDaemonAuditSink struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
	now  func() time.Time
	id   func() string
}

func newDaemonAuditSink(cfg AuditConfig) (daemonAuditSink, error) {
	if !cfg.Enabled {
		return noopDaemonAuditSink{}, nil
	}
	path := strings.TrimSpace(cfg.Path)
	if path == "" {
		return nil, fmt.Errorf("audit path is required when audit is enabled")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create audit directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	return &fileDaemonAuditSink{
		file: file,
		enc:  json.NewEncoder(file),
		now:  func() time.Time { return time.Now().UTC() },
		id:   newDaemonAuditEventID,
	}, nil
}

func (s *fileDaemonAuditSink) Record(ctx context.Context, event daemonAuditEvent) error {
	if s == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = s.id()
	}
	if strings.TrimSpace(event.Time) == "" {
		event.Time = s.now().Format(time.RFC3339Nano)
	}
	event.Type = strings.TrimSpace(event.Type)
	if event.Type == "" {
		event.Type = "daemon.audit_event"
	}
	event.Outcome = strings.TrimSpace(event.Outcome)
	event.PrincipalID = strings.TrimSpace(event.PrincipalID)
	event.TenantID = strings.TrimSpace(event.TenantID)
	event.WorkspaceID = strings.TrimSpace(event.WorkspaceID)
	event.Details = sanitizeAuditDetails(event.Details)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.enc.Encode(event); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func (s *fileDaemonAuditSink) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.file.Close()
	s.file = nil
	return err
}

func newDaemonAuditEventID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("audit-%d", time.Now().UTC().UnixNano())
	}
	return "audit-" + hex.EncodeToString(buf[:])
}

func sanitizeAuditDetails(details map[string]any) map[string]any {
	if details == nil {
		return nil
	}
	out := make(map[string]any, len(details))
	for key, value := range details {
		out[key] = sanitizeAuditValue(key, value)
	}
	return out
}

func sanitizeAuditValue(key string, value any) any {
	if isSensitiveAuditKey(key) {
		return auditRedactedValue
	}
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeAuditDetails(typed)
	case map[string]string:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = sanitizeAuditValue(k, v)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sanitizeAuditValue("", item)
		}
		return out
	case []map[string]any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sanitizeAuditDetails(item)
		}
		return out
	default:
		return value
	}
}

func isSensitiveAuditKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	if normalized == "" {
		return false
	}
	sensitiveFragments := []string{
		"api_key",
		"apikey",
		"access_key",
		"private_key",
		"secret",
		"token",
		"password",
		"passwd",
		"authorization",
		"auth_header",
		"credential",
		"bearer",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
