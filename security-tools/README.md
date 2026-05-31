# CyberAgent Security Tools

该目录用于沉淀 CyberAgent 的安全工具目录，与 `skills/`、`mcp/`、`plugins/` 分层解耦。

当前运行时安全工具仍主要来自：

- 内置工具定义：`internal/interfaces/daemon/agent.go`
- ToolCapability Registry：`internal/interfaces/daemon/agent_skills.go`
- 插件目录：`plugins/`
- MCP preset：`mcp/presets/`

本目录的目标：

1. 以文件化方式维护安全工具 catalog。
2. 区分 `builtin`、`plugin`、`mcp` 三类来源。
3. 描述工具的 intent、能力边界、风险级别、权限要求和示例。
4. 后续让 daemon 从该目录加载工具定义，再映射为 `ToolCapability`。

目录结构：

```text
security-tools/
├── catalog.json
├── builtin/
│   ├── http.request.json
│   ├── ports.scan.safe.json
│   └── scan.web.safe.json
└── plugins/
    ├── ffuf-low-rate.json
    ├── nuclei-safe.json
    └── python-js-route-analyzer.json
```

注意：本目录目前是文件化 catalog，运行时仍以 Go Registry 为准。

