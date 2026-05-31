# Content Discovery

## Intent

- `content_discovery`
- `directory_enum`
- `path_fuzzing`

## Goal

发现隐藏目录、路径和敏感文件。当前需要 MCP/plugin 提供 ffuf/gobuster/dirsearch 等价工具。

## Required tools

- `content.discovery`

## Optional tools

- `http.request`
- `plugins.list`
- MCP: `preset-ffuf`

## Risk

- `safe_scan`

## Fallback

- 先列出插件和 MCP 工具，确认是否有 ffuf/gobuster/dirsearch 等价能力。

