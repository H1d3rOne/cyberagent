# CyberAgent Skills

该目录用于沉淀 CyberAgent 的可组合安全分析技能。

当前运行时的内置 Skill Registry 仍在 Go 代码中：

- `internal/interfaces/daemon/agent_skills.go`

本目录作为后续“文件化 Skills”的源目录，目标是让 Agent 能像 Claude Code / Codex 的 skills 一样：

1. 从本地 `skills/*/SKILL.md` 读取技能说明。
2. 将技能映射到 `intent_tags`、`required_tools`、`optional_tools` 和风险级别。
3. 把技能注入 Planner Prompt。
4. 通过 ToolCapability / MCP / 插件完成执行。

第一批技能：

- `web-vulnerability-scan`
- `port-scan`
- `content-discovery`
- `js-route-analysis`
- `api-auth-test`
- `workspace-ops`

