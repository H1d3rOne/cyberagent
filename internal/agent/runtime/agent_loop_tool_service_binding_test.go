package runtime

import "testing"

func TestAgentLoopToolServiceBindingCloneIsolatesMetadataAndPreservesHandle(t *testing.T) {
	handle := &struct{ Name string }{Name: "daemon-services"}
	binding := AgentLoopToolServiceBinding{
		Kind:   "daemon_router_services",
		Handle: handle,
		Metadata: map[string]any{
			"adapter": "daemon",
			"nested":  map[string]any{"shared": true},
		},
	}

	cloned := binding.Clone()
	binding.Metadata["adapter"] = "mutated"
	binding.Metadata["new"] = true
	binding.Metadata["nested"].(map[string]any)["shared"] = false
	handle.Name = "mutated-services"

	if cloned.Kind != "daemon_router_services" || cloned.Handle != handle {
		t.Fatalf("clone changed binding identity or handle: %#v", cloned)
	}
	if cloned.Metadata["adapter"] != "daemon" {
		t.Fatalf("clone shares top-level metadata: %#v", cloned.Metadata)
	}
	if _, ok := cloned.Metadata["new"]; ok {
		t.Fatalf("clone unexpectedly saw new metadata key: %#v", cloned.Metadata)
	}
	if cloned.Metadata["nested"].(map[string]any)["shared"] != false {
		t.Fatalf("expected shallow metadata clone to preserve nested map sharing: %#v", cloned.Metadata)
	}
	if cloned.Handle.(*struct{ Name string }).Name != "mutated-services" {
		t.Fatalf("clone should preserve opaque handle reference: %#v", cloned.Handle)
	}
}
