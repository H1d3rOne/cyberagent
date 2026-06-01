# Schedule Management

## Intent

- `schedule`
- `cron`
- `recurring`
- `periodic`
- `scheduled_scan`
- `scheduled_prompt`

## Goal

把任意 Agent 指令（端口扫描、HTTP 检查、搜索、工作区操作等）或明确的 safe Web 漏扫注册成 cron 触发的周期任务；列出 / 启停 / 删除既有定时。该 skill 不会立即执行扫描——立即执行用对应的一次性工具。

Register any Agent instruction (port scan, HTTP check, search, workspace operation, etc.) or an explicit safe Web scan as a cron-driven recurring schedule; list / pause / delete existing entries. This skill never runs the action immediately — use the corresponding one-off tool for immediate work.

## When to use

User says any of: 定时 / 周期 / 每天 / 每小时 / 每隔 N / 每周 / 每月 / 凌晨 / nightly / hourly / daily / weekly / monthly / cron / scheduled / recurring / every N minutes·hours·days.

If the request has no recurring keyword and just asks to scan now, fall back to `web_vulnerability_scan`.

## Required tools

- `schedule.create`

## Optional tools

- `schedule.list`
- `schedule.delete`

## Risk

- `advanced`

## Inputs

- `cron`：5 字段标准 cron，或 `@every 1h`、`@hourly`、`@daily`、`@weekly`、`@monthly` 宏。
- `action`：默认用 `agent_prompt`；只有明确要求 recurring safe Web/API 漏扫时才用 `scan_web`。
  - `agent_prompt` 必填 `prompt`，保存去掉“定时/每天/7:20执行”等调度描述后的可执行指令，例如“扫描 example.com 开放端口 ports=80,443”；可选 `session_id`（默认当前会话）。
  - `scan_web` 必填 `target`（http/https URL），可选 `scope`、`profile`（默认 safe）。
- 可选 `name`、`enabled`。

## Examples

- 每天 7:20 扫描 `example.com` 开放端口 → `schedule.create cron="20 7 * * *" action=agent_prompt prompt="扫描 example.com 开放端口"`
- 每小时对 `https://example.com` 做 safe 漏扫 → `schedule.create cron="0 * * * *" action=scan_web target=https://example.com`
- 每 30 分钟探测一次接口状态 → `schedule.create cron="@every 30m" action=agent_prompt prompt="探测 https://example.com 状态"`
- 列出所有定时 → `schedule.list`
- 删除定时 `s_xxx` → `schedule.delete id=s_xxx`
