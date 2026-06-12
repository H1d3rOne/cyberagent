package runtime

const (
	// AgentLoopActionServiceContractName identifies the runtime-native action
	// service capability set carried by AgentLoopToolServiceBinding. Concrete
	// daemon adapters still own the service implementations; runtime owns the
	// capability names and clone boundary.
	AgentLoopActionServiceContractName = "runtime_agent_loop_action_services"
)

// AgentLoopActionServiceCapability names one service capability needed by
// planner action execution and registry handlers. Values are intentionally
// stable strings so daemon adapters can bind concrete implementations without
// exposing daemon-private routerServices through AgentLoopToolServiceBinding.
type AgentLoopActionServiceCapability string

const (
	AgentLoopActionServiceVersion            AgentLoopActionServiceCapability = "version"
	AgentLoopActionServicePlugins            AgentLoopActionServiceCapability = "plugins"
	AgentLoopActionServiceRuns               AgentLoopActionServiceCapability = "runs"
	AgentLoopActionServiceApprovals          AgentLoopActionServiceCapability = "approvals"
	AgentLoopActionServiceJobs               AgentLoopActionServiceCapability = "jobs"
	AgentLoopActionServiceAgent              AgentLoopActionServiceCapability = "agent"
	AgentLoopActionServiceScanManager        AgentLoopActionServiceCapability = "scan_manager"
	AgentLoopActionServiceSchedules          AgentLoopActionServiceCapability = "schedules"
	AgentLoopActionServiceProviders          AgentLoopActionServiceCapability = "providers"
	AgentLoopActionServiceWorkspace          AgentLoopActionServiceCapability = "workspace"
	AgentLoopActionServiceNetworkRequest     AgentLoopActionServiceCapability = "network_request"
	AgentLoopActionServicePortScanner        AgentLoopActionServiceCapability = "port_scanner"
	AgentLoopActionServiceWebSearch          AgentLoopActionServiceCapability = "web_search"
	AgentLoopActionServiceWebScan            AgentLoopActionServiceCapability = "web_scan"
	AgentLoopActionServiceScheduleAction     AgentLoopActionServiceCapability = "schedule_action"
	AgentLoopActionServiceDaemonStatusAction AgentLoopActionServiceCapability = "daemon_status_action"
	AgentLoopActionServiceRunContextApproval AgentLoopActionServiceCapability = "run_context_approval_action"
)

// AgentLoopActionServiceSet is the runtime-owned capability boundary for
// action/tool services. Capability values are opaque references owned by the
// daemon adapter, but lookup is keyed by runtime-defined capability names
// instead of one opaque Handle bundle.
type AgentLoopActionServiceSet struct {
	Capabilities map[AgentLoopActionServiceCapability]any
	Metadata     map[string]any
}

// Clone returns a copy with capability/metadata maps isolated. Capability
// values are intentionally preserved by reference so service implementations
// keep their original identity.
func (set AgentLoopActionServiceSet) Clone() AgentLoopActionServiceSet {
	cloned := AgentLoopActionServiceSet{
		Capabilities: CloneAgentLoopActionServiceCapabilities(set.Capabilities),
		Metadata:     CloneAgentLoopMapStringAny(set.Metadata),
	}
	return cloned
}

// Get returns the opaque service reference for a runtime action capability.
func (set AgentLoopActionServiceSet) Get(capability AgentLoopActionServiceCapability) (any, bool) {
	if set.Capabilities == nil {
		return nil, false
	}
	service, ok := set.Capabilities[capability]
	return service, ok
}

// CloneAgentLoopActionServiceSet clones an action service capability set.
func CloneAgentLoopActionServiceSet(set AgentLoopActionServiceSet) AgentLoopActionServiceSet {
	return set.Clone()
}

// CloneAgentLoopActionServiceCapabilities clones a capability map while
// preserving concrete service references.
func CloneAgentLoopActionServiceCapabilities(capabilities map[AgentLoopActionServiceCapability]any) map[AgentLoopActionServiceCapability]any {
	if capabilities == nil {
		return nil
	}
	cloned := make(map[AgentLoopActionServiceCapability]any, len(capabilities))
	for capability, service := range capabilities {
		cloned[capability] = service
	}
	return cloned
}
