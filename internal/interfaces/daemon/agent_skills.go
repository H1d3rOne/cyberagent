package daemon

import (
	"encoding/json"
	"fmt"
	"strings"

	agenttools "github.com/H1d3rOne/cyberagent/internal/agent/tools"
)

// Deprecated: use tools.SkillSpec from internal/agent/tools.
type agentSkillSpec = agenttools.SkillSpec

type agentToolCapabilitySpec struct {
	Name          string          `json:"name"`
	Source        string          `json:"source"`
	CatalogPath   string          `json:"catalog_path,omitempty"`
	IntentTags    []string        `json:"intent_tags"`
	CanDo         []string        `json:"can_do"`
	CannotDo      []string        `json:"cannot_do,omitempty"`
	Risk          string          `json:"risk"`
	RequiredLevel string          `json:"required_level"`
	AutoRun       bool            `json:"auto_run"`
	Description   string          `json:"description"`
	InputSchema   json.RawMessage `json:"input_schema,omitempty"`
	Examples      []string        `json:"examples,omitempty"`
}

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

type agentToolCapabilityMetadata struct {
	IntentTags []string
	CanDo      []string
	CannotDo   []string
	Examples   []string
	Source     string
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

func agentToolCapabilities(language string) []agentToolCapabilitySpec {
	metadata := agentToolCapabilityMetadataByName(language)
	securityToolCatalog := agentSecurityToolCatalogByName()
	defs := toolRegistry().LLMDefinitions()
	out := make([]agentToolCapabilitySpec, 0, len(defs))
	for _, tool := range defs {
		name := normalizeAgentPlannerActionName(tool.Name)
		if name == "" {
			name = tool.Name
		}
		risk, autoRun := agentCapabilityToolPolicy(name)
		meta := metadata[name]
		source := meta.Source
		if source == "" {
			source = "builtin"
		}
		catalogPath := ""
		if catalogTool, ok := securityToolCatalog[name]; ok {
			catalogPath = catalogTool.Path
		}
		out = append(out, agentToolCapabilitySpec{
			Name:          name,
			Source:        source,
			CatalogPath:   catalogPath,
			IntentTags:    append([]string{}, meta.IntentTags...),
			CanDo:         append([]string{}, meta.CanDo...),
			CannotDo:      append([]string{}, meta.CannotDo...),
			Risk:          risk,
			RequiredLevel: agentOperationLevelRequiredForAction(name),
			AutoRun:       autoRun,
			Description:   tool.Description,
			InputSchema:   tool.InputSchema,
			Examples:      append([]string{}, meta.Examples...),
		})
	}
	return out
}

// agentToolCapabilityMetadataByName returns per-tool capability metadata for
// the agent's prompt and skill registry. The data is now derived from the
// canonical tool registry (toolregistry.Registry) so adding a new tool means
// editing one ToolSpec instead of also editing this map. Tools without any
// capability hints (IntentTags / CanDo / CannotDo / Examples) are dropped to
// preserve the legacy "tool absent from map" semantics — the parity test in
// agent_toolregistry_capability_parity_test.go pins this.
func agentToolCapabilityMetadataByName(language string) map[string]agentToolCapabilityMetadata {
	derived := toolRegistry().CapabilityMetadataByName(normalizeAgentLanguage(language))
	out := make(map[string]agentToolCapabilityMetadata, len(derived))
	for name, meta := range derived {
		if len(meta.IntentTags) == 0 && len(meta.CanDo) == 0 && len(meta.CannotDo) == 0 && len(meta.Examples) == 0 {
			continue
		}
		out[name] = agentToolCapabilityMetadata{
			IntentTags: meta.IntentTags,
			CanDo:      meta.CanDo,
			CannotDo:   meta.CannotDo,
			Examples:   meta.Examples,
		}
	}
	return out
}

func agentCapabilityRegistryPrompt(language string) string {
	registry := agentSkillRegistry(language)
	skillLines := make([]string, 0, len(registry.Skills))
	for _, skill := range registry.Skills {
		skillLines = append(skillLines, fmt.Sprintf("- %s: intents=%s tools=%s risk=%s", skill.ID, strings.Join(skill.IntentTags, ","), strings.Join(skill.RequiredTools, ","), skill.Risk))
	}
	toolLines := make([]string, 0, len(registry.ToolCapabilities))
	for _, tool := range registry.ToolCapabilities {
		if len(tool.IntentTags) == 0 && len(tool.CanDo) == 0 {
			continue
		}
		cannot := ""
		if len(tool.CannotDo) > 0 {
			cannot = " cannot=" + strings.Join(tool.CannotDo, ",")
		}
		toolLines = append(toolLines, fmt.Sprintf("- %s: intents=%s can=%s%s level=%s risk=%s", tool.Name, strings.Join(tool.IntentTags, ","), strings.Join(tool.CanDo, ","), cannot, tool.RequiredLevel, tool.Risk))
	}
	if normalizeAgentLanguage(language) == "en" {
		return "Skill / Tool Capability Registry:\nSkills:\n" + strings.Join(skillLines, "\n") + "\nTools:\n" + strings.Join(toolLines, "\n") + "\nUse skills to understand how to do the task; use tool capabilities to decide what can actually be executed. If a required tool is missing, stop at Plan and ask for the matching MCP/plugin or exact fallback input."
	}
	return "Skill / Tool Capability Registry：\nSkills:\n" + strings.Join(skillLines, "\n") + "\nTools:\n" + strings.Join(toolLines, "\n") + "\n用 skill 理解任务怎么做；用 tool capability 判断当前到底能执行什么。缺少 required tool 时停在 Plan，要求接入对应 MCP/plugin 或提供精确 fallback 输入。"
}
