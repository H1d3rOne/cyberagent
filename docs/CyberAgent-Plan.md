# CyberAgent 总体计划

版本：v0.4-agent-native-correction  
日期：2026-05-25  
状态：规划基线纠偏；CyberAgent 必须优先满足 Claude Code / Codex 型可执行 Agent 能力

---

## 1. 项目定位

CyberAgent 是一个面向**授权目标**的 Web/API 安全分析 Agent。

核心目标：

```text
减少人工安全测试中的重复操作，
让 Agent 自动完成信息收集、分析、证据整理、报告生成，
同时保留人工确认和可控边界。
```

首期重点不是“全自动攻击”，而是：

```text
自动规划 → 调用工具 → 分析结果 → 形成证据链 → 输出报告
```

更准确地说，CyberAgent 不是“安全后台加聊天框”，而是：

```text
面向授权 Web/API 目标的 Cybersecurity Agent Workbench。

它应具备 Claude Code / Codex 类 Agent 的基础形态：
会话保持、任务目标、计划/Todo、工具调用循环、工作区上下文、可打断/继续、错误恢复、状态持久化。
```

Agent 原生能力是最高优先级验收约束，详见：

```text
docs/CyberAgent-Agent-Native-Benchmark.md
```

---

## 2. 产品形态规划

当前推荐路线：

```text
Go Core + CLI/TUI Agent Runtime + Local Web Agent Cockpit + Python Plugin Layer + Desktop Later
```

阶段顺序：

```text
第一阶段：CLI / TUI
第二阶段：Local API / Daemon
第三阶段：Local Web Agent Cockpit
第四阶段：Desktop
第五阶段：多用户 Web / Remote Runner
```

### 形态分工

| 形态 | 定位 | 优先级 |
|---|---|---|
| CLI | 自动化、脚本化、CI/CD、CTF、远程环境 | 高 |
| TUI | Agent 原生终端工作台，查看任务、Todo、工具调用、证据、确认操作 | 高 |
| Local Web | Agent Cockpit，降低操作门槛，承载 Chat / Todo / Tool Loop / Workspace Context | 高 |
| Desktop | 简化操作，面向非命令行用户，本地安全工作台 | 中 |
| Multi-user Web | 团队协作、多任务、远程 Runner、报告平台 | 后置 |

---

## 3. 技术路线初判

### 核心语言

推荐：

```text
Go 作为 Agent Core
Python 作为插件和分析生态层
```

### Go 负责

```text
任务调度
并发控制
工具执行
CLI/TUI
本地 API
事件流
证据管理
跨平台分发
```

### Python 负责

```text
安全工具适配
数据分析
JS/API 提取
OpenAPI 分析
报告增强
LLM 辅助分析
生态扩展
```

### 暂定技术栈

```text
Core: Go
CLI: Cobra
TUI: Bubble Tea / Lip Gloss
Plugin: Python subprocess + JSON 协议
Storage: bbolt 嵌入式 KV + JSONL 事件流 + 文件系统 artifacts；SQLite 后置评估
Report: Markdown + JSON，后续 HTML/PDF
Desktop: Wails 优先考虑
Web/API: 后续 Local API / Remote API 复用 Core，具体方案待定
```

---

## 4. 总体架构草案

```text
CLI / TUI / Desktop / Web
          ↓
   Interface Adapter
          ↓
      Agent Core
          ↓
  Job Orchestrator
          ↓
      Runner Layer
          ↓
 Tool Plugins / External Tools
          ↓
 Evidence Store / Report
```

核心模块：

```text
Agent Core
  ├── Planner Runtime      任务规划、计划更新、步骤跳过/重排
  ├── Agent Session        会话、消息、active task、todo、tool call、context refs 持久化
  ├── Agent Tool Loop      plan → tool call → observe → update todo → next action
  ├── Scheduler            步骤调度、并发控制、取消/重试
  ├── Policy Engine        Scope Guard、Risk Policy、确认策略、禁止动作策略
  ├── Runner Layer         Local / Docker / Remote 执行抽象
  ├── Command Executor     受控命令执行，统一 timeout / env / cwd / stdout / stderr
  ├── Tool Registry        内置工具、外部工具、插件能力统一注册
  ├── Plugin Runtime       Python / 外部进程插件运行
  ├── Event Bus            实时事件，写入 events.jsonl，供 CLI/TUI/API 订阅
  ├── State Store          Job / Step 状态持久化
  ├── Agent State Store    Agent session / task / todo / tool call / context ref 持久化
  ├── Artifact Store       原始输出、HTML、JSONL、截图、工具产物落盘
  ├── Evidence Store       证据索引、hash、来源 step、关联 finding
  ├── Finding Analyzer     结果归一化、去重、风险评级、置信度
  ├── Reporter             Markdown / JSON 报告生成
  ├── Redactor             token、cookie、authorization header 等敏感字段脱敏
  └── Doctor               外部工具、Python 环境、插件依赖检查
```

工具能力分层：

```text
Agent 基础工具层：保证任务、状态、事件、证据、插件、执行可控
内置安全能力层：提供轻量、低风险、零依赖的 Web/API 分析原语
外部安全工具层：通过插件接入 nmap / ffuf / nuclei / Playwright 等专业工具
LLM 增强层：作为 Analyzer / Reporter 插件，不作为 Core 必需依赖
```

---

## 5. 核心对象设计方向

后续需要优先确定这些数据契约：

```text
AgentSession 会话
AgentMessage 消息
AgentTask    当前任务
AgentTodo    计划/Todo
AgentContextRef 工作区上下文引用
Target      目标
Scope       授权范围
Job         任务
Plan        执行计划
Step        执行步骤
ToolCall    工具调用
Evidence    证据
Finding     发现
Report      报告
Event       事件
Plugin      插件
Runner      执行器
```

这些对象一旦稳定，CLI、TUI、Desktop、Web 都可以复用。

---

## 6. MVP 范围建议

第一版 MVP 聚焦：

```text
授权 Web/API 目标分析
Agent 原生工作流：Chat → Task/Todo → Tool Loop → Evidence/Report
```

### 最小输入

```text
target: http://localhost:8080
scope: localhost,127.0.0.1
mode: web
```

### 输出目录

```text
runs/{job_id}/
  scope.json
  plan.json
  events.jsonl
  commands.log
  evidence/
  findings.json
  report.md
```

### 第一批能力

```text
目标可达性检查
HTTP 指纹识别
页面抓取
链接提取
JS/API 路由提取
基础目录枚举
nuclei 低风险模板扫描
结果归一化
Markdown 报告生成
```

---

## 7. 工具与插件集成规划

CyberAgent 的工具体系分为三类：

```text
Agent 基础工具：Agent 自己运行所需的 runtime 能力
内置安全能力：Core 自带的轻量 Web/API 分析原语
外部安全工具：通过插件/适配器接入的专业工具
```

### 7.1 Agent 基础工具层

这层不直接承担漏洞扫描，而是保证 Agent 稳定、可控、可复现地运行。

MVP 必须内置：

```text
Config Manager       读取项目配置、用户配置、扫描 profile、默认策略
Run Manager          创建 runs/{job_id} 工作区、路径规范、清理策略
State Store          Job / Plan / Step 状态快照，MVP 使用 bbolt 嵌入式 KV，SQLite 后置评估
Event Bus            统一事件流，写入 events.jsonl，并供 TUI / 后续 API 订阅
Logger               普通日志、调试日志、错误日志，避免污染 evidence
Command Executor     受控命令执行封装，统一 timeout / env / cwd / stdout / stderr
Command Recorder     所有外部命令写入 commands.log，支持复现和审计
Policy Engine        Scope Guard、Risk Policy、确认策略、禁止动作策略
Planner Runtime      Plan / Step 生成、更新、跳过、重排，不直接绑定具体 UI
Tool Registry        安全工具、基础工具、插件能力统一注册与查询
Rate Limiter         请求频率、并发数、per-host 限制
Retry/Timeout        步骤级超时、失败重试、取消任务
Plugin Manager       插件发现、manifest 读取、版本检查、启用/禁用
Dependency Doctor    检查外部工具、Python 环境、插件依赖是否可用
Artifact Store       原始输出、截图、HTML、JSONL、工具结果统一落盘
Evidence Indexer     证据元数据、hash、来源 step、关联 finding
Redactor             对 token、cookie、authorization header 等敏感字段做脱敏策略
Template Renderer    Markdown / JSON 报告模板渲染
Cache/Dedup          URL、响应、证据、finding 去重，避免重复请求和重复报告
```

后续可内置，但不进入 MVP：

```text
Credential Vault     授权测试凭据、cookie、API token 的本地安全存储
Browser Controller   浏览器会话管理；具体 Playwright 仍建议作为插件/组件接入
Notification         扫描完成、失败、需要确认等通知
Update Manager       插件索引、模板、规则集更新
Metrics/Telemetry    本地性能指标；默认不上传
Remote Sync          Desktop/Web/Remote Runner 场景下的任务同步
LLM Gateway          可选模型调用抽象；MVP 可只定义接口，不作为核心依赖
Prompt/Context Store 可选提示词、摘要、上下文包管理；避免 LLM 直接读取全部原始证据
```

### 7.2 MVP 内置安全能力

内置安全能力指由 Agent Core 自带、跨平台、低风险、零外部二进制依赖的基础能力。MVP 内置能力只覆盖 Web/API 分析的通用原语，不内置重型扫描器。

```text
Scope Matcher        目标 / URL / IP / Hostname 范围校验
HTTP Probe           GET / HEAD / OPTIONS 基础请求、状态码、跳转链、响应时间
HTTP Fetcher         页面抓取、响应头 / body 采样、MIME 判断、大小限制
TLS Inspector        证书基础信息、过期时间、协议版本摘要
Header Checker       安全响应头检查，例如 CSP / HSTS / X-Frame-Options / CORS
Cookie Checker       Secure / HttpOnly / SameSite 等 Cookie 属性检查
Robots/Sitemap       robots.txt / sitemap.xml 抓取与 URL 提取
HTML Extractor       links / forms / scripts / meta / comments 基础提取
JS Route Extractor   轻量静态路由提取，复杂 AST 分析交给 Python 插件
API Hint Detector    从页面、JS、响应中发现 /api、GraphQL、OpenAPI 线索
TCP Port Check       少量端口连通性检查，不替代 nmap
Content Discovery    小字典、低速率目录探测，不替代 ffuf
Evidence Hasher      证据文件 hash、去重、元数据记录
Report Generator     Markdown / JSON 基础报告生成
Doctor               检查外部工具和插件依赖是否可用
```

### 7.3 外部安全工具插件

以下工具不内置为 Core 依赖，而是通过插件 / 适配器接入：

```text
nmap        专业端口扫描和服务识别
ffuf        高性能目录 / 参数枚举
nuclei      模板化漏洞扫描
httpx       大规模 HTTP 探测
katana      爬虫与链接发现
playwright  浏览器自动化、截图、前端运行态分析
semgrep     源码安全分析
gitleaks    secret 检测
trufflehog  secret 检测
zap         DAST 代理扫描
sqlmap      SQL 注入验证，默认高风险确认
```

### 7.4 工具接入原则

所有外部工具调用必须经过 Tool Plugin Interface / Tool Adapter。Core 负责任务调度、scope 校验、风险确认、Runner 执行和证据落盘；插件负责能力声明、参数映射、输出解析和结果归一化。

```text
Tool Plugin
  ├── manifest：名称、版本、能力、依赖、风险等级、输入/输出 schema
  ├── command builder：把 Step 参数转换为受控命令
  ├── parser：把 stdout/stderr/artifact 解析为结构化 evidence
  ├── analyzer：可选，把 evidence 转换为 finding
  ├── risk policy：声明是否需要确认、是否可自动执行
  └── artifact policy：声明产物路径、保留策略、脱敏策略
```

基础工具边界原则：

```text
Agent 基础工具属于 Core Runtime，不属于安全扫描插件。
基础工具必须稳定、跨平台、可测试，尽量零外部依赖。
外部命令只能通过 Command Executor / Runner 执行，不能由插件绕过。
所有基础工具都要服务于可复现、可审计、可取消、可扩展。
```

---

## 8. 性能与扩展性原则

前期必须考虑性能和扩展性，但不提前重型化。

### 设计原则

```text
轻实现，重边界
单机优先，保留多 Runner 能力
CLI/TUI 优先，Core 不依赖 UI
插件进程隔离，避免污染主程序
事件驱动，便于未来 WebSocket/SSE
原始证据落盘，结构化摘要入库
```

### 关键性能设计

```text
异步任务执行
可取消任务
并发限制
请求速率限制
超时控制
失败重试
大输出分块处理
LLM 只看摘要，不直接吞原始大日志
```

---

## 9. 安全边界设计

CyberAgent 必须内置 Scope Guard。

默认规则：

```text
没有 scope 不执行主动扫描
高风险动作需要确认
所有命令记录到 commands.log
所有发现必须有证据
默认不进行破坏性操作
默认不进行密码爆破
默认不做持久化、横向移动、数据破坏
```

动作风险等级：

```text
Low: 页面抓取、指纹识别、低频请求
Medium: 目录枚举、模板扫描、认证测试
High: exploit 验证、爆破、写入型测试、破坏性 payload
```

---

## 10. 阶段路线图

### Phase 0：需求与架构确认

当前阶段。

产出：

```text
产品定位
用户画像
使用场景
功能边界
技术选型
核心数据模型
MVP 范围
```

### Phase 1：CLI/TUI MVP

目标：

```text
能完成一次本地授权 Web 目标分析闭环
```

能力：

```text
创建任务
执行计划
调用工具
实时日志
保存证据
生成报告
```

### Phase 2：插件体系

目标：

```text
让工具和分析能力可扩展
```

能力：

```text
Tool Plugin Interface
Plugin Manifest
Python subprocess + JSON/JSONL 插件协议
Tool Registry
Dependency Doctor
输出归一化
Evidence Indexer
finding 去重
风险评级
LLM Analyzer / Reporter 增强插件，可选
```

### Phase 3：Local API / Daemon

目标：

```text
让 Desktop/Web 可以复用 Agent Core
```

能力：

```text
本地 HTTP API
任务管理
事件流
报告查询
证据查看
```

### Phase 4：Desktop

目标：

```text
降低使用门槛
```

能力：

```text
创建扫描
查看计划
实时日志
Finding 列表
证据浏览
报告导出
配置管理
```

### Phase 5：Web / Remote Runner

目标：

```text
团队化、多任务、远程执行
```

能力：

```text
多用户
远程 Runner
任务队列
对象存储
团队报告
权限控制
审计日志
```

---

## 11. 已确认需求基线

### A. 目标用户

首期锁定：

```text
1. 个人安全研究员
2. CTF / 靶场玩家
3. 授权 Web/API 测试人员
```

暂不作为首期主用户：

```text
企业安全管理者
大型多用户安全团队
普通非安全用户
```

### B. 首期目标场景

MVP 聚焦：

```text
本地靶场 Web 分析
明确授权 Web/API 目标安全分析
CTF / 竞赛环境中的 Web/API 辅助分析
```

后置场景：

```text
OpenAPI 深度导入
源码 + 运行时联合分析
企业资产扫描
持续监控
多用户协作
```

### C. 首期交互方式

已确认：

```text
CLI + TUI First
CLI 用于自动化、脚本化、远程环境和 CI
TUI 用于 Agent 主要交互与 run 查看
TUI v1 只读打开已有 run，不创建、不修改、不审批任务
Desktop 后置，但架构上预留
Web/Remote Runner 后置
```

### D. 自动化程度

已确认 Agent 自动化边界：

```text
信息收集和低风险验证默认自动执行
medium 风险按 profile 限制执行或进入确认
high / critical 风险必须 HITL 审批
forbidden / destructive 动作默认禁止
MVP 不做自动 exploit、爆破、横向移动或破坏性 payload
```

### E. 报告标准

MVP 报告优先面向：

```text
技术人员
授权测试记录
CTF / 靶场 writeup 辅助
内部修复沟通
```

后续再扩展：

```text
管理层摘要
客户交付模板
PDF/HTML 美化报告
Issue Tracker 工单同步
```

---

## 12. 当前初步结论

暂定：

```text
项目名：CyberAgent
定位：授权 Web/API 安全分析 Agent
首期形态：CLI + TUI
核心语言：Go
插件语言：Python
Desktop：后续包装，不重写核心
Web：后续团队化能力
MVP：本地/授权 Web 目标扫描、证据保存、报告生成
工具策略：Agent 基础工具内置；轻量安全能力内置；重型安全工具插件化
LLM 策略：增强插件，不作为 Core 必需依赖
```

---

## 13. 计划冻结与变更控制

本轮规划基线已冻结：

```text
v0.1：初始计划草案
v0.2：明确目标用户、首期场景、MVP 边界
v0.3：冻结规划基线，关闭待确认问题，形成实现前计划包
```

后续如需修改范围，按变更控制处理：

```text
新增 MVP 能力 → 必须同步更新 Requirements / MVP Spec / Task Breakdown / Test Plan
调整技术选型 → 必须新增或更新 ADR
改变安全边界 → 必须同步更新 Risk Plan / Policy / Test Plan
改变交互形态 → 必须同步更新 Roadmap / Milestones / Release Plan
```

---

## 14. Agent 工程化融合补充

参考 `Agent-100-Days` 的 Agent 学习与工程化路径，CyberAgent 在计划层新增以下设计原则：

```text
Prompt 不做核心逻辑：确定性逻辑写入 Core / Policy / State Machine。
工具系统需要治理：工具必须有 schema、风险等级、依赖、输出和使用边界。
能力上限显式声明：低风险自动，高风险 HITL，破坏性默认禁止。
上下文工程优先：LLM 只读取裁剪、摘要、脱敏后的 context pack。
记忆系统分层：MVP 只做 Run Memory，后续扩展 Target/Tool/User Memory。
反思作为质量门：失败、冲突、高风险、报告前触发 reflection。
HITL 是系统内建流程：审批、修正、接管都要进入事件和状态。
Gateway + Runtime + Plugin + Channel：CLI/TUI/Desktop/Web 都复用同一 Runtime。
```

新增架构文档：

```text
docs/CyberAgent-Agent-Architecture.md
```

该文档定义：

```text
CyberAgent 的 Agent 能力模型
系统组成
Runtime 执行循环
MVP Agent 能力边界
分阶段实现路线
```


核心契约文档：

```text
docs/CyberAgent-Core-Contracts.md
```

该文档定义：

```text
Target / Scope / Job / Plan / Step / ToolCall / Event
Artifact / Evidence / Finding / Report
PluginManifest / Runner / PolicyDecision / HITLRequest
ReflectionResult / ContextPack
Job Workspace 规范
MVP 默认决策建议
```


MVP 规格文档：

```text
docs/CyberAgent-MVP-Spec.md
```

该文档定义：

```text
MVP 用户故事
MVP CLI 命令设计
MVP TUI 页面草案
safe profile 执行步骤
MVP 内置工具和可选插件
输出目录规范
验收标准
已决问题
```


实现蓝图文档：

```text
docs/CyberAgent-Implementation-Blueprint.md
```

该文档定义：

```text
推荐仓库结构
Go package 边界
核心接口规划
Job / Step 状态机
事件流规划
Workspace 与存储规划
插件系统规划
CLI / TUI 实现规划
测试策略
首批实现顺序
MVP Done Definition
```

---

## 15. 已确认实现决策

```text
Go module: github.com/H1d3rOne/cyberagent
CLI framework: Cobra
TUI framework: Bubble Tea / Lip Gloss
MVP State Store: bbolt 嵌入式 KV + JSONL 事件流 + 文件系统 artifacts
SQLite: MVP 阶段跳过；后续如需要复杂关系查询、报表过滤、Web 多任务检索再评估引入
optional_nuclei_safe: 默认不启用，用户显式启用
TUI v1: 只读打开已有 run，不创建、不修改、不审批任务
```

说明：

```text
State Store 是当前任务状态快照，用于恢复、TUI 展示、replay 和故障排查。
events.jsonl 是事实流，state.json 是当前快照。
TUI v1 先作为 run viewer，实时控制和 HITL approval 后置。
```

---

## 16. 规划文档索引

完整规划文档索引见：

```text
docs/CyberAgent-Docs-Index.md
```

规划完成状态见：

```text
docs/CyberAgent-Planning-Status.md
```

新增规划文档：

```text
docs/CyberAgent-Roadmap.md
docs/CyberAgent-Milestones.md
docs/CyberAgent-Task-Breakdown.md
docs/CyberAgent-Prioritization.md
docs/CyberAgent-ADR-Index.md
docs/CyberAgent-Risk-Plan.md
docs/CyberAgent-Test-Plan.md
docs/CyberAgent-Release-Plan.md
```
