# CyberAgent Agent 架构与实现规划

版本：v0.1  
日期：2026-05-23  
状态：Agent 架构基线已确认；MVP 实现主链路已跑通

---

## 1. 文档目标

本文定义 CyberAgent 应该如何作为一个真正的 Agent 实现，而不是简单的扫描器、脚本集合或 LLM ChatBot。

本文重点回答：

```text
CyberAgent 应该具备哪些能力？
由哪些核心部分组成？
如何把 Agent 工程化思想融合到 Web/API 安全分析场景？
MVP 应该先实现哪些 Agent 能力？
哪些能力后置？
```

参考资料：

```text
Agent-100-Days: https://github.com/flingjie/Agent-100-Days
```

该项目的核心启发包括：

```text
不要把 Prompt 当逻辑层。
工具系统需要清晰边界，否则工具越多 Agent 越混乱。
Agent 必须有明确能力上限和 Human-in-the-Loop 边界。
Agent 需要工具调用、上下文工程、记忆系统、反思、任务分解、Gateway/Runtime、插件与多通道能力。
从 Demo 到平台，关键在于运行时、状态、插件、通道、可观测性和可恢复性。
```

---

## 2. CyberAgent 的 Agent 定义

CyberAgent 的定义：

```text
一个面向授权 Web/API 目标的安全分析 Agent。
它接收目标与范围，生成分析计划，选择内置能力或插件工具执行，
收集和索引证据，基于证据生成 Finding，并输出可复现报告。
```

它不是：

```text
不是单纯 LLM 聊天机器人。
不是单纯扫描器。
不是固定流程脚本。
不是全自动攻击系统。
```

它应该具备：

```text
目标理解
计划生成
工具选择
受控执行
观察与证据收集
结果分析
反思与重排
人类审批
报告生成
状态恢复
插件扩展
多入口复用
```

---

## 3. 从 Agent-100-Days 融合到 CyberAgent 的设计原则

### 3.1 Prompt 不做核心逻辑

原则：

```text
确定性逻辑写在 Go Core / Policy / State Machine 中。
Prompt 只用于解释、总结、辅助分析和报告增强。
```

落地到 CyberAgent：

```text
Scope 校验不能交给 LLM。
风险等级判断不能只靠 Prompt。
是否执行高风险步骤不能由模型自由决定。
工具参数校验必须由 schema 和 policy 完成。
状态转换必须由 Scheduler / State Machine 控制。
```

### 3.2 工具越多，越需要工具治理

原则：

```text
工具不是越多越好，而是要有能力边界、使用条件、风险等级和输出 schema。
```

落地到 CyberAgent：

```text
Tool Registry 负责工具发现和能力筛选。
Planner 每一步只暴露当前可用的一小组候选工具。
Tool Plugin 必须声明输入、输出、风险、依赖和 artifact policy。
LLM 如参与工具选择，只能在 shortlist 中选择，不能直接访问全部工具池。
```

### 3.3 能力上限必须显式声明

原则：

```text
Agent 要知道自己能做什么、不能做什么、什么情况下需要人。
```

落地到 CyberAgent：

```text
低风险动作可自动执行。
中风险动作可按 profile 自动或确认。
高风险动作必须确认。
破坏性动作默认禁止。
无 scope 不主动扫描。
```

### 3.4 上下文工程优先于“塞更多信息”

原则：

```text
LLM 不应该直接读取全部 raw evidence，而应读取经过裁剪、摘要、索引后的 context pack。
```

落地到 CyberAgent：

```text
原始证据落盘。
结构化 evidence 入索引。
LLM 只读取当前 step 所需摘要、关键响应片段、finding 候选和 policy。
大 HTML / JS / 工具日志通过 parser 先结构化。
```

### 3.5 记忆系统必须分层和隔离

原则：

```text
记忆不是聊天记录堆积，而是可检索、可过期、可隔离的结构化状态。
```

落地到 CyberAgent：

```text
Run Memory：当前任务状态、工具调用、证据摘要。
Target Memory：同一授权目标的历史发现和技术指纹。
Tool Memory：工具成功率、失败原因、版本差异。
User Preference：输出格式、profile、常用 scope，不存敏感凭据。
```

MVP 只实现 Run Memory；Target Memory、Tool Memory、User Preference 后置。

### 3.6 反思不是“多问一次模型”，而是质量门

原则：

```text
Reflection 应该有触发条件、输入、输出和决策动作。
```

落地到 CyberAgent：

```text
工具失败后反思：重试、跳过、换工具、降级。
证据冲突时反思：标记不确定、补充验证、降低置信度。
报告前反思：检查 finding 是否都有证据和复现步骤。
高风险步骤前反思：判断是否必须进入 HITL。
```

### 3.7 HITL 是系统设计的一部分

原则：

```text
人在关键节点参与审批、修正或接管，不是事后救火。
```

落地到 CyberAgent：

```text
Approval：高风险步骤执行前审批。
Correction：用户修正 scope、认证信息、误报 finding。
Takeover：用户接管某一步，Agent 暂停并记录人工结果。
```

### 3.8 Gateway + Runtime + Plugin + Channel 思维

原则：

```text
Agent 平台不是单入口程序，而是多通道接入同一个 Runtime。
```

落地到 CyberAgent：

```text
CLI / TUI / Desktop / Web 都是 Channel。
Gateway 负责接收请求、规范化输入、路由到 Runtime。
Runtime 负责计划、工具调用、状态、证据和事件。
Plugin 提供工具、分析、报告、LLM Provider 等扩展。
```

---

## 4. CyberAgent 能力模型

### 4.1 感知能力 Perception

Agent 要能感知和结构化输入：

```text
用户目标
授权 scope
扫描 profile
目标可达性
HTTP 响应
HTML / JS / API 线索
工具输出
历史状态
人工审批结果
```

MVP 能力：

```text
URL 解析
scope 校验
HTTP probe
HTTP fetch
HTML/JS 轻量提取
工具输出读取
```

### 4.2 规划能力 Planning

Agent 要能把目标拆成可执行步骤：

```text
目标规范化
执行计划生成
步骤依赖关系
步骤风险评级
工具候选选择
失败后的重排
```

MVP 先使用确定性 planner：

```text
web-safe-profile → 固定基础计划 + 按工具可用性增减步骤
```

后续再加入 LLM 辅助：

```text
基于 evidence 解释下一步建议
基于失败原因建议换工具
基于目标特征补充分析路径
```

### 4.3 工具选择能力 Tool Selection

Agent 要基于能力、风险、依赖和 profile 选择工具。

```text
Tool Registry 查询能力
Dependency Doctor 检查工具是否可用
Policy Engine 判断是否允许
Planner 选择当前步骤最合适工具
```

工具选择优先级：

```text
内置轻量能力 > 已安装低风险插件 > 需要确认的插件 > 禁止执行
```

### 4.4 执行能力 Execution

所有执行必须可控：

```text
timeout
cancel
retry
rate limit
per-host concurrency
stdout/stderr capture
artifact capture
commands.log
events.jsonl
```

MVP 执行路径：

```text
Step → Policy Check → Runner → Builtin Tool / Tool Plugin → Artifact Store → Event Bus
```

### 4.5 观察与证据能力 Observation & Evidence

Agent 不能只输出结论，必须输出证据链。

```text
每个 Step 产生 artifacts。
每个 artifact 生成 hash 和 metadata。
每个 evidence 指向 artifact 和来源 step。
每个 finding 必须引用 evidence。
```

### 4.6 分析能力 Analysis

Agent 要把原始数据转为可用安全结论：

```text
响应头问题
Cookie 安全属性问题
公开路径或敏感入口
JS/API 路由线索
工具扫描 finding
证据冲突与置信度
风险等级和修复建议
```

### 4.7 反思能力 Reflection

反思用于质量控制和计划修正。

触发点：

```text
步骤失败
工具输出为空或异常
证据冲突
高风险步骤前
报告生成前
finding 置信度低
```

输出动作：

```text
continue
retry
skip
replan
request_approval
request_correction
mark_uncertain
```

### 4.8 Human-in-the-Loop 能力

HITL 触发条件：

```text
高风险动作
scope 模糊
认证状态不明确
可能改变目标状态
证据不足但影响较大
用户配置为 interactive profile
```

TUI 中应支持：

```text
Approve
Reject
Modify parameters
Skip
Take over
```

### 4.9 记忆能力 Memory

MVP：

```text
Run Memory：当前 job 的计划、步骤、事件、工具调用、证据摘要。
```

后续：

```text
Target Memory：目标历史指纹和发现。
Tool Memory：工具版本、失败模式、输出解析经验。
User Preference：报告风格、默认 profile、常用 scope。
```

### 4.10 上下文能力 Context Engineering

LLM context pack 应由 Context Builder 生成。

每次 LLM 调用只包含：

```text
任务目标
授权 scope 摘要
当前 plan / step
相关 evidence 摘要
工具候选 shortlist
risk policy
期望 JSON schema
```

禁止默认包含：

```text
全部原始 HTML
全部 JS
全部工具日志
完整 cookie/token
无关历史任务
```

### 4.11 RAG / 知识能力

MVP 不做复杂 RAG。

后续可引入：

```text
安全知识库
漏洞知识库
工具说明知识库
报告模板知识库
历史 finding 知识库
```

用途：

```text
解释漏洞影响
补充修复建议
匹配 CWE/OWASP 分类
帮助报告标准化
```

### 4.12 报告能力 Reporting

报告必须可读、可复现、可追溯。

MVP 输出：

```text
report.md
findings.json
evidence/
events.jsonl
commands.log
```

---

## 5. 系统组成

### 5.1 Channel / Interface Layer

入口层：

```text
CLI       自动化、脚本、CI/CD、远程环境
TUI       主要交互界面，查看计划、日志、证据、Finding、审批
Desktop   后置，本地工作台
Web       后置，团队协作与远程 Runner
```

职责：

```text
接收用户输入
展示实时事件
发起 HITL 决策
查看证据和报告
不承载核心业务逻辑
```

### 5.2 Gateway Layer

Gateway 是多入口的统一控制面，MVP 可先是内部模块，后续变成 Local API。

职责：

```text
规范化请求
创建 Job
加载配置
绑定用户会话或本地会话
路由到 Agent Runtime
向 Channel 返回事件和结果
```

### 5.3 Agent Runtime

Runtime 是核心执行循环。

职责：

```text
组装上下文
生成或加载 plan
执行 step
调用工具
处理事件
更新状态
触发反思
触发 HITL
生成报告
```

### 5.4 Planner Runtime

职责：

```text
根据 target/profile/tool capability 生成 plan
把复杂目标拆成 Step
标注风险等级和依赖
根据执行结果重排或跳过步骤
```

### 5.5 Policy Engine

职责：

```text
Scope Guard
Risk Policy
HITL Trigger
Rate Policy
Forbidden Action Policy
Credential Handling Policy
Evidence Redaction Policy
```

### 5.6 Tool System

组成：

```text
Tool Registry
Builtin Tools
Tool Plugin Interface
Plugin Manager
Dependency Doctor
Command Executor
Runner
```

核心原则：

```text
Core 不直接写死外部工具命令。
插件不能绕过 Command Executor。
工具调用必须可审计、可取消、可复现。
```

### 5.7 Runner Layer

MVP：

```text
Local Runner
```

后续：

```text
Docker Runner
Remote Runner
K8s Runner
```

### 5.8 Storage Layer

组成：

```text
State Store
Event Store
Artifact Store
Evidence Index
Finding Store
Report Store
```

MVP 存储：

```text
runs/{job_id}/
  target.json
  scope.json
  plan.json
  state/bolt.db
  events.jsonl
  commands.log
  evidence/
  findings.json
  report.md
```

### 5.9 LLM Enhancement Layer

LLM 不作为 Core 必需依赖。

能力：

```text
Analyzer Plugin
Reporter Plugin
Reflection Assistant
Plan Explanation Assistant
Remediation Suggestion Assistant
```

边界：

```text
不能绕过 scope。
不能直接执行工具。
不能决定高风险动作自动执行。
不能读取未脱敏敏感信息。
输出必须符合 schema 并经过验证。
```

---

## 6. Agent Runtime 执行循环

推荐循环：

```text
1. Receive Request
2. Normalize Target & Scope
3. Create Job Workspace
4. Build Initial Plan
5. Preflight / Doctor
6. For each Step:
   6.1 Check Scope
   6.2 Check Risk Policy
   6.3 If needed, request HITL approval
   6.4 Select Builtin Tool or Plugin
   6.5 Execute via Runner
   6.6 Capture artifacts
   6.7 Parse evidence
   6.8 Update state and events
   6.9 Trigger reflection if needed
   6.10 Replan / retry / skip if needed
7. Normalize Findings
8. Report Reflection Gate
9. Generate Report
10. Save and Return Summary
```

---

## 7. MVP Agent 能力边界

### 7.1 MVP 必须具备

```text
确定性 Web safe profile planner
Scope Guard
Run Manager
State Store
Event Bus
Command Recorder
Policy Engine
Local Runner
Builtin HTTP Probe / Fetcher
Header / Cookie Checker
HTML Extractor
轻量 JS/API Hint Detector
Evidence Store
Finding Normalizer
Markdown + JSON Report
Doctor
CLI 启动入口
TUI 基础观察入口，可分阶段实现
```

### 7.2 MVP 可选具备

```text
nuclei 低风险插件
ffuf 低速目录枚举插件
Python Analyzer 插件
LLM Reporter 插件
bbolt state store
```

### 7.3 MVP 不做

```text
复杂 RAG
长期记忆
多用户 Web
Desktop
远程 Runner
自动 exploit
自动爆破
自动横向移动
企业资产监控
```

---

## 8. 实现路线建议

### Phase A：核心契约

产出：

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
```

### Phase B：Runtime 骨架

实现：

```text
Run Manager
State Store
Event Bus
Logger
Policy Engine
Command Recorder
Local Runner
```

### Phase C：内置 Web 分析闭环

实现：

```text
Scope Matcher
HTTP Probe
HTTP Fetcher
Header Checker
Cookie Checker
HTML Extractor
API Hint Detector
Report Generator
```

目标：

```text
输入 URL → 执行 safe plan → 保存 evidence → 生成 findings.json/report.md
```

### Phase D：插件系统

实现：

```text
Plugin Manifest
Tool Registry
Plugin Manager
subprocess + JSON/JSONL 插件协议
Dependency Doctor
```

首批插件建议：

```text
nuclei-safe
ffuf-low-rate
python-js-route-analyzer
```

### Phase E：TUI

实现：

```text
Plan View
Events View
Evidence View
Findings View
HITL Approval View
Report View
```

### Phase F：LLM 增强

实现：

```text
LLM Gateway
Context Builder
Analyzer Plugin
Reporter Plugin
Reflection Assistant
```

### Phase G：多入口复用

实现：

```text
Local API / Daemon
Desktop/Wails
Web/Remote Runner
```

---

## 9. 关键设计约束

```text
确定性逻辑优先，LLM 只增强。
没有 scope 不主动扫描。
所有外部命令必须经 Runner。
所有命令进入 commands.log。
所有重要输出进入 evidence/。
所有 finding 必须引用 evidence。
所有高风险动作必须 HITL。
所有插件必须声明能力、依赖、风险和 schema。
所有 Channel 不承载核心逻辑，只调用 Runtime。
所有大上下文进入 LLM 前必须摘要、裁剪、脱敏。
```

---

## 10. 已确认架构问题

```text
1. MVP Release 必须包含 TUI v1 只读 run viewer；CLI safe scan 可以作为更早里程碑先跑通。
2. MVP 默认不启用 nuclei-safe；仅在用户显式启用 optional plugin 时运行。
3. State Store 使用 bbolt 嵌入式 KV；SQLite 在需要复杂查询、多任务过滤、Web 团队化后再评估。
4. LLM Gateway 第一版只定义接口、ContextPack 和插件边界，不默认接入 Provider。
5. Reflection 第一版采用规则实现；LLM Reflection 后置为可选增强。
6. MVP 只做 Run Memory；Target Memory 放到 v1 稳定期，User/Team Memory 放到 Desktop/Web/Remote Runner 阶段。
```

规划约束：所有 Channel 只调用 Runtime，不承载核心安全逻辑；所有智能增强都不能绕过 Policy / Evidence / Event / Command Recorder。
