# CyberAgent Task Breakdown

版本：v0.2-agent-native-correction  
日期：2026-05-25  
状态：任务拆解纠偏；Agent-Native 能力提升为 P0/P1 主线

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

## 1.1 Agent-Native 纠偏原则

后续任务优先级必须服从：

```text
Agent 会话 > Agent 当前任务 > Agent Todo > Agent Tool Loop > Workspace Context > UI 展示
```

不能再把“聊天框 + 扫描状态卡”当成 Agent。

---

## 2. P0 任务：Agent-Native Runtime

### P0-A01：Agent State Contracts

内容：

```text
AgentSession
AgentMessage
AgentTask
AgentTodo
AgentToolCall
AgentContextRef
AgentNextAction
```

验收：

```text
可 JSON 序列化
可保存/读取
字段覆盖 session、goal、todo、tool call、workspace refs、next action
```

### P0-A02：Agent Session Store

内容：

```text
后端 State Store 保存 Agent 会话状态
不能只依赖浏览器 localStorage
daemon 重启后可恢复 session/messages/task/todos/tool_calls/context_refs
```

验收：

```text
Create / Get / Update session 可用
daemon 重启后 GET session 可恢复
缺失 session 有明确 not_found / recreate 行为
```

### P0-A03：Agent Task / Todo Manager

内容：

```text
从用户 prompt 创建 active task
生成 todo list
根据工具结果更新 pending/running/done/blocked
记录 blocked reason 和关联 tool/context
```

验收：

```text
一次 scan 意图能生成至少 4 个 todo
工具完成后 todo 状态变化可观察
session state 返回 active task 和 todos
```

### P0-A04：Agent Tool Loop

内容：

```text
User Message
→ Load Session
→ Understand Goal
→ Update Task/Todo
→ Select Tool
→ Policy Gate
→ Execute Tool
→ Observe
→ Update Todo/Context
→ Next Action
→ Persist
```

验收：

```text
不再只是 prompt keyword → single API route
每次 send message 返回 messages/task/todos/tool_calls/context_refs/next_action
工具失败可标记 blocked 或 fallback
```

### P0-A05：Workspace Context Reader

内容：

```text
读取 runs
读取 run detail/events/artifacts/report/findings/approvals
读取 plugins/doctor/version/health
生成 ContextRef
```

验收：

```text
“分析这个 run/finding/artifact”能绑定对应 context refs
Agent response 可引用 artifact/finding/report 链接
```

---

## 3. P0 任务：Core Runtime 闭环

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

## 4. P0 任务：内置 Web 分析闭环

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

## 5. P1 任务：Local Web Agent Cockpit

内容：

```text
Chat / Execution Stream
Current Task
Todo
Tool Calls
Workspace Context
Approvals
New Task / Stop / Continue
```

验收：

```text
首页优先呈现 Agent 工作台
刷新/切页可恢复 session state
扫描页不展示全局 AI provider
总览页只读，不能启动扫描
设置页统一保存 daemon / AI provider / language / operator
```

---

## 6. P1 任务：增强内置能力

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

## 7. P1 任务：CLI 完整化

```text
scan web 真正执行 Runtime
report 打开/显示报告路径
replay 读取 events.jsonl
plugins list 读取 plugin manifests
doctor 检查插件和外部工具
```

---

## 8. P1 任务：TUI Run Viewer / Agent View

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

Agent-Native 后续扩展：

```text
Session View
Task / Todo View
Tool Calls View
Context Refs View
Approval Control
```

---

## 9. P1 任务：插件系统 MVP

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

## 10. P2 任务：LLM 增强

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

## 11. P2 任务：Desktop / Multi-user Web / Remote API

```text
Local API / Daemon
Wails Desktop
任务历史
报告浏览
配置管理
多用户 Web 平台后置
```

---

## 12. P3 暂缓任务

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

## 13. 推荐实现顺序

```text
1. P0-A01 Agent State Contracts
2. P0-A02 Agent Session Store
3. P0-A03 Agent Task / Todo Manager
4. P0-A04 Agent Tool Loop
5. P0-A05 Workspace Context Reader
6. Local Web Agent Cockpit 显示 Current Task / Todo / Tool Calls / Context
7. P0-01 Core Contracts
8. P0-02 Run Workspace
9. P0-03 State Store
10. P0-04 Event Store
11. P0-05 Scope Guard
12. P0-06 Risk Policy
13. P0-07 Safe Profile Planner
14. P0-08 HTTP Probe
15. P0-09 HTTP Fetcher
16. P0-10 Header Checker
17. P0-11 HTML Extractor
18. P0-12 Evidence/Finding/Report
19. CLI scan web wiring
20. Cookie/Robots/API Hint/Content Discovery
21. Plugin System
22. TUI Run Viewer / Agent View
23. MVP Stabilization
```
