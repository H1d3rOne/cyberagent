# CyberAgent Implementation Blueprint

版本：v0.2-agent-native-correction  
日期：2026-05-25  
状态：实现蓝图纠偏；新增 Agent-Native Runtime、Session Store、Task/Todo、Tool Loop 与 Workspace Context 边界

---

## 1. 文档目标

本文把已经确认的需求、Agent 架构、核心契约和 MVP 规格转化为后续实现蓝图。

本文只定义：

```text
推荐目录结构
Go package 边界
核心接口规划
状态机规划
事件流规划
存储规划
插件目录规划
TUI 规划
测试策略
首批实现顺序
```

本文不实现代码。

关联文档：

```text
docs/CyberAgent-Plan.md
docs/CyberAgent-Requirements.md
docs/CyberAgent-Agent-Architecture.md
docs/CyberAgent-Core-Contracts.md
docs/CyberAgent-MVP-Spec.md
```

---

## 2. 实现总原则

```text
1. 先契约，后实现。
2. 先 Agent-Native Runtime，后普通 Dashboard 能力。
3. 先 Runtime 骨架，后安全能力。
4. 先内置闭环，后外部插件。
5. 先 CLI/TUI/Local Web Agent Cockpit，后 Desktop/多用户 Web。
5. 先本地 Local Runner，后 Docker / Remote Runner。
6. LLM 只定义接口，MVP 不默认依赖。
7. 所有外部命令都经过 Runner / Command Executor。
8. 所有网络动作都经过 Scope Guard / Rate Limiter。
9. 所有结论都必须有 Evidence。
10. 所有状态变化都必须有 Event。
11. Agent 会话、任务、Todo、工具调用和上下文引用必须可持久化恢复。
12. 前端 localStorage 只能作为 UI 兜底缓存，不能替代 daemon State Store。
```

---

## 3. 推荐仓库结构

建议采用 Go 主体 + Python 插件的单仓结构。

```text
CyberAgent/
  README.md
  go.mod
  go.sum
  Makefile
  .gitignore

  cmd/
    cyberagent/
      main.go                    # CLI 入口，后续接 Cobra

  internal/
    app/
      app.go                     # 应用组装，依赖注入入口
      version.go

    core/
      contracts/                 # 核心数据模型，对应 Core Contracts
        agent_session.go
        agent_task.go
        agent_todo.go
        agent_context.go
        target.go
        scope.go
        job.go
        plan.go
        step.go
        toolcall.go
        event.go
        artifact.go
        evidence.go
        finding.go
        report.go
        plugin.go
        runner.go
        policy.go
        hitl.go
        reflection.go
        contextpack.go

      runtime/                   # Agent Runtime 执行循环
        runtime.go
        executor.go
        lifecycle.go

      planner/                   # 确定性 Planner
        planner.go
        web_safe.go
        profiles.go

      scheduler/                 # Step 调度、依赖、重试、取消
        scheduler.go
        state_machine.go

      policy/                    # Scope Guard / Risk Policy / HITL Trigger
        scope_guard.go
        risk_policy.go
        decision.go

      events/                    # Event Bus / Event Store
        bus.go
        store.go
        types.go

      findings/                  # Finding 归一化、去重、评级
        normalizer.go
        dedup.go
        severity.go

      reporting/                 # Markdown / JSON 报告
        reporter.go
        markdown.go
        json.go
        templates/
          report.md.tmpl

    runtime/
      config/                    # Config Manager
        config.go
        profiles.go
      runs/                      # Run Manager / workspace
        manager.go
        layout.go
      state/                     # State Store，MVP bbolt，SQLite 后续评估
        store.go
        json_store.go
      logging/
        logger.go
      command/                   # Command Executor / Recorder
        executor.go
        recorder.go
      ratelimit/
        limiter.go
      redaction/
        redactor.go
      cache/
        cache.go
      doctor/
        doctor.go

    tools/
      registry/                  # Tool Registry
        registry.go
        capability.go
      builtin/
        scope_matcher.go
        http_probe.go
        http_fetcher.go
        header_checker.go
        cookie_checker.go
        robots_sitemap.go
        html_extractor.go
        js_route_light.go
        api_hint_detector.go
        content_discovery.go
        evidence_hasher.go
      external/
        adapter.go               # 外部工具适配抽象

    plugins/
      manager.go                 # Plugin Manager
      manifest.go
      protocol.go                # subprocess JSON/JSONL 协议草案
      doctor.go

    runners/
      runner.go                  # Runner interface
      local/
        local_runner.go
      docker/                    # 后置，占位
        README.md
      remote/                    # 后置，占位
        README.md

    interfaces/
      cli/
        root.go
        scan.go
        report.go
        replay.go
        doctor.go
        plugins.go
      tui/
        app.go
        model.go
        views/
          plan.go
          events.go
          evidence.go
          findings.go
          report.go
          approval.go

    llm/
      gateway.go                 # MVP 只定义接口
      context_builder.go
      README.md

    agent/
      state.go                   # AgentSession / Task / Todo / ToolCall / ContextRef service model
      session_store.go           # Agent session persistence over runtime/state or filesystem adapter
      task_manager.go            # active task / todo lifecycle
      tool_loop.go               # plan → tool call → observe → update todo → next action
      workspace_context.go       # run/artifact/finding/report/approval refs
      responder.go               # message + structured state response assembly

  plugins/
    nuclei-safe/
      plugin.yaml
      README.md
    ffuf-low-rate/
      plugin.yaml
      README.md
    python-js-route-analyzer/
      plugin.yaml
      analyzer.py
      README.md

  schemas/
    contracts/
      target.schema.json
      scope.schema.json
      job.schema.json
      plan.schema.json
      step.schema.json
      finding.schema.json
    plugins/
      plugin-manifest.schema.json
      nuclei-safe.input.schema.json
      nuclei-safe.output.schema.json

  templates/
    reports/
      markdown.default.md.tmpl

  docs/
    CyberAgent-Plan.md
    CyberAgent-Requirements.md
    CyberAgent-Agent-Architecture.md
    CyberAgent-Core-Contracts.md
    CyberAgent-MVP-Spec.md
    CyberAgent-Implementation-Blueprint.md

  testdata/
    web/
      simple-site/
      headers/
      html/
      js/
    runs/
      sample-run/
```

---

## 4. Go Package 边界

### 4.1 `cmd/cyberagent`

职责：

```text
程序入口
初始化 CLI
传递版本信息
调用 internal/app 组装好的应用
```

不应包含：

```text
扫描逻辑
工具调用逻辑
报告生成逻辑
```

### 4.2 `internal/app`

职责：

```text
应用组装
依赖注入
配置加载
Runtime 初始化
CLI/TUI 入口绑定
```

### 4.3 `internal/core/contracts`

职责：

```text
核心数据结构
枚举定义
基础校验方法
JSON 序列化约定
```

要求：

```text
尽量不依赖其他业务包。
可以被 CLI/TUI/Runtime/Plugin/Report 共同引用。
```

### 4.4 `internal/core/runtime`

职责：

```text
Agent Runtime 主执行循环
Job 生命周期管理
Plan 执行协调
Step 执行入口
Reflection Gate 调用
Report 收尾
```

### 4.5 `internal/core/planner`

职责：

```text
根据 Target / Scope / Profile / Tool Capability 生成 Plan
MVP 先实现 web safe profile
后续支持 replan
```

### 4.6 `internal/core/scheduler`

职责：

```text
Step 依赖判断
Step 状态机
并发控制
重试
取消
跳过
```

### 4.7 `internal/core/policy`

职责：

```text
Scope Guard
Risk Policy
Forbidden Action Policy
HITL Trigger
Rate Policy
Redaction Policy
```

### 4.8 `internal/runtime/*`

职责：

```text
Agent 基础工具层
配置、运行目录、状态、日志、命令执行、速率限制、脱敏、doctor
```

### 4.9 `internal/tools/*`

职责：

```text
内置安全能力
工具注册表
外部工具适配层
```

### 4.10 `internal/plugins`

职责：

```text
插件发现
Manifest 读取
依赖检查
协议封装
插件启用/禁用
```

### 4.11 `internal/runners`

职责：

```text
执行环境抽象
MVP 实现 Local Runner
后续扩展 Docker / Remote / K8s
```

### 4.12 `internal/interfaces`

职责：

```text
CLI/TUI 入口
显示状态
读取事件
发起审批
```

不应包含：

```text
Core 业务逻辑
工具执行逻辑
Finding 生成逻辑
```

---

## 5. 核心接口规划

以下是后续实现时建议优先稳定的接口方向，本文只做规划。

### 5.1 Runtime 接口

```text
Runtime.CreateJob(request) -> Job
Runtime.Plan(jobID) -> Plan
Runtime.Run(jobID) -> JobResult
Runtime.Cancel(jobID) -> CancelResult
Runtime.Resume(jobID) -> JobResult
Runtime.GetStatus(jobID) -> JobStatus
Runtime.StreamEvents(jobID) -> EventStream
```

### 5.2 Planner 接口

```text
Planner.BuildPlan(target, scope, profile, capabilities) -> Plan
Planner.Replan(job, currentPlan, reason) -> Plan
```

MVP：

```text
只实现 BuildPlan。
Replan 先定义接口，后置实现。
```

### 5.3 Scheduler 接口

```text
Scheduler.RunPlan(ctx, job, plan) -> ExecutionResult
Scheduler.RunStep(ctx, job, step) -> StepResult
Scheduler.CancelStep(stepID) -> bool
```

### 5.4 Policy Engine 接口

```text
Policy.CheckScope(targetRef, scope) -> PolicyDecision
Policy.CheckStep(step, job, scope) -> PolicyDecision
Policy.CheckToolCall(toolCall, step, scope) -> PolicyDecision
Policy.RequiresApproval(step, decision) -> bool
```

### 5.5 Runner 接口

```text
Runner.Submit(ctx, step, toolSpec) -> ToolCallResult
Runner.Cancel(stepID) -> bool
Runner.Healthcheck() -> RunnerStatus
Runner.CollectArtifacts(stepID) -> Artifact[]
```

### 5.6 Tool 接口

内置工具和外部工具适配器都应抽象为类似能力：

```text
Tool.Name() -> string
Tool.Capabilities() -> []Capability
Tool.Risk() -> RiskLevel
Tool.InputSchema() -> SchemaRef
Tool.OutputSchema() -> SchemaRef
Tool.Execute(ctx, input, runtimeContext) -> ToolResult
```

### 5.7 Plugin Manager 接口

```text
PluginManager.Discover(paths) -> []PluginManifest
PluginManager.Enable(pluginID) -> error
PluginManager.Disable(pluginID) -> error
PluginManager.Doctor(pluginID) -> PluginHealth
PluginManager.GetCapabilities() -> []Capability
```

### 5.8 Event Bus 接口

```text
EventBus.Publish(event) -> error
EventBus.Subscribe(jobID) -> EventStream
EventStore.Append(event) -> error
EventStore.Read(jobID, filter) -> []Event
```

### 5.9 Store 接口

```text
StateStore.SaveJob(job) -> error
StateStore.LoadJob(jobID) -> Job
StateStore.SavePlan(plan) -> error
StateStore.LoadPlan(jobID) -> Plan
StateStore.SaveStep(step) -> error
StateStore.ListSteps(jobID) -> []Step
```

### 5.10 Evidence / Finding 接口

```text
ArtifactStore.Save(artifact) -> ArtifactRef
EvidenceStore.Add(evidence) -> Evidence
EvidenceStore.List(jobID) -> []Evidence
FindingStore.Add(finding) -> Finding
FindingStore.List(jobID) -> []Finding
```

---

## 6. 状态机规划

### 6.1 Job 状态机

```text
created
  ↓
planning
  ↓
ready
  ↓
running ─────→ waiting_approval
  ↓                  ↓
paused              running
  ↓                  ↓
succeeded / partially_succeeded / failed / cancelled
```

关键规则：

```text
created 只能进入 planning 或 cancelled。
planning 成功后进入 ready。
ready 执行后进入 running。
running 遇到 HITL 进入 waiting_approval。
waiting_approval 只能由用户决策恢复、跳过或取消。
任何运行中状态都可以进入 cancelled。
最终态不可再执行，只能 replay 或生成报告。
```

### 6.2 Step 状态机

```text
pending
  ↓
blocked / ready
  ↓
running ─────→ waiting_approval
  ↓                  ↓
succeeded / failed / skipped / cancelled
```

关键规则：

```text
依赖未满足时为 blocked。
依赖满足后进入 ready。
执行前必须有 policy.checked event。
需要审批时进入 waiting_approval。
被拒绝时进入 skipped。
执行异常进入 failed。
```

---

## 7. Event Flow 规划

### 7.1 一次 Step 的标准事件序列

```text
step.ready
policy.checked
tool.selected
step.started
tool.started
artifact.created
evidence.created
tool.finished
step.finished
```

如果需要审批：

```text
policy.checked
approval.requested
approval.resolved
step.started 或 step.skipped
```

如果失败：

```text
step.started
tool.started
tool.failed
reflection.created
step.failed 或 step.retrying 或 step.skipped
```

### 7.2 Event 设计要求

```text
Event 必须是 append-only。
Event 不保存大块原始输出。
Event 中只保存摘要和 artifact 引用。
TUI/API 只订阅 Event，不直接窥探 Runtime 内部状态。
```

---

## 8. Workspace 与存储规划

### 8.1 MVP Workspace

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

### 8.2 MVP 存储选择

默认：

```text
bbolt 嵌入式 KV 保存状态快照和轻量索引
JSONL 保存 append-only 事件流
文件系统保存 artifacts / evidence / report
```

后续升级：

```text
SQLite metadata store
Postgres / Object Storage，Web/Remote Runner 阶段
```

### 8.3 存储边界

```text
state/bolt.db 保存当前状态快照和轻量索引。
events.jsonl 保存事实流。
artifacts/ 保存原始产物。
evidence/ 保存可引用证据。
findings.json 保存归一化结论。
reports/ 保存最终报告。
```

---

## 9. Safe Profile Step 规划

MVP `web safe profile` 的实现顺序：

```text
1. scope_check
2. target_normalize
3. workspace_init
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

### 9.1 MVP 可先实现的最小闭环

如果要进一步压缩第一轮实现，可先实现：

```text
scope_check
target_normalize
workspace_init
http_probe
http_fetch_homepage
header_check
html_extract
evidence_index
finding_normalize
report_generate
```

该闭环必须已经能产出：

```text
events.jsonl
evidence/index.json
findings.json
reports/report.md
```

---

## 10. 内置工具实现规划

### 10.1 第一批必须实现

```text
Scope Matcher
HTTP Probe
HTTP Fetcher
Header Checker
HTML Extractor
Evidence Hasher
Report Generator
Doctor Basic
```

### 10.2 第二批实现

```text
Cookie Checker
Robots/Sitemap Fetcher
JS Route Extractor Light
API Hint Detector
Content Discovery Small
Redactor
Cache/Dedup
```

### 10.3 后置实现

```text
TLS Inspector
TCP Port Check
Advanced JS Analyzer
Browser Controller
Credential Vault
```

---

## 11. 插件系统规划

### 11.1 MVP 插件协议

推荐第一版采用：

```text
subprocess + JSON/JSONL
```

插件分两类：

```text
Command Builder Plugin：声明如何构造外部命令，但命令仍由 Runner 执行。
Analyzer Plugin：读取 workspace 内 artifact，输出 evidence/finding JSON。
```

### 11.2 插件 Manifest 必须包含

```text
id
name
version
type
runtime
capabilities
risk
requires_approval
dependencies
input_schema
output_schema
artifact_policy
supported_profiles
entrypoint
```

### 11.3 插件目录建议

```text
plugins/{plugin-id}/
  plugin.yaml
  README.md
  schemas/
    input.schema.json
    output.schema.json
  scripts/
    analyzer.py
```

### 11.4 首批插件规格

```text
nuclei-safe
  类型：Tool Plugin
  风险：medium
  默认：可选，缺失时跳过

ffuf-low-rate
  类型：Tool Plugin
  风险：medium
  默认：可选，需要受限速率

python-js-route-analyzer
  类型：Analyzer Plugin
  风险：low
  默认：可选，只读 workspace artifact
```

---

## 12. CLI 实现规划

### 12.1 MVP 命令

```text
cyberagent scan web
cyberagent report
cyberagent replay
cyberagent doctor
cyberagent plugins list
cyberagent tui
```

### 12.2 CLI 职责

```text
解析参数
调用 app/runtime
订阅事件并打印摘要
处理交互式审批，MVP 可先用 prompt
输出报告路径
返回合理 exit code
```

### 12.3 CLI 不做

```text
不直接执行工具
不直接生成 finding
不直接写 evidence
不绕过 Runtime
```

---

## 13. TUI 实现规划

### 13.1 MVP TUI 优先级

建议：

```text
第一阶段：只读打开已有 run
第二阶段：订阅实时 events
第三阶段：支持 HITL approval
```

### 13.2 页面

```text
Plan View
Events View
Evidence View
Findings View
Report View
Approval View
```

### 13.3 TUI 数据来源

```text
events.jsonl
state.json
plan.json
evidence/index.json
findings.json
reports/report.md
```

后续实时模式：

```text
EventBus.Subscribe(jobID)
```

---

## 14. LLM 接口规划

MVP 只定义，不默认实现 Provider。

### 14.1 LLM Gateway 接口方向

```text
LLMGateway.Complete(ctx, request) -> response
LLMGateway.SupportsStructuredOutput() -> bool
LLMGateway.Name() -> string
```

### 14.2 Context Builder

```text
ContextBuilder.Build(jobID, purpose, refs) -> ContextPack
```

### 14.3 MVP 不允许

```text
LLM 直接执行工具
LLM 直接扩大 scope
LLM 直接读取全部 evidence 目录
LLM 自动批准高风险步骤
```

---

## 15. 错误处理规划

错误分类：

```text
UserInputError       用户输入错误，例如 target 无效、scope 缺失
PolicyError          策略拒绝，例如超出 scope
ToolUnavailableError 工具缺失
ToolExecutionError   工具执行失败
ParseError           输出解析失败
StorageError         文件/状态写入失败
NetworkError         网络请求失败
TimeoutError         超时
InternalError        未预期错误
```

处理策略：

```text
用户输入错误：直接失败，不创建主动扫描。
策略拒绝：记录 warning/error event，Step skipped 或 failed。
工具缺失：可选工具 skipped，必需工具 failed。
解析失败：保留 artifact，降低 confidence，触发 reflection。
网络失败：按 retry policy 重试。
存储失败：终止任务，避免丢证据。
```

---

## 16. 测试策略

### 16.1 单元测试

优先测试：

```text
Target normalize
Scope Matcher
Risk Policy
State Machine
Event serialization
Workspace layout
Header Checker
HTML Extractor
Finding Normalizer
Redactor
```

### 16.2 集成测试

测试：

```text
scan web safe profile against testdata/simple-site
doctor 缺失工具降级
optional plugin skipped
events.jsonl 完整性
finding 必须引用 evidence
report.md 生成
```

### 16.3 Golden Test

适合：

```text
report.md 输出
events.jsonl 示例
findings.json 示例
HTML extractor 输出
JS route light 输出
```

### 16.4 Contract Test

适合：

```text
Plugin Manifest schema
Plugin output schema
ContextPack schema
Finding schema
```

### 16.5 安全边界测试

必须覆盖：

```text
无 scope 拒绝扫描
target 超出 scope 拒绝扫描
redirect 超出 scope 停止
high risk step require approval
forbidden step never runs
plugin cannot bypass command executor
sensitive headers redacted
```

---

## 17. 首批实现顺序建议

后续真正写代码时，建议按以下顺序，不要先写 TUI 或插件。

### Step 1：项目骨架

```text
go.mod
cmd/cyberagent
internal/app
internal/core/contracts
基础 README / Makefile
```

### Step 2：Contracts + JSON 序列化

```text
Target
Scope
Job
Plan
Step
Event
Artifact
Evidence
Finding
Report
```

### Step 3：Run Manager + Workspace

```text
runs/{job_id} 创建
基础文件写入
events.jsonl append
bbolt state save/load
```

### Step 4：Policy + Scope Guard

```text
scope check
target check
redirect check
risk decision
```

### Step 5：Event Bus + State Machine

```text
job/step 状态转换
event append
CLI 事件输出
```

### Step 6：内置 HTTP 闭环

```text
HTTP Probe
HTTP Fetcher
Header Checker
HTML Extractor
Evidence Index
Finding Normalizer
Report Generator
```

### Step 7：CLI scan/report/replay/doctor

```text
scan web
report
replay
doctor
```

### Step 8：Content Discovery + Cookie + Robots/Sitemap

```text
Cookie Checker
Robots/Sitemap
Content Discovery Small
API Hint Detector
```

### Step 9：插件系统

```text
Plugin Manifest
Plugin Manager
Command Executor
nuclei-safe optional
```

### Step 10：TUI

```text
打开已有 run
查看 plan/events/findings/report
后续再做实时和 approval
```

---

## 18. MVP Done Definition

MVP 实现完成必须满足：

```text
1. 能通过 CLI 创建并执行 web safe scan。
2. 没有 scope 时拒绝主动扫描。
3. 所有 step 都有事件记录。
4. 所有外部命令都写入 commands.log。
5. 所有 evidence 都可追溯到 artifact/step。
6. 所有 finding 都引用 evidence。
7. 能生成 Markdown 报告。
8. doctor 能检查可选工具并降级。
9. TUI 能打开 run 并查看 plan/events/findings/report。
10. 测试覆盖 scope、policy、事件、报告、证据链。
```

---

## 19. 暂缓事项

以下内容不要在 MVP 第一轮实现：

```text
Desktop
多用户 Web Dashboard
Remote Runner
Docker Runner
K8s Runner
多用户系统
账户权限
PDF 报告
复杂 RAG
长期 Target Memory
自动 exploit
自动修复代码
插件市场
自动更新系统
```

---

## 20. 已确认实现决策

```text
Go module: github.com/H1d3rOne/cyberagent
CLI framework: Cobra
TUI framework: Bubble Tea / Lip Gloss
MVP State Store: bbolt 嵌入式 KV + JSONL 事件流 + 文件系统 artifacts
SQLite: MVP 阶段跳过；后续如需要复杂关系查询、报表过滤、Web 多任务检索再评估引入
optional_nuclei_safe: 默认不启用，用户显式 --use-plugin nuclei-safe 或配置启用
TUI v1: 只读打开已有 run，不负责创建/修改/审批任务
Agent-Native: AgentSession / Task / Todo / ToolCall / ContextRef 必须进入 daemon State Store
Local Web Cockpit: 当前阶段作为 Agent Channel，不等同于后置的多用户 Web Dashboard
```

说明：

```text
State Store 用于保存 Job / Plan / Step 的当前状态快照，支撑恢复、replay、TUI 展示和故障排查。
Agent Session Store 用于保存 Agent 会话、消息、当前任务、Todo、工具调用、上下文引用，支撑 Claude Code / Codex 型持续任务体验。
events.jsonl 是 append-only 事实流；state.json 是当前状态快照。
TUI v1 只读取 run 目录中的 plan/events/evidence/findings/report；实时订阅和 HITL approval 后置。
```

实现阶段入口已规划完成；只有用户明确说“开始实现”或给出等价指令后，才继续扩展代码。
