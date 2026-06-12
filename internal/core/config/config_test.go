package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultHasSafeProfile(t *testing.T) {
	cfg := Default()
	p, err := cfg.GetProfile("safe")
	if err != nil {
		t.Fatal(err)
	}
	if p.RiskLevel != "low" {
		t.Errorf("expected risk_level low, got %s", p.RiskLevel)
	}
	if p.RateLimit.GlobalRPS != 20 {
		t.Errorf("expected global_rps 20, got %f", p.RateLimit.GlobalRPS)
	}
}

func TestGetProfileMissing(t *testing.T) {
	cfg := Default()
	_, err := cfg.GetProfile("aggressive")
	if err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	content := `
profiles:
  custom:
    name: custom
    risk_level: medium
    rate_limit:
      global_rps: 50
      per_host_rps: 25
      concurrency: 20
defaults:
  profile: custom
  out_dir: output
daemon:
  multi_user:
    enabled: true
    storage_root: data/multi-user
  auth:
    web_sessions:
      enabled: true
      cookie_name: ca_session
      ttl_seconds: 3600
      storage_path: data/auth/sessions.jsonl
      principal_resolver_enabled: true
    rbac:
      enabled: true
      policy_path: data/auth/rbac.yaml
      audit_enabled: true
      enforcement_enabled: true
  audit:
    enabled: true
    path: data/audit/events.jsonl
  admin:
    bootstrap_token_file: data/admin/bootstrap-token
    membership_api_enabled: true
  rate_limit:
    enabled: true
    requests_per_minute: 240
    burst: 60
  hardening:
    require_loopback: true
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Profile != "custom" {
		t.Errorf("expected default profile custom, got %s", cfg.Defaults.Profile)
	}
	if cfg.Defaults.OutDir != "output" {
		t.Errorf("expected out_dir output, got %s", cfg.Defaults.OutDir)
	}
	if cfg.Daemon.MultiUser.Enabled == nil || !*cfg.Daemon.MultiUser.Enabled || cfg.Daemon.MultiUser.StorageRoot != "data/multi-user" {
		t.Errorf("expected daemon multi_user config to load, got %#v", cfg.Daemon.MultiUser)
	}
	if cfg.Daemon.Auth.WebSessions.Enabled == nil || !*cfg.Daemon.Auth.WebSessions.Enabled ||
		cfg.Daemon.Auth.WebSessions.CookieName != "ca_session" ||
		cfg.Daemon.Auth.WebSessions.TTLSeconds != 3600 ||
		cfg.Daemon.Auth.WebSessions.StoragePath != "data/auth/sessions.jsonl" ||
		cfg.Daemon.Auth.WebSessions.PrincipalResolverEnabled == nil || !*cfg.Daemon.Auth.WebSessions.PrincipalResolverEnabled {
		t.Errorf("expected daemon auth web_sessions config to load, got %#v", cfg.Daemon.Auth.WebSessions)
	}
	if cfg.Daemon.Auth.RBAC.Enabled == nil || !*cfg.Daemon.Auth.RBAC.Enabled ||
		cfg.Daemon.Auth.RBAC.PolicyPath != "data/auth/rbac.yaml" ||
		cfg.Daemon.Auth.RBAC.AuditEnabled == nil || !*cfg.Daemon.Auth.RBAC.AuditEnabled ||
		cfg.Daemon.Auth.RBAC.EnforcementEnabled == nil || !*cfg.Daemon.Auth.RBAC.EnforcementEnabled {
		t.Errorf("expected daemon auth rbac config to load, got %#v", cfg.Daemon.Auth.RBAC)
	}
	if cfg.Daemon.Audit.Enabled == nil || !*cfg.Daemon.Audit.Enabled || cfg.Daemon.Audit.Path != "data/audit/events.jsonl" {
		t.Errorf("expected daemon audit config to load, got %#v", cfg.Daemon.Audit)
	}
	if cfg.Daemon.Admin.BootstrapTokenFile != "data/admin/bootstrap-token" ||
		cfg.Daemon.Admin.MembershipAPIEnabled == nil || !*cfg.Daemon.Admin.MembershipAPIEnabled {
		t.Errorf("expected daemon admin bootstrap token file to load, got %#v", cfg.Daemon.Admin)
	}
	if cfg.Daemon.RateLimit.Enabled == nil || !*cfg.Daemon.RateLimit.Enabled || cfg.Daemon.RateLimit.RequestsPerMinute != 240 || cfg.Daemon.RateLimit.Burst != 60 {
		t.Errorf("expected daemon rate_limit config to load, got %#v", cfg.Daemon.RateLimit)
	}
	if cfg.Daemon.Hardening.RequireLoopback == nil || !*cfg.Daemon.Hardening.RequireLoopback {
		t.Errorf("expected daemon hardening config to load, got %#v", cfg.Daemon.Hardening)
	}
	p, err := cfg.GetProfile("custom")
	if err != nil {
		t.Fatal(err)
	}
	if p.RateLimit.GlobalRPS != 50 {
		t.Errorf("expected global_rps 50, got %f", p.RateLimit.GlobalRPS)
	}
	// safe profile should still exist from defaults
	_, err = cfg.GetProfile("safe")
	if err != nil {
		t.Error("expected safe profile to still exist after merge")
	}
}

func TestLoadFromMissingFileReturnsDefault(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Profile != "safe" {
		t.Errorf("expected default profile safe, got %s", cfg.Defaults.Profile)
	}
}

func TestMergeOverlayWins(t *testing.T) {
	base := Default()
	overlay := Config{
		Defaults: Defaults{OutDir: "custom_runs"},
		Plugins:  PluginsConfig{Enabled: []string{"nuclei-safe"}},
		Daemon: DaemonConfig{
			Listen:            "127.0.0.1:9999",
			RunsDir:           "daemon-runs",
			TokenFile:         ".cyberagent/daemon-token",
			MaxConcurrentJobs: 3,
			MultiUser: DaemonMultiUserConfig{
				Enabled:     boolPtr(true),
				StorageRoot: "data/multi-user",
			},
			Auth: DaemonAuthConfig{
				WebSessions: DaemonWebSessionAuthConfig{
					Enabled:                  boolPtr(true),
					CookieName:               "ca_session",
					TTLSeconds:               3600,
					StoragePath:              "data/auth/sessions.jsonl",
					PrincipalResolverEnabled: boolPtr(true),
				},
				RBAC: DaemonRBACConfig{
					Enabled:            boolPtr(true),
					PolicyPath:         "data/auth/rbac.yaml",
					AuditEnabled:       boolPtr(true),
					EnforcementEnabled: boolPtr(true),
				},
			},
			Audit: DaemonAuditConfig{
				Enabled: boolPtr(true),
				Path:    "data/audit/events.jsonl",
			},
			Admin: DaemonAdminConfig{
				BootstrapTokenFile:   "data/admin/bootstrap-token",
				MembershipAPIEnabled: boolPtr(true),
			},
			RateLimit: DaemonRateLimitConfig{
				Enabled:           boolPtr(true),
				RequestsPerMinute: 240,
				Burst:             60,
			},
			Hardening: DaemonHardeningConfig{
				RequireLoopback: boolPtr(true),
			},
		},
	}
	result := merge(base, overlay)
	if result.Defaults.OutDir != "custom_runs" {
		t.Errorf("expected out_dir custom_runs, got %s", result.Defaults.OutDir)
	}
	if len(result.Plugins.Enabled) != 1 || result.Plugins.Enabled[0] != "nuclei-safe" {
		t.Errorf("expected plugins.enabled [nuclei-safe], got %v", result.Plugins.Enabled)
	}
	if result.Defaults.Profile != "safe" {
		t.Error("merge should preserve base values not overridden")
	}
	if result.Daemon.Listen != "127.0.0.1:9999" || result.Daemon.RunsDir != "daemon-runs" || result.Daemon.TokenFile != ".cyberagent/daemon-token" || result.Daemon.MaxConcurrentJobs != 3 {
		t.Errorf("daemon config not merged: %#v", result.Daemon)
	}
	if result.Daemon.MultiUser.Enabled == nil || !*result.Daemon.MultiUser.Enabled || result.Daemon.MultiUser.StorageRoot != "data/multi-user" {
		t.Errorf("daemon multi_user config not merged: %#v", result.Daemon.MultiUser)
	}
	if result.Daemon.Auth.WebSessions.Enabled == nil || !*result.Daemon.Auth.WebSessions.Enabled ||
		result.Daemon.Auth.WebSessions.CookieName != "ca_session" ||
		result.Daemon.Auth.WebSessions.TTLSeconds != 3600 ||
		result.Daemon.Auth.WebSessions.StoragePath != "data/auth/sessions.jsonl" ||
		result.Daemon.Auth.WebSessions.PrincipalResolverEnabled == nil || !*result.Daemon.Auth.WebSessions.PrincipalResolverEnabled {
		t.Errorf("daemon auth web_sessions config not merged: %#v", result.Daemon.Auth.WebSessions)
	}
	if result.Daemon.Auth.RBAC.Enabled == nil || !*result.Daemon.Auth.RBAC.Enabled ||
		result.Daemon.Auth.RBAC.PolicyPath != "data/auth/rbac.yaml" ||
		result.Daemon.Auth.RBAC.AuditEnabled == nil || !*result.Daemon.Auth.RBAC.AuditEnabled ||
		result.Daemon.Auth.RBAC.EnforcementEnabled == nil || !*result.Daemon.Auth.RBAC.EnforcementEnabled {
		t.Errorf("daemon auth rbac config not merged: %#v", result.Daemon.Auth.RBAC)
	}
	if result.Daemon.Audit.Enabled == nil || !*result.Daemon.Audit.Enabled || result.Daemon.Audit.Path != "data/audit/events.jsonl" {
		t.Errorf("daemon audit config not merged: %#v", result.Daemon.Audit)
	}
	if result.Daemon.Admin.BootstrapTokenFile != "data/admin/bootstrap-token" ||
		result.Daemon.Admin.MembershipAPIEnabled == nil || !*result.Daemon.Admin.MembershipAPIEnabled {
		t.Errorf("daemon admin config not merged: %#v", result.Daemon.Admin)
	}
	if result.Daemon.RateLimit.Enabled == nil || !*result.Daemon.RateLimit.Enabled || result.Daemon.RateLimit.RequestsPerMinute != 240 || result.Daemon.RateLimit.Burst != 60 {
		t.Errorf("daemon rate_limit config not merged: %#v", result.Daemon.RateLimit)
	}
	if result.Daemon.Hardening.RequireLoopback == nil || !*result.Daemon.Hardening.RequireLoopback {
		t.Errorf("daemon hardening config not merged: %#v", result.Daemon.Hardening)
	}
}

func TestLoadNoConfigFiles(t *testing.T) {
	// With no project or user config, Load should return defaults
	orig, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(orig)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Profile != "safe" {
		t.Errorf("expected safe, got %s", cfg.Defaults.Profile)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
