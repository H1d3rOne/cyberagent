# ADR-0002: CLI/TUI First，Desktop/Web 后置

状态：Accepted  
日期：2026-05-23

## 决策

首期产品形态为 CLI + TUI，Desktop/Web 后置。

## 原因

```text
Agent 最自然的运行方式是 CLI/TUI。
CLI 适合自动化、远程环境、CTF、脚本化。
TUI 适合查看计划、事件、证据和报告。
Desktop/Web 会引入更高工程复杂度。
```

## 影响

```text
Core 不依赖 UI。
CLI/TUI 只是 Channel。
Desktop/Web 后续复用 Runtime / Local API。
```
