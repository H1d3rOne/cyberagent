package desktop

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartEmbeddedDaemonPublishesLoopbackWebViewBootstrap(t *testing.T) {
	runtime, err := StartEmbeddedDaemon(context.Background(), BootstrapConfig{
		RunsDir:   t.TempDir(),
		TokenFile: filepath.Join(t.TempDir(), "config", "token"),
		LogDir:    filepath.Join(t.TempDir(), "logs"),
	})
	if err != nil {
		t.Fatalf("StartEmbeddedDaemon() error = %v", err)
	}
	defer stopEmbeddedDaemonForTest(t, runtime)

	contract := runtime.Contract()
	bootstrap := runtime.WebViewBootstrap()
	if !strings.HasPrefix(contract.APIBaseURL, "http://127.0.0.1:") {
		t.Fatalf("APIBaseURL = %q, want loopback ephemeral URL", contract.APIBaseURL)
	}
	if bootstrap.APIBaseURL != contract.APIBaseURL {
		t.Fatalf("WebView APIBaseURL = %q, want %q", bootstrap.APIBaseURL, contract.APIBaseURL)
	}
	if bootstrap.AuthorizationType != "Bearer" || strings.TrimSpace(bootstrap.AuthorizationToken) == "" {
		t.Fatalf("invalid WebView auth bootstrap: %#v", bootstrap)
	}
	if contract.AuthMode != AuthModeBearerTokenFile {
		t.Fatalf("AuthMode = %q, want %q", contract.AuthMode, AuthModeBearerTokenFile)
	}
	if contract.Permissions.NetworkBinding != "loopback_ephemeral_only" {
		t.Fatalf("NetworkBinding = %q", contract.Permissions.NetworkBinding)
	}

	req, err := http.NewRequest(http.MethodGet, contract.APIBaseURL+"/api/v1/healthz", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Authorization", bootstrap.AuthorizationType+" "+bootstrap.AuthorizationToken)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("desktop embedded health request: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestRuntimeContractDoesNotSerializeBearerToken(t *testing.T) {
	runtime, err := StartEmbeddedDaemon(context.Background(), BootstrapConfig{
		RunsDir:   t.TempDir(),
		TokenFile: filepath.Join(t.TempDir(), "config", "token"),
	})
	if err != nil {
		t.Fatalf("StartEmbeddedDaemon() error = %v", err)
	}
	defer stopEmbeddedDaemonForTest(t, runtime)

	bootstrap := runtime.WebViewBootstrap()
	encoded, err := json.Marshal(runtime.Contract())
	if err != nil {
		t.Fatalf("Marshal(RuntimeContract) error = %v", err)
	}
	if strings.Contains(string(encoded), bootstrap.AuthorizationToken) {
		t.Fatalf("RuntimeContract JSON leaked bearer token: %s", encoded)
	}
	if !strings.Contains(string(encoded), AuthModeBearerTokenFile) {
		t.Fatalf("RuntimeContract JSON missing auth mode: %s", encoded)
	}
}

func TestBootstrapConfigRejectsNonLoopbackListenHost(t *testing.T) {
	if _, err := (BootstrapConfig{ListenHost: "0.0.0.0"}).Normalize(); err == nil {
		t.Fatal("Normalize() accepted non-loopback ListenHost")
	}
	cfg, err := (BootstrapConfig{ListenHost: "localhost"}).Normalize()
	if err != nil {
		t.Fatalf("Normalize(localhost) error = %v", err)
	}
	if cfg.ListenHost != DefaultListenHost {
		t.Fatalf("ListenHost = %q, want %q", cfg.ListenHost, DefaultListenHost)
	}
}

func stopEmbeddedDaemonForTest(t *testing.T, runtime *EmbeddedDaemon) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := runtime.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}
