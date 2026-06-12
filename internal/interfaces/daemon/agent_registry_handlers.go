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
		resolver, webScanService, err := webScanActionServiceFromRegistryRuntime(in, "scan.web.safe")
		if err != nil {
			return nil, err
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
		messages, calls, err := webScanService.WebScan(ctx, prompt, in.Language, resolver)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "ports.scan.safe", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		portScannerService, err := portScannerActionServiceFromRegistryRuntime(in, "ports.scan.safe")
		if err != nil {
			return nil, err
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
		messages, calls, err := portScannerService.PortScan(ctx, prompt, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "http.request", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		rt, networkService, err := networkRequestActionServiceFromRegistryRuntime(in, "http.request")
		if err != nil {
			return nil, err
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
		profileHeaders, profileUsage, profileErr := agentResolveAgentLoopAuthProfileHeaders(rt.authorizationContext(), authProfileID, targetURL)
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
		messages, calls, err := networkService.HTTPRequest(ctx, method, targetURL, headers, body, in.Language, profileUsage)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.command", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.command")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Command(ctx, in.SessionID, cmd, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.list")
		if err != nil {
			return nil, err
		}
		relPath := firstNonEmpty(
			stringFromAny(in.RawArgs["path"]),
			stringFromAny(in.RawArgs["directory"]),
			stringFromAny(in.RawArgs["dir"]),
			stringFromAny(in.RawArgs["input"]),
			".",
		)
		messages, calls, err := workspaceService.List(in.SessionID, relPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.tree", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.tree")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Tree(in.SessionID, relPath, depth, limit, boolFromAny(in.RawArgs["include_hidden"]), in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.read", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.read")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Read(in.SessionID, relPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.search", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.search")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Search(in.SessionID, query, relPath, boolFromAny(in.RawArgs["regex"]), boolFromAny(in.RawArgs["case_sensitive"]), limit, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	attachDaemonWorkspaceMutationHandlers(r)
	setDaemonToolHandler(r, "daemon.health", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, statusService, err := daemonStatusActionServiceFromRegistryRuntime(in, "daemon.health")
		if err != nil {
			return nil, err
		}
		messages, calls, err := statusService.Health(ctx, in.Language, resolver)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "plugins.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, statusService, err := daemonStatusActionServiceFromRegistryRuntime(in, "plugins.list")
		if err != nil {
			return nil, err
		}
		messages, calls, err := statusService.Plugins(ctx, in.Language, resolver)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "runs.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, statusService, err := daemonStatusActionServiceFromRegistryRuntime(in, "runs.list")
		if err != nil {
			return nil, err
		}
		messages, calls, err := statusService.Runs(ctx, in.Language, resolver)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	attachDaemonRunHandlers(r)
	setDaemonToolHandler(r, "schedule.create", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, scheduleService, err := scheduleActionServiceFromRegistryRuntime(in, "schedule.create")
		if err != nil {
			return nil, err
		}
		messages, calls, err := scheduleService.ScheduleCreate(ctx, in.SessionID, in.Language, in.Operator, resolver, in.RawArgs)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "schedule.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, scheduleService, err := scheduleActionServiceFromRegistryRuntime(in, "schedule.list")
		if err != nil {
			return nil, err
		}
		messages, calls, err := scheduleService.ScheduleList(ctx, in.Language, resolver)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "schedule.delete", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, scheduleService, err := scheduleActionServiceFromRegistryRuntime(in, "schedule.delete")
		if err != nil {
			return nil, err
		}
		messages, calls, err := scheduleService.ScheduleDelete(ctx, in.Language, resolver, in.RawArgs)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "web.search", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, webSearchService, err := webSearchActionServiceFromRegistryRuntime(in, "web.search")
		if err != nil {
			return nil, err
		}
		messages, calls, err := webSearchService.WebSearch(ctx, in.Language, resolver, in.RawArgs)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "web.fetch", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_, networkService, err := networkRequestActionServiceFromRegistryRuntime(in, "web.fetch")
		if err != nil {
			return nil, err
		}
		messages, calls, err := networkService.WebFetch(ctx, in.Language, in.RawArgs)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
}

func attachDaemonWorkspaceMutationHandlers(r *toolregistry.Registry) {
	setDaemonToolHandler(r, "workspace.files.write", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.write")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Write(in.SessionID, relPath, content, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.patch", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.patch")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Patch(in.SessionID, relPath, oldText, newText, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.diff", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.diff")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Diff(in.SessionID, relPath, oldText, newText, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.diff_many", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.diff_many")
		if err != nil {
			return nil, err
		}
		changes := workspaceChangeInputsFromAny(in.RawArgs["changes"])
		if len(changes) == 0 {
			changes = workspaceChangeInputsFromAny(in.RawArgs["input"])
		}
		messages, calls, err := workspaceService.DiffMany(in.SessionID, changes, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.rebase", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.rebase")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Rebase(in.SessionID, changeID, oldText, newText, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.apply", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.apply")
		if err != nil {
			return nil, err
		}
		changeID := firstNonEmpty(
			stringFromAny(in.RawArgs["change_id"]),
			stringFromAny(in.RawArgs["id"]),
			stringFromAny(in.RawArgs["input"]),
		)
		groupID := firstNonEmpty(stringFromAny(in.RawArgs["group_id"]), stringFromAny(in.RawArgs["group"]))
		if groupID != "" {
			messages, calls, err := workspaceService.ApplyGroup(in.SessionID, groupID, in.Language)
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		if boolFromAny(in.RawArgs["all"]) || strings.EqualFold(changeID, "all") || strings.EqualFold(stringFromAny(in.RawArgs["scope"]), "all") {
			messages, calls, err := workspaceService.ApplyAll(in.SessionID, in.Language)
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		if changeID == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.apply requires change_id", map[string]any{"action": "workspace.files.apply"}, nil)
		}
		messages, calls, err := workspaceService.Apply(in.SessionID, changeID, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.discard", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.discard")
		if err != nil {
			return nil, err
		}
		changeID := firstNonEmpty(
			stringFromAny(in.RawArgs["change_id"]),
			stringFromAny(in.RawArgs["id"]),
			stringFromAny(in.RawArgs["input"]),
		)
		groupID := firstNonEmpty(stringFromAny(in.RawArgs["group_id"]), stringFromAny(in.RawArgs["group"]))
		if groupID != "" {
			messages, calls, err := workspaceService.DiscardGroup(in.SessionID, groupID, in.Language)
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		if boolFromAny(in.RawArgs["all"]) || strings.EqualFold(changeID, "all") || strings.EqualFold(stringFromAny(in.RawArgs["scope"]), "all") {
			messages, calls, err := workspaceService.DiscardAll(in.SessionID, in.Language)
			return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
		}
		if changeID == "" {
			return nil, services.NewError(services.CodeBadRequest, "workspace.files.discard requires change_id", map[string]any{"action": "workspace.files.discard"}, nil)
		}
		messages, calls, err := workspaceService.Discard(in.SessionID, changeID, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.delete", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.delete")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Delete(in.SessionID, relPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.move", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.move")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Move(in.SessionID, fromPath, toPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.copy", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.copy")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Copy(in.SessionID, fromPath, toPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "workspace.files.mkdir", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		_ = ctx
		workspaceService, err := workspaceActionServiceFromRegistryRuntime(in, "workspace.files.mkdir")
		if err != nil {
			return nil, err
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
		messages, calls, err := workspaceService.Mkdir(in.SessionID, relPath, in.Language)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
}

func attachDaemonRunHandlers(r *toolregistry.Registry) {
	setDaemonToolHandler(r, "runs.context", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, runContextApprovalService, err := runContextApprovalActionServiceFromRegistryRuntime(in, "runs.context")
		if err != nil {
			return nil, err
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
		messages, calls, err := runContextApprovalService.RunContext(ctx, in.SessionID, runID, contextKind, in.Language, resolver)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "reports.draft", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, runContextApprovalService, err := runContextApprovalActionServiceFromRegistryRuntime(in, "reports.draft")
		if err != nil {
			return nil, err
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
		messages, calls, err := runContextApprovalService.ReportDraft(ctx, in.SessionID, runID, in.Language, resolver, outputPath)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonToolHandler(r, "runs.approvals.list", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, runContextApprovalService, err := runContextApprovalActionServiceFromRegistryRuntime(in, "runs.approvals.list")
		if err != nil {
			return nil, err
		}
		runID := firstNonEmpty(
			stringFromAny(in.RawArgs["run_id"]),
			stringFromAny(in.RawArgs["run"]),
			stringFromAny(in.RawArgs["id"]),
		)
		messages, calls, err := runContextApprovalService.ApprovalsList(ctx, runID, in.Language, resolver)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
	setDaemonApprovalDecisionHandler(r, "runs.approvals.approve", false)
	setDaemonApprovalDecisionHandler(r, "runs.approvals.deny", true)
	setDaemonToolHandler(r, "runs.resume_approved", func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, runContextApprovalService, err := runContextApprovalActionServiceFromRegistryRuntime(in, "runs.resume_approved")
		if err != nil {
			return nil, err
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
		messages, calls, err := runContextApprovalService.ResumeApproved(ctx, in.SessionID, prompt, in.Language, in.Operator, resolver)
		return agentRegistryHandlerResult{Messages: messages, ToolCalls: calls}, err
	})
}

func setDaemonApprovalDecisionHandler(r *toolregistry.Registry, name string, deny bool) {
	setDaemonToolHandler(r, name, func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
		resolver, runContextApprovalService, err := runContextApprovalActionServiceFromRegistryRuntime(in, name)
		if err != nil {
			return nil, err
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
		messages, calls, err := runContextApprovalService.ApprovalDecision(ctx, in.SessionID, prompt, in.Language, in.Operator, resolver)
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

func workspaceActionServiceFromRegistryRuntime(in toolregistry.HandlerInput, action string) (daemonWorkspaceActionService, error) {
	rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
	if !ok {
		return nil, servicesMissingRegistryRuntimeError(action)
	}
	resolver, ok := rt.actionServiceResolver()
	if !ok {
		return nil, servicesMissingRegistryRuntimeError(action)
	}
	workspaceService, ok := resolver.Workspace()
	if !ok {
		return nil, servicesMissingRegistryRuntimeError(action)
	}
	return workspaceService, nil
}

func networkRequestActionServiceFromRegistryRuntime(in toolregistry.HandlerInput, action string) (agentRegistryHandlerRuntime, daemonNetworkRequestActionService, error) {
	rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
	if !ok {
		return agentRegistryHandlerRuntime{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	resolver, ok := rt.actionServiceResolver()
	if !ok {
		return agentRegistryHandlerRuntime{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	networkService, ok := resolver.NetworkRequest()
	if !ok {
		return agentRegistryHandlerRuntime{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	return rt, networkService, nil
}

func portScannerActionServiceFromRegistryRuntime(in toolregistry.HandlerInput, action string) (daemonPortScannerActionService, error) {
	rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
	if !ok {
		return nil, servicesMissingRegistryRuntimeError(action)
	}
	resolver, ok := rt.actionServiceResolver()
	if !ok {
		return nil, servicesMissingRegistryRuntimeError(action)
	}
	portScannerService, ok := resolver.PortScanner()
	if !ok {
		return nil, servicesMissingRegistryRuntimeError(action)
	}
	return portScannerService, nil
}

func webSearchActionServiceFromRegistryRuntime(in toolregistry.HandlerInput, action string) (daemonAgentLoopActionServiceResolver, daemonWebSearchActionService, error) {
	rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	resolver, ok := rt.actionServiceResolver()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	webSearchService, ok := resolver.WebSearch()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	return resolver, webSearchService, nil
}

func webScanActionServiceFromRegistryRuntime(in toolregistry.HandlerInput, action string) (daemonAgentLoopActionServiceResolver, daemonWebScanActionService, error) {
	rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	resolver, ok := rt.actionServiceResolver()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	webScanService, ok := resolver.WebScan()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	return resolver, webScanService, nil
}

func scheduleActionServiceFromRegistryRuntime(in toolregistry.HandlerInput, action string) (daemonAgentLoopActionServiceResolver, daemonScheduleActionService, error) {
	rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	resolver, ok := rt.actionServiceResolver()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	scheduleService, ok := resolver.ScheduleAction()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	return resolver, scheduleService, nil
}

func daemonStatusActionServiceFromRegistryRuntime(in toolregistry.HandlerInput, action string) (daemonAgentLoopActionServiceResolver, daemonStatusActionService, error) {
	rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	resolver, ok := rt.actionServiceResolver()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	statusService, ok := resolver.DaemonStatus()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	return resolver, statusService, nil
}

func runContextApprovalActionServiceFromRegistryRuntime(in toolregistry.HandlerInput, action string) (daemonAgentLoopActionServiceResolver, daemonRunContextApprovalActionService, error) {
	rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	resolver, ok := rt.actionServiceResolver()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	runContextApprovalService, ok := resolver.RunContextApproval()
	if !ok {
		return daemonAgentLoopActionServiceResolver{}, nil, servicesMissingRegistryRuntimeError(action)
	}
	return resolver, runContextApprovalService, nil
}
