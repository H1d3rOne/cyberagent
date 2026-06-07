package daemon

import (
	"context"
	"fmt"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

func attachDaemonToolHandlers(r *toolregistry.Registry) {
	setDaemonToolHandler(r, "scan.web.safe", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("scan.web.safe")
		}
		target := firstNonEmpty(
			stringFromAny(in.RawArgs["target"]),
			stringFromAny(in.RawArgs["url"]),
			stringFromAny(in.RawArgs["input"]),
		)
		scope := firstNonEmpty(
			stringFromAny(in.RawArgs["scope"]),
			stringFromAny(in.RawArgs["scope_url"]),
			target,
		)
		prompt := "扫描 " + target
		if scope != "" {
			prompt += " scope=" + scope
		}
		messages, calls, err := rt.Manager.handleAgentScan(ctx, prompt, in.Language, rt.legacyRouterServices())
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "ports.scan.safe", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("ports.scan.safe")
		}
		target := firstNonEmpty(
			stringFromAny(in.RawArgs["target"]),
			stringFromAny(in.RawArgs["host"]),
			stringFromAny(in.RawArgs["url"]),
			stringFromAny(in.RawArgs["input"]),
		)
		if target == "" {
			return nil, services.NewError(services.CodeBadRequest, "ports.scan.safe requires target or host", map[string]any{"action": "ports.scan.safe"}, nil)
		}
		prompt := "扫描 " + target + " 开放端口"
		if portsSpec := agentPortSpecFromPlannerArgument(in.RawArgs["ports"]); portsSpec != "" {
			prompt += " ports=" + portsSpec
		}
		messages, calls, err := rt.Manager.handleAgentPortScan(ctx, prompt, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "http.request", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("http.request")
		}
		targetURL := firstNonEmpty(
			stringFromAny(in.RawArgs["url"]),
			stringFromAny(in.RawArgs["target"]),
			stringFromAny(in.RawArgs["input"]),
		)
		method := firstNonEmpty(stringFromAny(in.RawArgs["method"]), "GET")
		if targetURL == "" {
			return nil, services.NewError(services.CodeBadRequest, "http.request requires url", map[string]any{"action": "http.request"}, nil)
		}
		targetURL = agentNormalizeURLInput(targetURL)
		headers := stringMapFromAny(in.RawArgs["headers"])
		if contentType := stringFromAny(in.RawArgs["content_type"]); contentType != "" {
			headers["Content-Type"] = contentType
		}
		authProfileID := firstNonEmpty(
			stringFromAny(in.RawArgs["auth_profile_id"]),
			stringFromAny(in.RawArgs["auth_profile"]),
			stringFromAny(in.RawArgs["profile_id"]),
			stringFromAny(in.RawArgs["profile"]),
		)
		profileHeaders, profileUsage, profileErr := agentResolveHTTPAuthProfileHeaders(rt.AuthProfiles, authProfileID, rt.DefaultAuthProfileID, targetURL)
		if profileErr != nil {
			return nil, profileErr
		}
		for key, value := range profileHeaders {
			if _, exists := headers[key]; !exists {
				headers[key] = value
			}
		}
		body := firstNonEmpty(
			stringFromAny(in.RawArgs["body"]),
			stringFromAny(in.RawArgs["content"]),
			stringFromAny(in.RawArgs["data"]),
			stringFromAny(in.RawArgs["input_body"]),
		)
		messages, calls, err := rt.Manager.handleAgentHTTPRequest(ctx, method, targetURL, headers, body, in.Language, profileUsage)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
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
	attachDaemonWorkspaceMutationHandlers(r)
	setDaemonToolHandler(r, "daemon.health", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("daemon.health")
		}
		messages, calls, err := rt.Manager.handleAgentHealth(ctx, in.Language, rt.legacyRouterServices())
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "plugins.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("plugins.list")
		}
		messages, calls, err := rt.Manager.handleAgentPlugins(ctx, in.Language, rt.legacyRouterServices())
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "runs.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("runs.list")
		}
		messages, calls, err := rt.Manager.handleAgentRuns(ctx, in.Language, rt.legacyRouterServices())
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	attachDaemonRunHandlers(r)
	setDaemonToolHandler(r, "schedule.create", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("schedule.create")
		}
		messages, calls, err := rt.Manager.handleAgentScheduleCreate(in.SessionID, in.Language, in.Operator, rt.legacyRouterServices(), in.RawArgs)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "schedule.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("schedule.list")
		}
		messages, calls, err := rt.Manager.handleAgentScheduleList(in.Language, rt.legacyRouterServices())
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "schedule.delete", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("schedule.delete")
		}
		messages, calls, err := rt.Manager.handleAgentScheduleDelete(in.Language, rt.legacyRouterServices(), in.RawArgs)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "web.search", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("web.search")
		}
		messages, calls, err := rt.Manager.handleAgentWebSearch(ctx, in.Language, rt.legacyRouterServices(), in.RawArgs)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "web.fetch", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("web.fetch")
		}
		messages, calls, err := rt.Manager.handleAgentWebFetch(ctx, in.Language, in.RawArgs)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
}

func attachDaemonWorkspaceMutationHandlers(r *toolregistry.Registry) {
	setDaemonToolHandler(r, "workspace.files.write", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.write")
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["file"]),
			stringFromAny(in.RawArgs["filename"]),
		)
		content := firstNonEmpty(
			stringFromAny(in.RawArgs["content"]),
			stringFromAny(in.RawArgs["body"]),
			stringFromAny(in.RawArgs["text"]),
			stringFromAny(in.RawArgs["input"]),
		)
		if relPath == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.write requires path", map[string]any{"action": "workspace.files.write"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceWrite(in.SessionID, relPath, content, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.patch", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.patch")
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["file"]),
			stringFromAny(in.RawArgs["filename"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(in.RawArgs["old"]),
			stringFromAny(in.RawArgs["old_text"]),
			stringFromAny(in.RawArgs["before"]),
			stringFromAny(in.RawArgs["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(in.RawArgs["new"]),
			stringFromAny(in.RawArgs["new_text"]),
			stringFromAny(in.RawArgs["after"]),
			stringFromAny(in.RawArgs["replace"]),
		)
		if relPath == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.patch requires path", map[string]any{"action": "workspace.files.patch"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspacePatch(in.SessionID, relPath, oldText, newText, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.diff", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.diff")
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["file"]),
			stringFromAny(in.RawArgs["filename"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(in.RawArgs["old"]),
			stringFromAny(in.RawArgs["old_text"]),
			stringFromAny(in.RawArgs["before"]),
			stringFromAny(in.RawArgs["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(in.RawArgs["new"]),
			stringFromAny(in.RawArgs["new_text"]),
			stringFromAny(in.RawArgs["after"]),
			stringFromAny(in.RawArgs["replace"]),
		)
		if relPath == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.diff requires path", map[string]any{"action": "workspace.files.diff"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceDiff(in.SessionID, relPath, oldText, newText, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.diff_many", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.diff_many")
		}
		changes := workspaceChangeInputsFromAny(in.RawArgs["changes"])
		if len(changes) == 0 {
			changes = workspaceChangeInputsFromAny(in.RawArgs["input"])
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceDiffMany(in.SessionID, changes, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.rebase", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.rebase")
		}
		changeID := firstNonEmpty(
			stringFromAny(in.RawArgs["change_id"]),
			stringFromAny(in.RawArgs["id"]),
		)
		oldText := firstNonEmpty(
			stringFromAny(in.RawArgs["old"]),
			stringFromAny(in.RawArgs["old_text"]),
			stringFromAny(in.RawArgs["before"]),
			stringFromAny(in.RawArgs["search"]),
		)
		newText := firstNonEmpty(
			stringFromAny(in.RawArgs["new"]),
			stringFromAny(in.RawArgs["new_text"]),
			stringFromAny(in.RawArgs["after"]),
			stringFromAny(in.RawArgs["replace"]),
		)
		messages, calls, err := rt.Manager.handleAgentWorkspaceRebase(in.SessionID, changeID, oldText, newText, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.apply", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.apply")
		}
		changeID := firstNonEmpty(
			stringFromAny(in.RawArgs["change_id"]),
			stringFromAny(in.RawArgs["id"]),
			stringFromAny(in.RawArgs["input"]),
		)
		groupID := firstNonEmpty(stringFromAny(in.RawArgs["group_id"]), stringFromAny(in.RawArgs["group"]))
		if groupID != "" {
			messages, calls, err := rt.Manager.handleAgentWorkspaceApplyGroup(in.SessionID, groupID, in.Language)
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		if boolFromAny(in.RawArgs["all"]) || strings.EqualFold(changeID, "all") || strings.EqualFold(stringFromAny(in.RawArgs["scope"]), "all") {
			messages, calls, err := rt.Manager.handleAgentWorkspaceApplyAll(in.SessionID, in.Language)
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		if changeID == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.apply requires change_id", map[string]any{"action": "workspace.files.apply"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceApply(in.SessionID, changeID, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.discard", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.discard")
		}
		changeID := firstNonEmpty(
			stringFromAny(in.RawArgs["change_id"]),
			stringFromAny(in.RawArgs["id"]),
			stringFromAny(in.RawArgs["input"]),
		)
		groupID := firstNonEmpty(stringFromAny(in.RawArgs["group_id"]), stringFromAny(in.RawArgs["group"]))
		if groupID != "" {
			messages, calls, err := rt.Manager.handleAgentWorkspaceDiscardGroup(in.SessionID, groupID, in.Language)
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		if boolFromAny(in.RawArgs["all"]) || strings.EqualFold(changeID, "all") || strings.EqualFold(stringFromAny(in.RawArgs["scope"]), "all") {
			messages, calls, err := rt.Manager.handleAgentWorkspaceDiscardAll(in.SessionID, in.Language)
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		if changeID == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.discard requires change_id", map[string]any{"action": "workspace.files.discard"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceDiscard(in.SessionID, changeID, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.delete", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.delete")
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["file"]),
			stringFromAny(in.RawArgs["filename"]),
			stringFromAny(in.RawArgs["input"]),
		)
		if relPath == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.delete requires path", map[string]any{"action": "workspace.files.delete"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceDelete(in.SessionID, relPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.move", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.move")
		}
		fromPath := firstNonEmpty(
			stringFromAny(in.RawArgs["from"]),
			stringFromAny(in.RawArgs["source"]),
			stringFromAny(in.RawArgs["old"]),
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["file"]),
			stringFromAny(in.RawArgs["filename"]),
		)
		toPath := firstNonEmpty(
			stringFromAny(in.RawArgs["to"]),
			stringFromAny(in.RawArgs["target"]),
			stringFromAny(in.RawArgs["new"]),
			stringFromAny(in.RawArgs["dest"]),
			stringFromAny(in.RawArgs["destination"]),
		)
		if fromPath == "" || toPath == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.move requires from and to", map[string]any{"action": "workspace.files.move"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceMove(in.SessionID, fromPath, toPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.copy", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.copy")
		}
		fromPath := firstNonEmpty(
			stringFromAny(in.RawArgs["from"]),
			stringFromAny(in.RawArgs["source"]),
			stringFromAny(in.RawArgs["old"]),
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["file"]),
			stringFromAny(in.RawArgs["filename"]),
		)
		toPath := firstNonEmpty(
			stringFromAny(in.RawArgs["to"]),
			stringFromAny(in.RawArgs["target"]),
			stringFromAny(in.RawArgs["new"]),
			stringFromAny(in.RawArgs["dest"]),
			stringFromAny(in.RawArgs["destination"]),
		)
		if fromPath == "" || toPath == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.copy requires from and to", map[string]any{"action": "workspace.files.copy"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceCopy(in.SessionID, fromPath, toPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.mkdir", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("workspace.files.mkdir")
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["directory"]),
			stringFromAny(in.RawArgs["dir"]),
			stringFromAny(in.RawArgs["input"]),
		)
		if relPath == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.mkdir requires path", map[string]any{"action": "workspace.files.mkdir"}, nil)
		}
		messages, calls, err := rt.Manager.handleAgentWorkspaceMkdir(in.SessionID, relPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
}

func attachDaemonRunHandlers(r *toolregistry.Registry) {
	setDaemonToolHandler(r, "runs.context", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("runs.context")
		}
		runID := firstNonEmpty(
			stringFromAny(in.RawArgs["run_id"]),
			stringFromAny(in.RawArgs["run"]),
			stringFromAny(in.RawArgs["id"]),
		)
		contextKind := firstNonEmpty(
			stringFromAny(in.RawArgs["context"]),
			stringFromAny(in.RawArgs["kind"]),
			stringFromAny(in.RawArgs["section"]),
			stringFromAny(in.RawArgs["type"]),
		)
		messages, calls, err := rt.Manager.handleAgentRunContextTool(ctx, in.SessionID, runID, contextKind, in.Language, rt.legacyRouterServices())
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "reports.draft", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("reports.draft")
		}
		runID := firstNonEmpty(
			stringFromAny(in.RawArgs["run_id"]),
			stringFromAny(in.RawArgs["run"]),
			stringFromAny(in.RawArgs["id"]),
		)
		outputPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["output_path"]),
			stringFromAny(in.RawArgs["file"]),
		)
		messages, calls, err := rt.Manager.handleAgentReportDraft(ctx, in.SessionID, runID, in.Language, rt.legacyRouterServices(), outputPath)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "runs.approvals.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("runs.approvals.list")
		}
		runID := firstNonEmpty(
			stringFromAny(in.RawArgs["run_id"]),
			stringFromAny(in.RawArgs["run"]),
			stringFromAny(in.RawArgs["id"]),
		)
		if runID == "" {
			messages, calls, err := rt.Manager.handleAgentApprovals(ctx, in.Language, rt.legacyRouterServices())
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		messages, calls, err := rt.Manager.handleAgentRunContextAnalysis(ctx, "读取运行 "+runID+" 的 approvals 审批队列", in.Language, rt.legacyRouterServices())
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonApprovalDecisionHandler(r, "runs.approvals.approve", false)
	setDaemonApprovalDecisionHandler(r, "runs.approvals.deny", true)
	setDaemonToolHandler(r, "runs.resume_approved", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError("runs.resume_approved")
		}
		runID := firstNonEmpty(
			stringFromAny(in.RawArgs["run_id"]),
			stringFromAny(in.RawArgs["run"]),
			stringFromAny(in.RawArgs["id"]),
		)
		prompt := "恢复已批准当前运行"
		if runID != "" {
			prompt = "恢复已批准运行 " + runID
		}
		messages, calls, err := rt.Manager.handleAgentResumeApproved(ctx, in.SessionID, prompt, in.Language, in.Operator, rt.legacyRouterServices())
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
}

func setDaemonApprovalDecisionHandler(r *toolregistry.Registry, name string, deny bool) {
	setDaemonToolHandler(r, name, func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
		if !ok || rt.Manager == nil {
			return nil, servicesMissingRegistryRuntimeError(name)
		}
		runID := firstNonEmpty(
			stringFromAny(in.RawArgs["run_id"]),
			stringFromAny(in.RawArgs["run"]),
		)
		approvalID := firstNonEmpty(
			stringFromAny(in.RawArgs["approval_id"]),
			stringFromAny(in.RawArgs["approval"]),
			stringFromAny(in.RawArgs["step_id"]),
			stringFromAny(in.RawArgs["step"]),
			stringFromAny(in.RawArgs["id"]),
		)
		comment := stringFromAny(in.RawArgs["comment"])
		verb := "批准"
		if deny {
			verb = "拒绝"
		}
		prompt := verb + "当前审批"
		if runID != "" && approvalID != "" {
			prompt = fmt.Sprintf("%s运行 %s 的 %s", verb, runID, approvalID)
		} else if runID != "" {
			prompt = fmt.Sprintf("%s运行 %s 的当前审批", verb, runID)
		} else if approvalID != "" {
			prompt = fmt.Sprintf("%s %s", verb, approvalID)
		}
		if strings.TrimSpace(comment) != "" {
			prompt += " comment=" + comment
		}
		messages, calls, err := rt.Manager.handleAgentApprovalDecision(ctx, in.SessionID, prompt, in.Language, in.Operator, rt.legacyRouterServices())
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
