package daemon

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

const (
	DefaultListen                     = "127.0.0.1:8787"
	DefaultRunsDir                    = "runs"
	DefaultMaxConcurrentJobs          = 1
	DefaultRateLimitRequestsPerMinute = 120
	DefaultRateLimitBurst             = 30
	DefaultWebSessionCookieName       = "cyberagent_session"
	DefaultWebSessionTTLSeconds       = 8 * 60 * 60
	defaultAuditEventsRelativePath    = ".daemon/audit/events.jsonl"
)

// Config controls the local daemon adapter.
type Config struct {
	Listen            string
	RunsDir           string
	TokenFile         string
	MaxConcurrentJobs int
	Build             services.BuildInfo
	MultiUser         MultiUserConfig
	Auth              AuthConfig
	Audit             AuditConfig
	Admin             AdminConfig
	RateLimit         RateLimitConfig
	Hardening         HardeningConfig
}

type MultiUserConfig struct {
	Enabled     bool
	StorageRoot string
}

type AuthConfig struct {
	WebSessions WebSessionAuthConfig
	RBAC        RBACConfig
}

type WebSessionAuthConfig struct {
	Enabled                  bool
	CookieName               string
	TTLSeconds               int
	StoragePath              string
	PrincipalResolverEnabled bool
}

type RBACConfig struct {
	Enabled            bool
	PolicyPath         string
	AuditEnabled       bool
	EnforcementEnabled bool
}

type AuditConfig struct {
	Enabled bool
	Path    string
}

type AdminConfig struct {
	BootstrapTokenFile   string
	MembershipAPIEnabled bool
}

type RateLimitConfig struct {
	Enabled           bool
	RequestsPerMinute int
	Burst             int
}

type HardeningConfig struct {
	RequireLoopback bool
}

// DefaultTokenFile returns the default local token path.
func DefaultTokenFile() string {
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "cyberagent", "token")
	}
	return filepath.Join(".cyberagent", "token")
}

// Normalize fills safe local defaults.
func (c Config) Normalize() Config {
	if strings.TrimSpace(c.Listen) == "" {
		c.Listen = DefaultListen
	}
	if strings.TrimSpace(c.RunsDir) == "" {
		c.RunsDir = DefaultRunsDir
	}
	if strings.TrimSpace(c.TokenFile) == "" {
		c.TokenFile = DefaultTokenFile()
	}
	if c.MaxConcurrentJobs <= 0 {
		c.MaxConcurrentJobs = DefaultMaxConcurrentJobs
	}
	c.MultiUser.StorageRoot = strings.TrimSpace(c.MultiUser.StorageRoot)
	c.Auth.WebSessions.CookieName = strings.TrimSpace(c.Auth.WebSessions.CookieName)
	c.Auth.WebSessions.StoragePath = strings.TrimSpace(c.Auth.WebSessions.StoragePath)
	if c.Auth.WebSessions.Enabled {
		if c.Auth.WebSessions.CookieName == "" {
			c.Auth.WebSessions.CookieName = DefaultWebSessionCookieName
		}
		if c.Auth.WebSessions.TTLSeconds <= 0 {
			c.Auth.WebSessions.TTLSeconds = DefaultWebSessionTTLSeconds
		}
	}
	c.Auth.RBAC.PolicyPath = strings.TrimSpace(c.Auth.RBAC.PolicyPath)
	c.Audit.Path = strings.TrimSpace(c.Audit.Path)
	if c.Audit.Enabled && c.Audit.Path == "" {
		c.Audit.Path = filepath.Join(c.RunsDir, filepath.FromSlash(defaultAuditEventsRelativePath))
	}
	c.Admin.BootstrapTokenFile = strings.TrimSpace(c.Admin.BootstrapTokenFile)
	if c.RateLimit.Enabled {
		if c.RateLimit.RequestsPerMinute <= 0 {
			c.RateLimit.RequestsPerMinute = DefaultRateLimitRequestsPerMinute
		}
		if c.RateLimit.Burst <= 0 {
			c.RateLimit.Burst = DefaultRateLimitBurst
		}
	}
	return c
}
