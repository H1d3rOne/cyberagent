package daemon

import (
	"context"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func attachDaemonToolHandlers(r *toolregistry.Registry) {
	setDaemonToolHandler(r, "daemon.health", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("daemon.health")
		}
		messages, calls, err := rt.Manager.handleAgentHealth(ctx, in.Language, rt.Services)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "plugins.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("plugins.list")
		}
		messages, calls, err := rt.Manager.handleAgentPlugins(ctx, in.Language, rt.Services)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "runs.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("runs.list")
		}
		messages, calls, err := rt.Manager.handleAgentRuns(ctx, in.Language, rt.Services)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
}

func setDaemonToolHandler(r *toolregistry.Registry, name string, handler toolregistry.Handler) {
	if spec, ok := r.Lookup(name); ok {
		spec.Handler = handler
	}
}

func servicesMissingRegistryRuntimeError(action string) error {
	return services.NewError(services.CodeInternal, "registry handler runtime missing", map[string]any{"action": action}, nil)
}
