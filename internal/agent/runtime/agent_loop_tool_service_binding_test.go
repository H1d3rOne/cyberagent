package runtime

import "testing"

func TestAgentLoopToolServiceBindingCloneIsolatesMetadataAndPreservesHandle(t *testing.T) {
	handle := &struct{ Name string }{Name: "daemon-services"}
	jobs := &struct{ Name string }{Name: "jobs"}
	binding := AgentLoopToolServiceBinding{
		Kind:   "daemon_router_services",
		Handle: handle,
		ActionServices: AgentLoopActionServiceSet{
			Capabilities: map[AgentLoopActionServiceCapability]any{
				AgentLoopActionServiceJobs: jobs,
			},
			Metadata: map[string]any{
				"contract": AgentLoopActionServiceContractName,
			},
		},
		Metadata: map[string]any{
			"adapter": "daemon",
			"nested":  map[string]any{"shared": true},
		},
	}

	cloned := binding.Clone()
	binding.ActionServices.Capabilities[AgentLoopActionServiceJobs] = &struct{ Name string }{Name: "mutated-jobs"}
	binding.ActionServices.Metadata["contract"] = "mutated-contract"
	binding.Metadata["adapter"] = "mutated"
	binding.Metadata["new"] = true
	binding.Metadata["nested"].(map[string]any)["shared"] = false
	handle.Name = "mutated-services"
	jobs.Name = "mutated-jobs-ref"

	if cloned.Kind != "daemon_router_services" || cloned.Handle != handle {
		t.Fatalf("clone changed binding identity or handle: %#v", cloned)
	}
	if got, ok := cloned.ActionServices.Get(AgentLoopActionServiceJobs); !ok || got != jobs {
		t.Fatalf("clone did not preserve action service ref: %#v", cloned.ActionServices)
	}
	if cloned.ActionServices.Metadata["contract"] != AgentLoopActionServiceContractName {
		t.Fatalf("clone shares action service metadata: %#v", cloned.ActionServices.Metadata)
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
	if cloned.ActionServices.Capabilities[AgentLoopActionServiceJobs].(*struct{ Name string }).Name != "mutated-jobs-ref" {
		t.Fatalf("clone should preserve action service references: %#v", cloned.ActionServices.Capabilities)
	}
}
