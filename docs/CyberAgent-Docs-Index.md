# CyberAgent Documentation Index

版本：v0.1  
日期：2026-05-23  
状态：规划与实现状态索引完成

---

## 1. 阅读顺序

建议按以下顺序阅读：

```text
1. CyberAgent-Plan.md
2. CyberAgent-Requirements.md
3. CyberAgent-Agent-Architecture.md
4. CyberAgent-Core-Contracts.md
5. CyberAgent-MVP-Spec.md
6. CyberAgent-Implementation-Blueprint.md
7. CyberAgent-Roadmap.md
8. CyberAgent-Milestones.md
9. CyberAgent-Task-Breakdown.md
10. CyberAgent-Prioritization.md
11. CyberAgent-ADR-Index.md
12. CyberAgent-Risk-Plan.md
13. CyberAgent-Test-Plan.md
14. CyberAgent-Release-Plan.md
15. CyberAgent-Planning-Audit.md
16. CyberAgent-Implementation-Status.md
17. CyberAgent-Smoke-Test.md
```

---

## 2. 文档说明

| 文档 | 用途 |
|---|---|
| `CyberAgent-Plan.md` | 总计划和关键决策 |
| `CyberAgent-Requirements.md` | 需求对齐和 MVP 边界 |
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
| `CyberAgent-Smoke-Test.md` | MVP smoke test 命令与结果记录 |

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
