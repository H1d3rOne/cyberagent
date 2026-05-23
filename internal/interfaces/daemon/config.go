package daemon

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

const (
	DefaultListen            = "127.0.0.1:8787"
	DefaultRunsDir           = "runs"
	DefaultMaxConcurrentJobs = 1
)

// Config controls the local daemon adapter.
type Config struct {
	Listen            string
	RunsDir           string
	TokenFile         string
	MaxConcurrentJobs int
	Build             services.BuildInfo
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
	return c
}
