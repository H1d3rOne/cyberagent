package prompt

import (
	"strings"
	"testing"
)

func TestBuildSystemContainsTarget(t *testing.T) {
	result := BuildSystem(BuilderConfig{
		Target: "https://example.com",
		Scope:  []string{"example.com"},
	})
	if !strings.Contains(result, "https://example.com") {
		t.Error("system prompt should contain target")
	}
}

func TestBuildSystemContainsScope(t *testing.T) {
	result := BuildSystem(BuilderConfig{
		Target: "https://example.com",
		Scope:  []string{"example.com", "api.example.com"},
	})
	if !strings.Contains(result, "- example.com") {
		t.Error("system prompt should list scope entries")
	}
	if !strings.Contains(result, "- api.example.com") {
		t.Error("system prompt should list all scope entries")
	}
}

func TestBuildSystemContainsInstructions(t *testing.T) {
	result := BuildSystem(BuilderConfig{
		Target:       "https://example.com",
		Instructions: "Focus on authentication issues",
	})
	if !strings.Contains(result, "Focus on authentication issues") {
		t.Error("system prompt should contain custom instructions")
	}
}

func TestBuildSystemContainsTools(t *testing.T) {
	result := BuildSystem(BuilderConfig{
		Target:    "https://example.com",
		ToolNames: []string{"http_probe", "check_headers"},
	})
	if !strings.Contains(result, "- http_probe") {
		t.Error("system prompt should list tools")
	}
	if !strings.Contains(result, "- check_headers") {
		t.Error("system prompt should list all tools")
	}
}

func TestBuildSystemContainsConstraints(t *testing.T) {
	result := BuildSystem(BuilderConfig{
		Target: "https://example.com",
	})
	if !strings.Contains(result, "NEVER attempt to exploit") {
		t.Error("system prompt should contain constraints")
	}
	if !strings.Contains(result, "NEVER access targets outside") {
		t.Error("system prompt should contain scope constraint")
	}
}

func TestBuildSystemMinimalConfig(t *testing.T) {
	result := BuildSystem(BuilderConfig{
		Target: "http://test.local",
	})
	if !strings.Contains(result, "# Role") {
		t.Error("system prompt should contain role block")
	}
	if !strings.Contains(result, "## Execution Model") {
		t.Error("system prompt should contain execution model block")
	}
	if !strings.Contains(result, "## PLAN") {
		t.Error("system prompt should contain PLAN phase")
	}
	if !strings.Contains(result, "## STEP") {
		t.Error("system prompt should contain STEP phase")
	}
	if !strings.Contains(result, "## STEPREPLAN") {
		t.Error("system prompt should contain STEPREPLAN phase")
	}
	if !strings.Contains(result, "## FINAL_ANSWER") {
		t.Error("system prompt should contain FINAL_ANSWER phase")
	}
	if strings.Contains(result, "## Additional Instructions") {
		t.Error("should not contain instructions section when empty")
	}
	if strings.Contains(result, "## Available Tools") {
		t.Error("should not contain tools section when empty")
	}
}
