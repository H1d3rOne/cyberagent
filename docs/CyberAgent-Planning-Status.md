# CyberAgent Planning Status

版本：v0.1  
日期：2026-05-23  
状态：规划阶段已完成；实现阶段已启动

---

## 1. 当前状态

```text
规划阶段：完成并冻结
实现阶段：已按用户确认进入实现；MVP 主链路已跑通，Stabilization/Release 收尾中
```

---

## 2. 已完成规划文档

```text
[x] CyberAgent-Plan.md
[x] CyberAgent-Requirements.md
[x] CyberAgent-Agent-Architecture.md
[x] CyberAgent-Core-Contracts.md
[x] CyberAgent-MVP-Spec.md
[x] CyberAgent-Implementation-Blueprint.md
[x] CyberAgent-Roadmap.md
[x] CyberAgent-Milestones.md
[x] CyberAgent-Task-Breakdown.md
[x] CyberAgent-Prioritization.md
[x] CyberAgent-ADR-Index.md
[x] docs/adr/ADR-0001-go-core.md
[x] docs/adr/ADR-0002-cli-tui-first.md
[x] docs/adr/ADR-0003-plugin-tools.md
[x] docs/adr/ADR-0004-bbolt-state-store.md
[x] docs/adr/ADR-0005-llm-enhancement.md
[x] docs/adr/ADR-0006-nuclei-safe-disabled.md
[x] docs/adr/ADR-0007-tui-readonly-v1.md
[x] docs/adr/ADR-0008-skip-sqlite-mvp.md
[x] docs/adr/ADR-0009-policy-required.md
[x] docs/adr/ADR-0010-finding-evidence-required.md
[x] CyberAgent-Risk-Plan.md
[x] CyberAgent-Test-Plan.md
[x] CyberAgent-Release-Plan.md
[x] CyberAgent-Docs-Index.md
[x] CyberAgent-Planning-Audit.md
```

---

## 3. 已确认关键决策

```text
项目名：CyberAgent
Go module：github.com/H1d3rOne/cyberagent
首期形态：CLI + TUI
Core：Go
CLI：Cobra
TUI：Bubble Tea / Lip Gloss
插件层：Python / external command / JSONL
State Store：bbolt
Event Store：JSONL
Artifacts：filesystem
SQLite：MVP 跳过
LLM：增强层，默认不启用
nuclei-safe：可选，默认不启用
TUI v1：只读已有 run
Desktop：后置
Web：后置
Remote Runner：后置
```

---

## 4. 进入实现前的最后确认项

实现阶段已启动。当前默认收尾顺序：

```text
1. Scope Guard
2. Risk Policy
3. Safe Profile Planner
4. HTTP Probe / Fetcher
5. Evidence / Finding / Report
6. scan web wiring
7. TUI run viewer
```

---

## 5. 当前非计划代码状态

代码已进入 MVP 实现状态。当前主链路、内置工具、插件 MVP、TUI viewer 已基本完成；最新实现快照见 `docs/CyberAgent-Implementation-Status.md`。


---

## 6. 规划完成审计结论

```text
需求已锁定。
MVP 边界已锁定。
技术选型已锁定。
核心契约已定义。
任务拆解、优先级、里程碑、路线图、测试、风险、发布计划已完成。
开放问题已关闭并写入对应文档。
```

详细审计见：

```text
docs/CyberAgent-Planning-Audit.md
```
