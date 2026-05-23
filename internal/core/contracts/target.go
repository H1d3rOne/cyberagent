package contracts

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TargetType identifies the target input kind.
type TargetType string

const (
	TargetTypeWebURL TargetType = "web_url"
)

// Target describes what the user wants to analyze. Authorization boundaries live in Scope.
type Target struct {
	ID         string           `json:"id"`
	Type       TargetType       `json:"type"`
	Value      string           `json:"value"`
	Normalized TargetNormalized `json:"normalized"`
	Labels     []string         `json:"labels,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
	Metadata   Metadata         `json:"metadata,omitempty"`
}

// TargetNormalized is the canonical representation of a target.
type TargetNormalized struct {
	Scheme  string `json:"scheme"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	BaseURL string `json:"base_url"`
	Path    string `json:"path"`
}

// NewWebTarget creates and normalizes a web_url target.
func NewWebTarget(id, raw string) (Target, error) {
	n, err := NormalizeWebURL(raw)
	if err != nil {
		return Target{}, err
	}
	return Target{
		ID:         id,
		Type:       TargetTypeWebURL,
		Value:      raw,
		Normalized: n,
		CreatedAt:  Now(),
		Metadata:   Metadata{},
	}, nil
}

// NormalizeWebURL normalizes a HTTP(S) URL into TargetNormalized.
func NormalizeWebURL(raw string) (TargetNormalized, error) {
	if strings.TrimSpace(raw) == "" {
		return TargetNormalized{}, fmt.Errorf("target URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return TargetNormalized{}, fmt.Errorf("parse target URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return TargetNormalized{}, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return TargetNormalized{}, fmt.Errorf("target URL host is empty")
	}
	port := defaultPort(u.Scheme)
	if p := u.Port(); p != "" {
		_, scanErr := fmt.Sscanf(p, "%d", &port)
		if scanErr != nil || port <= 0 || port > 65535 {
			return TargetNormalized{}, fmt.Errorf("invalid port %q", p)
		}
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	hostPort := u.Hostname()
	if !(u.Scheme == "http" && port == 80) && !(u.Scheme == "https" && port == 443) {
		hostPort = fmt.Sprintf("%s:%d", u.Hostname(), port)
	}
	return TargetNormalized{
		Scheme:  u.Scheme,
		Host:    u.Hostname(),
		Port:    port,
		BaseURL: fmt.Sprintf("%s://%s", u.Scheme, hostPort),
		Path:    path,
	}, nil
}

func defaultPort(scheme string) int {
	if scheme == "https" {
		return 443
	}
	return 80
}
