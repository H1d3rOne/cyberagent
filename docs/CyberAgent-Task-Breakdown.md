# CyberAgent Task Breakdown

版本：v0.1  
日期：2026-05-23  
状态：任务拆解计划完成

---

## 1. 目标

本文把 CyberAgent MVP 拆解为可执行任务，但不进入代码实现。

任务优先级：

```text
P0：MVP 必须完成，否则不能形成闭环
P1：MVP 应该完成，缺失会影响体验但不阻塞核心闭环
P2：可后置到 MVP 后
P3：明确暂缓
```

---

## 2. P0 任务：Core Runtime 闭环

### P0-01：Core Contracts

内容：

```text
Target
Scope
Job
Plan
Step
ToolCall
Event
Artifact
Evidence
Finding
Report
PluginManifest
Runner
PolicyDecision
HITLRequest
ReflectionResult
ContextPack
```

验收：

```text
可 JSON 序列化
字段与 Core Contracts 文档一致
基础构造函数存在
基础单元测试覆盖
```

### P0-02：Run Workspace

内容：

```text
runs/{job_id} 创建
目录结构创建
基础文件 touch
路径 Layout 统一管理
```

验收：

```text
workspace 可创建
非法 job_id 被拒绝
目录结构符合 MVP Spec
```

### P0-03：State Store

内容：

```text
bbolt 保存 Job/Plan/Step 当前状态
Save/Load/List 基础接口
```

验收：

```text
可保存/读取 Job
可保存/读取 Plan
可保存/读取 Step
missing 返回明确错误
```

### P0-04：Event Store / Event Bus

内容：

```text
events.jsonl append-only
EventBus publish/subscribe
按 Job 读取事件
```

验收：

```text
事件可追加
事件可读取
TUI/API 后续可订阅
```

### P0-05：Scope Guard

内容：

```text
scheme 校验
host 校验
domain 校验
CIDR 校验
port 校验
path allow/deny 校验
redirect 再校验
```

验收：

```text
无 scope 拒绝主动扫描
target 超出 scope 拒绝
redirect 超出 scope 拒绝
Denied 规则优先于 Allowed
```

### P0-06：Risk Policy

内容：

```text
info/low 自动允许
medium 根据 profile 决定
high/critical 需要审批
forbidden 永不执行
```

验收：

```text
能输出 allow / allow_with_limits / require_approval / deny
能生成 PolicyDecision
```

### P0-07：Safe Profile Planner

内容：

```text
生成 web safe profile plan
根据工具能力启用/跳过 optional steps
默认不启用 nuclei-safe
```

验收：

```text
有 scope 才生成主动网络 step
Plan steps 风险等级正确
Step dependencies 正确
```

---

## 3. P0 任务：内置 Web 分析闭环

### P0-08：HTTP Probe

内容：

```text
HEAD/GET 基础探测
状态码
响应时间
跳转链
```

验收：

```text
请求经过 Scope Guard
结果保存为 Artifact/Evidence
```

### P0-09：HTTP Fetcher

内容：

```text
抓取首页
保存 headers/body sample
限制响应大小
MIME 判断
```

验收：

```text
HTML 保存为 Artifact
headers 保存为 Artifact
```

### P0-10：Header Checker

内容：

```text
CSP
HSTS
X-Frame-Options
X-Content-Type-Options
Referrer-Policy
CORS 基础检查
```

验收：

```text
生成 info/low Finding
Finding 引用 Evidence
```

### P0-11：HTML Extractor

内容：

```text
links
forms
scripts
meta
comments，可选
```

验收：

```text
提取结果保存为 Evidence
URL 需要规范化和 scope 标记
```

### P0-12：Evidence/Finding/Report

内容：

```text
Evidence Indexer
Finding Normalizer
Markdown Reporter
JSON Reporter
```

验收：

```text
每个 Finding 至少一个 Evidence
report.md 可读
report.json 或 findings.json 可机器读取
```

---

## 4. P1 任务：增强内置能力

```text
Cookie Checker
Robots/Sitemap Fetcher
JS Route Extractor Light
API Hint Detector
Content Discovery Small
Redactor
Cache/Dedup
Command Recorder 完善
Doctor 完善
```

---

## 5. P1 任务：CLI 完整化

```text
scan web 真正执行 Runtime
report 打开/显示报告路径
replay 读取 events.jsonl
plugins list 读取 plugin manifests
doctor 检查插件和外部工具
```

---

## 6. P1 任务：TUI Run Viewer

```text
Plan View
Events View
Evidence View
Findings View
Report View
键位说明
错误状态展示
```

TUI v1 限制：

```text
只读已有 run
不创建任务
不审批任务
不实时控制任务
```

---

## 7. P1 任务：插件系统 MVP

```text
Plugin Manifest schema
Plugin discovery
Plugin doctor
Command Builder plugin
Analyzer plugin
nuclei-safe，可选且默认不启用
ffuf-low-rate，可选
python-js-route-analyzer，可选
```

---

## 8. P2 任务：LLM 增强

```text
LLM Gateway
Context Builder
Reporter Plugin
Analyzer Plugin
Reflection Assistant
```

限制：

```text
LLM 不执行工具
LLM 不扩大 scope
LLM 不审批高风险动作
```

---

## 9. P2 任务：Desktop / Local API

```text
Local API / Daemon
Wails Desktop
任务历史
报告浏览
配置管理
```

---

## 10. P3 暂缓任务

```text
Web 多用户平台
Remote Runner
K8s Runner
插件市场
PDF 报告
复杂 RAG
长期 Target Memory
企业资产监控
自动 exploit
自动修复代码
```

---

## 11. 推荐实现顺序

```text
1. P0-01 Core Contracts
2. P0-02 Run Workspace
3. P0-03 State Store
4. P0-04 Event Store
5. P0-05 Scope Guard
6. P0-06 Risk Policy
7. P0-07 Safe Profile Planner
8. P0-08 HTTP Probe
9. P0-09 HTTP Fetcher
10. P0-10 Header Checker
11. P0-11 HTML Extractor
12. P0-12 Evidence/Finding/Report
13. CLI scan web wiring
14. Cookie/Robots/API Hint/Content Discovery
15. Plugin System
16. TUI Run Viewer
17. MVP Stabilization
```
