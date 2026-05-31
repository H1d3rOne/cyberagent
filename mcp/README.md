# CyberAgent MCP

该目录用于维护 CyberAgent 的 MCP 配置、预置 server 和后续协议适配器。

当前运行时 MCP Registry 仍在 Go 代码中：

- `internal/interfaces/daemon/agent_mcp.go`

当前持久化状态默认写入：

- `runs/.agent/mcp.json`

本目录的目标：

1. 保存内置 MCP preset 的文件化定义。
2. 后续让 daemon 从 `mcp/presets/*.json` 加载预置 server。
3. 实现 MCP JSON-RPC `tools/list` discovery。
4. 将 MCP tool 映射为 CyberAgent `ToolCapability`，再进入权限和策略网关。

当前 preset：

- `preset-nmap`
- `preset-ffuf`
- `preset-nuclei`
- `preset-semgrep`
- `preset-trivy`

