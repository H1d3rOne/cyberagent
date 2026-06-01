# Web Search

## Intent

- `web_search`
- `search`
- `lookup`
- `查询`
- `搜索`
- `联网搜索`

## Goal

使用已配置的 Web Search provider 检索公开 Web 信息，返回带来源的 title / URL / snippet 结果，供 Agent 后续总结或引用。该 skill 只做搜索，不读取具体页面正文，也不执行目标探测。

Search public web information through the configured Web Search provider and return sourced title / URL / snippet results for later synthesis. This skill searches only; it does not read page bodies or probe targets.

## When to use

User asks to search, look up, query, find current public information, CVE details, references, docs, advisories, or mentions 搜索 / 查询 / 查一下 / 搜一下 / 联网搜索.

If the user provides a specific URL to read or summarize, use `web_fetch`. If the user asks to test or probe a target URL, use `http.request` or the relevant scan skill instead.

## Required tools

- `web.search`

## Risk

- `safe_scan`

## Requirements

- Requires an active AI provider so the agent can choose tool calls.
- Requires Settings → Web Search to have a provider and API key configured.
- If the provider is not configured, return the `not_configured` tool error and tell the operator to configure Web Search settings.

## Inputs

- `query`：required search query.
- `max_results`：optional, default from settings or 5, capped at 10.
- `locale`：optional locale / language hint, e.g. `en-US` or `zh-CN`.

## Examples

- 查一下 `CVE-2024-1234` 的细节 → `web.search query="CVE-2024-1234 details" max_results=5`
- Search for recent references about an exploit chain → `web.search query="recent exploit chain references"`
- 搜索某个框架的最新安全公告 → `web.search query="framework security advisory latest" locale="zh-CN"`
