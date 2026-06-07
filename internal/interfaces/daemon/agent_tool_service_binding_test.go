package daemon

import (
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/stretchr/testify/require"
)

func TestAgentLoopToolServiceBindingFromRouterServicesPreservesLegacyReferences(t *testing.T) {
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	svc := routerServices{jobs: jobs}

	binding := agentLoopToolServiceBindingFromRouterServices(svc)
	recovered, ok := routerServicesFromAgentLoopToolServiceBinding(binding)
	require.True(t, ok)
	require.Same(t, jobs, recovered.jobs)
	require.Equal(t, daemonToolServiceBindingKind, binding.Kind)
	require.Equal(t, "daemon_legacy_router_services", binding.Metadata["adapter"])

	cloned := binding.Clone()
	binding.Metadata["adapter"] = "mutated"
	clonedRecovered, ok := routerServicesFromAgentLoopToolServiceBinding(cloned)
	require.True(t, ok)
	require.Same(t, jobs, clonedRecovered.jobs)
	require.Equal(t, "daemon_legacy_router_services", cloned.Metadata["adapter"])
}

func TestRouterServicesFromAgentLoopToolServiceBindingSupportsPointerHandle(t *testing.T) {
	jobs := newJobManager(jobManagerConfig{RunsDir: t.TempDir()})
	svc := &routerServices{jobs: jobs}

	recovered, ok := routerServicesFromAgentLoopToolServiceBinding(runtime.AgentLoopToolServiceBinding{Handle: svc})
	require.True(t, ok)
	require.Same(t, jobs, recovered.jobs)
}
