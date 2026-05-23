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
