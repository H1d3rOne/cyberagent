# CyberAgent ADR Index

版本：v0.1  
日期：2026-05-23  
状态：架构决策记录索引完成

---

## ADR 列表

```text
ADR-0001：采用 Go 作为 Agent Core
ADR-0002：CLI/TUI First，Desktop/Web 后置
ADR-0003：外部安全工具通过插件接入
ADR-0004：MVP State Store 使用 bbolt + JSONL + 文件系统
ADR-0005：LLM 作为增强层，不作为 Core 依赖
ADR-0006：nuclei-safe 默认不启用
ADR-0007：TUI v1 只读已有 run
ADR-0008：MVP 不使用 SQLite
ADR-0009：所有主动动作必须经过 Scope Guard / Risk Policy
ADR-0010：Finding 必须引用 Evidence
```
