package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	ProjectFile = ".cyberagent.yaml"
	UserDir     = ".cyberagent"
	UserFile    = "config.yaml"
)

type Config struct {
	Profiles map[string]Profile `yaml:"profiles"`
	Plugins  PluginsConfig      `yaml:"plugins"`
	Defaults Defaults           `yaml:"defaults"`
	Agent    AgentConfig        `yaml:"agent"`
	Daemon   DaemonConfig       `yaml:"daemon"`
}

type AgentConfig struct {
	Provider     string       `yaml:"provider"`
	Model        string       `yaml:"model"`
	BaseURL      string       `yaml:"base_url"`
	MaxTurns     int          `yaml:"max_turns"`
	Instructions string       `yaml:"instructions"`
	CustomTools  []CustomTool `yaml:"custom_tools"`
}

type DaemonConfig struct {
	Listen            string                `yaml:"listen"`
	RunsDir           string                `yaml:"runs_dir"`
	TokenFile         string                `yaml:"token_file"`
	MaxConcurrentJobs int                   `yaml:"max_concurrent_jobs"`
	MultiUser         DaemonMultiUserConfig `yaml:"multi_user"`
	Auth              DaemonAuthConfig      `yaml:"auth"`
	Audit             DaemonAuditConfig     `yaml:"audit"`
	Admin             DaemonAdminConfig     `yaml:"admin"`
	RateLimit         DaemonRateLimitConfig `yaml:"rate_limit"`
	Hardening         DaemonHardeningConfig `yaml:"hardening"`
}

type DaemonMultiUserConfig struct {
	Enabled     *bool  `yaml:"enabled"`
	StorageRoot string `yaml:"storage_root"`
}

type DaemonAuthConfig struct {
	WebSessions DaemonWebSessionAuthConfig `yaml:"web_sessions"`
	RBAC        DaemonRBACConfig           `yaml:"rbac"`
}

type DaemonWebSessionAuthConfig struct {
	Enabled                  *bool  `yaml:"enabled"`
	CookieName               string `yaml:"cookie_name"`
	TTLSeconds               int    `yaml:"ttl_seconds"`
	StoragePath              string `yaml:"storage_path"`
	PrincipalResolverEnabled *bool  `yaml:"principal_resolver_enabled"`
}

type DaemonRBACConfig struct {
	Enabled            *bool  `yaml:"enabled"`
	PolicyPath         string `yaml:"policy_path"`
	AuditEnabled       *bool  `yaml:"audit_enabled"`
	EnforcementEnabled *bool  `yaml:"enforcement_enabled"`
}

type DaemonAuditConfig struct {
	Enabled *bool  `yaml:"enabled"`
	Path    string `yaml:"path"`
}

type DaemonAdminConfig struct {
	BootstrapTokenFile     string `yaml:"bootstrap_token_file"`
	MembershipAPIEnabled   *bool  `yaml:"membership_api_enabled"`
	MembershipWriteEnabled *bool  `yaml:"membership_write_enabled"`
}

type DaemonRateLimitConfig struct {
	Enabled           *bool `yaml:"enabled"`
	RequestsPerMinute int   `yaml:"requests_per_minute"`
	Burst             int   `yaml:"burst"`
}

type DaemonHardeningConfig struct {
	RequireLoopback *bool `yaml:"require_loopback"`
}

type CustomTool struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Command     string            `yaml:"command"`
	Args        []string          `yaml:"args"`
	Env         map[string]string `yaml:"env"`
	Timeout     int               `yaml:"timeout"`
	InputSchema string            `yaml:"input_schema"`
}

type Profile struct {
	Name           string   `yaml:"name"`
	RiskLevel      string   `yaml:"risk_level"`
	EnabledPlugins []string `yaml:"enabled_plugins"`
	DisabledSteps  []string `yaml:"disabled_steps"`
	Timeouts       Timeouts `yaml:"timeouts"`
	RateLimit      RateConf `yaml:"rate_limit"`
}

type Timeouts struct {
	StepDefault int `yaml:"step_default"`
	HTTPProbe   int `yaml:"http_probe"`
	TLSInspect  int `yaml:"tls_inspect"`
	PortCheck   int `yaml:"port_check"`
}

type RateConf struct {
	GlobalRPS   float64 `yaml:"global_rps"`
	PerHostRPS  float64 `yaml:"per_host_rps"`
	Concurrency int     `yaml:"concurrency"`
}

type PluginsConfig struct {
	Dir     string   `yaml:"dir"`
	Enabled []string `yaml:"enabled"`
}

type Defaults struct {
	Profile string `yaml:"profile"`
	Scope   string `yaml:"scope"`
	OutDir  string `yaml:"out_dir"`
}

func Default() Config {
	return Config{
		Profiles: map[string]Profile{
			"safe": {
				Name:      "safe",
				RiskLevel: "low",
				Timeouts:  Timeouts{StepDefault: 30, HTTPProbe: 30, TLSInspect: 15, PortCheck: 30},
				RateLimit: RateConf{GlobalRPS: 20, PerHostRPS: 10, Concurrency: 10},
			},
		},
		Plugins: PluginsConfig{Dir: "plugins"},
		Defaults: Defaults{
			Profile: "safe",
			OutDir:  "runs",
		},
	}
}

// Load reads config from project file, user file, and merges them.
// Project config takes precedence over user config.
func Load() (Config, error) {
	cfg := Default()

	userCfg, err := loadUserConfig()
	if err != nil {
		return Config{}, fmt.Errorf("load user config: %w", err)
	}
	cfg = merge(cfg, userCfg)

	projectCfg, err := loadProjectConfig()
	if err != nil {
		return Config{}, fmt.Errorf("load project config: %w", err)
	}
	cfg = merge(cfg, projectCfg)

	return cfg, nil
}

// LoadFrom reads config from a specific file path.
func LoadFrom(path string) (Config, error) {
	cfg := Default()
	fileCfg, err := loadFile(path)
	if err != nil {
		return Config{}, err
	}
	return merge(cfg, fileCfg), nil
}

func loadUserConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, nil
	}
	path := filepath.Join(home, UserDir, UserFile)
	return loadFile(path)
}

func loadProjectConfig() (Config, error) {
	return loadFile(ProjectFile)
}

func loadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func merge(base, overlay Config) Config {
	if overlay.Defaults.Profile != "" {
		base.Defaults.Profile = overlay.Defaults.Profile
	}
	if overlay.Defaults.Scope != "" {
		base.Defaults.Scope = overlay.Defaults.Scope
	}
	if overlay.Defaults.OutDir != "" {
		base.Defaults.OutDir = overlay.Defaults.OutDir
	}
	if overlay.Plugins.Dir != "" {
		base.Plugins.Dir = overlay.Plugins.Dir
	}
	if len(overlay.Plugins.Enabled) > 0 {
		base.Plugins.Enabled = overlay.Plugins.Enabled
	}
	if overlay.Daemon.Listen != "" {
		base.Daemon.Listen = overlay.Daemon.Listen
	}
	if overlay.Daemon.RunsDir != "" {
		base.Daemon.RunsDir = overlay.Daemon.RunsDir
	}
	if overlay.Daemon.TokenFile != "" {
		base.Daemon.TokenFile = overlay.Daemon.TokenFile
	}
	if overlay.Daemon.MaxConcurrentJobs > 0 {
		base.Daemon.MaxConcurrentJobs = overlay.Daemon.MaxConcurrentJobs
	}
	if overlay.Daemon.MultiUser.Enabled != nil {
		enabled := *overlay.Daemon.MultiUser.Enabled
		base.Daemon.MultiUser.Enabled = &enabled
	}
	if overlay.Daemon.MultiUser.StorageRoot != "" {
		base.Daemon.MultiUser.StorageRoot = overlay.Daemon.MultiUser.StorageRoot
	}
	if overlay.Daemon.Auth.WebSessions.Enabled != nil {
		enabled := *overlay.Daemon.Auth.WebSessions.Enabled
		base.Daemon.Auth.WebSessions.Enabled = &enabled
	}
	if overlay.Daemon.Auth.WebSessions.CookieName != "" {
		base.Daemon.Auth.WebSessions.CookieName = overlay.Daemon.Auth.WebSessions.CookieName
	}
	if overlay.Daemon.Auth.WebSessions.TTLSeconds > 0 {
		base.Daemon.Auth.WebSessions.TTLSeconds = overlay.Daemon.Auth.WebSessions.TTLSeconds
	}
	if overlay.Daemon.Auth.WebSessions.StoragePath != "" {
		base.Daemon.Auth.WebSessions.StoragePath = overlay.Daemon.Auth.WebSessions.StoragePath
	}
	if overlay.Daemon.Auth.WebSessions.PrincipalResolverEnabled != nil {
		principalResolverEnabled := *overlay.Daemon.Auth.WebSessions.PrincipalResolverEnabled
		base.Daemon.Auth.WebSessions.PrincipalResolverEnabled = &principalResolverEnabled
	}
	if overlay.Daemon.Auth.RBAC.Enabled != nil {
		enabled := *overlay.Daemon.Auth.RBAC.Enabled
		base.Daemon.Auth.RBAC.Enabled = &enabled
	}
	if overlay.Daemon.Auth.RBAC.PolicyPath != "" {
		base.Daemon.Auth.RBAC.PolicyPath = overlay.Daemon.Auth.RBAC.PolicyPath
	}
	if overlay.Daemon.Auth.RBAC.AuditEnabled != nil {
		auditEnabled := *overlay.Daemon.Auth.RBAC.AuditEnabled
		base.Daemon.Auth.RBAC.AuditEnabled = &auditEnabled
	}
	if overlay.Daemon.Auth.RBAC.EnforcementEnabled != nil {
		enforcementEnabled := *overlay.Daemon.Auth.RBAC.EnforcementEnabled
		base.Daemon.Auth.RBAC.EnforcementEnabled = &enforcementEnabled
	}
	if overlay.Daemon.Audit.Enabled != nil {
		enabled := *overlay.Daemon.Audit.Enabled
		base.Daemon.Audit.Enabled = &enabled
	}
	if overlay.Daemon.Audit.Path != "" {
		base.Daemon.Audit.Path = overlay.Daemon.Audit.Path
	}
	if overlay.Daemon.Admin.BootstrapTokenFile != "" {
		base.Daemon.Admin.BootstrapTokenFile = overlay.Daemon.Admin.BootstrapTokenFile
	}
	if overlay.Daemon.Admin.MembershipAPIEnabled != nil {
		membershipAPIEnabled := *overlay.Daemon.Admin.MembershipAPIEnabled
		base.Daemon.Admin.MembershipAPIEnabled = &membershipAPIEnabled
	}
	if overlay.Daemon.Admin.MembershipWriteEnabled != nil {
		membershipWriteEnabled := *overlay.Daemon.Admin.MembershipWriteEnabled
		base.Daemon.Admin.MembershipWriteEnabled = &membershipWriteEnabled
	}
	if overlay.Daemon.RateLimit.Enabled != nil {
		enabled := *overlay.Daemon.RateLimit.Enabled
		base.Daemon.RateLimit.Enabled = &enabled
	}
	if overlay.Daemon.RateLimit.RequestsPerMinute > 0 {
		base.Daemon.RateLimit.RequestsPerMinute = overlay.Daemon.RateLimit.RequestsPerMinute
	}
	if overlay.Daemon.RateLimit.Burst > 0 {
		base.Daemon.RateLimit.Burst = overlay.Daemon.RateLimit.Burst
	}
	if overlay.Daemon.Hardening.RequireLoopback != nil {
		requireLoopback := *overlay.Daemon.Hardening.RequireLoopback
		base.Daemon.Hardening.RequireLoopback = &requireLoopback
	}
	if overlay.Profiles != nil {
		if base.Profiles == nil {
			base.Profiles = make(map[string]Profile)
		}
		for k, v := range overlay.Profiles {
			base.Profiles[k] = v
		}
	}
	return base
}

// GetProfile returns the named profile or an error if not found.
func (c Config) GetProfile(name string) (Profile, error) {
	if name == "" {
		name = c.Defaults.Profile
	}
	p, ok := c.Profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("profile %q not found in config", name)
	}
	return p, nil
}
