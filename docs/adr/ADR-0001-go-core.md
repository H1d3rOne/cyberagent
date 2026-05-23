# ADR-0001: 采用 Go 作为 Agent Core

状态：Accepted  
日期：2026-05-23

## 决策

CyberAgent Core 使用 Go 实现。

## 原因

```text
适合 CLI/TUI
并发和取消模型清晰
跨平台单文件分发体验好
适合长期运行的 Agent Runtime
比 Python 更适合作为系统骨架
比 Rust 开发成本更低
```

## 影响

```text
核心 Runtime、CLI、TUI、Runner、Policy、Event、State 使用 Go。
Python 继续作为插件和分析生态层。
```
