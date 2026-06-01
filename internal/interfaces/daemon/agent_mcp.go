package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
)

type agentMCPRegistryState struct {
	Servers []agentMCPServerConfig `json:"servers"`
}

type agentMCPServerConfig struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Transport   string               `json:"transport"`
	Command     string               `json:"command,omitempty"`
	Args        []string             `json:"args,omitempty"`
	URL         string               `json:"url,omitempty"`
	Env         map[string]string    `json:"env,omitempty"`
	Enabled     bool                 `json:"enabled"`
	Preset      bool                 `json:"preset,omitempty"`
	Tools       []agentMCPToolConfig `json:"tools,omitempty"`
	Status      string               `json:"status,omitempty"`
	LastError   string               `json:"last_error,omitempty"`
	LastSyncAt  *time.Time           `json:"last_sync_at,omitempty"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Description string               `json:"description,omitempty"`
}

type agentMCPToolConfig struct {
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	IntentTags    []string        `json:"intent_tags,omitempty"`
	CanDo         []string        `json:"can_do,omitempty"`
	CannotDo      []string        `json:"cannot_do,omitempty"`
	Risk          string          `json:"risk,omitempty"`
	RequiredLevel string          `json:"required_level,omitempty"`
	InputSchema   json.RawMessage `json:"input_schema,omitempty"`
	Examples      []string        `json:"examples,omitempty"`
	Enabled       bool            `json:"enabled"`
}

type agentMCPRegistryView struct {
	Servers          []agentMCPServerConfig    `json:"servers"`
	ToolCapabilities []agentToolCapabilitySpec `json:"tool_capabilities"`
	Count            int                       `json:"count"`
	EnabledCount     int                       `json:"enabled_count"`
}

type agentMCPDiscoverResponse struct {
	ServerID         string                    `json:"server_id,omitempty"`
	Status           string                    `json:"status"`
	Message          string                    `json:"message"`
	ToolCapabilities []agentToolCapabilitySpec `json:"tool_capabilities"`
}

func (m *agentManager) ListMCPRegistry(ctx context.Context) (agentMCPRegistryView, error) {
	if err := contextError(ctx); err != nil {
		return agentMCPRegistryView{}, err
	}
	state, err := m.loadMCPRegistry()
	if err != nil {
		return agentMCPRegistryView{}, err
	}
	view := agentMCPRegistryView{
		Servers:          cloneAgentMCPServers(state.Servers),
		ToolCapabilities: agentMCPToolCapabilitiesFromServers(state.Servers),
		Count:            len(state.Servers),
	}
	for _, server := range state.Servers {
		if server.Enabled {
			view.EnabledCount++
		}
	}
	return view, nil
}

func (m *agentManager) AddMCPServer(ctx context.Context, server agentMCPServerConfig) (agentMCPServerConfig, error) {
	if err := contextError(ctx); err != nil {
		return agentMCPServerConfig{}, err
	}
	server.Name = strings.TrimSpace(server.Name)
	server.Transport = normalizeAgentMCPTransport(server.Transport)
	if server.Name == "" {
		server.Name = firstNonEmpty(server.ID, "mcp-server")
	}
	if server.Transport == "" {
		return agentMCPServerConfig{}, services.NewError(services.CodeBadRequest, "unsupported MCP transport", map[string]any{"transport": server.Transport}, nil)
	}
	if server.Transport == "stdio" && strings.TrimSpace(server.Command) == "" {
		return agentMCPServerConfig{}, services.NewError(services.CodeBadRequest, "stdio MCP server requires command", map[string]any{"field": "command"}, nil)
	}
	if server.Transport == "http" && strings.TrimSpace(server.URL) == "" {
		return agentMCPServerConfig{}, services.NewError(services.CodeBadRequest, "http MCP server requires url", map[string]any{"field": "url"}, nil)
	}

	state, err := m.loadMCPRegistry()
	if err != nil {
		return agentMCPServerConfig{}, err
	}
	now := time.Now().UTC()
	server.ID = normalizeAgentMCPServerID(firstNonEmpty(server.ID, server.Name))
	if server.ID == "" {
		server.ID = strings.Replace(runs.NewJobID(now), "job_", "mcp_", 1)
	}
	server.CreatedAt = now
	server.UpdatedAt = now
	server.Status = firstNonEmpty(server.Status, "configured")
	server.Tools = normalizeAgentMCPTools(server.Tools)
	for _, existing := range state.Servers {
		if existing.ID == server.ID {
			return agentMCPServerConfig{}, services.NewError(services.CodeConflict, "MCP server already exists", map[string]any{"server_id": server.ID}, nil)
		}
	}
	state.Servers = append(state.Servers, server)
	if err := m.saveMCPRegistry(state); err != nil {
		return agentMCPServerConfig{}, err
	}
	return server, nil
}

func (m *agentManager) DeleteMCPServer(ctx context.Context, serverID string) (agentMCPServerConfig, error) {
	if err := contextError(ctx); err != nil {
		return agentMCPServerConfig{}, err
	}
	serverID = normalizeAgentMCPServerID(serverID)
	state, err := m.loadMCPRegistry()
	if err != nil {
		return agentMCPServerConfig{}, err
	}
	next := make([]agentMCPServerConfig, 0, len(state.Servers))
	var deleted agentMCPServerConfig
	for _, server := range state.Servers {
		if server.ID == serverID {
			deleted = server
			continue
		}
		next = append(next, server)
	}
	if deleted.ID == "" {
		return agentMCPServerConfig{}, services.NewError(services.CodeNotFound, "MCP server not found", map[string]any{"server_id": serverID}, nil)
	}
	state.Servers = next
	if err := m.saveMCPRegistry(state); err != nil {
		return agentMCPServerConfig{}, err
	}
	return deleted, nil
}

func (m *agentManager) SetMCPServerEnabled(ctx context.Context, serverID string, enabled bool) (agentMCPServerConfig, error) {
	if err := contextError(ctx); err != nil {
		return agentMCPServerConfig{}, err
	}
	serverID = normalizeAgentMCPServerID(serverID)
	state, err := m.loadMCPRegistry()
	if err != nil {
		return agentMCPServerConfig{}, err
	}
	now := time.Now().UTC()
	for idx := range state.Servers {
		if state.Servers[idx].ID != serverID {
			continue
		}
		state.Servers[idx].Enabled = enabled
		state.Servers[idx].UpdatedAt = now
		if enabled {
			state.Servers[idx].Status = "enabled"
		} else {
			state.Servers[idx].Status = "disabled"
		}
		if err := m.saveMCPRegistry(state); err != nil {
			return agentMCPServerConfig{}, err
		}
		return state.Servers[idx], nil
	}
	return agentMCPServerConfig{}, services.NewError(services.CodeNotFound, "MCP server not found", map[string]any{"server_id": serverID}, nil)
}

func (m *agentManager) DiscoverMCPTools(ctx context.Context, serverID string) (agentMCPDiscoverResponse, error) {
	if err := contextError(ctx); err != nil {
		return agentMCPDiscoverResponse{}, err
	}
	serverID = normalizeAgentMCPServerID(serverID)
	state, err := m.loadMCPRegistry()
	if err != nil {
		return agentMCPDiscoverResponse{}, err
	}
	now := time.Now().UTC()
	for idx := range state.Servers {
		if serverID != "" && state.Servers[idx].ID != serverID {
			continue
		}
		state.Servers[idx].LastSyncAt = &now
		if len(state.Servers[idx].Tools) == 0 {
			state.Servers[idx].Status = "configured"
			state.Servers[idx].LastError = "MCP JSON-RPC discovery is not enabled yet; declare tools manually or enable the protocol adapter in the next milestone."
		} else {
			state.Servers[idx].Status = "discovered"
			state.Servers[idx].LastError = ""
		}
		state.Servers[idx].UpdatedAt = now
	}
	if err := m.saveMCPRegistry(state); err != nil {
		return agentMCPDiscoverResponse{}, err
	}
	tools := agentMCPToolCapabilitiesFromServers(state.Servers)
	if serverID != "" {
		filtered := make([]agentToolCapabilitySpec, 0)
		for _, tool := range tools {
			if strings.EqualFold(tool.Source, "mcp:"+serverID) {
				filtered = append(filtered, tool)
			}
		}
		tools = filtered
	}
	return agentMCPDiscoverResponse{
		ServerID:         serverID,
		Status:           "ok",
		Message:          "MCP registry refreshed. Protocol-level tools/list is planned; manually declared tools are mapped to ToolCapability now.",
		ToolCapabilities: tools,
	}, nil
}

func (m *agentManager) agentMCPRegistryPrompt(language string) string {
	state, err := m.loadMCPRegistry()
	if err != nil || len(state.Servers) == 0 {
		return ""
	}
	tools := agentMCPToolCapabilitiesFromServers(state.Servers)
	if len(tools) == 0 {
		return ""
	}
	lines := make([]string, 0, len(tools))
	for _, tool := range tools {
		lines = append(lines, "- "+tool.Name+" source="+tool.Source+" intents="+strings.Join(tool.IntentTags, ",")+" can="+strings.Join(tool.CanDo, ",")+" level="+tool.RequiredLevel+" risk="+tool.Risk)
	}
	if normalizeAgentLanguage(language) == "en" {
		return "Enabled MCP ToolCapabilities:\n" + strings.Join(lines, "\n") + "\nMCP tools must still pass CyberAgent policy and approval gates before execution."
	}
	return "已启用 MCP ToolCapabilities：\n" + strings.Join(lines, "\n") + "\nMCP 工具仍必须经过 CyberAgent policy 和 approval gate 后才能执行。"
}

func (m *agentManager) loadMCPRegistry() (agentMCPRegistryState, error) {
	path := m.mcpRegistryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return agentMCPRegistryState{Servers: agentDefaultMCPServers()}, nil
		}
		return agentMCPRegistryState{}, services.NewError(services.CodeInternal, "read MCP registry", map[string]any{"path": path}, err)
	}
	var state agentMCPRegistryState
	if err := json.Unmarshal(data, &state); err != nil {
		return agentMCPRegistryState{}, services.NewError(services.CodeInternal, "decode MCP registry", map[string]any{"path": path}, err)
	}
	state.Servers = cloneAgentMCPServers(state.Servers)
	state.Servers = mergeAgentMCPDefaultPresets(state.Servers)
	return state, nil
}

func (m *agentManager) saveMCPRegistry(state agentMCPRegistryState) error {
	path := m.mcpRegistryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return services.NewError(services.CodeInternal, "create MCP registry directory", map[string]any{"path": filepath.Dir(path)}, err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return services.NewError(services.CodeInternal, "encode MCP registry", nil, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return services.NewError(services.CodeInternal, "write MCP registry", map[string]any{"path": path}, err)
	}
	return nil
}

func (m *agentManager) mcpRegistryPath() string {
	base := filepath.Dir(m.storeDir)
	if strings.TrimSpace(base) == "." || strings.TrimSpace(base) == "" {
		base = filepath.Join(DefaultRunsDir, ".agent")
	}
	return filepath.Join(base, "mcp.json")
}

func agentMCPToolCapabilitiesFromServers(servers []agentMCPServerConfig) []agentToolCapabilitySpec {
	out := make([]agentToolCapabilitySpec, 0)
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		for _, tool := range normalizeAgentMCPTools(server.Tools) {
			if !tool.Enabled {
				continue
			}
			name := strings.TrimSpace(tool.Name)
			if name == "" {
				continue
			}
			out = append(out, agentToolCapabilitySpec{
				Name:          agentMCPToolName(server.ID, name),
				Source:        "mcp:" + server.ID,
				IntentTags:    append([]string{}, tool.IntentTags...),
				CanDo:         append([]string{}, tool.CanDo...),
				CannotDo:      append([]string{}, tool.CannotDo...),
				Risk:          firstNonEmpty(tool.Risk, "medium"),
				RequiredLevel: normalizeAgentOperationLevel(firstNonEmpty(tool.RequiredLevel, "advanced")),
				AutoRun:       false,
				Description:   tool.Description,
				InputSchema:   cloneRawMessage(tool.InputSchema),
				Examples:      append([]string{}, tool.Examples...),
			})
		}
	}
	return out
}

func agentDefaultMCPServers() []agentMCPServerConfig {
	if servers := loadAgentMCPPresetFiles(agentAssetPath("mcp", "presets")); len(servers) > 0 {
		return servers
	}
	return agentDefaultMCPServersBuiltin()
}

func loadAgentMCPPresetFiles(dir string) []agentMCPServerConfig {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	now := time.Now().UTC()
	out := make([]agentMCPServerConfig, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var server agentMCPServerConfig
		if err := json.Unmarshal(data, &server); err != nil {
			continue
		}
		server.ID = normalizeAgentMCPServerID(firstNonEmpty(server.ID, server.Name))
		server.Transport = normalizeAgentMCPTransport(server.Transport)
		if server.ID == "" || server.Transport == "" {
			continue
		}
		server.Preset = true
		server.Enabled = false
		server.Status = firstNonEmpty(server.Status, "preset_unverified")
		if server.CreatedAt.IsZero() {
			server.CreatedAt = now
		}
		if server.UpdatedAt.IsZero() {
			server.UpdatedAt = now
		}
		server.Tools = normalizeAgentMCPTools(server.Tools)
		out = append(out, server)
	}
	return out
}

func agentDefaultMCPServersBuiltin() []agentMCPServerConfig {
	now := time.Now().UTC()
	return []agentMCPServerConfig{
		{
			ID:          "preset-nmap",
			Name:        "Nmap MCP",
			Transport:   "stdio",
			Command:     "mcp-nmap",
			Enabled:     false,
			Preset:      true,
			Status:      "preset_unverified",
			Description: "Preset for an MCP server that exposes bounded nmap/TCP service discovery tools. Disabled until installed and verified locally.",
			CreatedAt:   now,
			UpdatedAt:   now,
			Tools: []agentMCPToolConfig{{
				Name:          "port_scan",
				Description:   "Run authorized nmap-style TCP port scan/service discovery through MCP.",
				IntentTags:    []string{"port_scan", "service_discovery", "nmap"},
				CanDo:         []string{"TCP port scanning", "optional service/version fingerprinting when explicitly authorized"},
				CannotDo:      []string{"scan without authorization", "bypass CyberAgent policy"},
				Risk:          "medium",
				RequiredLevel: "safe_scan",
				Enabled:       true,
			}},
		},
		{
			ID:          "preset-ffuf",
			Name:        "ffuf MCP",
			Transport:   "stdio",
			Command:     "mcp-ffuf",
			Enabled:     false,
			Preset:      true,
			Status:      "preset_unverified",
			Description: "Preset for content discovery/path fuzzing with ffuf-compatible MCP tooling.",
			CreatedAt:   now,
			UpdatedAt:   now,
			Tools: []agentMCPToolConfig{{
				Name:          "content_discovery",
				Description:   "Run bounded authorized content discovery with a supplied wordlist/profile.",
				IntentTags:    []string{"content_discovery", "directory_enum", "path_fuzzing", "ffuf"},
				CanDo:         []string{"discover hidden paths", "test wordlist entries against authorized HTTP targets"},
				CannotDo:      []string{"port_scan", "unbounded brute force"},
				Risk:          "medium",
				RequiredLevel: "safe_scan",
				Enabled:       true,
			}},
		},
		{
			ID:          "preset-nuclei",
			Name:        "Nuclei MCP",
			Transport:   "stdio",
			Command:     "mcp-nuclei",
			Enabled:     false,
			Preset:      true,
			Status:      "preset_unverified",
			Description: "Preset for template-based vulnerability checks through a Nuclei-compatible MCP server.",
			CreatedAt:   now,
			UpdatedAt:   now,
			Tools: []agentMCPToolConfig{{
				Name:          "template_scan",
				Description:   "Run selected nuclei templates against authorized targets.",
				IntentTags:    []string{"web_vulnerability_scan", "template_scan", "nuclei"},
				CanDo:         []string{"run selected safe templates", "return findings/evidence"},
				CannotDo:      []string{"run unreviewed destructive templates", "replace CyberAgent safe profile"},
				Risk:          "high",
				RequiredLevel: "advanced",
				Enabled:       true,
			}},
		},
		{
			ID:          "preset-semgrep",
			Name:        "Semgrep MCP",
			Transport:   "stdio",
			Command:     "mcp-semgrep",
			Enabled:     false,
			Preset:      true,
			Status:      "preset_unverified",
			Description: "Preset for static code analysis in the Agent workspace with Semgrep-compatible MCP tooling.",
			CreatedAt:   now,
			UpdatedAt:   now,
			Tools: []agentMCPToolConfig{{
				Name:          "code_scan",
				Description:   "Run Semgrep rules against workspace code.",
				IntentTags:    []string{"workspace", "code_analysis", "sast", "semgrep"},
				CanDo:         []string{"static code scan", "return rule findings with file locations"},
				CannotDo:      []string{"modify files"},
				Risk:          "low",
				RequiredLevel: "read_only",
				Enabled:       true,
			}},
		},
		{
			ID:          "preset-trivy",
			Name:        "Trivy MCP",
			Transport:   "stdio",
			Command:     "mcp-trivy",
			Enabled:     false,
			Preset:      true,
			Status:      "preset_unverified",
			Description: "Preset for dependency/container/IaC scanning with Trivy-compatible MCP tooling.",
			CreatedAt:   now,
			UpdatedAt:   now,
			Tools: []agentMCPToolConfig{{
				Name:          "supply_chain_scan",
				Description:   "Run dependency/container/IaC vulnerability scanning.",
				IntentTags:    []string{"supply_chain", "dependency_scan", "container_scan", "trivy"},
				CanDo:         []string{"dependency vulnerability scan", "container image scan", "IaC misconfiguration scan"},
				CannotDo:      []string{"exploit vulnerabilities", "modify images"},
				Risk:          "low",
				RequiredLevel: "read_only",
				Enabled:       true,
			}},
		},
	}
}

func mergeAgentMCPDefaultPresets(servers []agentMCPServerConfig) []agentMCPServerConfig {
	seen := make(map[string]struct{}, len(servers))
	out := cloneAgentMCPServers(servers)
	for _, server := range out {
		seen[server.ID] = struct{}{}
	}
	for _, preset := range agentDefaultMCPServers() {
		if _, ok := seen[preset.ID]; ok {
			continue
		}
		out = append(out, preset)
	}
	return out
}

func normalizeAgentMCPTransport(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "stdio", "command", "cmd":
		return "stdio"
	case "http", "sse", "streamable_http", "streamable-http":
		return "http"
	default:
		return ""
	}
}

func normalizeAgentMCPServerID(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = regexp.MustCompile(`[^a-z0-9_.-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_.")
	if len(value) > 80 {
		value = value[:80]
	}
	return value
}

func agentMCPToolName(serverID, toolName string) string {
	serverID = normalizeAgentMCPServerID(serverID)
	toolName = normalizeAgentMCPServerID(toolName)
	if serverID == "" || toolName == "" {
		return ""
	}
	return "mcp__" + serverID + "__" + toolName
}

func normalizeAgentMCPToolActionName(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(value, "mcp__"):
		parts := strings.Split(value, "__")
		if len(parts) >= 3 {
			return agentMCPToolName(parts[1], strings.Join(parts[2:], "__"))
		}
	case strings.HasPrefix(value, "mcp."):
		legacy := strings.TrimPrefix(value, "mcp.")
		parts := strings.SplitN(legacy, ".", 2)
		if len(parts) == 2 {
			return agentMCPToolName(parts[0], parts[1])
		}
	}
	return ""
}

func normalizeAgentMCPTools(tools []agentMCPToolConfig) []agentMCPToolConfig {
	out := make([]agentMCPToolConfig, 0, len(tools))
	for _, tool := range tools {
		tool.Name = strings.TrimSpace(tool.Name)
		if tool.Name == "" {
			continue
		}
		if strings.TrimSpace(tool.Risk) == "" {
			tool.Risk = "medium"
		}
		if strings.TrimSpace(tool.RequiredLevel) == "" {
			tool.RequiredLevel = "advanced"
		}
		tool.RequiredLevel = normalizeAgentOperationLevel(tool.RequiredLevel)
		if !tool.Enabled {
			// Keep backwards-compatible JSON concise: omitted bool means false.
			// For newly declared tools through API, the handler defaults this to true.
			tool.Enabled = false
		}
		out = append(out, tool)
	}
	return out
}

func cloneAgentMCPServers(servers []agentMCPServerConfig) []agentMCPServerConfig {
	out := make([]agentMCPServerConfig, 0, len(servers))
	for _, server := range servers {
		server.Args = append([]string{}, server.Args...)
		if server.Env != nil {
			env := make(map[string]string, len(server.Env))
			for k, v := range server.Env {
				env[k] = v
			}
			server.Env = env
		}
		server.Tools = cloneAgentMCPTools(server.Tools)
		out = append(out, server)
	}
	return out
}

func cloneAgentMCPTools(tools []agentMCPToolConfig) []agentMCPToolConfig {
	out := make([]agentMCPToolConfig, 0, len(tools))
	for _, tool := range tools {
		tool.IntentTags = append([]string{}, tool.IntentTags...)
		tool.CanDo = append([]string{}, tool.CanDo...)
		tool.CannotDo = append([]string{}, tool.CannotDo...)
		tool.Examples = append([]string{}, tool.Examples...)
		tool.InputSchema = cloneRawMessage(tool.InputSchema)
		out = append(out, tool)
	}
	return out
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}
