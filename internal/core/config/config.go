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
}

type AgentConfig struct {
	Provider     string       `yaml:"provider"`
	Model        string       `yaml:"model"`
	BaseURL      string       `yaml:"base_url"`
	MaxTurns     int          `yaml:"max_turns"`
	Instructions string       `yaml:"instructions"`
	CustomTools  []CustomTool `yaml:"custom_tools"`
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
