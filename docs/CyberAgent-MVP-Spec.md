# CyberAgent MVP Spec

版本：v0.3-agent-native-correction  
日期：2026-05-25  
状态：MVP 规格纠偏；Agent-Native 工作流进入 MVP 验收边界

---

## 1. 文档目标

本文把 CyberAgent 的首个可交付版本收敛为明确的 MVP 规格。

MVP 目标：

```text
给定一个明确授权的 Web URL，CyberAgent 能在 safe profile 下完成一次低风险 Web/API 安全分析闭环：
创建任务 → 生成计划 → 执行内置分析步骤 → 可选调用插件 → 保存证据 → 生成 Finding → 输出报告。
```

同时，MVP 必须体现 Agent 原生工作方式：

```text
Chat → Current Task → Todo / Plan → Tool Call → Observation → Context Update → Report / Next Action
```

不能只交付普通扫描控制台或聊天壳。

本文定义：

```text
用户故事
MVP 边界
CLI 命令设计
TUI 页面草案
safe profile 执行步骤
内置工具清单
可选插件清单
输出目录规范
验收标准
非目标
已决问题
```

---

## 2. MVP 总体结论

MVP 已确认：

```text
产品形态：CLI + 基础 TUI + Local Web Agent Cockpit
核心语言：Go
插件语言：Python / 外部命令
目标类型：web_url
执行环境：Local Runner
状态存储：bbolt 嵌入式 KV + JSONL 事件流 + Agent Session State，SQLite 后置评估
LLM：只定义 Gateway / ContextPack，不默认启用
插件：nuclei-safe 可选但默认不启用；ffuf-low-rate 可选；Python JS analyzer 可选
报告：Markdown + JSON
证据：artifact + evidence index
风险策略：低风险自动，中高风险确认，破坏性禁止
Agent 能力：session / message / active task / todo / tool call / context refs 持久化
```

---

## 3. MVP 用户画像

### 3.1 首期用户

```text
个人安全研究员
CTF / 靶场玩家
授权 Web/API 测试人员
```

### 3.2 用户能力假设

用户应具备：

```text
会使用命令行
知道目标是否授权
理解 URL / HTTP / 状态码 / 请求响应
能阅读 Markdown 报告
能判断 Finding 是否需要进一步验证
```

### 3.3 非首期用户

MVP 暂不优先面向：

```text
普通非技术用户
企业安全管理者
大型多用户安全团队
只需要可视化报表的客户
```

---

## 4. MVP 核心用户故事

### Story 1：创建一次授权 Web 分析任务

```text
作为安全研究员，
我希望输入一个 Web URL 和授权 scope，
让 Agent 自动创建任务并生成执行计划，
以便我确认它会做什么。
```

验收：

```text
能创建 job workspace。
能生成 target.json / scope.json / job.json / plan.json。
如果没有 scope，拒绝主动扫描。
```

### Story 2：执行低风险 Web 分析

```text
作为授权测试人员，
我希望 Agent 自动执行低风险检查，
包括可达性、响应头、cookie、HTML 链接、JS/API 线索，
以减少重复手工操作。
```

验收：

```text
能完成 HTTP probe / fetch / header check / cookie check / html extraction。
所有步骤写入 events.jsonl。
所有原始产物写入 artifacts/。
所有证据写入 evidence/。
```

### Story 3：查看实时执行状态

```text
作为用户，
我希望在 CLI/TUI 中看到当前任务进度、步骤状态和关键日志，
以便判断任务是否正常。
```

验收：

```text
CLI 能输出步骤进度。
TUI 至少能查看 Plan / Events / Findings / Report。
```

### Story 4：生成可复现报告

```text
作为测试人员，
我希望 Agent 输出 Markdown 和 JSON 报告，
每个 Finding 都包含证据、影响、复现方式和修复建议。
```

验收：

```text
生成 reports/report.md。
生成 reports/report.json 或 findings.json。
Finding 必须引用 evidence_id。
```

### Story 5：工具缺失时可降级

```text
作为用户，
我希望即使没有安装 nuclei / ffuf 等外部工具，
Agent 也能完成内置低风险分析，
并告诉我缺少哪些增强工具。
```

验收：

```text
cyberagent doctor 能检查依赖。
外部工具缺失时不影响内置流程。
报告中说明哪些可选插件被跳过。
```

### Story 6：中高风险动作需要确认

```text
作为用户，
我希望 Agent 在执行高风险动作前暂停并请求确认，
避免误操作授权目标。
```

验收：

```text
Step risk 为 high 时进入 waiting_approval。
用户可 approve / reject / skip。
审批结果写入 events.jsonl。
```

### Story 7：像 Claude Code / Codex 一样保持 Agent 任务会话

```text
作为用户，
我希望 Agent 能保持当前安全任务、聊天上下文、计划、Todo、工具调用和工作区引用，
以便刷新页面、切换菜单或重启 daemon 后仍可继续工作。
```

验收：

```text
Agent session 持久化到 daemon State Store。
messages / active task / todos / tool calls / context refs 可恢复。
浏览器 localStorage 只能作为 UI 兜底缓存，不能作为唯一状态源。
后端 session 丢失时，前端不丢聊天历史，后端可重建并继续。
```

### Story 8：通过 Agent Tool Loop 执行安全任务

```text
作为用户，
我希望用自然语言指挥 Agent，而不是手动理解每个 API 或页面，
Agent 自动规划、调用工具、观察结果、更新 Todo，并给出下一步。
```

验收：

```text
每次用户任务生成 Current Task。
Agent 返回结构化 Todo。
每次工具调用返回 ToolCall 记录。
ToolCall 结果更新 Todo / ContextRefs。
UI 展示 Current Task / Todo / Tool Calls / Workspace Context。
```

---

## 5. MVP 非目标

MVP 不做：

```text
多用户 Web 平台
Desktop App
远程 Runner
Docker Runner
K8s Runner
复杂 RAG
长期 Target Memory
自动 exploit
自动爆破
自动横向移动
攻击持久化
破坏性 payload
企业资产持续监控
PDF 报告
自动创建 Jira/GitHub Issue
源码审计完整流程
认证态浏览器自动化完整流程
```

---

## 6. CLI 命令设计

### 6.1 任务扫描

```bash
cyberagent scan web \
  --target http://localhost:8080 \
  --scope localhost,127.0.0.1 \
  --profile safe
```

可选参数：

```text
--out runs/custom_job_id
--no-external-tools
--use-plugin nuclei-safe
--rate-limit 2
--timeout 1800
--interactive
--non-interactive
--format markdown,json
```

行为：

```text
解析 target。
解析 scope。
创建 job workspace。
生成 safe profile plan。
执行内置步骤。
可选执行已启用插件。
生成报告。
```

### 6.2 TUI 启动

```bash
cyberagent tui
```

或者打开指定任务：

```bash
cyberagent tui runs/job_20260523_032000_a1b2c3
```

### 6.3 查看报告

```bash
cyberagent report runs/job_20260523_032000_a1b2c3
```

可选：

```bash
cyberagent report runs/job_20260523_032000_a1b2c3 --format markdown
cyberagent report runs/job_20260523_032000_a1b2c3 --format json
```

### 6.4 任务回放

```bash
cyberagent replay runs/job_20260523_032000_a1b2c3
```

MVP replay 只要求：

```text
按 events.jsonl 展示执行过程。
不要求重新执行网络请求。
```

### 6.5 环境检查

```bash
cyberagent doctor
```

检查：

```text
Core 版本
配置目录
runs 目录可写性
Python 是否可用
已安装插件
nuclei / ffuf / nmap 等可选工具是否可用
默认模板/字典是否存在
```

### 6.6 插件列表

```bash
cyberagent plugins list
cyberagent plugins doctor
```

MVP 可只实现只读列表，不实现插件安装市场。

---

## 7. TUI 页面草案

MVP TUI 可以先做只读观察 + 简单审批。

### 7.1 总体布局

```text
┌─────────────────────────────────────────────────────────────┐
│ CyberAgent | job_20260523_032000_a1b2c3 | running | safe     │
├───────────────┬─────────────────────────────────────────────┤
│ Sidebar       │ Main View                                   │
│               │                                             │
│ Plan          │ selected content                            │
│ Events        │                                             │
│ Evidence      │                                             │
│ Findings      │                                             │
│ Report        │                                             │
│ Approval      │                                             │
└───────────────┴─────────────────────────────────────────────┘
```

### 7.2 Plan View

显示：

```text
Step ID
Step Name
Status
Risk
Tool
Duration
Dependencies
```

### 7.3 Events View

显示：

```text
时间
事件类型
步骤
级别
消息
```

### 7.4 Evidence View

显示：

```text
Evidence ID
Type
Title
Target Ref
Source Step
Artifact Path
Confidence
```

### 7.5 Findings View

显示：

```text
Severity
Title
Confidence
Affected Target
Evidence Count
Source Tool
```

### 7.6 Report View

显示：

```text
报告路径
报告摘要
打开/导出提示
```

### 7.7 Approval View

当有 HITLRequest：

```text
风险等级
请求动作
可能影响
参数摘要
Approve / Reject / Modify / Skip
```

MVP 可先支持：

```text
Approve
Reject
Skip
```

Modify 后置。

---

## 8. Safe Profile 执行步骤

MVP safe profile 默认计划：

```text
1. scope_check
2. target_normalize
3. job_workspace_init
4. doctor_preflight
5. http_probe
6. http_fetch_homepage
7. header_check
8. cookie_check
9. robots_fetch
10. sitemap_fetch
11. html_extract
12. js_asset_collect
13. js_route_extract_light
14. api_hint_detect
15. content_discovery_small
16. optional_nuclei_safe
17. evidence_index
18. finding_normalize
19. report_reflection_gate
20. report_generate
```

### 8.1 Step 风险分级

```text
scope_check                 info
job_workspace_init          info
doctor_preflight            info
http_probe                  low
http_fetch_homepage         low
header_check                info
cookie_check                info
robots_fetch                low
sitemap_fetch               low
html_extract                info
js_asset_collect            low
js_route_extract_light      info
api_hint_detect             info
content_discovery_small     low / medium，取决于请求数和速率
optional_nuclei_safe        medium，默认可配置是否确认
evidence_index              info
finding_normalize           info
report_reflection_gate      info
report_generate             info
```

### 8.2 Step 执行策略

```text
所有网络请求必须经过 Scope Guard。
所有网络请求必须经过 Rate Limiter。
所有重定向目标必须再次校验 Scope。
所有步骤必须写 Event。
所有原始响应必须成为 Artifact 或被明确丢弃并记录原因。
所有 Finding 必须引用 Evidence。
```

---

## 9. MVP 内置工具清单

### 9.1 Agent 基础工具

MVP 必须内置：

```text
Config Manager
Run Manager
State Store
Event Bus
Logger
Command Executor
Command Recorder
Policy Engine
Planner Runtime
Tool Registry
Rate Limiter
Retry/Timeout
Plugin Manager
Dependency Doctor
Artifact Store
Evidence Indexer
Redactor
Template Renderer
Cache/Dedup
```

### 9.2 内置安全能力

MVP 必须内置：

```text
Scope Matcher
HTTP Probe
HTTP Fetcher
Header Checker
Cookie Checker
Robots/Sitemap Fetcher
HTML Extractor
JS Route Extractor Light
API Hint Detector
Content Discovery Small
Evidence Hasher
Report Generator
Doctor
```

MVP 可选内置：

```text
TLS Inspector
TCP Port Check
```

---

## 10. MVP 插件清单

### 10.1 可选插件：nuclei-safe

用途：

```text
运行低风险 nuclei 模板，输出 nuclei.jsonl，并归一化为 Evidence/Finding。
```

默认策略：

```text
默认不启用。
用户需要通过 --use-plugin nuclei-safe 或配置文件显式启用。
```

要求：

```text
nuclei 缺失时跳过。
只允许 safe profile 允许的模板集合。
不得运行 destructive / fuzzing / intrusive 标签模板。
输出必须落盘。
```

### 10.2 可选插件：ffuf-low-rate

用途：

```text
低速目录发现，替代或增强内置 Content Discovery Small。
```

要求：

```text
请求数和速率受 Scope limits 限制。
中风险时需要确认。
```

### 10.3 可选插件：python-js-route-analyzer

用途：

```text
对 JS 进行更复杂的路由/API endpoint 提取。
```

要求：

```text
只读取当前 job workspace 内已保存 JS artifact。
不直接访问网络。
输出 JSON/JSONL。
```

---

## 11. 输出目录规范

MVP 输出：

```text
runs/{job_id}/
  target.json
  scope.json
  job.json
  plan.json
  state/
    bolt.db
  events.jsonl
  commands.log
  artifacts/
    stdout/
    stderr/
    http/
    tool-results/
    screenshots/
  evidence/
    evidence.jsonl
    index.json
  findings.json
  reports/
    report.md
    report.json
```

### 11.1 必须存在

```text
target.json
scope.json
job.json
plan.json
events.jsonl
commands.log
evidence/index.json
findings.json
reports/report.md
```

### 11.2 可选存在

```text
artifacts/tool-results/nuclei.jsonl
artifacts/tool-results/ffuf.json
artifacts/http/homepage.html
reports/report.json
```

---

## 12. Finding 输出要求

每个 Finding 必须包含：

```text
id
title
severity
confidence
category
affected_targets
evidence_ids
description
impact
reproduction
remediation
source.tool
source.step_id
created_at
```

### 12.1 MVP 典型 Finding

```text
Missing Security Header
Insecure Cookie Attribute
Exposed robots.txt Interesting Path
Sitemap Reveals Hidden Route
Interesting API Endpoint Found
Directory Discovery Result
Nuclei Template Finding
```

注意：

```text
“发现 API endpoint”默认是 info，不直接认定漏洞。
“缺少安全响应头”通常是 low 或 info，取决于上下文。
```

---

## 13. 报告结构

`reports/report.md` 推荐结构：

```text
# CyberAgent Report

## 1. Summary
- Target
- Scope
- Profile
- Time
- Overall Result

## 2. Execution Overview
- Steps executed
- Steps skipped
- Tools used
- Optional plugins unavailable

## 3. Findings
- Severity
- Title
- Confidence
- Affected Target
- Evidence
- Reproduction
- Impact
- Remediation

## 4. Evidence Index
- Evidence ID
- Type
- Artifact Path

## 5. Commands and Reproducibility
- commands.log path
- replay instructions

## 6. Limitations
- Scope limits
- Skipped high-risk checks
- Missing optional tools
```

---

## 14. Policy 与风险验收

MVP 必须满足：

```text
无 scope 不主动扫描。
请求目标超出 scope 时拒绝。
重定向超出 scope 时停止跟随并记录。
高风险 step 不自动执行。
forbidden step 永不执行。
外部命令只能通过 Command Executor。
插件不能绕过 Runner。
敏感字段必须脱敏后进入报告。
```

---

## 15. LLM MVP 边界

MVP 只定义：

```text
LLM Gateway Interface
ContextPack Contract
LLM Plugin 类型
```

MVP 默认不启用：

```text
自动 LLM 规划
自动 LLM 工具调用
自动 LLM exploit 生成
自动 LLM 高风险决策
```

允许后续增强：

```text
报告语言润色
Finding 影响解释
修复建议扩展
工具输出摘要
```

---

## 16. 验收标准

### 16.1 基础闭环验收

给定：

```bash
cyberagent scan web --target http://localhost:8080 --scope localhost --profile safe
```

应产生：

```text
runs/{job_id}/target.json
runs/{job_id}/scope.json
runs/{job_id}/job.json
runs/{job_id}/plan.json
runs/{job_id}/events.jsonl
runs/{job_id}/commands.log
runs/{job_id}/evidence/index.json
runs/{job_id}/findings.json
runs/{job_id}/reports/report.md
```

### 16.2 Scope 验收

```text
没有 --scope 时拒绝主动扫描。
target 不在 scope 时拒绝执行。
重定向超出 scope 时停止并记录 warning event。
```

### 16.3 证据验收

```text
每个 Finding 至少引用一个 Evidence。
每个 Evidence 指向 Artifact 或明确说明是派生证据。
Artifact 有 path、hash、size、source step。
```

### 16.4 事件验收

```text
每个 Step 至少产生 started 和 finished/failed/skipped 事件。
Policy check 必须产生 event。
Report 生成必须产生 event。
```

### 16.5 降级验收

```text
nuclei 未安装时，doctor 能报告缺失。
scan 不因 nuclei 缺失失败。
报告中记录 optional_nuclei_safe skipped。
```

### 16.6 TUI 验收

```text
能打开已有 run。
能查看 plan。
能查看 events。
能查看 findings。
能看到 report 路径。
```

---

## 17. MVP 里程碑

### Milestone 1：契约冻结

产出：

```text
Core Contracts 确认
MVP Spec 确认
safe profile 确认
```

### Milestone 2：Runtime 骨架设计完成

产出：

```text
模块边界
包结构规划
状态机草案
事件流草案
```

### Milestone 3：内置 Web 分析闭环设计完成

产出：

```text
内置工具接口
safe profile step schema
report template
```

### Milestone 4：插件协议设计完成

产出：

```text
Plugin Manifest schema
subprocess JSON/JSONL protocol
nuclei-safe 插件规格
```

### Milestone 5：TUI 设计完成

产出：

```text
页面结构
键位设计
事件订阅方式
审批交互方式
```

---

## 18. 已确认实现决策

```text
Go module: github.com/H1d3rOne/cyberagent
CLI framework: Cobra
TUI framework: Bubble Tea / Lip Gloss
MVP State Store: bbolt 嵌入式 KV + JSONL 事件流 + 文件系统 artifacts
SQLite: MVP 阶段跳过；后续按需求评估
optional_nuclei_safe: 默认不启用，需要用户显式启用
TUI v1: 只读打开已有 run；创建任务、实时控制、审批后置
```

---

## 19. 已决问题

```text
1. Content Discovery Small 默认内置 32 个常见低风险路径；safe profile 下最多允许用户扩展到 64 个路径。
2. report.json 与 findings.json 分开：findings.json 保存归一化 Finding 列表，reports/report.json 保存报告级元数据、摘要、Finding 引用和导出结构。
3. commands.log 采用 JSONL 记录所有工具调用：内置工具记录 kind=builtin 的伪命令/参数摘要，外部工具记录 kind=external 的真实 argv/cwd/env 摘要；敏感字段必须脱敏。
4. MVP v0.1 不默认支持持久化 auth header/cookie 输入；认证态扫描后置到 v0.2，并必须经过 Redactor 与本地安全存储设计。
5. safe profile 默认 max_total_requests=100，per_host_concurrency=2，per_host_rps=1，redirect_max=5，单响应默认最大 2 MiB。
```

---

## 20. 规格完成状态

本文档已确认 MVP 规格。后续只在以下情况下更新：

```text
需求范围变更
安全策略变更
输出目录结构变更
验收标准变更
```

进入实现前应以本文档、Core Contracts、Implementation Blueprint、Task Breakdown 和 Test Plan 为准。
