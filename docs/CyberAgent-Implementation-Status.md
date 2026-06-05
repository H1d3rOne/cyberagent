# CyberAgent Implementation Status

版本：v0.1.3 Agent-Native Increment
日期：2026-05-27
状态：v0.1.2 Local Daemon / Web API 本地 RC 验证完成；v0.1.3-v0.1.8 Agent-native 主链路、上下文闭环、Workspace 变更流、Cockpit 交互、本地工具调用体验、聊天自然语言控制、Run Workspace Context 读取、自然语言扫描工作流状态闭环、Continue Active Run Context、Active Run Evidence Action Auto Plan、Evidence Action Stepper、Evidence Action Stage Conclusion 与 Stepper Final Convergence 已进入可验证状态

---

## 1. 当前结论

```text
规划阶段：已完成
MVP 主链路：已跑通
插件系统 MVP：已接入三类声明插件，其中 ffuf-low-rate 在 safe profile 下默认审批跳过
MVP Stabilization：本地验证通过
MVP Release：v0.1.0 本地 release 准备完成；远程发布未执行
v0.1.1 RC：本地实现、文档、测试、release notes 已准备；tag/push/GitHub Release 未执行
v0.1.2 Planning：计划、实施拆解、API Contract 与 daemon smoke test 草案已完成
v0.1.2 Implementation：Daemon Read-only API、Approval write/resume API、scan web async job API 与 job status/cancel MVP 已完成
v0.1.2 RC：本地 gofmt/test/vet/build 与 daemon curl smoke 已通过；release notes 草案已准备
v0.1.3-v0.1.8 Agent-native：session/task/todo/tool_calls/context_refs/next_action 持久化、Agent Cockpit、Timeline trace polish、控制流、本地 Dashboard 自动 daemon 接入、Agent state/context/control alias API、capabilities endpoint discovery、聊天自然语言 continue/revise/stop/new_task 控制、Continue Active Run Context、Auto-continue Stepper、Active Run Evidence Action Auto Plan、Evidence Action Stepper、Evidence Action Stage Conclusion、Evidence Action Stepper Final Convergence、Chat-native Approval Decision、Chat-native Resume Approved、Planner Approval Tool Actions、Cockpit Approval Agent Shortcuts、Agent Cockpit Inspector Tabs、自然语言扫描工作流状态闭环、自然语言 run workspace context 读取、自然语言 workspace command/list/read/write/patch/diff/apply、规则 Agent 多命令顺序执行、确定性工具结果 routed synthesis、Agent reasoning / plan revision 可见化、Planner 多动作 execution queue 可追踪化、Planner 变更 workspace 显式意图 gate、active_run approval/report/evidence/artifact context snapshot、session run_summary、step deep-link、执行链、运行审计搜索、单 step 详情抽屉、Run Evidence 工作台、Agent Evidence Explain、Evidence Export、Evidence → Action、Evidence Action TODO Plan、Evidence Action Approval Gate、Cockpit approval gate UX、Approval resolution state refresh、后端 approval gate resolved state convergence、Agent Cockpit 会话保持、Slash 指令、Slash 控制命令、输入历史、本地工具快捷入口、自然语言控制快捷入口、能力分组可视化已完成增量实现
Production Agent Runtime：daemon 默认启用 SQLite runtime engine 与 runtime event bridge；planner action 执行已删除 legacy per-tool switch fallback，builtin action 统一走 Tool Registry handler；routePrompt 的 read-only/system 分支、safe HTTP GET/HEAD/OPTIONS、ports.scan.safe、scan.web.safe、workspace diff/rebase staged-change 与 reports.draft 本地路由已改为 heuristic action → registry dispatch，并为 local routing 工具结果写入 deterministic source metadata，trace timeline 与 Cockpit Timeline 均可见；高风险/写动作评估结论是先继续分批 registry 化，再启动 engine-native turn lifecycle 下沉；如需回滚 runtime engine，可设置 `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0`
Next：继续 Agent 多步自然语言执行体验与更强执行策略；如需远程发布，需用户显式授权创建 tag / push / GitHub Release
```

---

## 2. 已完成能力

### CLI / TUI

```text
[x] cyberagent version
[x] cyberagent doctor
[x] cyberagent plugins list
[x] cyberagent plugins doctor
[x] cyberagent plugins doctor --json
[x] cyberagent scan web
[x] cyberagent report <run-dir> --format md|json
[x] cyberagent replay <run-dir>
[x] scan web / replay skipped steps 摘要与 next actions
[x] cyberagent tui <run-dir>（Plan / Events / Approvals / Evidence / Findings / Report）
[x] cyberagent daemon（Local HTTP API skeleton）
```

### Core Runtime

```text
[x] Core Contracts
[x] Run Workspace
[x] bbolt State Store
[x] JSONL Event Store
[x] Command Recorder: commands.log
[x] Artifact Index: artifacts/index.json
[x] Scope Guard
[x] Risk Policy
[x] Safe Profile Planner
[x] Job / Step lifecycle
[x] Evidence / Finding / Report 链路
```

### Core Services / v0.1.2

```text
[x] ServiceError / ErrorCode 边界模型
[x] VersionService: GetVersion / Doctor
[x] PluginService: ListPlugins / Doctor
[x] RunService: ListRuns / GetRun / GetRunSummary
[x] RunService: GetReport / GetArtifactIndex / GetEvents cursor polling
[x] RunService path traversal / missing run 错误映射测试
[x] ApprovalService: list / approve / deny
[x] ApprovalService: resume-approved
[x] ScanService: StartWebScan validation skeleton
[x] Daemon JobManager: async scan execution / status / cancel MVP
```

### Local Daemon / v0.1.2

```text
[x] Daemon config: listen / runs-dir / token-file / max-concurrent-jobs
[x] cyberagent daemon --help
[x] Default listen: 127.0.0.1:8787
[x] Token file load/create with 0600 permission
[x] Bearer token auth middleware
[x] Unified /api/v1 response envelope
[x] ServiceError -> HTTP status mapping
[x] GET /api/v1/healthz
[x] GET /api/v1/version
[x] GET /api/v1/doctor
[x] GET /api/v1/plugins / plugins/doctor
[x] GET /api/v1/runs / report / artifacts / events / approvals
[x] Approval write HTTP API
[x] POST /api/v1/runs/{run_id}/approvals/{approval_id}/approve
[x] POST /api/v1/runs/{run_id}/approvals/{approval_id}/deny
[x] POST /api/v1/runs/{run_id}/resume-approved
[x] POST /api/v1/scans/web
[x] GET /api/v1/jobs/{job_id}
[x] POST /api/v1/jobs/{job_id}/cancel
[x] Scan async job manager / job status / cancel
```

### Agent Runtime / v0.1.3

```text
[x] Agent Session API：create/get/list/send/control/capabilities
[x] Agent API aliases：GET /state、GET /context、POST /continue、POST /stop、POST /new-task，便于 Web/TUI/CLI 像 Claude Code/Codex 一样直接恢复状态和控制任务
[x] Agent capabilities endpoint discovery：/api/v1/agent/capabilities 显式暴露 sessions/messages/state/context/control/stream endpoint 清单，前端类型已同步
[x] Agent State 持久化：messages / active_task / todos / tool_calls / context_refs / next_action
[x] Planner JSON action 与模型原生 tool_calls 解析
[x] 工具执行闭环：plan → tool → observe → synthesize → persist
[x] 确定性工具 routed synthesis：配置 AI provider 时，规则 Agent / daemon tool 分支执行完成后也会进入 observe → synthesize，并可更新 TODO / next_action
[x] 控制流：continue / stop / revise / new_task
[x] 聊天自然语言控制：在 Agent 聊天框直接输入“继续/下一步/修订计划/停止/新任务”会触发 control 状态机；配置 AI provider 时 continue/revise 会先更新 task/todo/next_action，再继续进入 LLM 执行循环，避免被误判为普通聊天或 runs 查询
[x] 自然语言扫描工作流状态闭环：聊天输入“扫描 <target> scope=<scope>”会启动 safe profile daemon job，并把 target/scope/profile/job/run/next_step 写入 tool result；同步 active_task.active_run_id、scan TODO 进度、run/job context_refs 和 auto_continue next_action，下一步可直接跟踪运行、读取报告和证据
[x] Continue Active Run Context：聊天输入“继续 / 请继续监控当前任务 / 继续跟踪”时，若会话存在 active_run，会先触发 control 状态更新，再自动读取该 run 的最佳上下文（pending approvals、report_json、findings、artifact_index 或 events），沉淀 `GET /api/v1/runs/{run_id}/context` 工具观察、context_refs 与 TODO 状态
[x] Active Run Evidence Action Auto Plan：Continue Active Run Context 读取 report_json/findings 且存在 findings 时，会自动追加“关联证据链 / 误报检查与影响边界 / 修复建议与最小复测计划”后续 TODO，并把 next_action 指向可继续执行的证据行动计划；终态 run 同步不会误将这些后续 TODO 自动完成
[x] Evidence Action Stepper：行动计划生成后再次输入“继续”会重新读取 active run context，并一次推进一个 evidence action TODO（证据链关联 → 误报检查/影响边界 → 修复/复测计划），保持每一步都有工具观察支撑
[x] Evidence Action Stage Conclusion：每次 Evidence Action Stepper 推进后都会追加可读阶段性 agent 结论消息，说明“已生成行动计划 / 证据链关联结论 / 误报检查与影响边界 / 修复与复测计划”，避免只看到 TODO 状态变化
[x] Evidence Action Stepper Final Convergence：连续“继续”完成三段证据行动 TODO 后，session 收敛为 idle/done，并保留 `wait_user` next_action 和 run summary prompt，避免被泛化 `finished` 状态覆盖
[x] Chat-native Approval Decision：聊天中输入“批准/拒绝运行 <run_id> 的 <approval_id/step_id>，comment=...”会调用 approval write service，记录 resolution metadata，刷新 approval gate TODO、context_refs 和 next_action
[x] Chat-native Resume Approved：聊天中输入“恢复已批准运行 <run_id> / resume-approved”会调用 resume-approved service，沉淀 approval_resume tool_call/context_refs，并把后续跟踪 TODO 恢复为 running
[x] Planner Approval Tool Actions：LLM Planner tools/capabilities 已暴露 runs.approvals.list / approve / deny / resume_approved；approve/deny/resume 在 capabilities 中标记 medium / non-auto-run，且必须匹配最新用户显式审批/恢复指令才允许执行，并保留 action queue metadata
[x] Cockpit Approval Agent Shortcuts：审批队列以 Agent 批准 / Agent 拒绝 / Agent 恢复并跟踪为主操作，直接 API approve/deny/resume 降级为次级小按钮；Slash Commands 增加 /approve、/deny、/resume-approved；输入 placeholder 与快捷 chips 同步提示审批/恢复 Agent 能力
[x] Agent Cockpit Inspector Tabs：右侧上下文从超长信息堆叠收敛为运行簿 / 轨迹 / 能力 / 工作区四个焦点页签，页签状态持久化到 localStorage，降低视觉噪音并更贴近 Claude Code / Codex 型 Agent IDE 结构
[x] Auto-continue Stepper：当上轮 next_action.auto_continue=true 且 suggested_prompt 可降级为低风险只读本地工具动作时，聊天输入“继续”会自动执行下一步工具、追加 observation、沉淀 auto_continue tool metadata，并保持扫描/审批/写文件等高风险动作不会被自动执行
[x] active_run 终态同步：成功收敛 idle/wait_user，失败或取消收敛 blocked
[x] 自然语言 workspace.command
[x] 规则 Agent 多命令顺序执行：例如“先执行 pwd 和 ls -la，再总结”会顺序生成多个 workspace.command tool_call，并记录 command_index / command_count
[x] 自然语言 workspace.files.list
[x] 自然语言 workspace.files.read
[x] 自然语言 workspace.files.write
[x] 自然语言 workspace.files.patch：单文件精确 old/new 替换与 diff preview
[x] 自然语言 workspace.files.diff/apply/discard：先生成 staged diff，不落盘；再按 change_id 应用或丢弃；支持“应用/丢弃所有待应用修改”；apply 前若文件已变化会标记 conflict，conflict 可继续丢弃清理
[x] Multi-file staged change set：workspace.files.diff_many 生成共享 group_id，支持 apply-group / discard-group；apply-group 全量预检，冲突时不部分落盘
[x] Conflict rebase/regenerate：workspace.files.rebase 可基于当前文件 old 文本重新生成 staged change，旧 change 标记 superseded
[x] workspace file context_ref 自动沉淀
[x] Local Dashboard loopback bootstrap：/api/local/daemon-config 自动注入本地 daemon URL/token
[x] Agent Cockpit：会话列表、聊天、任务/Todo、Tool Calls、Context、Next Action 展示
[x] Agent Cockpit 会话保持：按 session 缓存聊天消息与输入草稿，保留左侧会话列表、置顶、本地搜索和刷新；聊天输入支持 ↑/↓ 历史导航
[x] Agent Cockpit Slash 指令：支持 /health、/tools、/runs、/workspace(/ws)、/ls、/open、/write、/diff、/rebase、/apply、/discard、/scan、/continue、/revise、/stop、/new、/clear；支持唯一前缀匹配、Tab 补全、点击填入与输入区快捷提示
[x] Agent Cockpit 本地工具快捷入口：Health/Tools/Runs/Workspace 快捷动作明确触发 daemon.health / plugins.list / runs.list / workspace.command 规则工具分支，避免配置 AI provider 后退化成普通聊天
[x] Agent Cockpit 自然语言控制入口：Quick actions、Slash Commands 和输入 placeholder 显示 Continue / Revise / Stop / New task，用户可直接像 Claude Code/Codex 一样通过聊天控制 Agent 状态机
[x] Agent Cockpit 能力可视化：Agent Capabilities 展示 capabilities、execution stages、内置工具数量，并按命令执行 / 文件与变更 / 运行态 / 插件分组展示 13 个 built-in tools
[x] Agent Timeline trace polish：Timeline 区分 planner / workspace tool / synthesis / auto_continue / next_action / approval gate，并展示 command_index / command_count 与 suggested_prompt
[x] Agent Reasoning / Plan Revision 可见性：Planner/Synthesis reasoning 写入 agent message；Chat 气泡、Runbook 推理面板、Timeline reasoning 阶段可同步展示，并持久化到 session
[x] Planner Action Queue Trace：LLM Planner 多个结构化 actions 执行时，每个 tool_call 都沉淀 action_queue_id / action_index / action_count / action_name；Cockpit Timeline 与工具徽章显示队列步骤
[x] Planner Mutating Workspace Action Explicit Intent Gate：LLM Planner 请求 workspace.files.write/patch/diff/diff_many/rebase/apply/discard 时，必须匹配最新用户消息的显式写入、修改、应用或丢弃意图；否则生成 policy_denied tool_call，保留 action queue 元数据并进入 synthesis，不落盘
[x] Agent approval/run/context 闭环：active run 恢复/读取时自动补齐 run context snapshot；pending approvals 自动把 next_action 标记为 approval_required；approval_queue / run / report_json / artifact_index / finding context_refs 自动沉淀；Run counts 增加 pending_approvals；report_json / finding / artifact_index refs 会提炼关键 finding / evidence / artifact 摘要；session list 返回 run_summary 供左侧会话卡片快速扫读
[x] 自然语言 Run Workspace Context 读取：聊天输入带 run_id 的 report_json / artifact_index / findings / events / approvals 上下文分析请求，会路由到 `GET /api/v1/runs/{run_id}/context` 工具观察，读取报告、证据索引、发现、事件时间线和审批队列样本，并沉淀 `run:<run_id>` 与对应 context_ref
[x] Agent Cockpit Staged Changes：pending_changes、diff preview、Open、Apply、Discard、Apply all、Discard all、conflict error 展示；冲突 Rebase 支持内联 old/new、工作区选区填充、片段预览与完整文件级预览
[x] Workspace command 执行加固：只读白名单、git 子命令白名单、workdir/参数逃逸限制、HTTP policy_denied/bad_request 映射
[x] RunService 只列出真实 run workspace：`.agent` 与普通非 run 目录不再污染最近扫描 / 历史列表
[x] python-js-route-analyzer 插件兼容旧 Python：移除 PEP 604 `| None` / 内置泛型写法，保证本机 python3 测试链路可运行
```

### Builtin Web/API Tools

```text
[x] HTTP Probe
[x] HTTP Fetcher
[x] Header Checker
[x] Cookie Checker
[x] robots.txt Fetch + Parse
[x] sitemap.xml Fetch + Parse
[x] HTML Extractor
[x] JavaScript Asset Collector
[x] Light JS Route Extractor
[x] API Hint Detector
[x] Small Content Discovery
[x] TLS Inspector
[x] TCP common web port check
```

### Plugin MVP

```text
[x] Plugin Manifest discovery
[x] Plugin Doctor dependency check
[x] Plugin Doctor schema check
[x] external_command runner boundary
[x] subprocess_jsonl runner boundary
[x] nuclei-safe: explicit opt-in, missing dependency skipped
[x] python-js-route-analyzer: explicit opt-in, offline JS artifact analysis
[x] ffuf-low-rate: explicit opt-in, safe profile 下 requires_approval 并跳过审计
```

---

## 3. 当前验证

已通过：

```bash
go test ./...
```

关键测试覆盖：

```text
scope required
safe planner optional plugins disabled by default
plugins doctor --json machine-readable health output
nuclei-safe explicit opt-in + missing dependency skip
python-js-route-analyzer explicit opt-in + route evidence
ffuf-low-rate explicit opt-in + approval skip
ffuf-low-rate approved 后可通过 replay --resume-approved 恢复执行
plugin timeout / output_limit / invalid_output 可审计并不会破坏 safe scan
run metadata / effective config / plugin snapshot 可复现快照
commands.log audit trail
state store step lifecycle
report/evidence/finding output
scan web / replay skipped steps summary
report plugin failure reason / plugin_failures output
TUI run viewer
```

---

## 3.1 最新本地验证记录

已通过：

```bash
go test ./...
go vet ./...
go build -ldflags "-X main.version=v0.1.0-test -X main.commit=local -X main.date=2026-05-24" -o bin/cyberagent ./cmd/cyberagent
```

Smoke test 已通过：

```text
testdata/simple-site
runs/smoke-simple-site
version / doctor / plugins list / plugins doctor / scan web / report / replay
report.json job.status = succeeded
report.md status = succeeded
events.jsonl contains job.finished
```

v0.1.1 P0 smoke test 已通过：

```text
docs/CyberAgent-v0.1.1-Smoke-Test.md
runs/smoke-v011-simple-site
runs/smoke-v011-ffuf-approval
baseline safe scan
metadata/effective_config/plugins.snapshot JSON assertions
no-scope rejection
ffuf-low-rate approval pending
approvals approve audit trail
fake ffuf resume-approved
CLI skipped step summary 由 go test ./internal/interfaces/cli 覆盖
report plugin failure reason 由 go test ./internal/core/reporting ./internal/core/runtime ./internal/interfaces/cli 覆盖
```

GoReleaser 已通过：

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean --skip=publish
```

v0.1.1 RC 本地发布前验证已通过：

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build -ldflags "-X main.version=v0.1.1-rc -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
go run "$(go env GOMODCACHE)/github.com/goreleaser/goreleaser/v2@v2.15.4" check -f .goreleaser.yml
```

备注：

```text
一次在线 go run github.com/goreleaser/goreleaser/v2@v2.15.4 check 因 Go proxy/sumdb 网络超时失败；
随后使用本地 module cache 中的 GoReleaser v2.15.4 对 .goreleaser.yml 执行 check，通过。
未创建 v0.1.1 tag，未 push，未发布 GitHub Release。
```

v0.1.2 Core Service API Slice 1 验证已通过：

```bash
gofmt -w internal/core/services
go test ./internal/core/services
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-services-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

Slice 1 边界记录：

```text
已新增 internal/core/services，仅提供内部 service 层能力。
本批未新增 daemon command，未新增 HTTP handler，未改变 CLI 行为。
RunService 支持读取已有 run 的 summary/detail/report/artifacts/events。
PluginService 复用现有 plugins.Manager / plugins.Doctor。
VersionService 复用 runtime doctor。
ApprovalService / ScanService / JobManager 未纳入 Slice 1；后续 Slice 已完成。
```

v0.1.2 Core Service API Slice 2 验证已通过：

```bash
gofmt -w internal/core/services internal/runtime/approvals
go test ./internal/core/services ./internal/runtime/approvals
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-services-slice2-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

Slice 2 当时边界：

```text
ApprovalService 已支持 list / approve / deny / resume-approved。
approve/deny 复用现有 approvals runtime，并支持 service/API 传入 operator/comment。
ScanService 当前只提供 StartWebScan request/response DTO 与 target/scope 校验骨架。
异步执行、job status/cancel、daemon command 和 HTTP handler 已在后续 daemon milestones 完成。
```

v0.1.2 Daemon Skeleton 验证已通过：

```bash
gofmt -w internal/interfaces/daemon internal/interfaces/cli
go test ./internal/interfaces/daemon ./internal/interfaces/cli
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-daemon-skeleton-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

当前边界：

```text
已新增 internal/interfaces/daemon local HTTP adapter。
cyberagent daemon 已接入 CLI。
P0 默认监听 127.0.0.1:8787，默认使用 bearer token。
token 文件不存在时自动生成，权限为 0600。
HTTP API 已统一 envelope，并完成 /api/v1/healthz 与 /api/v1/version。
doctor/plugins/runs/report/artifacts/events/approvals/approval write 已在后续 slice 接入；scans/jobs HTTP handler 仍待接入。
```

v0.1.2 Daemon Read-only API 验证已通过：

```bash
gofmt -w internal/interfaces/daemon internal/runtime/doctor
go test ./internal/interfaces/daemon ./internal/runtime/doctor
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-daemon-readonly-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

当前边界：

```text
已接入 /api/v1/doctor。
已接入 /api/v1/plugins 与 /api/v1/plugins/doctor。
已接入 /api/v1/runs、/api/v1/runs/{run_id}。
已接入 /api/v1/runs/{run_id}/report?format=json|md。
已接入 /api/v1/runs/{run_id}/artifacts。
已接入 /api/v1/runs/{run_id}/events?cursor=&limit=。
已接入 /api/v1/runs/{run_id}/approvals 只读列表。
已接入 /api/v1/runs/{run_id}/approvals/{approval_id}/approve。
已接入 /api/v1/runs/{run_id}/approvals/{approval_id}/deny。
已接入 /api/v1/runs/{run_id}/resume-approved。
doctor report JSON 字段已补充稳定 json tag。
Approval write API 与 resume-approved API 已复用 ApprovalService，并覆盖 bad JSON / missing approval / conflict / traversal 负向测试。
scan web async job API、job status/cancel 已接入 daemon router，默认 safe/no_external_tools，并覆盖 validation/concurrency/cancel 测试。
```

v0.1.2 Daemon Approval Write API 验证已通过：

```bash
gofmt -w internal/core/services/approvals.go internal/interfaces/daemon/router.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon ./internal/core/services
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-daemon-approval-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

v0.1.2 Daemon Async Scan Job API 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/jobs.go internal/interfaces/daemon/router.go internal/interfaces/daemon/server.go internal/interfaces/daemon/server_test.go
test -z "$(gofmt -l cmd internal)"
go test ./...
go vet ./...
go build -ldflags "-X main.version=v0.1.2-daemon-jobs-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

v0.1.2 Daemon curl smoke test 已通过：

```text
v0.1.2 daemon curl smoke passed
commit=0b39198
base_url=http://127.0.0.1:18787
target=http://127.0.0.1:18080
run_id=job_20260523_233011_f4ad61
approval_run_id=job_20260523_233014_1e42ec
```

v0.1.3 Agent-native 增量验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go internal/interfaces/cli/dashboard.go internal/interfaces/cli/root_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgent(CapabilitiesAPI|NaturalLanguageInvokesWorkspaceTools)' -count=1
go test ./internal/interfaces/cli ./internal/interfaces/daemon
go test ./...
cd web/frontend && npm run build
```

v0.1.3 覆盖重点：

```text
Agent capabilities 暴露 workspace.files.write / workspace.files.patch / workspace.files.diff / workspace.files.apply / workspace.files.discard。
自然语言“写入文件 notes.md 内容: ...”触发 workspace.files.write。
自然语言“先执行 pwd 和 ls -la，再总结”会触发两个有序 workspace.command tool_call，result 中包含 command_index / command_count，响应消息汇总每个命令的状态和关键输出。
配置 AI provider 后，确定性工具分支也会在返回原始工具结果后自动调用 LLM synthesis，基于 observations 生成总结、TODO 和 next_action。
自然语言“修改文件 notes.md old: ... new: ...”触发 workspace.files.patch，并返回 diff preview。
自然语言“预览修改文件 notes.md old: ... new: ...”触发 workspace.files.diff，生成 pending change_id 且不修改文件。
自然语言“应用 change_id: ...”触发 workspace.files.apply，重新校验 old 文本后落盘，并把 pending change 标记为 applied；若文件内容已变化导致 old 文本不再唯一匹配，则返回 409 并把 pending change 标记为 conflict。
自然语言“丢弃 change_id: ...”触发 workspace.files.discard，不修改文件并把 pending/conflict change 标记为 discarded。
自然语言“应用所有待应用修改 / 丢弃所有待应用修改”会批量处理所有 pending staged changes，并返回 succeeded/failed 统计。
HTTP / Agent tool 支持 `workspace.files.diff_many`：一次预览多个文件修改，返回共享 `group_id` 与每个文件的 `change_id`；可按 group_id 批量 apply/discard；apply-group 会先预检所有文件，任一冲突则返回 409 且不写入任何文件。
HTTP / Agent tool 支持 `workspace.files.rebase`：对 pending/conflict change 提供当前文件中的新 old 文本后，生成新的 pending change 并 supersede 旧 change；Agent Cockpit 冲突项提供内联 Rebase 表单、工作区选区填充 old 文本、本地片段 diff 预览与完整文件级 diff 预览，未打开文件、未命中、多处命中等状态会在提交前提示。
自然语言读取同一文件可验证 patch 后内容。
session 持久化保存 workspace.files.write / workspace.files.patch / workspace.files.diff / workspace.files.apply / workspace.files.discard tool_call 与 pending_changes。
Agent Cockpit 展示 pending_changes，可在右侧查看 diff 并执行 Apply / Discard / Apply all / Discard all；apply 冲突时刷新 session 并展示 conflict error，冲突项可 Discard 清理，也可用 Rebase 表单基于当前文件生成新的 staged change。
context_refs 保存 workspace_file:notes.md。
Dashboard 启动时可从本地 loopback bootstrap 自动读取 daemon URL/token，避免首次请求 401。
真实浏览器 smoke 已验证：/agent 自然语言写入 handoff.md、读取 handoff.md、刷新后会话保持、tool call 与工作区文件仍可见。
真实浏览器 smoke 发现并修复：runs/.agent 内部状态目录不再作为“最近扫描”显示；RunService 已收紧为只列出真实 run workspace；/api/v1/runs 不返回 .agent 或普通非 run 目录，直接访问 /api/v1/runs/.agent 返回 400 bad_request。
Workspace command 已收紧为低风险读命令白名单；rm/mv/python/node/curl 等默认拒绝；`../`、绝对路径和逃逸 workdir 会被拒绝并映射为稳定 API 错误。
Agent 会话读取 active_run 时会同步 pending approvals：若存在待审批请求，next_action 变为 approval_required，并在 Cockpit 中展示审批计数、step_id/policy/risk_summary、approve/deny/resume 操作；approval check 生成 run 与 approval context_refs。
Agent 会话读取/列出 active_run 时会自动补齐 run context snapshot：根据当前 run summary/counts/report availability 生成 run、report_json、artifact_index、finding、approval context_refs，并在可安全覆盖的情况下刷新 ActiveTask summary。
active_run context snapshot 会优先读取 report.json，提炼 lead finding / lead evidence；artifact_index ref 会读取 report artifacts 或 fallback artifacts/index.json，摘要可直接包含 Missing CSP / HTTP status / headers.json / evidence_001 这类关键证据线索。
Agent session list response 已新增 run_summary；Agent Cockpit 左侧会话卡片展示 active run 状态、findings/artifacts/events/pending approvals 指标与关键上下文摘要，并支持全部 / 运行 / 审批 / 发现 / 阻塞筛选、跨会话搜索与本地置顶收藏。
运行详情页支持 `?step=step_id` 深链定位；Agent Cockpit 审批队列中的 step_id 可直接跳转到对应 run timeline，ScanDetail 会高亮匹配事件、审批和 finding。
运行详情页新增按 step 聚合的执行链视图，串联 events / approvals / findings / artifacts，并可点击 step 卡片复用深链定位。
运行详情页新增审计搜索与类型过滤：可跨 step/event/finding/artifact/approval 搜索，并按错误、警告、发现、证据、审批过滤执行链与事件列表。
运行详情页新增单 step 详情抽屉：打开 `?step=step_id` 后集中展示该 step 的事件、审批、发现、证据和状态计数，便于按 Agent 执行步骤复盘。
运行详情页新增 Run Evidence 工作台：聚合 findings/artifacts/approvals/report summary，提供 overview、分类视图、lead finding、latest artifact、pending approval、关键 evidence steps，并可跳转 step 或联动审计过滤。
Agent Cockpit 新增 Agent Evidence Explain 快捷动作：基于 active_run、context_refs、pending approvals 自动构造解释证据 prompt，让 Agent 给出证据结论、支撑引用、审批阻塞和下一步安全动作。
Run Evidence 工作台新增 Markdown / JSON 导出：可生成包含 run/counts/findings/artifacts/approvals/evidence_steps/step_groups/report 的审计证据包。
Agent Cockpit 新增 Evidence → Action：基于 active_run/context_refs/pending approvals 构造“证据转行动计划”prompt，并通过 control revise 生成可继续执行的 next_action/suggested_prompt。
Evidence → Action 后端回归测试已覆盖 revise/continue 状态闭环：active_run、hydrated context_refs、approval gate、running todo 与 suggested_prompt 不丢失。
Evidence → Action 后端会将证据行动 prompt 转为多步 TODO：待审批 gate 优先，然后读取证据上下文、关联 finding/artifact/step、形成安全动作与复测计划。
Evidence → Action 若包含 pending approvals / 待审批 / approval gate，control revise 与 continue 都会保持 next_action=approval_required，避免误导 Agent 在审批未处理时继续执行。
Cockpit approval gate UX 已对齐后端语义：approval_required 时禁用 Continue，提示先处理审批队列，不再提示“点击继续执行”。
Cockpit approve/deny/resume-approved 后会立即刷新 active scan、live events、approval queue、当前 Agent session 与 session list，避免审批处理后 UI 停留在旧 gate 状态。
后端读取 active_run session 时会在 pending approvals 清空后自动将 approval_required 收敛为 continue，清理审批 gate TODO 的 blocked 状态并恢复后续 TODO，避免 UI 刷新后仍卡在旧 gate。
RunService counts 已增加 pending_approvals；前端 Active Run 卡片展示 findings/artifacts/events/pending approvals，并可一键让 Agent 总结运行证据。
Agent Cockpit Runbook 增强 Context refs 面板，合并 session context_refs、active run、findings 与 approval gate，形成 task → todo → tool_calls → context_refs → run/approval 的可视化闭环。
Agent Cockpit Timeline 已增强为 trace 视图：区分 planner、workspace tool、tool_synthesis、auto_continue、next_action、approval gate 等阶段，并显示多命令顺序和 next suggested_prompt。
python-js-route-analyzer 插件已改用 `typing.Optional/List/Dict/Set`，修复旧 Python 环境下 `Path | None` 语法导致插件测试失败的问题。
```

v0.1.4 Agent context snapshot 摘要验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentActiveRunApprovalGateAndContextRefs' -count=1
go test ./internal/interfaces/daemon -run 'TestHTTPAgentActiveRunApprovalGateAndContextRefs|TestHTTPAgentTerminalActiveRunSyncsOnReadAndList|TestHTTPAgentSessionAndMessageAPIs' -count=1
cd web/frontend && npm run build
go test ./...
```

v0.1.4 approval gate resolved state convergence 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentTerminalActiveRunSyncsOnReadAndList|TestHTTPAgentEvidenceToActionControlPreservesRunAndApprovalContext|TestHTTPAgentActiveRunApprovalGateAndContextRefs' -count=1
cd web/frontend && npm run build
go test ./...
```

v0.1.5 规则 Agent 多命令顺序执行验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentNaturalLanguageInvokesWorkspaceTools' -count=1
cd web/frontend && npm run build
go test ./...
```

v0.1.5 routed tool synthesis 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentRoutedToolsAreSynthesizedWhenProviderConfigured|TestHTTPAgentNaturalLanguageInvokesWorkspaceTools|TestHTTPAgentLLMPlannerInvokesStructuredToolActions' -count=1
cd web/frontend && npm run build
go test ./...
```

v0.1.5 Timeline trace polish 验证已通过：

```bash
cd web/frontend && npm run build
go test ./internal/interfaces/daemon -run 'TestHTTPAgentRoutedToolsAreSynthesizedWhenProviderConfigured|TestHTTPAgentNaturalLanguageInvokesWorkspaceTools' -count=1
go test ./...
```

v0.1.6 Agent reasoning / plan revision 可见化验证已通过：

```bash
cd web/frontend && npm run build
go test ./internal/interfaces/daemon -run 'TestHTTPAgentMessageAppliesLLMPlannerState|TestHTTPAgentLLMPlannerInvokesStructuredToolActions|TestHTTPAgentRoutedToolsAreSynthesizedWhenProviderConfigured|TestHTTPAgentNaturalLanguageInvokesWorkspaceTools' -count=1
go test ./...
```

v0.1.6 Planner action queue trace 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
cd web/frontend && npm run build
go test ./internal/interfaces/daemon -run 'TestHTTPAgentLLMPlannerInvokesStructuredToolActions|TestHTTPAgentLLMNativeToolCallsAreExecutedAndSynthesized' -count=1
go test ./...
```

v0.1.6 Agent API alias / capabilities discovery 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/router.go internal/interfaces/daemon/server_test.go
cd web/frontend && npm run build
go test ./internal/interfaces/daemon -run 'TestHTTPAgentCapabilitiesAPI|TestHTTPAgentSessionAndMessageAPIs' -count=1
go test ./...
```

v0.1.8 Agent Cockpit Slash / 本地工具调用 UX 验证已通过：

```bash
cd web/frontend && npm run build
go test ./internal/interfaces/daemon ./internal/interfaces/cli
go test ./internal/interfaces/daemon -run 'TestHTTPAgent(NaturalLanguageControlIntents|SessionCompactsLongConversationWithMemorySummary|CockpitShortcutPromptsRouteToLocalTools)' -count=1
go test ./...

# 8080 dashboard 已加载最新 dist：
curl -s http://127.0.0.1:8080/agent | rg 'assets/index-'
sed -n '1,30p' internal/interfaces/web/frontend/dist/index.html | rg 'assets/index-'
```

真实浏览器 smoke（Playwright CLI）已通过：

- `/tools` → 触发 `GET /api/v1/plugins`，Timeline / Runbook 显示 `plugins.list` 工具观察与 context ref。
- `/health` → 触发 `GET /api/v1/doctor`，显示 daemon 在线、版本、检查项与缺失可选工具。
- `/runs` → 触发 `GET /api/v1/runs`，显示最近运行列表与 `runs-dir` 工具观察。
- “工作区”快捷指令 → 发送 `执行命令 \`pwd\` 和 \`ls -la\``，顺序生成 2 个 `workspace.command` tool_call，显示 `1/2`、`2/2`、`risk:low`、`auto` 与命令输出。
- Agent 输入区已显示 Slash 快捷入口、`Enter` / `Tab` / `↑/↓` / `/` 辅助提示；Slash Commands 已包含 `/continue`、`/revise`、`/stop` 控制命令；Agent Capabilities 已按命令执行 / 文件与变更 / 运行态 / 插件分组展示 13 个内置工具。
- 聊天自然语言控制已覆盖：`继续：...`、`修订计划：...`、`停止：...`、`新任务：清空任务...` 会进入 control 状态机；配置 AI provider 时 `Continue the analysis...` 仍会调用 LLM，避免自然语言控制抢断长会话总结链路。
- Agent Cockpit Quick actions 与 Slash Commands 已暴露 Continue / Revise / Stop / New task，并在输入 placeholder 中提示自然语言控制能力。
- 自然语言 Run Workspace Context 读取已覆盖：`report json`、`artifact index`、`findings`、`events`、`approvals` 请求均路由到 run context 工具，返回对应样本数据并沉淀 context_refs。

v0.1.8 Run Workspace Context Tools 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run TestHTTPAgentNaturalLanguageRunWorkspaceContext -count=1
```

v0.1.8 Natural-language Scan Workflow State 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentSessionContinuityAcrossMessages|TestHTTPAgentNaturalLanguageRunWorkspaceContext' -count=1
```

v0.1.8 Continue Active Run Context 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgent(SessionContinuityAcrossMessages|NaturalLanguageControlIntents|SessionCompactsLongConversationWithMemorySummary|NaturalLanguageRunWorkspaceContext)' -count=1
```

v0.1.8 Active Run Evidence Action Auto Plan 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentSessionContinuityAcrossMessages' -count=1
```

v0.1.8 Evidence Action Stepper 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentSessionContinuityAcrossMessages' -count=1
```

v0.1.8 Evidence Action Stage Conclusion 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentSessionContinuityAcrossMessages' -count=1
```

v0.1.8 Evidence Action Stepper Final Convergence 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgentSessionContinuityAcrossMessages' -count=1
```

v0.1.8 Chat-native Approval Decision 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgent(ChatNativeApprovalDecisions|ActiveRunApprovalGateAndContextRefs|EvidenceToActionControlPreservesRunAndApprovalContext)' -count=1
go test ./internal/interfaces/daemon ./internal/interfaces/cli
npm run build --prefix web/frontend
go test ./...
```

v0.1.8 Chat-native Resume Approved 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgent(ChatNativeApprovalDecisions|ChatNativeResumeApproved|ActiveRunApprovalGateAndContextRefs)' -count=1
go test ./internal/interfaces/daemon ./internal/interfaces/cli
npm run build --prefix web/frontend
go test ./...
```

v0.1.8 Planner Approval Tool Actions 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgent(PlannerApprovalActionRequiresExplicitPrompt|ChatNativeApprovalDecisions|ChatNativeResumeApproved|LLMPlannerInvokesStructuredToolActions|CapabilitiesAPI)' -count=1
go test ./internal/interfaces/daemon ./internal/interfaces/cli
npm run build --prefix web/frontend
go test ./...
```

v0.1.8 Cockpit Approval Agent Shortcuts 验证已通过：

```bash
npm run build --prefix web/frontend
go test ./internal/interfaces/daemon ./internal/interfaces/cli
go test ./...
```

v0.1.8 Agent Cockpit Inspector Tabs / Browser Smoke 验证已通过：

```bash
npm run build --prefix web/frontend
go build -o .tmp/cyberagent-dev ./cmd/cyberagent

PATH="$PWD/.tmp/noopbin:$PATH" .tmp/cyberagent-dev daemon --listen 127.0.0.1:8787
PATH="$PWD/.tmp/noopbin:$PATH" .tmp/cyberagent-dev dashboard --addr 127.0.0.1:8080 --daemon-url http://127.0.0.1:8787

# Playwright browser smoke:
# open /agent → visible Agent chat + session list + Inspector tabs
# click 轨迹 / 能力 / 工作区 → each panel renders expected content
```

v0.1.8 Auto-continue Stepper 验证已通过：

```bash
gofmt -w internal/interfaces/daemon/agent.go internal/interfaces/daemon/server_test.go
go test ./internal/interfaces/daemon -run 'TestHTTPAgent(NaturalLanguageContinueExecutesAutoContinuePrompt|NaturalLanguageControlIntents|LLMPlannerInvokesStructuredToolActions|CapabilitiesAPI)' -count=1
```

---

## 4. 里程碑状态

| Milestone | 状态 | 说明 |
|---|---|---|
| M0 Planning Complete | 完成 | 规划文档已冻结 |
| M1 Project Skeleton Ready | 完成 | CLI 可编译运行 |
| M2 Core Runtime Ready | 完成 | State/Event/Policy/Scope/Workspace 已实现 |
| M3 Safe Profile Plan Ready | 完成 | safe web plan 已实现 |
| M4 Builtin Web Analysis Ready | 完成 | 内置 Web 分析闭环已实现 |
| M5 CLI Safe Scan Ready | 完成 | `scan web` 可生成 run/report |
| M6 Plugin MVP Ready | 基本完成 | 三个插件均进入 MVP 链路；ffuf 当前默认审批跳过 |
| M7 TUI Run Viewer Ready | 完成 | 只读打开已有 run |
| M8 MVP Release Candidate | 完成 | README/限制/示例/test/vet/build/smoke/GoReleaser snapshot 已完成 |
| M9 MVP v0.1 Release | 本地准备完成 | 本地 git/origin/release notes/GoReleaser snapshot 完成；本地 tag 指向本提交，尚未 push/release 到 GitHub |
| M10 v0.1.1 RC | 本地准备完成 | v0.1.1 P0/P1、release notes、test/vet/build 本地验证完成；尚未创建 tag、push 或 GitHub Release |
| M11 v0.1.2 Planning | 完成 | 选择 Local Daemon / Web API foundation；计划文档已新增，后续实现已推进至 M20 |
| M12 v0.1.2 Implementation Breakdown | 完成 | 工作流、tickets、依赖、验收、测试与提交节奏已拆解，后续实现已推进至 M20 |
| M13 v0.1.2 API Contract Draft | 完成 | HTTP API envelope、auth、错误码、endpoint 与请求/响应示例已形成草案；scan/job API 已实现 |
| M14 v0.1.2 Daemon Smoke Test Draft | 完成 | daemon 启动、认证、scan/job/report/artifact/event/approval/cancel smoke 流程已形成草案；curl smoke 已本地通过 |
| M15 v0.1.2 Core Service Slice 1 | 完成 | ServiceError、VersionService、PluginService、RunService 只读能力已实现并测试；daemon 后续 milestones 已完成 |
| M16 v0.1.2 Core Service Slice 2 | 完成 | ApprovalService、ResumeApproved 与 ScanService 校验骨架已实现并测试；daemon job manager 已在 M20 完成 |
| M17 v0.1.2 Daemon Skeleton | 完成 | cyberagent daemon、token auth、response envelope、healthz/version API 已实现并测试；后续业务 API 已逐步接入 |
| M18 v0.1.2 Daemon Read-only API | 完成 | doctor/plugins/runs/report/artifacts/events/approvals list 已接入并测试；approval write 已在 M19 完成，scan/job API 已在 M20 完成 |
| M19 v0.1.2 Daemon Approval Write API | 完成 | approval approve/deny/resume-approved 已接入并测试；scan/job API 已在 M20 完成 |
| M20 v0.1.2 Daemon Async Scan Job API | 完成 | POST /scans/web、GET /jobs/{job_id}、POST /jobs/{job_id}/cancel 已接入并测试 |
| M21 v0.1.3 Agent-Native Foundation | 完成 | Agent session/task/todo/tool_calls/context_refs/next_action、控制流、Planner/tool loop 与 Agent Cockpit 已完成第一轮 |
| M22 v0.1.3 Local Dashboard + Workspace Tools | 完成 | Dashboard loopback daemon bootstrap、active_run 终态同步、自然语言 workspace command/list/read/write/patch 已完成并测试 |
| M23 v0.1.4 Workspace Staged Edit | 完成 | 单文件与多文件 diff preview → pending_changes/group_id → apply/discard/rebase；group apply 冲突不部分落盘，可审计且 apply 前重新校验 |
| M24 v0.1.4 Active Run Evidence Snapshot | 完成 | active_run 自动 hydrate run/report_json/artifact_index/finding/approval refs，并从 report.json / artifacts index 提炼 finding/evidence/artifact 摘要 |
| M25 v0.1.4 Session Run Summary | 完成 | Agent session list 暴露 run_summary，Cockpit 左侧会话卡片展示 run 指标、关键上下文、快速筛选、跨会话搜索与置顶收藏 |
| M26 v0.1.4 Step Timeline Deep Link | 完成 | /scan/{run_id}?step={step_id} 自动定位并高亮事件/审批/finding；Agent 审批 step_id 可直接跳转 |
| M27 v0.1.4 Step Execution Chain | 完成 | ScanDetail 按 step_id 聚合 events/approvals/findings/artifacts，形成可审计执行链 |
| M28 v0.1.4 Run Audit Search | 完成 | ScanDetail 支持跨 step/event/finding/artifact/approval 的审计搜索与类型过滤 |
| M29 v0.1.4 Step Detail Drawer | 完成 | ScanDetail 按 step 打开详情抽屉，集中复盘事件、审批、发现与证据 |
| M30 v0.1.4 Run Evidence Workbench | 完成 | ScanDetail 聚合 findings/artifacts/approvals/report summary，并联动 step 深链与审计过滤 |
| M31 v0.1.4 Agent Evidence Explain | 完成 | Agent Cockpit 可一键基于 active run/context refs/pending approvals 让 Agent 解释证据与下一步 |
| M32 v0.1.4 Run Evidence Export | 完成 | Run Evidence 工作台支持 Markdown / JSON 证据包导出 |
| M33 v0.1.4 Evidence to Action | 完成 | Agent Cockpit 可将 active run evidence 转为 next_action/suggested_prompt 并由 Continue 执行 |
| M34 v0.1.4 Evidence Action TODO Plan | 完成 | control revise 将 Evidence → Action prompt 转为有序 TODO，审批 gate 优先 |
| M35 v0.1.4 Evidence Action Approval Gate | 完成 | Evidence → Action 含审批 gate 时 revise/continue 均保持 approval_required |
| M36 v0.1.4 Cockpit Approval Gate UX | 完成 | approval_required 时 Cockpit 禁用 Continue 并引导先处理审批队列 |
| M37 v0.1.4 Approval Resolution State Refresh | 完成 | approve/deny/resume 后刷新 active scan、approval queue、Agent session 和 session list |
| M38 v0.1.4 Approval Gate State Convergence | 完成 | pending approvals 清空后后端 session read 自动将 approval_required 收敛为 continue，并清理 blocked 审批 TODO |
| M39 v0.1.5 Rule Agent Sequential Commands | 完成 | 自然语言中多个低风险命令可顺序执行，沉淀多个 workspace.command tool_call 并带 command_index / command_count |
| M40 v0.1.5 Routed Tool Synthesis | 完成 | 配置 AI provider 时，规则 Agent / daemon tool 分支执行后自动 observe→synthesize，更新 TODO 与 next_action |
| M41 v0.1.5 Timeline Trace Polish | 完成 | Cockpit Timeline 展示 planner/tool/synthesis/auto/next/gate 阶段、命令顺序和 suggested_prompt |
| M42 v0.1.6 Agent Reasoning / Plan Revision Visibility | 完成 | Planner/Synthesis reasoning 随 agent message 返回并持久化；Cockpit Chat、Runbook、Timeline 展示推理摘要、TODO 进度和 next_action |
| M43 v0.1.6 Planner Action Queue Trace | 完成 | Planner 多个结构化 actions 共享 action_queue_id，并按 action_index/action_count/action_name 在 API、持久化 session、Cockpit Timeline/工具徽章中可追踪 |
| M44 v0.1.6 Planner Mutating Workspace Intent Gate | 完成 | LLM Planner 对 write/patch/diff/apply/discard 等 workspace 变更动作必须匹配最新用户消息显式意图；否则以 policy_denied tool_call 进入 observation/synthesis，保留队列元数据且不落盘 |
| M45 v0.1.6 Agent API Aliases / Capability Discovery | 完成 | 补齐 benchmark 点名的 /state、/context、/continue、/stop、/new-task 端点别名，并在 capabilities 中暴露 endpoint 清单；前端 API client 类型同步 |
| M46 v0.1.8 Agent Cockpit Slash & Session UX | 完成 | Cockpit 支持会话消息/草稿缓存、输入历史、Slash 命令面板、Tab 补全、点击填入、/continue、/revise、/stop、/new 与 /clear |
| M47 v0.1.8 Agent Local Tool Shortcuts & Capability UX | 完成 | Health/Tools/Runs/Workspace 快捷入口明确触发 daemon.health/plugins.list/runs.list/workspace.command 本地工具分支；Capabilities 按工具类别分组展示 |
| M48 v0.1.8 Chat-native Agent Control | 完成 | 聊天输入“继续/修订计划/停止/新任务”可直接驱动 Agent control 状态机；Continue/Revise 在 AI provider 配置下保持 LLM 执行链；Cockpit Quick actions、Slash Commands 与 placeholder 已同步 |
| M49 v0.1.8 Run Workspace Context Tools | 完成 | 聊天输入带 run_id 的 report_json/artifact_index/findings/events/approvals 上下文请求会读取对应 run workspace，并以工具观察、样本摘要和 context_refs 进入 Agent 会话 |
| M50 v0.1.8 Natural-language Scan Workflow State | 完成 | 聊天自然语言启动扫描后，Agent 会同步 target/scope/profile/job/run/next_step、scan TODO、run/job context_refs 与 auto_continue next_action，形成从启动扫描到跟踪运行/读取报告的可恢复工作流 |
| M51 v0.1.8 Continue Active Run Context | 完成 | 聊天输入“继续/请继续监控/继续跟踪”且会话存在 active_run 时，自动读取 pending approvals/report_json/findings/artifact_index/events 中最合适的 run context，并把观察、context_refs 与 TODO 状态写回会话 |
| M52 v0.1.8 Active Run Evidence Action Auto Plan | 完成 | active run report_json/findings 读取到 findings 后，自动生成证据链关联、误报检查、修复建议和复测计划 TODO；终态 run 同步保留这些后续行动，不误标完成 |
| M53 v0.1.8 Evidence Action Stepper | 完成 | 自动证据行动计划可通过连续“继续”逐步推进，每步重新读取 active run context 并推进一个 TODO，避免停留在静态计划 |
| M54 v0.1.8 Evidence Action Stage Conclusion | 完成 | Evidence Action Stepper 每步都会追加可读 agent 结论消息，解释证据链、误报/影响、修复/复测阶段的实际含义 |
| M55 v0.1.8 Evidence Action Stepper Final Convergence | 完成 | 连续“继续”完成证据链、误报/影响、修复/复测三个行动 TODO 后，保留 wait_user next_action，引导用户导出报告、复测或开始新任务 |
| M56 v0.1.8 Chat-native Approval Decision | 完成 | Agent 聊天可直接批准/拒绝 active run 或显式 run 的 approval/step，调用 ApprovalService 写入 resolution，沉淀 approval_decision tool_call/context_refs，并在 pending 清空后恢复 next_action=continue |
| M57 v0.1.8 Chat-native Resume Approved | 完成 | Agent 聊天可直接执行“恢复已批准运行 / resume-approved”，调用 ApprovalService.ResumeApproved，记录 approval_resume tool_call、resume_approved context_ref，并恢复后续 tracking TODO |
| M58 v0.1.8 Planner Approval Tool Actions | 完成 | LLM Planner 可在明确用户指令下调用 approval list/approve/deny/resume-approved 工具；capabilities 暴露工具 schema 与 medium/non-auto-run 风险标记，执行链保留 action_queue_id/action_name，审批写入 operator/comment metadata |
| M59 v0.1.8 Cockpit Approval Agent Shortcuts | 完成 | Agent Cockpit 审批队列将 Agent approve/deny/resume-approved 作为主操作，直接 API 按钮作为次级 fallback；Slash Commands 和输入提示同步暴露审批/恢复能力，避免用户只能使用普通系统按钮 |
| M60 v0.1.8 Cockpit Inspector Tabs | 完成 | Agent Cockpit 右侧 Inspector 收敛为运行簿 / 轨迹 / 能力 / 工作区四个焦点页签，减少长页面堆叠，并持久化当前页签便于继续工作 |
| M61 v0.1.8 Auto-continue Stepper | 完成 | 无需再次调用 LLM 时，“继续”可执行上轮明确标记 auto_continue 的低风险只读 suggested_prompt；扫描、审批决策、resume-approved、workspace 写入/应用/丢弃仍需显式用户指令或审批 gate |

---

## 5. 待完成项

### MVP Stabilization

```text
[x] 生成或整理一个可提交的示例 run / testdata
[x] 最终检查 README 与 docs 是否完全一致
[x] 明确 LLM agent 代码路径是 experimental，不作为 MVP Core 默认路径
[x] 清理重复 release 配置：.goreleaser.yaml / .goreleaser.yml 二选一
[x] 最终 smoke test：build + scan + report + replay + plugin doctor；TUI 由测试覆盖 LoadRun/viewer
```

### Release

```text
[x] 初始化或确认 git 仓库
[x] 配置远程 GitHub 仓库：github.com/H1d3rOne/cyberagent
[x] 准备 v0.1.0 release notes
[x] 验证 goreleaser build
[x] 创建本地 tag v0.1.0
[ ] 推送 tag v0.1.0
[ ] 发布 GitHub Release 二进制产物
```

### v0.1.1 下一阶段

```text
[x] 生成 v0.1.1 计划文档：docs/CyberAgent-v0.1.1-Plan.md
[x] CI: test/vet/build
[x] release checklist 与 README 安装说明更新
[x] PolicyDecision(require_approval) / approval request 持久化结构
[x] approvals list/approve/deny CLI MVP
[x] resume approved step
[x] plugin timeout/output/schema hardening
[x] run metadata/effective config/plugin snapshot
[x] v0.1.1 smoke test 文档
[x] plugins doctor --json
[x] CLI/TUI/report UX polish
[x] v0.1.1 release notes：docs/release-notes/v0.1.1.md
[x] v0.1.1 本地 tag 可准备（未创建；需用户显式授权）
```

### v0.1.2 Local Daemon / Web API Foundation

```text
[x] 选择 v0.1.2 方向：Web API / Local Daemon foundation
[x] 生成 v0.1.2 计划文档：docs/CyberAgent-v0.1.2-Plan.md
[x] 生成 v0.1.2 实施拆解文档：docs/CyberAgent-v0.1.2-Implementation-Breakdown.md
[x] 生成 v0.1.2 API Contract 草案：docs/api/v0.1.2-api-contract.md
[x] 生成 v0.1.2 daemon smoke test 草案：docs/CyberAgent-v0.1.2-Daemon-Smoke-Test.md
[x] Core Service API Slice 1：ServiceError / VersionService / PluginService / RunService 只读能力
[x] Core Service API Slice 2：ApprovalService / ResumeApproved / ScanService validation skeleton
[x] cyberagent daemon 命令
[x] 本地 token auth / 127.0.0.1 默认监听
[x] /api/v1/healthz 与 /api/v1/version
[x] /api/v1/doctor 与 plugins API
[x] run/report/artifact/event/approval list 查询 API
[x] approval approve/deny/resume-approved API
[x] scan web async job API
[x] job status/cancel MVP
[x] daemon smoke test 本地验证通过
[x] v0.1.2 release notes：docs/release-notes/v0.1.2.md
```

### v0.1.3 Agent-Native Correction

```text
[x] 生成 v0.1.3 Agent-Native 纠偏计划：docs/CyberAgent-v0.1.3-Agent-Native-Plan.md
[x] 后端 Agent Session 持久化
[x] 后端 ActiveTask / Todo / ToolCall / ContextRef / NextAction
[x] 后端 Planner JSON 与原生 tool_calls 低风险内置工具执行
[x] 后端 continue / stop / revise / new_task 控制流
[x] 前端 Agent Cockpit 与会话列表
[x] 总览 / Agent / 任务职责边界调整
[x] 设置保存与统一 AI provider 配置
[x] Local Dashboard 自动接入 daemon URL/token
[x] Agent active_run 终态同步修复
[x] 自然语言 workspace command/list/read/write/patch/diff/apply
[x] workspace file context ref 沉淀
[x] workspace pending_changes 持久化，支持 staged diff/apply/discard
[x] Agent Cockpit pending_changes UI，支持查看 diff、Apply 与 Discard
[x] Agent approval/run/context 可视化闭环：pending approvals 自动进入 approval_required，active_run 恢复自动补齐 run context snapshot，Cockpit 展示 active run 指标、审批详情与上下文引用
[x] active_run context snapshot 摘要提炼：report_json / finding / artifact_index refs 包含 lead finding / evidence / artifact，ActiveTask summary 同步关键证据摘要
[x] Agent session list run_summary：左侧会话列表展示 run 状态、findings/artifacts/events/pending approvals 与关键上下文摘要，并支持全部/运行/审批/发现/阻塞筛选、跨会话搜索、本地置顶收藏
[x] Step timeline deep-link：Agent Cockpit 审批 step_id → /scan/{run_id}?step={step_id}，运行详情自动高亮事件/审批/finding
[x] Step execution chain：运行详情按 step 聚合 events/approvals/findings/artifacts，并可点击 step 卡片定位
[x] Run audit search/filter：运行详情支持跨 step/event/finding/artifact/approval 搜索，并按错误/警告/发现/证据/审批过滤执行链和事件列表
[x] Step detail drawer：运行详情按 step 展示事件、审批、发现和证据详情抽屉，关闭后清除 step 深链
[x] Run Evidence workbench：运行详情聚合 findings/artifacts/approvals/report summary，提供 overview、分类视图、关键 evidence steps 与 step 跳转
[x] Agent Evidence Explain：Cockpit 基于 active_run/context_refs/pending approvals 自动构造 prompt，让 Agent 解释证据、支撑引用、审批 gate 与下一步动作
[x] Run Evidence Export：运行详情可导出 Markdown / JSON evidence package，包含 run/counts/findings/artifacts/approvals/evidence_steps/step_groups/report
[x] Evidence to Action：Cockpit 基于 active_run/context_refs/pending approvals 自动构造行动计划 prompt，通过 revise 写入 next_action/suggested_prompt；后端测试覆盖 revise/continue 状态闭环
[x] Evidence Action TODO Plan：后端识别 Evidence → Action prompt 并生成多步 TODO，覆盖审批 gate、证据读取、finding/artifact/step 关联和复测计划
[x] Evidence Action Approval Gate：Evidence → Action 含 pending approvals/待审批/approval gate 时，revise/continue 均保持 next_action=approval_required
[x] Cockpit Approval Gate UX：approval_required 时禁用 Continue，系统提示和 Next Action 区域都引导先处理审批队列
[x] Approval Resolution State Refresh：approve/deny/resume-approved 后同步刷新 active scan、approvals、Agent session 和 session list
[x] Approval Gate State Convergence：pending approvals 清空后，后端自动恢复 next_action=continue，并清理审批 gate TODO 的 blocked 状态
[x] Chat-native Approval Decision：聊天消息可自然语言执行 approve/deny，支持 comment 元数据、approval_id/step_id 解析、active_run 兜底、唯一 pending approval 自动选择和多审批显式指定保护
[x] Chat-native Resume Approved：聊天消息可自然语言执行 resume-approved，支持 active_run 兜底、显式 run_id、approval_resume metadata、resume_approved context_ref 与恢复后 tracking TODO
[x] Planner Approval Tool Actions：capabilities 与 Planner schema 增加 runs.approvals.list / approve / deny / runs.resume_approved；高影响审批写动作通过 latest prompt explicit intent gate 防误执行
[x] Cockpit Approval Agent Shortcuts：pending approval 卡片以 Agent approve/deny 为主，API approve/deny 为次；resolved approval 区域以 Agent resume + track 为主，API resume 为次；Slash quick chips 会根据 active run / pending approvals 动态给出审批指令
[x] Rule Agent Sequential Commands：规则 Agent 支持从自然语言中提取多个低风险命令并顺序执行，补齐“先做 A 再做 B”的基础 Agent 执行体验
[x] Routed Tool Synthesis：确定性工具结果可自动进入 LLM synthesis，使自然语言工具分支也具备 observe → summarize → next_action 闭环
[x] Timeline Trace Polish：前端执行 Timeline 以阶段 badge、命令顺序、auto/gate/next 标记呈现 Agent 执行轨迹
[x] Agent Reasoning / Plan Revision Visibility：Agent 回复、Runbook 和 Timeline 可见 reasoning，后端测试覆盖 reasoning 返回与持久化
[x] Planner Action Queue Trace：多动作 Planner 执行链具备 action_queue_id/action_index/action_count/action_name 元数据，前端可显示队列步骤
[x] Planner Mutating Workspace Intent Gate：Planner 请求变更 workspace 但用户最新消息未明确授权时，后端拒绝执行并返回 policy_denied observation，synthesis 引导用户明确路径/内容/变更意图
[x] Agent API Aliases / Capability Discovery：补齐 `/state`、`/context`、`/continue`、`/stop`、`/new-task`，并通过 capabilities 暴露 endpoint 清单
[x] RunService 只列出真实 run workspace，避免 runs/.agent 或普通非 run 目录污染 run/history UI
[x] 回归测试覆盖 Agent capabilities 与自然语言 workspace 工具链
```

### 后置能力

```text
[ ] Desktop/Wails
[ ] Local API / Daemon 稳定版
[ ] Web 多用户平台
[ ] Remote Runner
[ ] 长期 Target/User/Team Memory
[ ] LLM Gateway 作为可选增强稳定化
```
