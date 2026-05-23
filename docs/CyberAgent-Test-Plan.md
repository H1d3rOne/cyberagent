# CyberAgent Test Plan

版本：v0.1  
日期：2026-05-23  
状态：测试计划完成

---

## 1. 测试目标

CyberAgent 的测试目标不是只验证代码能跑，而是验证：

```text
授权范围不被突破
风险策略可拦截危险动作
事件流可追踪
证据链可复现
报告结论有依据
插件不能绕过 Core
TUI/CLI 只通过 Runtime 获取状态
```

---

## 2. 测试分层

```text
Unit Test          单元测试
Contract Test      契约测试
Integration Test   集成测试
Golden Test        固定输出测试
Boundary Test      安全边界测试
CLI Test           命令行行为测试
TUI Test           只读 run viewer 测试
Plugin Test        插件协议测试
Regression Test    回归测试
```

---

## 3. Unit Test 范围

P0 必测：

```text
Target Normalize
Scope Matcher
Risk Policy
Job/Step 状态机
Event JSONL Store
bbolt State Store
Run Manager
Header Checker
HTML Extractor
Evidence Indexer
Finding Normalizer
Redactor
```

验收：

```text
go test ./... 通过
核心包测试覆盖关键错误路径
```

---

## 4. Contract Test 范围

测试对象：

```text
Target Schema
Scope Schema
Job Schema
Plan Schema
Step Schema
Event Schema
Artifact Schema
Evidence Schema
Finding Schema
Plugin Manifest Schema
ContextPack Schema
```

验收：

```text
示例 JSON 能通过 schema 校验
缺少必填字段时失败
风险等级/状态枚举非法时失败
```

---

## 5. Integration Test 范围

MVP 集成测试：

```text
创建 run workspace
生成 safe profile plan
执行内置 HTTP probe/fetch/header/html 流程
生成 events.jsonl
generate evidence/index.json
generate findings.json
generate reports/report.md
```

推荐测试靶场：

```text
testdata/web/simple-site
testdata/web/security-headers
testdata/web/js-routes
testdata/web/redirect-out-of-scope
```

---

## 6. Golden Test 范围

固定输出测试：

```text
report.md 模板输出
events.jsonl 示例输出
findings.json 示例输出
header checker 输出
HTML extractor 输出
JS route light 输出
```

原则：

```text
Golden 输出变化必须人工 review。
报告文字可允许小范围变动，但字段结构不能随意变化。
```

---

## 7. 安全边界测试

必须覆盖：

```text
无 scope 拒绝主动扫描
target host 不在 scope 拒绝
端口不在 scope 拒绝
scheme 不在 scope 拒绝
path 命中 denied 拒绝
redirect 到 scope 外停止
high risk step require_approval
forbidden step deny
插件不能绕过 Runner
LLM 不能执行工具
Finding 无 Evidence 时拒绝进入报告
敏感 header/cookie/token 被脱敏
```

---

## 8. CLI 测试

命令：

```text
cyberagent version
cyberagent doctor
cyberagent scan web
cyberagent report
cyberagent replay
cyberagent plugins list
```

验收：

```text
参数缺失时有清晰错误
无 scope 时拒绝扫描
可选工具缺失不导致 safe scan 失败
命令 exit code 合理
```

---

## 9. TUI 测试

TUI v1 只读已有 run。

测试范围：

```text
打开有效 run
打开不存在 run
打开损坏 run
查看 Plan
查看 Events
查看 Evidence
查看 Findings
查看 Report
```

不测试：

```text
任务创建
实时控制
审批高风险动作
```

---

## 10. 插件测试

范围：

```text
manifest 读取
依赖检查
禁用插件不运行
nuclei-safe 默认不启用
显式启用才运行
插件输出 schema 校验
插件错误时主流程降级
```

---

## 11. 性能与稳定性测试

MVP 基础指标：

```text
小型目标 safe scan 可在合理时间完成
大响应 body 会被限制和采样
事件写入不丢失
状态快照可恢复
重复运行不会污染旧 run
```

---

## 12. 测试完成定义

MVP 测试完成必须满足：

```text
go test ./... 通过
P0 安全边界测试全部通过
至少一个 simple-site 集成测试通过
Finding evidence 约束测试通过
报告 golden test 通过
插件默认禁用测试通过
```
