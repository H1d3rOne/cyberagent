# ADR-0009: 所有主动动作必须经过 Scope Guard / Risk Policy

状态：Accepted  
日期：2026-05-23

## 决策

所有主动网络请求、外部工具调用、高风险步骤必须经过 Policy Engine。

## 原因

```text
安全 Agent 的核心是授权范围和风险控制。
插件和 LLM 不能绕过策略。
```

## 影响

```text
Step 执行前必须有 PolicyDecision。
重定向目标必须重新检查 scope。
forbidden step 永不执行。
high risk step 必须 HITL。
```
