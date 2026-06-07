package daemon

import "github.com/H1d3rOne/cyberagent/internal/agent/runtime"

const daemonToolServiceBindingKind = "daemon_router_services"

func agentLoopToolServiceBindingFromRouterServices(svc routerServices) runtime.AgentLoopToolServiceBinding {
	return runtime.AgentLoopToolServiceBinding{
		Kind:   daemonToolServiceBindingKind,
		Handle: svc,
		Metadata: map[string]any{
			"adapter": "daemon_legacy_router_services",
		},
	}
}

func routerServicesFromAgentLoopToolServiceBinding(binding runtime.AgentLoopToolServiceBinding) (routerServices, bool) {
	switch svc := binding.Handle.(type) {
	case routerServices:
		return svc, true
	case *routerServices:
		if svc == nil {
			return routerServices{}, false
		}
		return *svc, true
	default:
		return routerServices{}, false
	}
}

func legacyRouterServicesFromAgentLoopToolServiceBinding(binding runtime.AgentLoopToolServiceBinding) routerServices {
	svc, _ := routerServicesFromAgentLoopToolServiceBinding(binding)
	return svc
}
