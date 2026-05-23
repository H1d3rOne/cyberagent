# CyberAgent Risk Plan

版本：v0.1  
日期：2026-05-23  
状态：风险计划完成

---

## 1. 风险分类

```text
产品范围风险
安全边界风险
技术复杂度风险
插件生态风险
LLM 风险
性能风险
用户体验风险
发布风险
```

---

## 2. 主要风险与缓解

| 风险 | 等级 | 缓解策略 |
|---|---|---|
| MVP 范围膨胀 | 高 | 严格 P0/P1/P2/P3，Desktop/Web 后置 |
| 插件绕过安全边界 | 高 | 所有插件必须经 Runner/Policy/Command Recorder |
| 无 scope 扫描 | 高 | 无 scope 不生成主动网络 Step |
| 高风险动作误执行 | 高 | high/critical 必须 HITL，forbidden 永不执行 |
| LLM 产生错误决策 | 高 | LLM 只增强，不执行、不审批、不扩大 scope |
| 工具输出过大 | 中 | 原始落盘，结构化摘要入 Evidence |
| TUI 过早复杂化 | 中 | TUI v1 只读 run viewer |
| SQLite 过早引入 | 中 | MVP 使用 bbolt，复杂查询后置 |
| 外部工具缺失导致失败 | 中 | doctor 检查，可选工具缺失降级 |
| 报告不可复现 | 高 | Finding 必须引用 Evidence 和 reproduction |

---

## 3. 风险处置规则

```text
安全边界风险必须立即处理。
影响 MVP 闭环的风险优先处理。
体验类风险可以后置。
插件和 LLM 风险默认保守处理。
```

---

## 4. MVP 风险红线

```text
不允许无 scope 主动扫描。
不允许插件绕过 Runner 执行命令。
不允许 high risk step 自动执行。
不允许 Finding 无 Evidence。
不允许 LLM 自动批准动作。
不允许默认运行 nuclei-safe。
```
