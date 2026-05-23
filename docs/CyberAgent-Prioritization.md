# CyberAgent Prioritization Plan

版本：v0.1  
日期：2026-05-23  
状态：优先级计划完成

---

## 1. 优先级原则

```text
安全边界 > 证据链 > 可复现 > 核心闭环 > 工具丰富度 > UI 体验 > LLM 智能化
```

---

## 2. 必须优先保护的能力

任何功能实现都不能破坏：

```text
Scope Guard
Risk Policy
Event Log
Evidence Chain
Command Recorder
Artifact Store
```

---

## 3. MVP 优先级矩阵

| 能力 | 优先级 | 原因 |
|---|---|---|
| Scope Guard | P0 | 没有范围控制不能主动扫描 |
| Event Store | P0 | Agent 状态和 TUI/API 都依赖事件 |
| State Store | P0 | 任务恢复和当前状态展示必需 |
| HTTP Probe/Fetch | P0 | Web 分析基础能力 |
| Evidence/Finding | P0 | 安全结论必须可追溯 |
| Markdown Report | P0 | 用户可读输出 |
| Header/HTML 分析 | P0 | MVP 低风险价值来源 |
| Cookie/Robots/API Hint | P1 | 增强分析覆盖面 |
| TUI Run Viewer | P1 | 改善 Agent 体验，但不阻塞 CLI 闭环 |
| Plugin System | P1 | 扩展能力，但 MVP 内置闭环优先 |
| nuclei-safe | P1 | 可选增强，默认不启用 |
| LLM Gateway | P2 | 增强而非核心 |
| Desktop | P2 | 降低门槛，核心稳定后做 |
| Web/Remote Runner | P3 | 团队化后置 |

---

## 4. 取舍规则

如果开发时间不足：

```text
保留 P0，砍 P1/P2。
保留 CLI，砍 TUI 实时能力。
保留内置分析，砍外部插件。
保留 Markdown，砍 PDF/HTML。
保留规则 Reflection，砍 LLM Reflection。
```

如果功能冲突：

```text
安全边界优先于自动化。
证据完整优先于报告美观。
可复现优先于扫描速度。
低风险默认优先于覆盖率。
```
