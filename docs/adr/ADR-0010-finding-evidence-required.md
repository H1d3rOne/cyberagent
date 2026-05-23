# ADR-0010: Finding 必须引用 Evidence

状态：Accepted  
日期：2026-05-23

## 决策

所有 Finding 必须引用至少一个 Evidence。

## 原因

```text
报告必须可复现。
安全结论必须可追溯。
避免 LLM 或工具输出直接形成不可验证结论。
```

## 影响

```text
Finding Normalizer 需要检查 evidence_ids。
Report Reflection Gate 需要检查证据链完整性。
无证据的结论只能作为 note/observation，不能成为 Finding。
```
