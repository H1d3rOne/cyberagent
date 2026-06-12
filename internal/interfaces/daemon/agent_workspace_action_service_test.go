package daemon

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/runtime"
	"github.com/H1d3rOne/cyberagent/internal/agent/toolregistry"
	"github.com/stretchr/testify/require"
)

type recordingWorkspaceActionService struct {
	calls []recordingWorkspaceActionCall
}

type recordingWorkspaceActionCall struct {
	method    string
	sessionID string
	language  string
	args      map[string]any
}

func (service *recordingWorkspaceActionService) record(method, sessionID, language string, args map[string]any) ([]agentMessage, []agentToolCall, error) {
	service.calls = append(service.calls, recordingWorkspaceActionCall{
		method:    method,
		sessionID: sessionID,
		language:  language,
		args:      cloneMapStringAny(args),
	})
	tool := runningAgentToolCall("workspace."+method, method)
	tool = finishAgentToolCall(tool, nil)
	tool.Result = map[string]any{
		"method": method,
		"args":   cloneMapStringAny(args),
	}
	return []agentMessage{{
		ID:        "msg-workspace-" + method,
		Role:      "tool",
		Content:   "workspace service " + method,
		ToolCalls: []agentToolCall{tool},
	}}, []agentToolCall{tool}, nil
}

func (service *recordingWorkspaceActionService) Command(ctx context.Context, sessionID, command, language string) ([]agentMessage, []agentToolCall, error) {
	_ = ctx
	return service.record("Command", sessionID, language, map[string]any{"command": command})
}

func (service *recordingWorkspaceActionService) List(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("List", sessionID, language, map[string]any{"path": relPath})
}

func (service *recordingWorkspaceActionService) Tree(sessionID, relPath string, depth, limit int, includeHidden bool, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Tree", sessionID, language, map[string]any{"path": relPath, "depth": depth, "limit": limit, "include_hidden": includeHidden})
}

func (service *recordingWorkspaceActionService) Read(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Read", sessionID, language, map[string]any{"path": relPath})
}

func (service *recordingWorkspaceActionService) Search(sessionID, query, relPath string, regexMode, caseSensitive bool, limit int, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Search", sessionID, language, map[string]any{"query": query, "path": relPath, "regex": regexMode, "case_sensitive": caseSensitive, "limit": limit})
}

func (service *recordingWorkspaceActionService) Write(sessionID, relPath, content, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Write", sessionID, language, map[string]any{"path": relPath, "content": content})
}

func (service *recordingWorkspaceActionService) Patch(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Patch", sessionID, language, map[string]any{"path": relPath, "old": oldText, "new": newText})
}

func (service *recordingWorkspaceActionService) Diff(sessionID, relPath, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Diff", sessionID, language, map[string]any{"path": relPath, "old": oldText, "new": newText})
}

func (service *recordingWorkspaceActionService) DiffMany(sessionID string, changes []agentWorkspaceChangeInput, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("DiffMany", sessionID, language, map[string]any{"changes": changes})
}

func (service *recordingWorkspaceActionService) Rebase(sessionID, changeID, oldText, newText, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Rebase", sessionID, language, map[string]any{"change_id": changeID, "old": oldText, "new": newText})
}

func (service *recordingWorkspaceActionService) Apply(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Apply", sessionID, language, map[string]any{"change_id": changeID})
}

func (service *recordingWorkspaceActionService) ApplyAll(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("ApplyAll", sessionID, language, map[string]any{"all": true})
}

func (service *recordingWorkspaceActionService) ApplyGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("ApplyGroup", sessionID, language, map[string]any{"group_id": groupID})
}

func (service *recordingWorkspaceActionService) Discard(sessionID, changeID, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Discard", sessionID, language, map[string]any{"change_id": changeID})
}

func (service *recordingWorkspaceActionService) DiscardAll(sessionID, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("DiscardAll", sessionID, language, map[string]any{"all": true})
}

func (service *recordingWorkspaceActionService) DiscardGroup(sessionID, groupID, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("DiscardGroup", sessionID, language, map[string]any{"group_id": groupID})
}

func (service *recordingWorkspaceActionService) Delete(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Delete", sessionID, language, map[string]any{"path": relPath})
}

func (service *recordingWorkspaceActionService) Move(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Move", sessionID, language, map[string]any{"from": fromPath, "to": toPath})
}

func (service *recordingWorkspaceActionService) Copy(sessionID, fromPath, toPath, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Copy", sessionID, language, map[string]any{"from": fromPath, "to": toPath})
}

func (service *recordingWorkspaceActionService) Mkdir(sessionID, relPath, language string) ([]agentMessage, []agentToolCall, error) {
	return service.record("Mkdir", sessionID, language, map[string]any{"path": relPath})
}

func TestAgentLoopToolServiceBindingWithWorkspaceActionServiceInjectsRuntimeCapability(t *testing.T) {
	workspace := &recordingWorkspaceActionService{}
	binding := runtime.AgentLoopToolServiceBinding{
		Kind: daemonToolServiceBindingKind,
		ActionServices: runtime.AgentLoopActionServiceSet{
			Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
				runtime.AgentLoopActionServiceJobs: newJobManager(jobManagerConfig{RunsDir: t.TempDir()}),
			},
			Metadata: map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
		},
		Metadata: map[string]any{"adapter": "test"},
	}

	injected := agentLoopToolServiceBindingWithWorkspaceActionService(binding, workspace)
	resolver, ok := agentLoopActionServiceResolverFromActionServices(injected.ActionServices)
	require.True(t, ok)
	resolved, ok := resolver.Workspace()
	require.True(t, ok)
	require.Same(t, workspace, resolved)
	require.Equal(t, "workspace", injected.ActionServices.Metadata["workspace_capability"])
	require.Equal(t, "daemon_workspace_action_service", injected.ActionServices.Metadata["workspace_adapter"])
	require.Equal(t, "runtime_capability", injected.Metadata["workspace_action_service"])

	_, originalHasWorkspace := binding.ActionServices.Capabilities[runtime.AgentLoopActionServiceWorkspace]
	require.False(t, originalHasWorkspace, "workspace injection must clone instead of mutating the input binding")

	existing := &recordingWorkspaceActionService{}
	withExisting := injected.Clone()
	withExisting.ActionServices.Capabilities[runtime.AgentLoopActionServiceWorkspace] = existing
	reinjected := agentLoopToolServiceBindingWithWorkspaceActionService(withExisting, workspace)
	resolver, ok = agentLoopActionServiceResolverFromActionServices(reinjected.ActionServices)
	require.True(t, ok)
	resolved, ok = resolver.Workspace()
	require.True(t, ok)
	require.Same(t, existing, resolved, "existing workspace capability must not be overwritten")
}

func TestExecuteRegistryHandlerAttachesWorkspaceActionServiceCapability(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	called := false
	spec := &toolregistry.ToolSpec{
		Name: "test.workspace.capability.injected",
		Handler: func(ctx context.Context, in toolregistry.HandlerInput) (any, error) {
			_ = ctx
			called = true
			rt, ok := in.Runtime.(agentRegistryHandlerRuntime)
			require.True(t, ok)
			resolver, ok := rt.actionServiceResolver()
			require.True(t, ok)
			workspace, ok := resolver.Workspace()
			require.True(t, ok)
			adapter, ok := workspace.(daemonWorkspaceManagerActionService)
			require.True(t, ok)
			require.Same(t, mgr, adapter.manager)
			return map[string]any{"workspace_capability": true}, nil
		},
	}

	result, err := mgr.executeAgentRegistryHandlerResultWithToolServices(context.Background(), "workspace_capability_injected", "en", "tester", "read_only", testAgentLoopToolServices(), spec, nil, runtime.AgentLoopAuthorization{})
	require.NoError(t, err)
	require.True(t, called)
	require.Len(t, result.ToolCalls, 1)
	require.Equal(t, true, result.ToolCalls[0].Result["workspace_capability"])
}

func TestWorkspaceRegistryHandlersUseRuntimeWorkspaceActionServiceCapability(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		wantMethod string
	}{
		{name: "workspace.command", args: map[string]any{"command": "pwd"}, wantMethod: "Command"},
		{name: "workspace.files.list", args: map[string]any{"path": "src"}, wantMethod: "List"},
		{name: "workspace.files.tree", args: map[string]any{"path": "src", "depth": 2, "limit": 7, "include_hidden": true}, wantMethod: "Tree"},
		{name: "workspace.files.read", args: map[string]any{"path": "README.md"}, wantMethod: "Read"},
		{name: "workspace.search", args: map[string]any{"query": "TODO", "path": ".", "regex": true, "case_sensitive": true, "limit": 3}, wantMethod: "Search"},
		{name: "workspace.files.write", args: map[string]any{"path": "notes.md", "content": "hello"}, wantMethod: "Write"},
		{name: "workspace.files.patch", args: map[string]any{"path": "notes.md", "old": "hello", "new": "world"}, wantMethod: "Patch"},
		{name: "workspace.files.diff", args: map[string]any{"path": "notes.md", "old": "hello", "new": "world"}, wantMethod: "Diff"},
		{name: "workspace.files.diff_many", args: map[string]any{"changes": []map[string]any{{"path": "notes.md", "old": "hello", "new": "world"}}}, wantMethod: "DiffMany"},
		{name: "workspace.files.rebase", args: map[string]any{"change_id": "change_1", "old": "hello", "new": "world"}, wantMethod: "Rebase"},
		{name: "workspace.files.apply", args: map[string]any{"change_id": "change_1"}, wantMethod: "Apply"},
		{name: "workspace.files.apply", args: map[string]any{"all": true}, wantMethod: "ApplyAll"},
		{name: "workspace.files.apply", args: map[string]any{"group_id": "group_1"}, wantMethod: "ApplyGroup"},
		{name: "workspace.files.discard", args: map[string]any{"change_id": "change_1"}, wantMethod: "Discard"},
		{name: "workspace.files.discard", args: map[string]any{"all": true}, wantMethod: "DiscardAll"},
		{name: "workspace.files.discard", args: map[string]any{"group_id": "group_1"}, wantMethod: "DiscardGroup"},
		{name: "workspace.files.delete", args: map[string]any{"path": "notes.md"}, wantMethod: "Delete"},
		{name: "workspace.files.move", args: map[string]any{"from": "notes.md", "to": "archive/notes.md"}, wantMethod: "Move"},
		{name: "workspace.files.copy", args: map[string]any{"from": "notes.md", "to": "archive/notes.md"}, wantMethod: "Copy"},
		{name: "workspace.files.mkdir", args: map[string]any{"path": "reports"}, wantMethod: "Mkdir"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.wantMethod, func(t *testing.T) {
			workspace := &recordingWorkspaceActionService{}
			spec, ok := toolRegistry().Lookup(tt.name)
			require.True(t, ok, "%s missing from registry", tt.name)
			output, err := spec.Handler(context.Background(), toolregistry.HandlerInput{
				SessionID:      "workspace_capability_handlers",
				Operator:       "tester",
				Language:       "en",
				OperationLevel: "workspace_write",
				RawArgs:        cloneMapStringAny(tt.args),
				Runtime: agentRegistryHandlerRuntime{
					Services: runtime.AgentLoopToolServiceBinding{
						Kind:   daemonToolServiceBindingKind,
						Handle: registryPanicActionServiceFallback{},
						ActionServices: runtime.AgentLoopActionServiceSet{
							Capabilities: map[runtime.AgentLoopActionServiceCapability]any{
								runtime.AgentLoopActionServiceWorkspace: workspace,
							},
							Metadata: map[string]any{"contract": runtime.AgentLoopActionServiceContractName},
						},
						Metadata: map[string]any{"action_service_contract": runtime.AgentLoopActionServiceContractName},
					},
				},
			})
			require.NoError(t, err)
			result, ok := output.(agentRegistryHandlerResult)
			require.True(t, ok)
			require.Len(t, workspace.calls, 1)
			require.Equal(t, tt.wantMethod, workspace.calls[0].method)
			require.Equal(t, "workspace_capability_handlers", workspace.calls[0].sessionID)
			require.Equal(t, "en", workspace.calls[0].language)
			require.Len(t, result.ToolCalls, 1)
			require.Equal(t, tt.wantMethod, result.ToolCalls[0].Result["method"])
		})
	}
}

func TestWorkspaceRegistryHandlerSourceAvoidsDirectManagerWorkspaceCalls(t *testing.T) {
	data, err := os.ReadFile("agent_registry_handlers.go")
	require.NoError(t, err)
	src := string(data)
	require.Contains(t, src, "workspaceActionServiceFromRegistryRuntime")
	require.NotContains(t, src, "rt.Manager.handleAgentCommand(")
	require.NotContains(t, src, "rt.Manager.handleAgentWorkspace")
	for _, action := range []string{
		"workspace.command",
		"workspace.files.list",
		"workspace.files.tree",
		"workspace.files.read",
		"workspace.search",
		"workspace.files.write",
		"workspace.files.patch",
		"workspace.files.diff",
		"workspace.files.diff_many",
		"workspace.files.rebase",
		"workspace.files.apply",
		"workspace.files.discard",
		"workspace.files.delete",
		"workspace.files.move",
		"workspace.files.copy",
		"workspace.files.mkdir",
	} {
		require.Contains(t, src, "workspaceActionServiceFromRegistryRuntime(in, \""+action+"\")")
	}

	adapter, err := os.ReadFile("agent_workspace_action_service.go")
	require.NoError(t, err)
	adapterSrc := string(adapter)
	require.True(t, strings.Contains(adapterSrc, "handleAgentCommand(") && strings.Contains(adapterSrc, "handleAgentWorkspaceWrite("), "workspace manager calls should be isolated to the daemon workspace adapter")
}
