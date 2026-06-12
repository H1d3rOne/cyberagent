package runtime

import "testing"

func TestAgentLoopActionServiceSetCloneIsolatesMapsAndPreservesRefs(t *testing.T) {
	jobs := &struct{ ID string }{ID: "jobs-1"}
	set := AgentLoopActionServiceSet{
		Capabilities: map[AgentLoopActionServiceCapability]any{
			AgentLoopActionServiceJobs: jobs,
		},
		Metadata: map[string]any{
			"contract": AgentLoopActionServiceContractName,
		},
	}

	cloned := set.Clone()
	set.Capabilities[AgentLoopActionServiceJobs] = &struct{ ID string }{ID: "jobs-2"}
	set.Metadata["contract"] = "mutated"
	jobs.ID = "jobs-mutated-ref"

	got, ok := cloned.Get(AgentLoopActionServiceJobs)
	if !ok || got != jobs {
		t.Fatalf("clone Get(jobs) = %#v, %v; want original jobs ref", got, ok)
	}
	if cloned.Metadata["contract"] != AgentLoopActionServiceContractName {
		t.Fatalf("clone shares metadata map: %#v", cloned.Metadata)
	}
	if got.(*struct{ ID string }).ID != "jobs-mutated-ref" {
		t.Fatalf("clone should preserve concrete service reference: %#v", got)
	}
	if _, ok := cloned.Get("unsupported"); ok {
		t.Fatalf("clone unexpectedly found unsupported capability")
	}
}
