package daemon

import agenttools "github.com/H1d3rOne/cyberagent/internal/agent/tools"

// Deprecated: use tools.SkillSpec from internal/agent/tools.
type agentSkillSpec = agenttools.SkillSpec

type agentMCPIntegrationSpec struct {
	Status      string   `json:"status"`
	Description string   `json:"description"`
	NextSteps   []string `json:"next_steps"`
}

type agentSkillRegistryResponse struct {
	Skills           []agentSkillSpec          `json:"skills"`
	ToolCapabilities []agentToolCapabilitySpec `json:"tool_capabilities"`
	SecurityTools    []agentSecurityToolSpec   `json:"security_tools,omitempty"`
	MCP              agentMCPIntegrationSpec   `json:"mcp"`
}

func agentSkillRegistry(language string) agentSkillRegistryResponse {
	language = normalizeAgentLanguage(language)
	return agentSkillRegistryResponse{
		Skills:           agentBuiltinSkills(language),
		ToolCapabilities: agentToolCapabilities(language),
		SecurityTools:    agentSecurityToolCatalog(),
		MCP: agentMCPIntegrationSpec{
			Status: agentText(language, "planned", "planned"),
			Description: agentText(
				language,
				"MCP 将作为外部工具发现与调用层接入；MCP tool 会先映射为 ToolCapability，再经过 CyberAgent 权限与策略网关。",
				"MCP will be integrated as the external tool discovery/invocation layer; MCP tools will first be mapped to ToolCapability and then pass through CyberAgent's permission and policy gateway.",
			),
			NextSteps: []string{
				agentText(language, "增加 MCP server 配置模型", "Add MCP server configuration model"),
				agentText(language, "实现 MCP tools/list discover", "Implement MCP tools/list discovery"),
				agentText(language, "把 MCP tool 映射到 ToolCapability 并接入审批", "Map MCP tools to ToolCapability and approval gates"),
			},
		},
	}
}

// Deprecated: use tools.BuiltinSkills from internal/agent/tools.
func agentBuiltinSkills(language string) []agentSkillSpec {
	return agenttools.BuiltinSkills(language, agentAssetRoot())
}

// Deprecated: use tools.AttachSkillFileSources from internal/agent/tools.
func attachAgentSkillFileSources(skills []agentSkillSpec) []agentSkillSpec {
	return agenttools.AttachSkillFileSources(skills, agentAssetRoot())
}
