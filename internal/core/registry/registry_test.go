package registry

import (
	"testing"
)

func TestNewRegistryHasDefaults(t *testing.T) {
	r := New()
	tools := r.List()
	if len(tools) == 0 {
		t.Fatal("expected default tools to be registered")
	}
}

func TestRegistryGet(t *testing.T) {
	r := New()
	entry, ok := r.Get("builtin.http_probe")
	if !ok {
		t.Fatal("expected builtin.http_probe to exist")
	}
	if entry.Type != ToolTypeBuiltin {
		t.Errorf("expected type builtin, got %s", entry.Type)
	}
	if entry.Risk != RiskLow {
		t.Errorf("expected risk low, got %s", entry.Risk)
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := New()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent tool to not be found")
	}
}

func TestRegistryRegisterCustom(t *testing.T) {
	r := New()
	r.Register(ToolEntry{
		Name:         "plugin.nuclei-safe",
		Type:         ToolTypePlugin,
		Risk:         RiskLow,
		Description:  "Nuclei safe templates",
		Capabilities: []string{"scan", "nuclei"},
		Enabled:      false,
	})
	entry, ok := r.Get("plugin.nuclei-safe")
	if !ok {
		t.Fatal("expected plugin.nuclei-safe to exist after registration")
	}
	if entry.Enabled {
		t.Error("expected plugin to be disabled by default")
	}
}

func TestRegistryEnableDisable(t *testing.T) {
	r := New()
	r.Register(ToolEntry{Name: "plugin.test", Type: ToolTypePlugin, Enabled: false})

	if !r.Enable("plugin.test") {
		t.Fatal("enable should return true")
	}
	entry, _ := r.Get("plugin.test")
	if !entry.Enabled {
		t.Error("expected tool to be enabled")
	}

	if !r.Disable("plugin.test") {
		t.Fatal("disable should return true")
	}
	entry, _ = r.Get("plugin.test")
	if entry.Enabled {
		t.Error("expected tool to be disabled")
	}
}

func TestRegistryEnableNonexistent(t *testing.T) {
	r := New()
	if r.Enable("nonexistent") {
		t.Error("enable nonexistent should return false")
	}
}

func TestRegistryListByType(t *testing.T) {
	r := New()
	r.Register(ToolEntry{Name: "plugin.a", Type: ToolTypePlugin, Enabled: true})
	r.Register(ToolEntry{Name: "plugin.b", Type: ToolTypePlugin, Enabled: true})

	plugins := r.ListByType(ToolTypePlugin)
	if len(plugins) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(plugins))
	}

	builtins := r.ListByType(ToolTypeBuiltin)
	if len(builtins) == 0 {
		t.Error("expected builtins to exist")
	}
}

func TestRegistryListEnabled(t *testing.T) {
	r := New()
	r.Register(ToolEntry{Name: "plugin.disabled", Type: ToolTypePlugin, Enabled: false})

	enabled := r.ListEnabled()
	for _, e := range enabled {
		if e.Name == "plugin.disabled" {
			t.Error("disabled plugin should not appear in enabled list")
		}
	}
}

func TestRegistryHas(t *testing.T) {
	r := New()
	if !r.Has("builtin.http_probe") {
		t.Error("expected Has to return true for existing tool")
	}
	if r.Has("nonexistent") {
		t.Error("expected Has to return false for missing tool")
	}
}
