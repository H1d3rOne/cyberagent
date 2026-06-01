package daemon

import (
	"context"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func attachDaemonToolHandlers(r *toolregistry.Registry) {
	setDaemonToolHandler(r, "workspace.command", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.command")
		}
		cmd := firstNonEmpty(
			stringFromAny(in.RawArgs["command"]),
			stringFromAny(in.RawArgs["cmd"]),
			stringFromAny(in.RawArgs["input"]),
		)
		if cmd == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.command requires command", map[string]any{"action": "workspace.command"}, nil)
		}
		if !agentPlannerCommandAllowed(cmd) {
			return nil, services.NewError(services.CodePolicyDenied, "planner command is not in the low-risk allowlist; ask the operator to run it explicitly", map[string]any{"command": cmd}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentCommand(ctx, in.SessionID, "command: "+cmd, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.list")
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["directory"]),
			stringFromAny(in.RawArgs["dir"]),
			stringFromAny(in.RawArgs["input"]),
			".",
		)
		messages, calls, err := rt.Manager.handleAgentWorkspaceList(in.SessionID, "path="+relPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.tree", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.tree")
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["directory"]),
			stringFromAny(in.RawArgs["dir"]),
			stringFromAny(in.RawArgs["input"]),
			".",
		)
		depth := intFromAny(in.RawArgs["depth"])
		if depth <= 0 {
			depth = 3
		}
		limit := intFromAny(in.RawArgs["limit"])
		if limit <= 0 {
			limit = 120
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceTree(in.SessionID, relPath, depth, limit, boolFromAny(in.RawArgs["include_hidden"]), in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.read", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.read")
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["file"]),
			stringFromAny(in.RawArgs["filename"]),
			stringFromAny(in.RawArgs["input"]),
		)
		if relPath == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.read requires path", map[string]any{"action": "workspace.files.read"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceRead(in.SessionID, "file="+relPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.search", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.search")
		}
		query := firstNonEmpty(
			stringFromAny(in.RawArgs["query"]),
			stringFromAny(in.RawArgs["pattern"]),
			stringFromAny(in.RawArgs["text"]),
			stringFromAny(in.RawArgs["input"]),
		)
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["directory"]),
			stringFromAny(in.RawArgs["dir"]),
			".",
		)
		if query == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.search requires query", map[string]any{"action": "workspace.search"}, nil)
		}
		limit := intFromAny(in.RawArgs["limit"])
		if limit <= 0 {
			limit = 80
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceSearch(in.SessionID, query, relPath, boolFromAny(in.RawArgs["regex"]), boolFromAny(in.RawArgs["case_sensitive"]), limit, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
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
