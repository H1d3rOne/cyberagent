# ADR-0005: LLM 作为增强层，不作为 Core 依赖

状态：Accepted  
日期：2026-05-23

## 决策

LLM 不作为 MVP Core 必需依赖，只作为 Analyzer / Reporter / Reflection 增强插件。

## 原因

```text
核心流程必须离线可运行。
安全边界不能依赖 Prompt。
成本、稳定性、供应商替换都需要解耦。
```

## 影响

```text
MVP 定义 LLM Gateway / ContextPack，但默认不启用 Provider。
LLM 不能执行工具、扩大 scope 或审批高风险动作。
```
