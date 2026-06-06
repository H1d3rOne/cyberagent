# CyberAgent TODO List

日期：2026-06-06  
状态：动态维护中  
当前分支：`feat/production-agent-runtime`  
当前基线：本分支当前 HEAD（持续滚动，见 `git log -1 --oneline`）

---

## 1. 当前结论

CyberAgent 已完成 MVP safe scan、Local Daemon / Web API、Agent Cockpit、v0.1.3-v0.1.8 Agent-native 主链路，以及 Production Agent Runtime 的小切片收敛。当前重点从“补功能”转向：

1. 保持当前分支小切片提交与验证记录清晰。
2. 完成发布前验证与远程 release 流程。
3. 继续收敛 legacy planner/routing 与 runtime engine 职责边界。
4. 补齐 P1/P2 体验、测试与生产可用性。

---

## 2. P0：当前工作树与发布准备

- [x] Review 当前 tracked 文件 diff，确认 Production Agent Runtime / registry dispatch 改动边界清晰；同时修正 `internal/interfaces/daemon/scheduler_test.go` gofmt 漂移。
- [x] 处理根目录未跟踪的 raw conversation export `.txt`：已归档到 `.tmp/archive/raw-conversations/`，并通过 `/.tmp/` 保持本地忽略。
- [x] 确认 `docs/CyberAgent-Optimization-Plan.md` 作为正式优化方案纳入版本控制。
- [x] 运行最小回归：
  - [x] `go test ./...`
  - [x] `go vet ./...`
  - [x] `npm test --prefix web/frontend -- --run`
  - [x] `npm run build --prefix web/frontend`
  - [x] `go build ... ./cmd/cyberagent`
- [x] 做一次 daemon / Agent Cockpit smoke 验证：已用 `.tmp/smoke` 启动 daemon，验证 healthz/version/capabilities、session create、`daemon health` registry dispatch 与 runtime SQLite DB 创建。
- [x] 准备并提交当前分支改动：本轮 registry dispatch/runtime engine 文档与测试改动已完成验证，将随本次提交固化。

---

## 3. P0.5：Production Agent Runtime / legacy 收敛

- [x] daemon 默认启用 SQLite runtime engine。
- [x] 保留 `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0` 回退开关。
- [x] 删除 planner action legacy per-tool switch fallback。
- [x] 将 read-only/system route 收敛为 heuristic action → Tool Registry dispatch。
- [x] 将 safe HTTP GET/HEAD/OPTIONS 收敛为 Tool Registry dispatch。
- [x] 将 non-safe HTTP POST/PUT/PATCH/DELETE 收敛为 Tool Registry dispatch；继续保留 `advanced` gate。
- [x] 将 `ports.scan.safe` 收敛为 Tool Registry dispatch。
- [x] 将 `scan.web.safe` 收敛为 Tool Registry dispatch。
- [x] 将 workspace staged diff/rebase route 收敛为 Tool Registry dispatch。
- [x] 将 `reports.draft` route 收敛为 Tool Registry dispatch。
- [x] 将 workspace write/patch/apply/discard/delete/move/copy/mkdir route 收敛为 Tool Registry dispatch；保留 `workspace_write` gate、explicit intent gate 与既有 conflict/atomicity 测试语义。
- [x] 独立迁移 approval approve/deny/resume-approved route 为 Tool Registry dispatch；保留 `approval_ops` gate、最新用户显式指令匹配、comment/audit/event parity。
- [x] 定义 engine 侧 `TurnExecutor` / `TurnContext` seam，先复用现有 daemon legacy executor，不改变 `SendMessage` 行为。
- [x] 将 `beginRuntimeTurn` / `finishRuntimeTurn` 状态迁移到 engine API，daemon 暂时保留 session adapter。
- [x] 新增 `runtime.Engine.RunLLMTurn` / `LLMTurnRunner` seam，并将 `SendMessage` 两处 LLM 主入口改为经 engine 调度现有 daemon `runAgentLLMToolLoop`；不改变 planner/tool/synthesis/session 行为。
- [x] 新增 `runtime.Engine.RunLLMStage` / `LLMStageRunner` seam，并将 `runAgentLLMToolLoop` 内部 planner chat 改为经 engine stage dispatch 到现有 daemon `handleAgentLLMChat`。
- [x] 将 `runAgentLLMToolLoop` 内部 tool synthesis 改为经 `LLMStageToolSynthesis` dispatch 到现有 daemon `synthesizeAgentToolResults`。
- [x] 将 `runAgentReplanActionContinuation` 的 auto-continue StepReplan synthesis 也改为经 `LLMStageToolSynthesis` dispatch。
- [x] 将 authorization approve resume、route-before-LLM 与 standalone route synthesis 分支改为经 `LLMStageToolSynthesis` dispatch。
- [x] 定义 `runtime.Engine.MutateSession` / `SessionMutator` seam，并将 `appendMessagesWithPlanner` 的 append messages + optional planner apply 改为经 engine dispatch 到现有 daemon session mutation。
- [x] 将 `SendMessage` 开头的 user message 记录、active task start/touch、history/memory snapshot 与 session persist 改为经 `SessionMutationRecordUserMessage` dispatch。
- [x] 将 `DecideAgentAuthorization` 的 approve/deny 授权状态写入、PendingAuthorization 清理、session/task/todo/next_action 更新与 persist/compaction/runtime events 改为经 `SessionMutationResolveAuthorization` dispatch。
- [x] 将 `ControlSession` 的 continue/revise/pause/cancel/stop/new_task session 状态写入与 persist/compaction/runtime events 改为经 `SessionMutationControlSession` dispatch；daemon job cancel 副作用仍保留在外层 control flow。
- [x] 将 `SendMessage` 的 no-LLM deterministic route 与 control auto-continue route 执行改为经 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` turn payload dispatch；实际 `routePrompt`、message append 与 response merge 语义保持不变。
- [x] 将 LLM provider failure fallback 的本地 route 也改为复用 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` adapter；AI failure warning、fallback message prepend 与 tool call merge 语义保持不变。
- [x] 将 LLM configured + route-before-LLM synthesis 分支的本地 route 也改为复用 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` adapter；step phase tagging、后续 synthesis 入口与 response merge 语义保持不变。
- [x] 将 authorization approve resume 与 route-before-LLM 后续 auto-continue 的 `runAgentReplanActionContinuation` 改为经 `runtime.Engine.ExecuteTurn` 的 `replan_action_continuation` turn payload dispatch；实际 action execution、StepReplan synthesis 与 append 语义保持原路径。
- [x] 将 authorization approve 后的单个 planner action execution 改为经 `runtime.Engine.ExecuteTurn` 的 `planner_action_execution` turn payload dispatch；实际 tool/action execution、错误 fallback message、phase tagging、authorization metadata 与后续 synthesis 语义保持不变。
- [x] 将 LLM planner loop 与 replan continuation 内部的单个 planner action execution 改为复用 `planner_action_execution` turn payload dispatch；保留 authorization gate、错误 fallback、action queue metadata、phase tagging、append 与 StepReplan synthesis 语义。
- [x] 为 planner loop / replan continuation 的 action/warning append 增加 turn-aware `SessionMutationAppendMessages` dispatch wrapper，使 append mutation 可携带 task/run/turn/prompt/origin/operation_level metadata；实际 append/persist/compaction/runtime events 仍复用 legacy helper。
- [x] 将 LLM planner chat 与 tool synthesis 的 planner-aware append/apply planner 也改为复用 turn-aware `SessionMutationAppendMessages` wrapper；planner apply mutation 现在可携带 planner_chat/tool_synthesis origin 与完整 turnRef。
- [x] 将 `SendMessage` 中已有 turnRef 的 deterministic route、route-before-LLM synthesis、LLM fallback route、control auto-continue 与 authorization-gate append 改为 turn-aware `SessionMutationAppendMessages` dispatch；`agent_session_flow.go` 剩余直接 append 仅保留在无 turnRef 的 `ControlSession` / `DecideAgentAuthorization` 外部入口。
- [x] 为 `ControlSession` / `DecideAgentAuthorization` 外部入口 append 生成轻量 runtime turnRef，使 daemon job cancel、approved action 与 authorization synthesis append 也经 turn-aware `SessionMutationAppendMessages` dispatch；不改变 Begin/Finish lifecycle 与原 control/authorization 状态语义。
- [x] 将 `DecideAgentAuthorization` approved flow 的外部 turnRef 贯穿 approved planner action execution、authorization tool synthesis 与 follow-up continuation runtime runner；runner dispatch 与 append mutation 现在共享同一 task/run/turn metadata，仍不发布额外 Begin/Finish lifecycle。
- [x] 补齐 session mutation compaction metadata：append messages、control session 与 resolve authorization 的 `memory_compacted` event 现在使用当前 mutation turnRef；legacy fallback 会按 session/message/auth seed 补齐 external turnRef，避免空 task/run/turn metadata，且不发布额外 Begin/Finish lifecycle。
- [x] 将 deterministic route 内部 capability-gap plan-only append/apply planner 改为 turn-aware `SessionMutationAppendMessages` dispatch；`routePromptForTurn` 会沿用外层 task/run/turn/prompt/origin/operation_level metadata，直接 `routePrompt` 调用仍保留 legacy wrapper 兼容。
- [x] 将 deterministic route 的 schedule create/list/delete 统一收敛为 `agentHeuristicActionsFromPrompt` → Tool Registry dispatch；create/delete 继续保留 `advanced` gate，list 保留 `read_only` gate。
- [x] 将 generic workspace fallback 收敛为 `agentHeuristicActionsFromPrompt` → `workspace.files.list` Tool Registry dispatch；保留 `read_only` gate，并用根目录 list fallback 替代 legacy `workspace.prepare` direct handler。
- [x] 将 no-LLM control continue 的 active-run 上下文读取改为经 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` dispatch；保留 contextKind 选择、evidence conclusion、`active_run_continue` append 与 response merge 语义。
- [x] 将 planner/manual authorization request 的 `PendingAuthorization` 创建、session status/next_action 更新与 persist 改为经 `SessionMutationRequestAuthorization` dispatch；daemon legacy mutator 继续复用原授权文案与 append 顺序。
- [x] 将 capability mismatch blocked state（`markAgentCapabilityBlocked`）改为经 `SessionMutationCapabilityBlocked` dispatch；blocked session/task/todo/next_action 语义与 mismatch step_replan message 返回顺序保持不变。
- [x] 抽出 planner action/replan cycle 共享 helper，让 `llm_tool_loop` 与 `replan_action_continuation` 共用 Action → append → tool synthesis → append 骨架；append origin、authorization/capability gate、response/tool parity 保持不变。
- [x] 将 planner action/replan cycle 共享 helper 的散列参数收敛为 daemon-private `agentPlannerActionReplanCycleRequest` / `State` / `Result` DTO，为后续替换成 runtime-neutral loop DTO 并下沉 engine-native runner 做准备；行为保持纯重构。
- [x] 将 planner action/replan cycle request 的 session/task/run/turn/prompt/origin/operation_level 边界收敛到 `runtime.TurnContext`，减少 daemon-only turnRef 在 shared helper 参数面中的扩散；现有 append、action executor 与 synthesis runner 仍复用 legacy adapter。
- [x] 将 planner action/replan cycle request 的 tool/service 输入封装为 cycle-private `agentPlannerActionReplanCycleToolContext`，shared cycle 不再直接暴露 `routerServices` 字段；legacy action executor 调用点临时解包以保持行为 parity。
- [x] 将 planner action/replan cycle request 内部的 auth profile 输入收敛为 cycle-private `agentPlannerActionReplanCycleAuthProfile` DTO；legacy action/synthesis adapter 边界再转换回 `agentHTTPAuthProfile`，并保持 secret/enabled 深拷贝隔离。
- [x] 将 planner action/replan cycle request/result 的 plan 输入输出封装为 cycle-private `agentPlannerActionReplanCyclePlan` handle；legacy planner response 只在 action executor、synthesis 与外层 loop 边界临时解包。
- [ ] 继续把剩余 session mutation 分批迁成 engine-native runner。
- [x] 最后将 `SendMessage` Begin/Finish lifecycle 收敛为 `runtime.Engine.RunTurnLifecycle` bounded runner；response merge branch body 仍保留 daemon helper 作为迁移 adapter。

---

## 4. P1：核心体验增强

- [x] Frontend route-level lazy / bundle splitting，降低初始 bundle。
- [x] 移动端 Agent Cockpit 降噪，避免横向滚动和信息堆叠。
- [x] 真实浏览器 E2E smoke。
- [x] a11y 回归：icon-only button、dialog/drawer 语义、焦点管理。
- [x] 视觉回归：毛玻璃背景、半透明卡片、approval pending、破坏性操作状态。

---

## 5. P2：生产可用性增强

- [ ] Run List benchmark。
- [ ] 可重建 run index。
- [ ] Tool Registry / MCP / Scheduler observability view。
- [ ] Timeline 大量事件渲染优化。
- [ ] Trace viewer JSON 分段、搜索、复制、下载增强。

---

## 6. 后置大能力

- [ ] Desktop / Wails。
- [ ] Local API / Daemon 稳定版。
- [ ] Web 多用户平台。
- [ ] Remote Runner。
- [ ] 长期 Target / User / Team Memory。
- [ ] LLM Gateway 作为可选增强稳定化。

---

## 7. 未完成项执行清单（滚动）

推荐顺序：先完成 P0.5 engine-native 收敛与语义保护测试，再补 P1 前端/QA smoke，最后推进 P2 生产可观测与大规模性能。

### 7.1 P0.5 Engine-native / legacy 收敛

- [x] P0.5-N1：盘点 remaining `SendMessage` branches 与 planner loop mutation 点，记录哪些已走 `ExecuteTurn` / `RunLLMTurn` / `RunLLMStage` / `MutateSession`，哪些仍是 daemon-owned legacy adapter。
- [x] P0.5-N2：将 planner authorization request（`PendingAuthorization` 创建、session status/next_action 更新、persist）拆成 `SessionMutationRequestAuthorization`，先经 engine `MutateSession` dispatch 到 daemon legacy mutator，不改变授权 gate 文案、PendingAuthorization、append 顺序。
- [x] P0.5-N3：将 capability mismatch blocked state（`markAgentCapabilityBlocked`）拆成独立 session mutation，保留 blocked todo/task/next_action 语义与 plan-only append 顺序。
- [x] P0.5-N4：为 LLM fallback manual authorization branch 补 turn/external mutation 覆盖测试，确认 warning + authorization message append 与 pending auth mutation metadata 对齐。
- [x] P0.5-N5：为 `runAgentLLMToolLoop` / `runAgentReplanActionContinuation` 输出一个 engine-native runner 设计草案：输入/输出 payload、authorization gate、action queue、synthesis、append/persist ownership 边界。
- [x] P0.5-N6：把 planner loop 的 action queue metadata 生成与 authorization/capability gate decision 封装为可测试的小 helper；保持 gate 顺序、authorization policy/reason、capability blocked、queue metadata 与 append/response 语义不变。
- [x] P0.5-N7：收敛 remaining `SendMessage` response merge 分支的 regression matrix，覆盖 control continue/revise、pending auth gate、route-before-LLM、LLM fallback、no-LLM route。
- [x] P0.5-N8：最后再考虑把 `SendMessage` 主编排拆成 engine turn lifecycle runner；该项必须等待 N1-N7 的 focused tests 通过。
- [x] P0.5-N9：抽出 planner action/replan cycle helper，让 `llm_tool_loop` 与 `replan_action_continuation` 共享 Action → append → tool synthesis → append 循环骨架；保持 daemon adapter、append origin、authorization/capability gate、response/tool parity 不变。
- [x] P0.5-N10：把 `runAgentPlannerActionReplanCycle` 的输入/输出先收敛为 daemon-private request/state/result DTO，减少 helper 参数面并明确 future engine-native loop runner 的 state 边界；不改变 action/synthesis/append/merge 行为。
- [x] P0.5-N11：将 shared cycle request 的 turn identity/prompt/origin/operation level 改为 `runtime.TurnContext`，helper 内部只在调用 legacy adapters 时临时转换为 `agentRuntimeTurnRef`；为后续 engine-native runner 直接消费 `TurnContext` 做前置。
- [x] P0.5-N12：拆分 shared cycle request 的 authorization 输入与 LLM synthesis request，`executeAgentPlannerActions` 不再直接依赖完整 `sendAgentMessageRequest`；保留 synthesis runner 请求兼容与行为 parity。
- [x] P0.5-N13：把 shared cycle request 的 LLM synthesis 输入缩小为 cycle-private LLM DTO，仅保留 origin/provider/model/API key/base URL/instructions/operation level/auth hints；operator prompt/language 等 turn 字段不再随 LLM request 进入 cycle。
- [x] P0.5-N14：把 shared cycle request 的 tool/service 输入封装为 cycle-private `agentPlannerActionReplanCycleToolContext`，cycle DTO 不再直接暴露 `routerServices`；legacy action executor 仍通过 adapter 解包同一 services。
- [x] P0.5-N15：把 shared cycle request 内部 auth profile 输入收敛为 cycle-private `agentPlannerActionReplanCycleAuthProfile` DTO；authorization 与 LLM request 都只在 legacy adapter 边界转换回 daemon `agentHTTPAuthProfile`。
- [x] P0.5-N16：把 shared cycle request/result 的 plan 输入输出封装为 cycle-private `agentPlannerActionReplanCyclePlan` handle；cycle DTO 不再直接暴露 `*agentPlannerResponse` 字段，legacy 边界临时解包保持 parity。

当前盘点（2026-06-06）：

- 已经走 engine seam：`record_user_message`、`append_messages`、`control_session`、`resolve_authorization`、`request_authorization`、`capability_blocked`；deterministic route / replan continuation / planner action execution；LLM turn runner；planner chat 与 tool synthesis stage。
- 已输出 engine-native runner 草案：建议将 `llm_tool_loop` 与 `replan_action_continuation` 统一为单个 loop runner 的两个 mode，明确请求/响应 payload、authorization gate、action queue metadata、synthesis、append/persist ownership 与迁移顺序。
- 已抽出 planner loop helper：`decideAgentPlannerActionGate` 负责无副作用计算 capability mismatch / explicit intent / operation level / manual authorization / allow 决策；`agentPlannerActionQueueMetadata` 负责统一 action queue metadata 字段与类型。
- 已抽出 planner action/replan cycle helper：`runAgentPlannerActionReplanCycle` 统一 `runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 的 Action → action append → tool synthesis → synthesis append 骨架；两种入口仍保留各自前置条件、append origin 与 response/tool merge 语义。
- 已把 shared cycle helper 的输入/输出整理为 daemon-private DTO：`agentPlannerActionReplanCycleRequest` 承载 turn/cycle-private LLM request/authorization/tool context/plan handle/options，`agentPlannerActionReplanCycleState` 承载 response/tool/history state，`agentPlannerActionReplanCycleResult` 承载新 state/final plan handle/continue decision；为后续替换 runtime-neutral DTO 做前置。
- shared cycle request 已使用 runtime-native turn boundary：`agentPlannerActionReplanCycleRequest.Turn` 现在是 `runtime.TurnContext`，session/task/run/turn/prompt/origin/operation_level 不再作为散列 daemon 参数传入；仅在落回 append/action/synthesis legacy adapter 时转换为 `agentRuntimeTurnRef`。
- shared cycle request 已拆分 authorization 与 LLM synthesis request：action execution 只读取 `agentPlannerActionReplanCycleAuthorization`，LLM synthesis 兼容路径由 cycle-private DTO 临时转换到 legacy request。
- shared cycle request 的 LLM synthesis 输入已缩小为 `agentPlannerActionReplanCycleLLMRequest`：只保留 provider/model/API key/base URL/instructions/origin/operation_level/auth hints，并在调用 legacy synthesis adapter 时临时转换回 `sendAgentMessageRequest`。
- shared cycle request 的 auth profile 输入已收敛为 `agentPlannerActionReplanCycleAuthProfile`：authorization 与 LLM request 都不再直接持有 `agentHTTPAuthProfile`，legacy action/synthesis adapter 边界再转换回 daemon HTTP auth profile，并补 secret/enabled 深拷贝隔离测试。
- shared cycle request/result 的 plan 输入输出已封装为 `agentPlannerActionReplanCyclePlan` handle：cycle DTO 不再直接暴露 `*agentPlannerResponse` 字段，legacy action executor、synthesis 与外层 loop 边界临时解包，后续可替换为 runtime-neutral plan/action DTO。
- shared cycle request 的 tool/service 输入已封装为 `agentPlannerActionReplanCycleToolContext`：cycle DTO 不再直接暴露 `routerServices` 字段，action execution legacy adapter 调用点临时解包同一 services，后续可替换成 runtime-neutral tool binding。
- `SendMessage` response merge regression matrix 已覆盖 control continue/revise、pending authorization gate、route-before-LLM synthesis，并复跑 no-LLM route、LLM fallback route/authorization、active-run continue parity focused tests。
- `SendMessage` Begin/Finish 已由 `runtime.Engine.RunTurnLifecycle` 包住 bounded turn body；turn_started → branch append/model/tool events → turn_finished/task terminal event 顺序已补回归。
- 仍是 daemon-owned legacy adapter：`runAgentLLMToolLoop` 的 planner JSON/native tool call 解析、authorization/capability gate 副作用、StepReplan 外层 loop/mode precondition 与 `SendMessage` response merge branch body；后续目标是把 cycle DTO 中剩余 daemon 类型（message/tool 与 LLM/tool adapter compatibility，plan handle 内部仍临时包 legacy response）继续替换成 runtime-neutral DTO，再移动到 engine-native runner。

### 7.2 P1 Frontend / QA

- [x] P1-F1：梳理 `web/frontend` 路由与初始 bundle，确定 Cockpit、scan/dashboard、trace 等 route-level lazy 边界。
- [x] P1-F2：实现 route-level lazy / bundle splitting，并记录 build 后主要 chunk 变化。
- [x] P1-F3：移动端 Agent Cockpit 降噪：优先处理 Timeline/Tool card 横向滚动、密集 metadata badge、approval pending 卡片堆叠。
- [x] P1-F4：补真实浏览器 E2E smoke：daemon health、session create、send deterministic route、approval pending/decision 最小链路。
- [x] P1-F5：补 a11y 回归：icon-only button label、dialog/drawer role、焦点返回、键盘可达。
- [x] P1-F6：补视觉回归清单：毛玻璃背景、半透明卡片、approval pending、破坏性操作状态。

当前盘点（2026-06-06，P1-F1）：

- 当前 `web/frontend/src/App.tsx` 静态导入所有页面，生产 build 仍是单入口页面 bundle：`assets/index-ZDGR-ykT.js 442.68 kB / gzip 127.29 kB`，CSS `50.80 kB / gzip 9.98 kB`。
- 已确定 route-level lazy 边界：
  - `/agent`：`AgentCockpitPage` + `agent/components/*` + `agent/state/*`，这是 cockpit 专属交互区，应单独 chunk。
  - `/scan/:id`：`ScanDetailPage`，trace/audit/evidence/approval 细节最多，应单独 chunk。
  - `/settings`：provider/auth/web search/operation level 配置表单，应单独 chunk。
  - `/tasks` 与 `/scan/new`：共享 `NewScanPage` + `SchedulesPanel`，应复用同一个 chunk。
  - `/skills` 与 `/mcp`：registry/observability 页面，可先分别 lazy；若 chunk 过碎，后续可合并为 registry chunk。
  - `/history`：run list/history 单独 lazy。
  - `/` 与 `/ops`：继续指向 `DashboardPage`；P1-F2 可让 dashboard route lazy 后由当前 route 立即加载，不再拖入 cockpit/settings/detail。
- P1-F2 实施建议：在 `App.tsx` 使用 `React.lazy` + `Suspense` 包住 route element，保留 `Layout` 为 eager shell，增加轻量 route loading fallback；build 后记录主入口 JS、route chunks 与 gzip 变化。
- P1-F2 已实现：`App.tsx` 改为 `React.lazy` + `Suspense` route element；`Layout`/shell 保持 eager，route fallback 为可访问 `role=status` glass loading skeleton。
- P1-F2 build 结果：初始 JS 从 `442.68 kB / gzip 127.29 kB` 降到 `233.51 kB / gzip 75.24 kB`（raw 约 -47.3%，gzip 约 -40.9%）；主要 route chunk 为 `/agent` `80.31 kB / gzip 23.18 kB`、`/scan/:id` `43.15 kB / gzip 11.22 kB`、`/settings` `32.35 kB / gzip 9.20 kB`、`/tasks` 与 `/scan/new` `23.29 kB / gzip 7.49 kB`、dashboard `6.41 kB / gzip 2.81 kB`。
- P1-F3 已实现：移动端 `/agent` 顶层由堆叠式 TaskTree + 主工作区改为轻量 MobileTaskSwitcher；桌面 TaskTree 仅在 `xl` 以上显示；小屏 header 隐藏 seq/task/displayHint 等低优先 metadata，主要按钮提升到 44px 触摸目标；Timeline tool cards 改到 `md` 以上双列，message bubble 放宽到 94%；选择工具后移动端使用底部 Tool Inspector sheet；Approval banner 动作按钮可换行并在小屏铺满；Composer 增加 safe-area padding 与移动端大按钮；移动端 agent scroll gutter 收敛，避免横向滚动。
- P1-F4 已实现：新增 `scripts/agent-cockpit-browser-smoke.mjs`，用系统 Chrome + Playwright 启动真实浏览器，随机端口启动 daemon 与 Vite dev server，并注入 `cyberagent_console_settings` 指向本轮 daemon token；脚本验证 `/agent` 可创建 session、发送 daemon health prompt、触发 `workspace.files.write` 授权 pending、点击“允许并继续”，最终通过 daemon workspace read API 校验 `e2e-smoke.txt` 内容为 `browser-smoke`。前端 `devDependencies` 已加入 `playwright`，不引入 `@playwright/test`，不下载浏览器。
- P1-F5 已实现：新增 `web/frontend/src/agent/components/dialog-a11y.ts` focus helper，Trace drawer 与移动端 Tool Inspector sheet 现在具备 `aria-labelledby`/`aria-describedby`、可编程聚焦、Escape 关闭、焦点返回；Composer textarea 增加真实 label 与键盘提示；TaskTree、mobile session list、timeline 工具/建议按钮补齐 focus-visible ring 与当前项语义；新增 `scripts/agent-cockpit-a11y-smoke.mjs`，真实浏览器校验 visible button accessible name、form label、dialog name、focus trap、Escape close 与 focus return。
- P1-F6 已实现：新增 `scripts/agent-cockpit-visual-smoke.mjs`，仍用 standalone Playwright + 系统 Chrome，不引入 `@playwright/test`；脚本随机端口启动 daemon 与 Vite，生成 `.tmp/e2e/.../screenshots/` 下 6 张可审查截图，并用 DOM computed style 断言 layered gradient backdrop、glass blur/fallback、半透明卡片、Trace drawer elevated overlay、approval pending warning、destructive deny hover 与移动端 approval layout 视觉契约。

### 7.3 P2 Production / Observability

- [ ] P2-R1：Run List benchmark：构造大量 runs/events 数据，记录 API 与前端渲染基线。
- [ ] P2-R2：实现可重建 run index 的恢复/校验流程，明确 SQLite/runtime store 与 runs dir 的 source of truth。
- [ ] P2-R3：Tool Registry / MCP / Scheduler observability view：展示 registry source、operation level gate、scheduler job 状态。
- [ ] P2-R4：Timeline 大量事件渲染优化：虚拟列表/分组/懒加载候选方案与回归测试。
- [ ] P2-R5：Trace viewer JSON 增强：分段渲染、搜索、复制、下载、错误定位。

### 7.4 后置大能力拆分

- [ ] L1：Desktop / Wails 技术 spike：daemon 嵌入、日志/更新、权限模型。
- [ ] L2：Local API / Daemon 稳定版：配置、token、runtime DB migration、兼容策略。
- [ ] L3：Web 多用户平台：租户/session isolation、认证、审计、配额。
- [ ] L4：Remote Runner：远端 workspace、队列、日志流、artifact 回收。
- [ ] L5：长期 Target / User / Team Memory：作用域、过期、可解释检索与删除。
- [ ] L6：LLM Gateway 稳定化：provider fallback、model capability registry、prompt/version trace。

---

## 8. 最新本地验证记录

2026-06-06 当前工作树验证通过：

本轮增量（P0.5-N1/N2/N3/N4/N5/N6/N7/N8：未完成项滚动清单 + authorization request / capability blocked session mutation seam + LLM fallback authorization branch 覆盖 + engine-native loop runner 草案 + planner action gate/helper parity + SendMessage response merge regression matrix + SendMessage turn lifecycle runner）：

```bash
go test -count=1 ./internal/agent/runtime -run 'TestRunTurnLifecycle|TestBeginTurn|TestFinishTurn'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageRunsInsideRuntimeTurnLifecycle|TestAgentManagerSendMessageResponseMerge|TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackAuthorizationUsesSessionMutations|TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor|TestAgentManagerSendMessageActiveRunContinueUsesTurnExecutor|TestAgentManagerEmitsMemoryCompactedRuntimeEvent|TestAgentManagerFinishRuntimeTurnPublishesTerminalEvents'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageResponseMerge|TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackAuthorizationUsesSessionMutations|TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor|TestAgentManagerSendMessageActiveRunContinueUsesTurnExecutor'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionGateDecision|TestAgentPlannerActionQueueMetadata|TestAgentManagerExecuteAgentPlannerActionsDispatchesPlannerActionsThroughTurnExecutor|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator'
git diff --check -- docs/CyberAgent-TODO.md docs/CyberAgent-Optimization-Plan.md
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageLLMFallbackAuthorizationUsesSessionMutations|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestCapabilityClassifierPreventsCommonNonEquivalentTools|TestAgentManagerExecuteAgentPlannerActionsDispatchesPlannerActionsThroughTurnExecutor'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestHTTPAgentPlannerApprovalActionRequiresExplicitPrompt|TestHTTPAgentChatNativeApprovalDecisions|TestHTTPAgentChatNativeResumeApproved|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners'
go test -count=1 ./internal/agent/runtime -run 'TestMutateSession'
go test -count=1 ./internal/interfaces/daemon
test -z "$(gofmt -l cmd internal)" && git diff --check && go test -count=1 ./... && go vet ./...
```

本轮增量（P0.5-N9：planner action/replan cycle 共享 helper）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions|TestAgentManagerSendMessageResponseMerge'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：`llm_tool_loop` 与 `replan_action_continuation` 共享 `runAgentPlannerActionReplanCycle` 后，planner chat/action/synthesis append origin、planner apply metadata、action queue metadata、tool/response merge parity focused tests、runtime seam focused tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N10：planner action/replan cycle DTO 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions|TestAgentManagerSendMessageResponseMerge'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle helper 改为 request/state/result DTO 后，LLM loop、replan continuation、planner action execution、SendMessage response merge focused tests、runtime seam focused tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N11：planner action/replan cycle TurnContext 边界）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions|TestAgentManagerSendMessageResponseMerge'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle request 使用 `runtime.TurnContext` 后，LLM loop、replan continuation、planner action execution、SendMessage response merge focused tests、runtime seam focused tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N12：planner action/replan cycle auth/request 边界拆分）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions|TestAgentManagerSendMessageResponseMerge'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle request 将 authorization profiles/default profile 从 LLM synthesis request 中拆出后，planner action execution 通过独立 authorization DTO 传递 auth profile，LLM synthesis 继续保留请求兼容；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N13：planner action/replan cycle LLM request DTO 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions|TestAgentManagerSendMessageResponseMerge|TestAgentPlannerActionReplanCycleLLMRequest'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle request 的 LLM synthesis 输入缩小为 cycle-private DTO 后，operator prompt/operator/language 不再随 LLM request 进入 cycle，auth profiles 复制隔离且 legacy synthesis 转换保持 provider/model/auth hint parity；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N14：planner action/replan cycle tool context 封装）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions|TestAgentManagerSendMessageResponseMerge|TestAgentPlannerActionReplanCycleLLMRequest'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle request 的 tool/service 输入封装为 cycle-private `agentPlannerActionReplanCycleToolContext` 后，cycle DTO 不再直接暴露 `routerServices` 字段；focused test 验证 legacy planner action execution 仍收到同一 services binding，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N15：planner action/replan cycle auth profile DTO 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions|TestAgentManagerSendMessageResponseMerge|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle request 的 authorization 与 LLM request 都改为持有 cycle-private `agentPlannerActionReplanCycleAuthProfile`，legacy action/synthesis adapter 边界再转换回 `agentHTTPAuthProfile`；新增测试覆盖 secret/enabled 深拷贝隔离，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N16：planner action/replan cycle plan handle 封装）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions|TestAgentManagerSendMessageResponseMerge|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle request/result 的 plan 输入输出封装为 cycle-private `agentPlannerActionReplanCyclePlan` handle 后，cycle DTO 不再直接暴露 `*agentPlannerResponse` 字段；legacy action executor、synthesis 与外层 loop 边界临时解包，新增 plan handle 边界测试，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

P1-F1 前端路由/bundle 盘点：

```bash
cd web/frontend && npm run build
cd web/frontend && npm test
```

结果：当前静态路由 build 通过，单入口 JS `442.68 kB / gzip 127.29 kB`，CSS `50.80 kB / gzip 9.98 kB`；Vitest `6 files / 25 tests` 通过。

P1-F2 route-level lazy / bundle splitting：

```bash
cd web/frontend && npm test && npm run build
```

结果：Vitest `6 files / 25 tests` 通过；build 后主入口 JS `233.51 kB / gzip 75.24 kB`，主要 route chunks：`agent-cockpit 80.31 kB / gzip 23.18 kB`、`scan-detail 43.15 kB / gzip 11.22 kB`、`settings 32.35 kB / gzip 9.20 kB`、`new-scan 23.29 kB / gzip 7.49 kB`、`dashboard 6.41 kB / gzip 2.81 kB`。

P1-F3 移动端 Agent Cockpit 降噪：

```bash
cd web/frontend && npm test && npm run build
git diff --check
```

结果：Vitest `6 files / 25 tests` 通过；production build 通过；移动端 Cockpit 降噪后 `/agent` route chunk 为 `85.80 kB / gzip 24.52 kB`，主入口 JS 保持 `233.51 kB / gzip 75.23 kB`。

P1-F4 真实浏览器 Agent Cockpit smoke：

```bash
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
node --check scripts/agent-cockpit-browser-smoke.mjs
node scripts/agent-cockpit-browser-smoke.mjs
git diff --check
```

结果：Vitest `6 files / 25 tests` 通过；production build 通过；browser smoke 返回 `ok: true`，已验证随机端口 daemon + Vite + Chrome 下 `/agent` session、daemon health prompt、manual authorization approval、workspace file write/read 最小链路；diff whitespace check 通过。

P1-F5 Agent Cockpit a11y 回归：

```bash
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
node --check scripts/agent-cockpit-browser-smoke.mjs
node --check scripts/agent-cockpit-a11y-smoke.mjs
node scripts/agent-cockpit-browser-smoke.mjs
node scripts/agent-cockpit-a11y-smoke.mjs
git diff --check
```

结果：Vitest `6 files / 25 tests` 通过；production build 通过；browser smoke 返回 `ok: true`；a11y smoke 返回 `ok: true`，覆盖 button names、form labels、dialog names、focus trap、Escape close、focus return；diff whitespace check 通过。

P1-F6 Agent Cockpit 视觉回归 smoke：

```bash
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
node --check scripts/agent-cockpit-visual-smoke.mjs
node scripts/agent-cockpit-visual-smoke.mjs
git diff --check
```

结果：Vitest `6 files / 25 tests` 通过；production build 通过；visual smoke 返回 `ok: true`，生成 6 张截图（initial desktop、session desktop、Trace drawer、approval pending desktop、destructive hover desktop、approval pending mobile），并覆盖 layered gradient backdrop、glass blur/fallback、semi-transparent cards、Trace drawer elevated overlay、approval pending warning、destructive hover 与移动端 approval layout；diff whitespace check 通过。

```bash
go test ./...
go test -count=1 ./...
go vet ./...
test -z "$(gofmt -l cmd internal)"
git diff --check
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
go build -ldflags "-X main.version=local -X main.commit=$(git rev-parse --short HEAD)" -o bin/cyberagent ./cmd/cyberagent
./bin/cyberagent version
./bin/cyberagent --version
./bin/cyberagent daemon --listen 127.0.0.1:18787 --runs-dir .tmp/smoke/runs --token-file .tmp/smoke/token
go test ./internal/interfaces/daemon
go test ./internal/interfaces/daemon -run 'TestRoutePromptWorkspace(Mutation|Diff|Rebase)|TestRoutePromptReportDraft'
go test ./internal/interfaces/daemon -run 'TestRoutePromptApproval|TestRoutePromptResume|TestApprovalOperationRouteAction|TestHTTPAgentChatNativeApprovalDecisions|TestHTTPAgentChatNativeResumeApproved'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestExecuteTurn|TestAgentManagerAttachEngineWiresDaemonTurnExecutor|TestAgentManagerFinishRuntimeTurnPublishesTerminalEvents|TestHTTPAgentChatNativeApprovalDecisions|TestHTTPAgentChatNativeResumeApproved'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestBeginTurn|TestFinishTurn|TestAgentManagerFinishRuntimeTurnPublishesTerminalEvents|TestHTTPAgentChatNativeApprovalDecisions|TestHTTPAgentChatNativeResumeApproved'
go test -count=1 ./internal/agent/runtime -run 'TestRunLLMTurn|TestExecuteTurn|TestBeginTurn|TestFinishTurn'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerAttachEngine|TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerStreamsLLMDeltasToRuntime|TestHTTPAgentMessageUsesConfiguredLLMProvider|TestHTTPAgentPlannerExecutesStructuredToolActions'
go test -count=1 ./internal/agent/runtime -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestBeginTurn|TestFinishTurn'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerAttachEngine|TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunRuntimeAgentLLMPlannerChat|TestAgentManagerStreamsLLMDeltasToRuntime|TestHTTPAgentMessageUsesConfiguredLLMProvider|TestHTTPAgentPlannerExecutesStructuredToolActions'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerAttachEngine|TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunRuntimeAgentLLMPlannerChat|TestAgentManagerRunRuntimeAgentLLMToolSynthesis|TestAgentManagerStreamsLLMDeltasToRuntime|TestHTTPAgentMessageUsesConfiguredLLMProvider|TestHTTPAgentPlannerExecutesStructuredToolActions'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunRuntimeAgentLLMToolSynthesis'
go test -count=1 ./internal/interfaces/daemon -run 'TestHTTPAgentPlannerApprovalActionRequiresExplicitPrompt|TestHTTPAgentRoutedToolsAreSynthesizedWhenProviderConfigured'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeControlSession|TestAgentManagerRunRuntimeResolveAuthorization|TestAgentManagerRunRuntimeRecordUserMessage|TestAgentManagerAppendMessagesWithPlanner|TestAgentManagerAttachEngine'
go test -count=1 ./internal/agent/runtime -run 'TestExecuteTurn'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRoute|TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor|TestAgentManagerAttachEngineWiresDaemonTurnExecutor'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor|TestHTTPAgentChatNativeApprovalDecisions|TestHTTPAgentChatNativeResumeApproved'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerExecuteAgentPlannerActionsDispatchesPlannerActionsThroughTurnExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestHTTPAgentPlannerExecutesStructuredToolActions'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef|TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestHTTPAgentPlannerExecutesStructuredToolActions'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan|TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerControlSessionJobCancelAppendUsesExternalTurnRef|TestAgentManagerDecideAgentAuthorizationApprovedActionAppendUsesExternalTurnRef'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerDecideAgentAuthorizationApprovedActionAppendUsesExternalTurnRef|TestAgentManagerControlSessionJobCancelAppendUsesExternalTurnRef'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerAppendMessagesForTurnCompactionUsesTurnRef|TestAgentManagerControlSessionCompactionUsesMutationTurnRef|TestAgentManagerResolveAuthorizationCompactionUsesMutationTurnRef|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerDeterministicRoutePlanOnlyAppendUsesTurnRef'
go test -count=1 ./internal/interfaces/daemon -run 'TestRoutePromptSchedule|TestSchedule'
go test -count=1 ./internal/interfaces/daemon -run 'TestRoutePromptAdvancedHTTP|TestRoutePromptSafeHTTP'
go test -count=1 ./internal/interfaces/daemon -run 'TestRoutePromptGenericWorkspace|TestRoutePromptWorkspace'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageActiveRunContinueUsesTurnExecutor|TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor'
go test -count=1 ./internal/interfaces/daemon -run 'TestRoutePrompt'
```

结果摘要：

```text
Go test / vet / build 通过。
gofmt / diff whitespace check 通过。
Frontend vitest 6 files / 25 tests 通过。
Frontend production build 通过。
P1-F1 route lazy 边界盘点完成：`App.tsx` 当前静态导入所有页面，单入口 JS `442.68 kB / gzip 127.29 kB`；已确定 `/agent`、`/scan/:id`、`/settings`、`/tasks` 与 `/scan/new`、`/skills` 与 `/mcp`、`/history`、`/` 与 `/ops` 的 lazy chunk 边界。
P1-F2 route-level lazy / bundle splitting 完成：`App.tsx` 使用 `React.lazy` + `Suspense` 延迟加载页面 route，主入口 JS 降为 `233.51 kB / gzip 75.24 kB`，Cockpit/ScanDetail/Settings/NewScan 等拆为 route chunks。
P1-F3 移动端 Agent Cockpit 降噪完成：新增移动端任务切换器，桌面 TaskTree 隐藏于小屏，header/timeline/tool cards/approval banner/composer 按移动端信息层级与触摸目标收敛，并补移动端 Tool Inspector bottom sheet。
Binary version 输出正常。
Daemon API smoke 通过：healthz/version/capabilities、Agent session create、自然语言 `daemon health` → `deterministic_route/registry_dispatch`、runtime SQLite DB 创建。
Workspace mutation route 小切片验证通过：write/patch/apply/apply-all/discard/discard-all/delete/move/copy/mkdir 均经 `deterministic_route/registry_dispatch`，且 `read_only` 不会越过 `workspace_write` gate。
Approval operation route 小切片验证通过：approve/deny/resume-approved 均经 `deterministic_route/registry_dispatch`，保留 `approval_ops` gate、显式指令匹配、approval comment 与原 approval/resume handler 审计事件语义。
TurnExecutor seam 小切片验证通过：`runtime.Engine.ExecuteTurn` 可稳定 dispatch attached executor；无 executor 时返回明确 blocked/`ErrTurnExecutorNotAttached`；daemon `AttachEngine` 会挂接 `daemon_legacy_adapter`，但 `SendMessage` 主流程、begin/finish runtime event parity 不变。
Turn lifecycle API 小切片验证通过：`runtime.Engine.BeginTurn` / `FinishTurn` 已接管 task/run/turn state 与 `turn_started`、`turn_finished`、`turn_failed`、approval/run terminal events 发布；daemon `beginRuntimeTurn` / `finishRuntimeTurn` 只做 session/response adapter，原 event order、status/reason/error data 与 approval/terminal run 语义保持不变。
LLM turn runner seam 小切片验证通过：`runtime.Engine.RunLLMTurn` 可稳定 dispatch attached runner；无 runner 时返回明确 blocked/`ErrLLMTurnRunnerNotAttached`；daemon `AttachEngine` 会挂接 `daemon_legacy_llm_tool_loop`，`SendMessage` 两处 LLM 主入口已改为经 engine seam 调度，但实际 planner/tool/synthesis/session mutation 仍复用原 `runAgentLLMToolLoop`。
LLM stage runner seam 小切片验证通过：`runtime.Engine.RunLLMStage` 可稳定 dispatch attached stage runner；无 runner 时返回明确 blocked/`ErrLLMStageRunnerNotAttached`；`runAgentLLMToolLoop` 的 planner chat 已改为经 engine stage seam 调度到 `handleAgentLLMChat`，model streaming 与 planner metadata 行为保持不变。
Tool synthesis stage 小切片验证通过：`runAgentLLMToolLoop` 内部 `synthesizeAgentToolResults` 调用已改为经 `LLMStageToolSynthesis` dispatch，实际执行仍复用原 synthesis 逻辑，StepReplan/final_answer 解析、tool_synthesis metadata 与 streaming 语义保持不变。
StepReplan continuation 小切片验证通过：`runAgentReplanActionContinuation` 的 auto-continue synthesis 已复用 `runRuntimeAgentLLMToolSynthesis`，后续 replan action 执行、tool_synthesis payload 与最终 final_answer 停止语义保持不变。
Session-flow synthesis 小切片验证通过：authorization approve resume、route-before-LLM synthesis 与 standalone route synthesis 分支已统一调用 `runRuntimeAgentLLMToolSynthesis`；无 engine/stage runner 时仍 fallback 到原 legacy synthesis。
Session mutation seam 小切片验证通过：`runtime.Engine.MutateSession` 可稳定 dispatch attached mutator；无 mutator 时返回明确 blocked/`ErrSessionMutatorNotAttached`；daemon `AttachEngine` 会挂接 `daemon_legacy_session`，`appendMessagesWithPlanner` 已经 engine dispatch，但实际 append/apply planner/persist/compaction/runtime events 仍复用原 legacy mutation。
User-message mutation 小切片验证通过：`SendMessage` 开头的 user message 写入已改为 `SessionMutationRecordUserMessage`；实际 operator/language/operation level 更新、ActiveTask start/touch、history/memory snapshot、persist 与 compaction 仍复用 daemon legacy helper，`BeginTurn` 和 memory_compacted event 顺序保持不变。
Authorization mutation 小切片验证通过：`DecideAgentAuthorization` 的 approve/deny session 状态写入已改为 `SessionMutationResolveAuthorization`；pending 校验、operation level uplift、approve/deny message、ActiveTask/Todos/NextAction、persist、memory compaction、approval_resolved 与 message runtime events 仍复用 daemon legacy helper，approve 后 action 执行与 synthesis/auto-continue 继续保持原路径。
Authorization request mutation 小切片验证通过：planner/manual authorization request 的 `PendingAuthorization` 创建、waiting_authorization 状态、next_action 与授权提示 message 已经通过 `SessionMutationRequestAuthorization` 调度；授权提示 append 仍由外层 turn-aware append mutation 保持原顺序。
Capability blocked mutation 小切片验证通过：planner capability mismatch 的 blocked session/task/todo/next_action 已经通过 `SessionMutationCapabilityBlocked` 调度；外层 mismatch `step_replan` message/tool 返回顺序保持不变。
LLM fallback authorization branch mutation metadata 覆盖通过：provider failure warning 与 authorization message append 经同一个 turn-aware `append_messages` mutation，pending authorization request mutation 的 task/run/turn/prompt/operation_level metadata 与 fallback turnRef 对齐。
Engine-native loop runner 草案已补齐：`llm_tool_loop` / `replan_action_continuation` 推荐统一为单个 loop runner 两种 mode，并明确输入/输出 payload、authorization/capability gate 顺序、action queue metadata、StepReplan synthesis、append/persist ownership 与后续 N6-N8 迁移步骤。
Control session mutation 小切片验证通过：`ControlSession` 的 bounded session 状态写入已改为 `SessionMutationControlSession`；continue/revise/pause/cancel/stop/new_task 的 session/task/todo/next_action/message、persist、memory compaction 与 message runtime events 仍复用 daemon legacy helper，stop/cancel 的 daemon job cancel 副作用继续保留在外层 control flow。
Deterministic route turn executor 小切片验证通过：`SendMessage` 的 no-LLM 本地规则 route 与 control auto-continue route 已改为 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` payload dispatch；实际 `routePrompt` 执行、local route source metadata、外层 appendMessages 与 response merge 仍保持原语义。
LLM fallback route 小切片验证通过：LLM provider failure 后的本地 fallback route 已复用 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` adapter；AI failure warning message、fallback messages prepend、tool calls merge 与 session append 顺序保持不变。
Route-before-LLM turn executor 小切片验证通过：LLM configured + deterministic pre-route 分支已复用 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` adapter；step phase tagging、toolCalls phase tagging、后续 LLM synthesis 入口与 response merge 仍保持原语义。
Replan continuation turn executor 小切片验证通过：authorization approve resume 与 route-before-LLM 后续 auto-continue 的 `runAgentReplanActionContinuation` 已经 `runtime.Engine.ExecuteTurn` dispatch 到 daemon legacy continuation adapter；实际 planner action execution、StepReplan synthesis、append 与 response merge 仍保持原语义。
Planner action execution turn executor 小切片验证通过：authorization approve 后的单个 planner action 已经 `runtime.Engine.ExecuteTurn` dispatch 到 daemon legacy `executeAgentPlannerAction` adapter；错误 fallback message、step phase tagging、authorization metadata、append/synthesis/auto-continue 语义保持原路径。
Planner loop action execution seam 小切片验证通过：LLM planner loop 与 replan continuation 内部的 `executeAgentPlannerActions` 已经按 action 复用 `planner_action_execution` adapter；authorization gate、action queue metadata、错误 fallback、phase tagging 与 StepReplan synthesis 语义保持原路径。
Turn-aware append session mutation 小切片验证通过：planner loop / replan continuation 的 action/warning append 已使用携带 task/run/turn/prompt/origin/operation_level 的 `SessionMutationAppendMessages` wrapper；默认 mutation 仍落到 daemon legacy append/persist/compaction/runtime events helper。
Turn-aware planner append mutation 小切片验证通过：LLM planner chat 初始 plan append 与 tool synthesis planner apply 已使用携带 planner_chat/tool_synthesis origin 的 turn-aware `SessionMutationAppendMessages` wrapper；planner apply、message append、response merge 与 auto-continue 语义保持不变。
Turn-aware SendMessage append mutation 小切片验证通过：`SendMessage` 的 deterministic route、route-before-LLM、LLM fallback route、control auto-continue 与 authorization-gate append 已统一携带当前 turnRef/prompt/origin/operation_level dispatch；默认 append/persist/compaction/runtime events 仍落到 daemon legacy helper。
SendMessage response merge regression matrix 验证通过：control continue/revise + LLM、pending authorization gate、route-before-LLM synthesis 已补专门覆盖，并复跑 no-LLM route、LLM fallback route/authorization 与 active-run continue parity focused tests。
SendMessage turn lifecycle runner 小切片验证通过：`runtime.Engine.RunTurnLifecycle` 已包住 `SendMessage` bounded turn body，`BeginTurn`/branch append/`FinishTurn` 顺序保持，response merge 分支拆入 daemon helper 但消息/工具/authorization gate parity 不变。
External control/authorization append mutation 小切片验证通过：`ControlSession` daemon job cancel append 与 `DecideAgentAuthorization` approved action / synthesis append 已生成轻量 external turnRef，并携带 task/run/turn/prompt/origin/operation_level 进入 `SessionMutationAppendMessages`；不发布额外 Begin/Finish lifecycle。
Authorization external runner turnRef threading 小切片验证通过：`DecideAgentAuthorization` approve 后的 planner action execution、tool synthesis 与 follow-up continuation runtime runner 现在共享 authorization external turnRef，runner request metadata 与 append mutation metadata 对齐，原 approval resolve/action/synthesis/auto-continue 语义保持不变。
Session mutation compaction metadata 小切片验证通过：append messages、control session 与 resolve authorization 的 `memory_compacted` event 现在携带 mutation task/run/turn metadata；legacy fallback 会补齐 external turnRef，不发布额外 Begin/Finish lifecycle。
Deterministic route plan-only append 小切片验证通过：capability-gap route 的 planner apply 已通过 `routePromptForTurn` 沿用外层 deterministic route turnRef，plan-only `append_messages` mutation metadata 与外层 task/run/turn 对齐。
Schedule route registry 化小切片验证通过：schedule create/list/delete 均经 `deterministic_route/registry_dispatch`；create/delete 在权限不足时不会调用 registry handler。
Advanced HTTP route registry 化小切片验证通过：POST/PUT/PATCH/DELETE 等 non-safe `http.request` route 已经 registry dispatch，`safe_scan` 不会越过 `advanced` gate。
Generic workspace fallback registry 化小切片验证通过：仅命中 generic workspace 的自然语言 route 现在映射到 `workspace.files.list` 并经 `deterministic_route/registry_dispatch` 执行。
Active-run continue context executor 小切片验证通过：无 LLM 的 control continue 会把 active run context prompt 交给 `deterministic_route` turn executor，append 仍使用 `active_run_continue` origin 与当前 turnRef。
```
