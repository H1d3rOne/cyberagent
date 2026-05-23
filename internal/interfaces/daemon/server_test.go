package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func TestNewServerCreatesTokenFileAndNormalizesDefaults(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "config", "token")
	server, err := NewServer(Config{TokenFile: tokenFile})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	if server.Addr() != DefaultListen {
		t.Fatalf("Addr() = %q, want %q", server.Addr(), DefaultListen)
	}
	if server.TokenFile() != tokenFile {
		t.Fatalf("TokenFile() = %q, want %q", server.TokenFile(), tokenFile)
	}
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if len(data) < 32 {
		t.Fatalf("token file too short: %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(tokenFile)
		if err != nil {
			t.Fatalf("stat token file: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("token file permissions = %o, want 600", got)
		}
	}
}

func TestHTTPAuthEnvelopeHealthAndVersion(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("test-token\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	server, err := NewServer(Config{
		TokenFile: tokenFile,
		Build: services.BuildInfo{
			Name:    "CyberAgent",
			Version: "vdaemon-test",
			Commit:  "abc123",
			Date:    "2026-05-24T00:00:00Z",
			Module:  "github.com/H1d3rOne/cyberagent",
		},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	noAuth := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "")
	if noAuth.StatusCode != http.StatusUnauthorized || noAuth.Body.OK || noAuth.Body.Error.Code != services.CodeUnauthorized {
		t.Fatalf("unexpected no-auth response: status=%d body=%#v", noAuth.StatusCode, noAuth.Body)
	}

	badAuth := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "wrong-token")
	if badAuth.StatusCode != http.StatusUnauthorized || badAuth.Body.Error.Code != services.CodeUnauthorized {
		t.Fatalf("unexpected bad-auth response: status=%d body=%#v", badAuth.StatusCode, badAuth.Body)
	}

	version := doDaemonRequest(t, httpServer.URL+"/api/v1/version", "test-token")
	if version.StatusCode != http.StatusOK || !version.Body.OK || version.Body.Error != nil {
		t.Fatalf("unexpected version response: status=%d body=%#v", version.StatusCode, version.Body)
	}
	if version.Body.Data["version"] != "vdaemon-test" || version.Body.Data["commit"] != "abc123" {
		t.Fatalf("unexpected version data: %#v", version.Body.Data)
	}

	health := doDaemonRequest(t, httpServer.URL+"/api/v1/healthz", "test-token")
	if health.StatusCode != http.StatusOK || !health.Body.OK || health.Body.Data["status"] != "ok" {
		t.Fatalf("unexpected health response: status=%d body=%#v", health.StatusCode, health.Body)
	}

	missing := doDaemonRequest(t, httpServer.URL+"/api/v1/missing", "test-token")
	if missing.StatusCode != http.StatusNotFound || missing.Body.OK || missing.Body.Error.Code != services.CodeNotFound {
		t.Fatalf("unexpected missing response: status=%d body=%#v", missing.StatusCode, missing.Body)
	}
}

type daemonHTTPResponse struct {
	StatusCode int
	Body       daemonTestEnvelope
}

type daemonTestEnvelope struct {
	OK    bool           `json:"ok"`
	Data  map[string]any `json:"data"`
	Error *ErrorBody     `json:"error"`
}

func doDaemonRequest(t *testing.T, url, token string) daemonHTTPResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	var body daemonTestEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return daemonHTTPResponse{StatusCode: resp.StatusCode, Body: body}
}
