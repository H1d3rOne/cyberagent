package daemon

import "testing"

func TestAgentExtractURLAllowsBareAuthorizedHost(t *testing.T) {
	got := agentExtractURL("扫描 wankey.xyz 端口")
	if got != "https://wankey.xyz" {
		t.Fatalf("agentExtractURL() = %q, want https://wankey.xyz", got)
	}
}

func TestAgentExtractURLPrefersExplicitHTTPURL(t *testing.T) {
	got := agentExtractURL("扫描 http://wankey.xyz:8080/app scope=wankey.xyz")
	if got != "http://wankey.xyz:8080/app" {
		t.Fatalf("agentExtractURL() = %q, want explicit URL", got)
	}
}

func TestAgentExtractURLDoesNotTreatWorkspaceFileAsHost(t *testing.T) {
	got := agentExtractURL("读取 report.md 并总结")
	if got != "" {
		t.Fatalf("agentExtractURL() = %q, want empty for workspace file", got)
	}
}
