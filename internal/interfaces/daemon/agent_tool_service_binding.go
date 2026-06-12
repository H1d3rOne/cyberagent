package daemon

import (
	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

const daemonToolServiceBindingKind = "daemon_router_services"

type daemonAgentLoopActionServiceResolver struct {
	services runtime.AgentLoopActionServiceSet
}

func daemonActionServiceSetFromRouterServices(svc routerServices) runtime.AgentLoopActionServiceSet {
	return runtime.AgentLoopActionServiceSet{
		Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
			runtime.AgentLoopActionServiceVersion:     svc.version,
			runtime.AgentLoopActionServicePlugins:     svc.plugins,
			runtime.AgentLoopActionServiceRuns:        svc.runs,
			runtime.AgentLoopActionServiceApprovals:   svc.approvals,
			runtime.AgentLoopActionServiceJobs:        svc.jobs,
			runtime.AgentLoopActionServiceAgent:       svc.agent,
			runtime.AgentLoopActionServiceScanManager: svc.scanManager,
			runtime.AgentLoopActionServiceSchedules:   svc.schedules,
			runtime.AgentLoopActionServiceProviders:   svc.providers,
		},
		Metadata: map[string]any{
			"contract":          runtime.AgentLoopActionServiceContractName,
			"adapter":           "daemon_router_services_to_action_services",
			"compatibility_ref": "routerServices",
		},
	}
}

func agentLoopActionServiceCapabilityValue[T any](set runtime.AgentLoopActionServiceSet, capability runtime.AgentLoopActionServiceCapability) (T, bool) {
	var zero T
	service, ok := set.Get(capability)
	if !ok {
		return zero, false
	}
	typed, ok := service.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

func (resolver daemonAgentLoopActionServiceResolver) VersionService() (services.VersionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[services.VersionService](resolver.services, runtime.AgentLoopActionServiceVersion); ok {
		return service, true
	}
	return services.VersionService{}, false
}

func (resolver daemonAgentLoopActionServiceResolver) PluginService() (services.PluginService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[services.PluginService](resolver.services, runtime.AgentLoopActionServicePlugins); ok {
		return service, true
	}
	return services.PluginService{}, false
}

func (resolver daemonAgentLoopActionServiceResolver) RunService() (services.RunService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[services.RunService](resolver.services, runtime.AgentLoopActionServiceRuns); ok {
		return service, true
	}
	return services.RunService{}, false
}

func (resolver daemonAgentLoopActionServiceResolver) ApprovalService() (services.ApprovalService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[services.ApprovalService](resolver.services, runtime.AgentLoopActionServiceApprovals); ok {
		return service, true
	}
	return services.ApprovalService{}, false
}

func (resolver daemonAgentLoopActionServiceResolver) JobManager() (*jobManager, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[*jobManager](resolver.services, runtime.AgentLoopActionServiceJobs); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) ScheduleManager() (*scheduleManager, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[*scheduleManager](resolver.services, runtime.AgentLoopActionServiceSchedules); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) ProviderStore() (web.ProviderStore, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[web.ProviderStore](resolver.services, runtime.AgentLoopActionServiceProviders); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) Workspace() (daemonWorkspaceActionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[daemonWorkspaceActionService](resolver.services, runtime.AgentLoopActionServiceWorkspace); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) NetworkRequest() (daemonNetworkRequestActionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[daemonNetworkRequestActionService](resolver.services, runtime.AgentLoopActionServiceNetworkRequest); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) PortScanner() (daemonPortScannerActionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[daemonPortScannerActionService](resolver.services, runtime.AgentLoopActionServicePortScanner); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) WebSearch() (daemonWebSearchActionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[daemonWebSearchActionService](resolver.services, runtime.AgentLoopActionServiceWebSearch); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) WebScan() (daemonWebScanActionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[daemonWebScanActionService](resolver.services, runtime.AgentLoopActionServiceWebScan); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) ScheduleAction() (daemonScheduleActionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[daemonScheduleActionService](resolver.services, runtime.AgentLoopActionServiceScheduleAction); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) DaemonStatus() (daemonStatusActionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[daemonStatusActionService](resolver.services, runtime.AgentLoopActionServiceDaemonStatusAction); ok {
		return service, service != nil
	}
	return nil, false
}

func (resolver daemonAgentLoopActionServiceResolver) RunContextApproval() (daemonRunContextApprovalActionService, bool) {
	if service, ok := agentLoopActionServiceCapabilityValue[daemonRunContextApprovalActionService](resolver.services, runtime.AgentLoopActionServiceRunContextApproval); ok {
		return service, service != nil
	}
	return nil, false
}

func agentLoopToolServiceBindingFromRouterServices(svc routerServices) runtime.AgentLoopToolServiceBinding {
	return runtime.AgentLoopToolServiceBinding{
		Kind:           daemonToolServiceBindingKind,
		ActionServices: daemonActionServiceSetFromRouterServices(svc),
		Metadata: map[string]any{
			"adapter":                 "daemon_legacy_router_services",
			"action_service_contract": runtime.AgentLoopActionServiceContractName,
			"service_surface":         "runtime_action_services",
		},
	}
}

func agentLoopToolServiceBindingWithWorkspaceActionService(binding runtime.AgentLoopToolServiceBinding, workspaceService daemonWorkspaceActionService) runtime.AgentLoopToolServiceBinding {
	binding = binding.Clone()
	if binding.ActionServices.Capabilities == nil {
		binding.ActionServices.Capabilities = map[runtime.AgentLoopActionServiceCapability]any{}
	}
	if _, exists := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWorkspace]; !exists && workspaceService != nil {
		binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWorkspace] = workspaceService
	}
	if binding.ActionServices.Metadata == nil {
		binding.ActionServices.Metadata = map[string]any{}
	}
	binding.ActionServices.Metadata["workspace_capability"] = string(runtime.AgentLoopActionServiceWorkspace)
	binding.ActionServices.Metadata["workspace_adapter"] = "daemon_workspace_action_service"
	if binding.Metadata == nil {
		binding.Metadata = map[string]any{}
	}
	binding.Metadata["workspace_action_service"] = "runtime_capability"
	return binding.Clone()
}

func agentLoopToolServiceBindingWithNetworkRequestActionService(binding runtime.AgentLoopToolServiceBinding, networkService daemonNetworkRequestActionService) runtime.AgentLoopToolServiceBinding {
	binding = binding.Clone()
	if binding.ActionServices.Capabilities == nil {
		binding.ActionServices.Capabilities = map[runtime.AgentLoopActionServiceCapability]any{}
	}
	if _, exists := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceNetworkRequest]; !exists && networkService != nil {
		binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceNetworkRequest] = networkService
	}
	if binding.ActionServices.Metadata == nil {
		binding.ActionServices.Metadata = map[string]any{}
	}
	binding.ActionServices.Metadata["network_request_capability"] = string(runtime.AgentLoopActionServiceNetworkRequest)
	binding.ActionServices.Metadata["network_request_adapter"] = "daemon_network_request_action_service"
	if binding.Metadata == nil {
		binding.Metadata = map[string]any{}
	}
	binding.Metadata["network_request_action_service"] = "runtime_capability"
	return binding.Clone()
}

func agentLoopToolServiceBindingWithPortScannerActionService(binding runtime.AgentLoopToolServiceBinding, portScannerService daemonPortScannerActionService) runtime.AgentLoopToolServiceBinding {
	binding = binding.Clone()
	if binding.ActionServices.Capabilities == nil {
		binding.ActionServices.Capabilities = map[runtime.AgentLoopActionServiceCapability]any{}
	}
	if _, exists := binding.ActionServices.Capabilities[runtime.AgentLoopActionServicePortScanner]; !exists && portScannerService != nil {
		binding.ActionServices.Capabilities[runtime.AgentLoopActionServicePortScanner] = portScannerService
	}
	if binding.ActionServices.Metadata == nil {
		binding.ActionServices.Metadata = map[string]any{}
	}
	binding.ActionServices.Metadata["port_scanner_capability"] = string(runtime.AgentLoopActionServicePortScanner)
	binding.ActionServices.Metadata["port_scanner_adapter"] = "daemon_port_scanner_action_service"
	if binding.Metadata == nil {
		binding.Metadata = map[string]any{}
	}
	binding.Metadata["port_scanner_action_service"] = "runtime_capability"
	return binding.Clone()
}

func agentLoopToolServiceBindingWithWebSearchActionService(binding runtime.AgentLoopToolServiceBinding, webSearchService daemonWebSearchActionService) runtime.AgentLoopToolServiceBinding {
	binding = binding.Clone()
	if binding.ActionServices.Capabilities == nil {
		binding.ActionServices.Capabilities = map[runtime.AgentLoopActionServiceCapability]any{}
	}
	if _, exists := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWebSearch]; !exists && webSearchService != nil {
		binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWebSearch] = webSearchService
	}
	if binding.ActionServices.Metadata == nil {
		binding.ActionServices.Metadata = map[string]any{}
	}
	binding.ActionServices.Metadata["web_search_capability"] = string(runtime.AgentLoopActionServiceWebSearch)
	binding.ActionServices.Metadata["web_search_adapter"] = "daemon_web_search_action_service"
	if binding.Metadata == nil {
		binding.Metadata = map[string]any{}
	}
	binding.Metadata["web_search_action_service"] = "runtime_capability"
	return binding.Clone()
}

func agentLoopToolServiceBindingWithWebScanActionService(binding runtime.AgentLoopToolServiceBinding, webScanService daemonWebScanActionService) runtime.AgentLoopToolServiceBinding {
	binding = binding.Clone()
	if binding.ActionServices.Capabilities == nil {
		binding.ActionServices.Capabilities = map[runtime.AgentLoopActionServiceCapability]any{}
	}
	if _, exists := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWebScan]; !exists && webScanService != nil {
		binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWebScan] = webScanService
	}
	if binding.ActionServices.Metadata == nil {
		binding.ActionServices.Metadata = map[string]any{}
	}
	binding.ActionServices.Metadata["web_scan_capability"] = string(runtime.AgentLoopActionServiceWebScan)
	binding.ActionServices.Metadata["web_scan_adapter"] = "daemon_web_scan_action_service"
	if binding.Metadata == nil {
		binding.Metadata = map[string]any{}
	}
	binding.Metadata["web_scan_action_service"] = "runtime_capability"
	return binding.Clone()
}

func agentLoopToolServiceBindingWithScheduleActionService(binding runtime.AgentLoopToolServiceBinding, scheduleService daemonScheduleActionService) runtime.AgentLoopToolServiceBinding {
	binding = binding.Clone()
	if binding.ActionServices.Capabilities == nil {
		binding.ActionServices.Capabilities = map[runtime.AgentLoopActionServiceCapability]any{}
	}
	if _, exists := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceScheduleAction]; !exists && scheduleService != nil {
		binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceScheduleAction] = scheduleService
	}
	if binding.ActionServices.Metadata == nil {
		binding.ActionServices.Metadata = map[string]any{}
	}
	binding.ActionServices.Metadata["schedule_action_capability"] = string(runtime.AgentLoopActionServiceScheduleAction)
	binding.ActionServices.Metadata["schedule_action_adapter"] = "daemon_schedule_action_service"
	if binding.Metadata == nil {
		binding.Metadata = map[string]any{}
	}
	binding.Metadata["schedule_action_service"] = "runtime_capability"
	return binding.Clone()
}

func agentLoopToolServiceBindingWithDaemonStatusActionService(binding runtime.AgentLoopToolServiceBinding, statusService daemonStatusActionService) runtime.AgentLoopToolServiceBinding {
	binding = binding.Clone()
	if binding.ActionServices.Capabilities == nil {
		binding.ActionServices.Capabilities = map[runtime.AgentLoopActionServiceCapability]any{}
	}
	if _, exists := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceDaemonStatusAction]; !exists && statusService != nil {
		binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceDaemonStatusAction] = statusService
	}
	if binding.ActionServices.Metadata == nil {
		binding.ActionServices.Metadata = map[string]any{}
	}
	binding.ActionServices.Metadata["daemon_status_action_capability"] = string(runtime.AgentLoopActionServiceDaemonStatusAction)
	binding.ActionServices.Metadata["daemon_status_action_adapter"] = "daemon_status_action_service"
	if binding.Metadata == nil {
		binding.Metadata = map[string]any{}
	}
	binding.Metadata["daemon_status_action_service"] = "runtime_capability"
	return binding.Clone()
}

func agentLoopToolServiceBindingWithRunContextApprovalActionService(binding runtime.AgentLoopToolServiceBinding, runContextApprovalService daemonRunContextApprovalActionService) runtime.AgentLoopToolServiceBinding {
	binding = binding.Clone()
	if binding.ActionServices.Capabilities == nil {
		binding.ActionServices.Capabilities = map[runtime.AgentLoopActionServiceCapability]any{}
	}
	if _, exists := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceRunContextApproval]; !exists && runContextApprovalService != nil {
		binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceRunContextApproval] = runContextApprovalService
	}
	if binding.ActionServices.Metadata == nil {
		binding.ActionServices.Metadata = map[string]any{}
	}
	binding.ActionServices.Metadata["run_context_approval_action_capability"] = string(runtime.AgentLoopActionServiceRunContextApproval)
	binding.ActionServices.Metadata["run_context_approval_action_adapter"] = "daemon_run_context_approval_action_service"
	if binding.Metadata == nil {
		binding.Metadata = map[string]any{}
	}
	binding.Metadata["run_context_approval_action_service"] = "runtime_capability"
	return binding.Clone()
}

func agentLoopActionServiceResolverFromActionServices(set runtime.AgentLoopActionServiceSet) (daemonAgentLoopActionServiceResolver, bool) {
	if len(set.Capabilities) == 0 {
		return daemonAgentLoopActionServiceResolver{}, false
	}
	return daemonAgentLoopActionServiceResolver{
		services: set.Clone(),
	}, true
}
