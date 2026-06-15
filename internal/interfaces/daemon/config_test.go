package daemon

import (
	"path/filepath"
	"testing"
)

func TestConfigNormalizeKeepsL3A7HardeningDefaultsNoOp(t *testing.T) {
	cfg := Config{}.Normalize()

	if cfg.Audit.Enabled || cfg.Audit.Path != "" {
		t.Fatalf("audit should default to disabled/no path, got %#v", cfg.Audit)
	}
	if cfg.Auth.WebSessions.Enabled ||
		cfg.Auth.WebSessions.CookieName != "" ||
		cfg.Auth.WebSessions.TTLSeconds != 0 ||
		cfg.Auth.WebSessions.StoragePath != "" ||
		cfg.Auth.WebSessions.PrincipalResolverEnabled {
		t.Fatalf("web session auth should default to disabled/no cookie store, got %#v", cfg.Auth.WebSessions)
	}
	if cfg.Auth.RBAC.Enabled || cfg.Auth.RBAC.PolicyPath != "" || cfg.Auth.RBAC.AuditEnabled || cfg.Auth.RBAC.EnforcementEnabled {
		t.Fatalf("rbac should default to disabled/no policy path, got %#v", cfg.Auth.RBAC)
	}
	if cfg.Admin.BootstrapTokenFile != "" || cfg.Admin.MembershipAPIEnabled || cfg.Admin.MembershipWriteEnabled {
		t.Fatalf("admin bootstrap should default to no-op, got %#v", cfg.Admin)
	}
	if cfg.RateLimit.Enabled || cfg.RateLimit.RequestsPerMinute != 0 || cfg.RateLimit.Burst != 0 {
		t.Fatalf("rate limit should default to disabled/no limits, got %#v", cfg.RateLimit)
	}
	if cfg.Hardening.RequireLoopback {
		t.Fatalf("hardening require_loopback should default to false/no-op, got %#v", cfg.Hardening)
	}
}

func TestConfigNormalizeL3A7ExplicitConfig(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "runs")
	cfg := Config{
		RunsDir: runsDir,
		Audit: AuditConfig{
			Enabled: true,
			Path:    "  ",
		},
		Auth: AuthConfig{
			WebSessions: WebSessionAuthConfig{
				Enabled:                  true,
				CookieName:               "  ",
				StoragePath:              "  data/auth/sessions.jsonl  ",
				PrincipalResolverEnabled: true,
			},
			RBAC: RBACConfig{
				Enabled:            true,
				PolicyPath:         "  data/auth/rbac.yaml  ",
				AuditEnabled:       true,
				EnforcementEnabled: true,
			},
		},
		Admin: AdminConfig{
			BootstrapTokenFile:     "  data/admin/bootstrap-token  ",
			MembershipAPIEnabled:   true,
			MembershipWriteEnabled: true,
		},
		RateLimit: RateLimitConfig{
			Enabled: true,
		},
		Hardening: HardeningConfig{
			RequireLoopback: true,
		},
	}.Normalize()

	wantAuditPath := filepath.Join(runsDir, ".daemon", "audit", "events.jsonl")
	if cfg.Audit.Path != wantAuditPath {
		t.Fatalf("audit path = %q, want %q", cfg.Audit.Path, wantAuditPath)
	}
	if !cfg.Auth.WebSessions.Enabled ||
		cfg.Auth.WebSessions.CookieName != DefaultWebSessionCookieName ||
		cfg.Auth.WebSessions.TTLSeconds != DefaultWebSessionTTLSeconds ||
		cfg.Auth.WebSessions.StoragePath != "data/auth/sessions.jsonl" ||
		!cfg.Auth.WebSessions.PrincipalResolverEnabled {
		t.Fatalf("web session auth explicit config not normalized: %#v", cfg.Auth.WebSessions)
	}
	if !cfg.Auth.RBAC.Enabled ||
		cfg.Auth.RBAC.PolicyPath != "data/auth/rbac.yaml" ||
		!cfg.Auth.RBAC.AuditEnabled ||
		!cfg.Auth.RBAC.EnforcementEnabled {
		t.Fatalf("rbac explicit config not normalized: %#v", cfg.Auth.RBAC)
	}
	if cfg.Admin.BootstrapTokenFile != "data/admin/bootstrap-token" {
		t.Fatalf("admin bootstrap token file not trimmed: %#v", cfg.Admin)
	}
	if !cfg.Admin.MembershipAPIEnabled {
		t.Fatalf("admin membership API gate should preserve explicit true: %#v", cfg.Admin)
	}
	if !cfg.Admin.MembershipWriteEnabled {
		t.Fatalf("admin membership write gate should preserve explicit true when API is enabled: %#v", cfg.Admin)
	}
	if cfg.RateLimit.RequestsPerMinute != DefaultRateLimitRequestsPerMinute || cfg.RateLimit.Burst != DefaultRateLimitBurst {
		t.Fatalf("rate limit defaults not filled when enabled: %#v", cfg.RateLimit)
	}
	if !cfg.Hardening.RequireLoopback {
		t.Fatalf("hardening require_loopback should preserve explicit true: %#v", cfg.Hardening)
	}
}

func TestConfigNormalizeDisablesMembershipWriteWhenAPIIsDisabled(t *testing.T) {
	cfg := Config{
		Admin: AdminConfig{
			MembershipAPIEnabled:   false,
			MembershipWriteEnabled: true,
		},
	}.Normalize()

	if cfg.Admin.MembershipWriteEnabled {
		t.Fatalf("membership write gate should be forced off when membership API is disabled: %#v", cfg.Admin)
	}
}
