# CyberAgent Documentation Index

版本：v0.3-agent-native
日期：2026-05-27
状态：v0.1.3/v0.1.4 Agent-Native 增量实现已进入可验证状态；继续以 Claude Code / Codex 型可执行 Agent 能力作为最高验收约束

---

## 1. 阅读顺序

建议按以下顺序阅读：

```text
1. CyberAgent-Plan.md
2. CyberAgent-Requirements.md
3. CyberAgent-Agent-Native-Benchmark.md
4. CyberAgent-Agent-Architecture.md
5. CyberAgent-Core-Contracts.md
6. CyberAgent-MVP-Spec.md
7. CyberAgent-Implementation-Blueprint.md
8. CyberAgent-Roadmap.md
9. CyberAgent-Milestones.md
10. CyberAgent-Task-Breakdown.md
11. CyberAgent-Prioritization.md
12. CyberAgent-ADR-Index.md
13. CyberAgent-Risk-Plan.md
14. CyberAgent-Test-Plan.md
15. CyberAgent-Release-Plan.md
16. CyberAgent-Planning-Audit.md
17. CyberAgent-Implementation-Status.md
18. CyberAgent-TODO.md
19. CyberAgent-Smoke-Test.md
20. CyberAgent-v0.1.1-Plan.md
21. CyberAgent-v0.1.1-Smoke-Test.md
22. CyberAgent-Optimization-Plan.md
23. CyberAgent-Release-Checklist.md
24. release-notes/v0.1.1.md
25. CyberAgent-v0.1.2-Plan.md
26. CyberAgent-v0.1.2-Implementation-Breakdown.md
27. api/v0.1.2-api-contract.md
28. CyberAgent-v0.1.2-Daemon-Smoke-Test.md
29. release-notes/v0.1.2.md
30. CyberAgent-v0.1.3-Agent-Native-Plan.md
31. CyberAgent-v0.1.4-Plan.md
32. CyberAgent-v0.1.5-Plan.md
33. CyberAgent-v0.1.6-Plan.md
34. CyberAgent-v0.1.7-Plan.md
35. CyberAgent-v0.1.8-Plan.md
```

---

## 2. 文档说明

| 文档 | 用途 |
|---|---|
| `CyberAgent-Plan.md` | 总计划和关键决策 |
| `CyberAgent-Requirements.md` | 需求对齐和 MVP 边界 |
| `CyberAgent-Agent-Native-Benchmark.md` | Claude Code / Codex 型 Agent 原生能力基线与验收标准 |
| `CyberAgent-Agent-Architecture.md` | Agent 能力模型和系统组成 |
| `CyberAgent-Core-Contracts.md` | 核心对象契约 |
| `CyberAgent-MVP-Spec.md` | MVP 规格和验收 |
| `CyberAgent-Implementation-Blueprint.md` | 实现蓝图 |
| `CyberAgent-Roadmap.md` | 长期路线图 |
| `CyberAgent-Milestones.md` | 里程碑计划 |
| `CyberAgent-Task-Breakdown.md` | 任务拆解 |
| `CyberAgent-Prioritization.md` | 优先级计划 |
| `CyberAgent-ADR-Index.md` | 架构决策索引 |
| `docs/adr/*` | 单个 ADR 详情 |
| `CyberAgent-Risk-Plan.md` | 风险计划 |
| `CyberAgent-Test-Plan.md` | 测试计划 |
| `CyberAgent-Release-Plan.md` | 发布计划 |
| `CyberAgent-Planning-Audit.md` | 规划完成审计与需求映射 |
| `CyberAgent-Implementation-Status.md` | MVP 实现进度快照与剩余项 |
| `CyberAgent-TODO.md` | 当前待办清单：工作树清理、发布准备、Production Agent Runtime 收敛、P1/P2 优化与后置能力 |
| `CyberAgent-Smoke-Test.md` | MVP smoke test 命令与结果记录 |
| `CyberAgent-v0.1.1-Plan.md` | v0.1.1 稳定性、HITL、插件加固和复现能力计划 |
| `CyberAgent-v0.1.1-Smoke-Test.md` | v0.1.1 P0 smoke test 命令、断言和验收 |
| `CyberAgent-Optimization-Plan.md` | 后续优化动态方案：毛玻璃 UI、runtime event catalog、UI state、approval/recovery 收敛、质量基线；由 Leader 根据成员反馈实时维护 |
| `CyberAgent-Release-Checklist.md` | 可复用发布前检查清单 |
| `release-notes/v0.1.1.md` | v0.1.1 release notes 草案；本地 RC 准备使用，远程发布需显式授权 |
| `CyberAgent-v0.1.2-Plan.md` | v0.1.2 Local Daemon / Web API foundation 计划；为 Desktop/Web 复用 Core 做准备 |
| `CyberAgent-v0.1.2-Implementation-Breakdown.md` | v0.1.2 实施拆解：工作流、tickets、依赖、验收、测试与提交节奏 |
| `api/v0.1.2-api-contract.md` | v0.1.2 Local Daemon HTTP API contract 草案：认证、响应 envelope、错误码、endpoint、请求/响应示例 |
| `CyberAgent-v0.1.2-Daemon-Smoke-Test.md` | v0.1.2 daemon smoke test 草案：启动、认证、scan/job/report/artifact/event/approval/cancel 验证 |
| `release-notes/v0.1.2.md` | v0.1.2 release notes 草案；本地 RC 准备使用，远程发布需显式授权 |
| `CyberAgent-v0.1.3-Agent-Native-Plan.md` | v0.1.3 Agent-Native 纠偏计划：Session/Task/Todo/Tool Loop/Workspace Context/Cockpit |
| `CyberAgent-v0.1.4-Plan.md` | v0.1.4 Agent-Native 打磨计划：Planner 异常、approval/run/context 闭环、staged edit、active_run report/evidence 摘要 |
| `CyberAgent-v0.1.5-Plan.md` | v0.1.5 Agent Auto-Continue 计划 |
| `CyberAgent-v0.1.6-Plan.md` | v0.1.6 Agent reasoning 可见性计划 |
| `CyberAgent-v0.1.7-Plan.md` | v0.1.7 Agent Workspace 计划 |
| `CyberAgent-v0.1.8-Plan.md` | v0.1.8 Command Execution 计划 |
| `CyberAgent-Agent-Cockpit-Smoke-Test.md` | Agent Cockpit 本地浏览器 smoke test：会话、聊天、Inspector 页签、能力目录、工作区和 daemon 在线验证 |

---

## 3. 当前关键决策

```text
Go module: github.com/H1d3rOne/cyberagent
Core: Go
CLI: Cobra
TUI: Bubble Tea / Lip Gloss
State Store: bbolt + JSONL + filesystem artifacts
SQLite: MVP 跳过，后续评估
Plugin: subprocess + JSON/JSONL
LLM: 增强层，不作为 Core 依赖
nuclei-safe: 默认不启用
TUI v1: 只读 run viewer
Agent-Native: 对标 Claude Code / Codex，必须具备 session / task / todo / tool loop / workspace context / state persistence
Web: Local Agent Cockpit 是 Agent Channel；多用户 Web 平台仍后置
Current release: v0.1.3/v0.1.4/v0.1.5/v0.1.6 Agent-Native 本地增量已验证，tag/push/GitHub Release 待显式授权
Next plan: v0.1.4+ Agent execution UX；单文件/多文件 workspace patch、diff→pending_changes/group_id→apply/discard/rebase、批量 apply/discard、apply conflict 标记/丢弃清理、apply-group 冲突不部分落盘、Workspace command 加固、Agent state/context/control alias API、capabilities endpoint discovery、规则 Agent 多命令顺序执行、确定性工具 routed synthesis、Timeline trace polish、Agent reasoning / plan revision 可见化、Planner mutating workspace intent gate、Cockpit Staged Changes 内联 Rebase/选区填充/片段 diff/完整文件级 diff 预览、审批/run/context 可视化闭环、active_run context snapshot 自动补齐、run 报告/最新 evidence 摘要自动提炼、session list run_summary/会话筛选/跨会话搜索/置顶收藏、step timeline deep-link、step execution chain、运行审计搜索/过滤、单 step 详情抽屉、Run Evidence 工作台、Agent Evidence Explain、Evidence Export、Evidence → Action、Evidence Action TODO Plan、Evidence Action Approval Gate、Cockpit Approval Gate UX、Approval Resolution State Refresh、Approval Gate State Convergence 已完成，后续优先增强多步自然语言任务和更强执行策略
```

---

## 4. 规划完成定义

规划阶段完成需要满足：

```text
需求明确
MVP 明确
契约明确
实现蓝图明确
路线图明确
任务拆解明确
优先级明确
ADR 明确
风险明确
测试明确
发布明确
规划完成审计明确
```
- CyberAgent-Design-Tokens.md
- CyberAgent-Frontend-Guidelines.md
- CyberAgent-Frontend-Checklist.md
