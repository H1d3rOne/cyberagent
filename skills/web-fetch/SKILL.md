# Web Fetch

## Intent

- `web_fetch`
- `page_read`
- `read_url`
- `fetch_url`
- `读取网页`

## Goal

读取一个明确给出的公开 HTTP(S) URL，返回 readable 文本或 raw 内容片段，供 Agent 总结页面、文章、公告或文档。该 skill 只执行 GET，并带 SSRF 防护。

Read an explicitly provided public HTTP(S) URL and return readable text or a raw content snippet for summarizing pages, articles, advisories, or docs. This skill is GET-only and protected by SSRF guards.

## When to use

User provides a specific URL and asks to read, fetch, open, summarize, extract, or inspect the page content.

If the user asks to search for information without a specific URL, use `web_search`. If the user asks to send POST/PUT/PATCH/DELETE, custom API requests, auth profiles, status/header probing, or target testing, use `http.request` instead.

## Required tools

- `web.fetch`

## Risk

- `safe_scan`

## Requirements

- Requires an active AI provider so the agent can choose tool calls.
- URL must be `http` or `https`.
- Private, loopback, link-local, multicast, CGNAT, unspecified, and cloud metadata hosts are refused.

## Inputs

- `url`：required public HTTP(S) URL.
- `mode`：optional, `readable` or `raw`; default `readable`.
- `max_chars`：optional response character cap; default 8000, capped at 20000.

## Examples

- 读取 `https://example.com/blog/x` 并总结 → `web.fetch url="https://example.com/blog/x" mode="readable"`
- Fetch a specific advisory page as raw text → `web.fetch url="https://example.com/advisory" mode="raw" max_chars=12000`
- 总结这篇文章的要点 → `web.fetch url="https://example.com/article"`
