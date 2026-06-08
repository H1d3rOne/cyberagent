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
- [x] 将 planner action/replan cycle state 的 response/history message 与 top-level tool call slice 封装为 cycle-private wrappers；外层 loop、legacy synthesis 与返回边界显式转换，并补深拷贝隔离测试。
- [x] 将 planner action/replan cycle request 的 memory summary 封装为 cycle-private `agentPlannerActionReplanCycleMemorySummary`，并保持 `LastMessageAt` 深拷贝隔离。
- [x] 将 planner action/replan cycle plan handle 的 legacy planner response 改为入口/出口深拷贝隔离，避免外层 legacy 指针与 cycle state 共享可变 plan/action/todo/next_action。
- [x] 将 planner action/replan cycle plan handle 内部改为 cycle-private plan/action DTO，不再直接持有 legacy `*agentPlannerResponse`；legacy planner response 仅在入口/出口 adapter 生成。
- [x] 将 planner action/replan cycle plan/action DTO 迁到 runtime-neutral `runtime.AgentLoopPlan` / `runtime.AgentLoopAction`；daemon 只保留 legacy planner response/action 转换 adapter。
- [x] 将 planner action/replan cycle memory summary DTO 迁到 runtime-neutral `runtime.AgentLoopMemorySummary`；daemon 只保留 legacy memory summary 转换 adapter。
- [x] 将 planner action/replan cycle authorization/auth profile DTO 迁到 runtime-neutral `runtime.AgentLoopAuthorization` / `runtime.AgentLoopAuthProfile`；daemon 只保留 legacy HTTP auth profile 转换 adapter。
- [x] 将 planner action/replan cycle LLM request DTO 迁到 runtime-neutral `runtime.AgentLoopLLMRequest`；daemon 只保留 legacy `sendAgentMessageRequest` 兼容转换 adapter。
- [x] 为 planner action/replan cycle plan handle 增加 cycle-native action/auto-continue 查询 helper，外层 loop 不再为 actions/`NextAction.AutoContinue` 判断提前解包 legacy plan。
- [x] 将 planner action/replan cycle plan handle 的 action count / auto-continue 查询状态固化为构造时快照，查询 helper 不再读取内部 legacy plan 指针。
- [x] 将 planner action/replan cycle plan handle 的 actions 也固化为构造时 `actionsSnapshot`，shared cycle action executor 不再为执行动作解包 legacy plan。
- [x] 将 `runAgentLLMToolLoop` 轮次间的 plan 状态保持为 `agentPlannerActionReplanCyclePlan` handle，auto-continue 下一轮不再把 cycle plan 解包回 legacy planner response。
- [x] 将 `runAgentLLMToolLoop` / `runAgentReplanActionContinuation` 轮次间的 state 保持为 `agentPlannerActionReplanCycleState` handle，外层 loop 不再每轮把 response/tool/history 解包回 legacy slice。
- [x] 将 shared cycle StepReplan synthesis legacy adapter 调用收敛到 cycle-private `runAgentPlannerActionReplanCycleSynthesis` helper，主 cycle 不再直接展开 LLM request/history/memory/action message 的 legacy 转换。
- [x] 将 shared cycle action execution legacy adapter 调用收敛到 cycle-private `runAgentPlannerActionReplanCycleActionExecution` helper，主 cycle 不再直接展开 tool context / auth profile / plan action legacy 转换。
- [x] 将 shared cycle action/synthesis result 的 phase tagging 与 state append 收敛为 cycle wrapper/state helper，主 cycle 仅在 append/synthesis adapter 边界临时展开 legacy messages/tool calls。
- [x] 新增 runtime-neutral `runtime.AgentLoopMode` / `runtime.AgentLoopRequest` / `runtime.AgentLoopResult`，先固化 engine-native loop runner 请求/结果 DTO 与 clone 隔离语义；暂不改变 daemon 执行行为。
- [x] 新增 runtime `Engine.RunAgentLoop` / `AgentLoopRunner` seam，并先挂接 daemon legacy runner adapter；`runAgentLLMToolLoop` / `replan_action_continuation` 的 shared cycle 通过 seam 调用，行为保持不变。
- [x] 将 shared cycle runner 的内部主状态推进到 runtime `AgentLoopRequest` / `AgentLoopResult` / `AgentLoopState`，daemon-private cycle DTO 只在 action execution / tool synthesis adapter 边界兼容转换。
- [x] 将 shared cycle action execution / tool synthesis helper 的输入输出推进到 runtime `AgentLoopRequest` / `AgentLoopResult`，legacy helper 保留为兼容 wrapper。
- [x] 将 `runRuntimeAgentPlannerActionReplanCycle` 外层 dispatch 入口推进到 runtime `AgentLoopRequest` / `AgentLoopResult`，`runAgentLLMToolLoop` / `runAgentReplanActionContinuation` 轮次间保持 runtime state/plan/result。
- [ ] 继续把真正 engine-owned loop runner 切换、completion audit 与剩余副作用 ownership 分批从 daemon adapter 迁到 runtime seam。
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
- [x] P0.5-N17：把 shared cycle state 的 response/history messages 与 top-level tool calls 封装为 cycle-private wrappers；外层 loop 和 legacy adapters 只通过显式 legacy 转换读写 state，并保持深拷贝隔离。
- [x] P0.5-N18：把 shared cycle request 的 memory summary 封装为 cycle-private `agentPlannerActionReplanCycleMemorySummary`；外层 loop 在入口包装 legacy memory，LLM synthesis legacy adapter 边界显式解包并保持深拷贝隔离。
- [x] P0.5-N19：把 shared cycle plan handle 的 legacy planner response 改为入口/出口深拷贝隔离；作为 N30 前置，避免外层 loop 或 legacy adapter 返回值共享可变指针。
- [x] P0.5-N20：给 shared cycle plan handle 增加 `actionCount` / `hasActions` / `shouldAutoContinue` 查询 helper，并让 `llm_tool_loop` / `replan_action_continuation` 的循环判断优先使用 handle，减少外层 legacy plan 解包。
- [x] P0.5-N21：把 shared cycle plan handle 的 `actionCount` / `shouldAutoContinue` 查询状态改为构造时的 `actionCountSnapshot` / `autoContinue` 快照；helper 不再从内部 legacy plan 指针重新计算。
- [x] P0.5-N22：把 shared cycle plan handle 的 action list 改为构造时 `actionsSnapshot` 快照；`runAgentPlannerActionReplanCycle` 调用 action executor 时传入 snapshot action list，不再通过 `legacyPlannerResponse()` 解包 plan actions。
- [x] P0.5-N23：把 `runAgentLLMToolLoop` 外层 auto-continue 循环改为持有并传递 `agentPlannerActionReplanCyclePlan` handle；下一轮执行使用 `cycle.Plan`，不再通过 `cycle.Plan.legacyPlannerResponse()` 回填 legacy plan。
- [x] P0.5-N24：把 `runAgentLLMToolLoop` / `runAgentReplanActionContinuation` 外层循环改为持有并传递 `agentPlannerActionReplanCycleState` handle；每轮不再把 `cycle.State` 解包成 legacy response/tool/history slice 后重新包装，只在入口/返回与 synthesis adapter 边界转换。
- [x] P0.5-N25：把 StepReplan synthesis legacy adapter 调用集中到 `runAgentPlannerActionReplanCycleSynthesis`，主流程不再直接展开 LLM request/history/memory/action message legacy 转换；返回 messages/tool calls/plan 继续以 cycle wrappers/handle 承载。
- [x] P0.5-N26：把 action execution legacy adapter 调用集中到 `runAgentPlannerActionReplanCycleActionExecution`，主流程不再直接展开 `legacyRouterServices()` / auth profile 转换 / plan action snapshot legacy 调用；返回 messages/tool calls 继续以 cycle wrappers 承载。
- [x] P0.5-N27：给 cycle message/tool wrappers 增加 `len` / `withPhase` / wrapper append helper，并让主 cycle 的 action/synthesis result phase tagging 与 response/tool/history append 走 wrapper/state helper；只在 append mutation 与 synthesis legacy adapter 边界临时展开 legacy slice。
- [x] P0.5-N28：将 cycle message/tool wrappers 内部改为 cycle-private message/tool DTO，不再直接持有 legacy `agentMessage` / `agentToolCall` slice；legacy 转换集中在 wrapper ingress/egress adapter，并覆盖字段保真与深拷贝隔离。
- [x] P0.5-N29：将 cycle memory summary wrapper 内部改为 cycle-private memory DTO，不再直接持有 legacy `*agentMemorySummary`；legacy 转换集中在 wrapper ingress/egress adapter，并覆盖 LastMessageAt 指针隔离。
- [x] P0.5-N30：将 cycle plan handle 内部改为 cycle-private plan/action DTO，不再直接持有 legacy `*agentPlannerResponse`；legacy 转换集中在 plan handle ingress/egress adapter，并覆盖 plan/action 字段保真与深拷贝隔离。
- [x] P0.5-N31：将 cycle plan/action DTO 迁到 runtime-neutral `runtime.AgentLoopPlan` / `runtime.AgentLoopAction`，daemon 只保留 legacy planner response/action 转换 adapter，并补 runtime clone 隔离测试。
- [x] P0.5-N32：将 cycle memory summary DTO 迁到 runtime-neutral `runtime.AgentLoopMemorySummary`，daemon 只保留 legacy memory summary 转换 adapter，并补 runtime `LastMessageAt` clone 隔离测试。
- [x] P0.5-N33：将 cycle authorization/auth profile DTO 迁到 runtime-neutral `runtime.AgentLoopAuthorization` / `runtime.AgentLoopAuthProfile`，daemon 只保留 legacy HTTP auth profile 转换 adapter，并补 runtime secret/enabled clone 隔离测试。
- [x] P0.5-N34：将 cycle LLM request DTO 迁到 runtime-neutral `runtime.AgentLoopLLMRequest`，daemon 只保留 legacy `sendAgentMessageRequest` 兼容转换 adapter，并补 runtime auth profile clone 隔离测试。
- [x] P0.5-N35：将 cycle tool call DTO 迁到 runtime-neutral `runtime.AgentLoopToolCall` / `runtime.AgentLoopToolCallError`，daemon 只保留 legacy `agentToolCall` 兼容转换 adapter，并补 runtime FinishedAt/Error/map clone 隔离测试。
- [x] P0.5-N36：将 cycle message/link DTO 迁到 runtime-neutral `runtime.AgentLoopMessage` / `runtime.AgentLoopLink`，daemon 只保留 legacy `agentMessage` 兼容转换 adapter，并补 runtime message clone 隔离测试。
- [x] P0.5-N37：将 cycle state DTO 迁到 runtime-neutral `runtime.AgentLoopState`，daemon state wrapper 仅持有 runtime state 并保留 legacy response/tool/history adapter，补 runtime state clone 隔离测试。
- [x] P0.5-N38：将 daemon LLM turn runner payload 的 LLM request/history/memory/result 迁到 runtime-neutral `runtime.AgentLoopTurnPayload` / `runtime.AgentLoopTurnResultPayload`，daemon 仅保留 service binding compatibility adapter，并补 payload clone / adapter 隔离测试。
- [x] P0.5-N39：将 LLM stage runner planner_chat/tool_synthesis payload 与 result 迁到 runtime-neutral `runtime.AgentLoopLLMStagePayload` / `runtime.AgentLoopLLMStageResultPayload`，daemon stage runner 仅在 ingress/egress adapter 转换 legacy request/history/memory/tool messages/plan，并补 clone/adapter 隔离测试。
- [x] P0.5-N40：新增 runtime-neutral `runtime.AgentLoopToolServiceBinding`，将 LLM turn payload、deterministic route / replan continuation / planner action execution payload 与 shared cycle tool context 的 service binding 从直接 `routerServices` 持有迁到 runtime binding；daemon 仅在 compatibility adapter 边界解包，并补 binding clone / jobs 引用保真测试。
- [x] P0.5-N41：将 registry handler runtime 的 service 输入也迁到 runtime-neutral `runtime.AgentLoopToolServiceBinding`；builtin handlers 只通过 daemon compatibility helper 临时解包 legacy services，并补 registry binding jobs 引用保真与 auth profile clone 回归。
- [x] P0.5-N42：新增 runtime-neutral `runtime.AgentLoopMode` / `runtime.AgentLoopRequest` / `runtime.AgentLoopResult`，把 loop runner ownership 的请求/结果契约从文档草案落到 runtime DTO，并补请求/结果 clone 隔离测试；本切片不接入 daemon 行为。
- [x] P0.5-N43：将 daemon shared cycle request/result 先增加 `runtime.AgentLoopRequest` / `runtime.AgentLoopResult` adapter 映射，保持现有 `runAgentPlannerActionReplanCycle` 执行路径不变，为后续 `AgentLoopRunner` seam 做准备。
- [x] P0.5-N44：新增 runtime `Engine.RunAgentLoop` / `AgentLoopRunner` seam，并先挂接 daemon legacy runner adapter；保持 `runAgentLLMToolLoop` / `replan_action_continuation` 行为不变。
- [x] P0.5-N45：将 shared cycle runner 的 action execution / tool synthesis 内部阶段进一步以 runtime `AgentLoopRequest` / `AgentLoopResult` / `AgentLoopState` 作为主状态传递，缩小 daemon-private cycle DTO ownership。
- [x] P0.5-N46：将 shared cycle action execution / tool synthesis helper 的输入输出继续收敛为 runtime DTO，减少 `agentPlannerActionReplanCycleRequest` / `State` wrapper 在内部 helper 间传递。
- [x] P0.5-N47：将 `runRuntimeAgentPlannerActionReplanCycle` 外层 dispatch 入口继续收敛为 runtime `AgentLoopRequest` / `AgentLoopResult`，让 `runAgentLLMToolLoop` / `runAgentReplanActionContinuation` 轮次间更少依赖 cycle request/result wrapper。
- [x] P0.5-N48：将 `runAgentPlannerActionReplanCycleAgentLoop` 主流程继续减少 `agentPlannerActionReplanCycleRequest` wrapper 依赖，优先把 turn/options/plan/state 读取拆成 runtime-native helper，只在 append / legacy adapter 边界转换。
- [x] P0.5-N49：将 shared cycle 主流程中的 plan/action 查询继续替换为 runtime-native helper，减少 `agentPlannerActionReplanCyclePlan` handle 参与主流程，仅在 legacy action/synthesis adapter 边界转换。
- [x] P0.5-N50：将 action execution helper 的 action list 输入继续收敛为 runtime `AgentLoopAction`，只在 `executeAgentPlannerActionList` legacy executor adapter 边界转换。
- [x] P0.5-N51：将 action gate / authorization request 路径继续改为 runtime `AgentLoopAction` helper，减少 `executeAgentPlannerActionList` 内部 legacy `agentPlannerAction` 传递，仅在 `runRuntimePlannerActionExecution` / authorization mutation adapter 边界转换。
- [x] P0.5-N52：将 planner action execution turn payload 继续收敛为 runtime `AgentLoopAction`，让 `daemonPlannerActionExecutionTurnPayload` 不再直接持有 legacy `agentPlannerAction`，只在 legacy `executeAgentPlannerAction` adapter 边界转换。
- [x] P0.5-N53：将 authorization request mutation payload 的 `Action` 继续收敛为 runtime `AgentLoopAction`，让 `daemonRequestAuthorizationMutationPayload` 不再直接持有 legacy `agentPlannerAction`，只在 legacy `requestAgentAuthorizationLegacy` adapter 边界转换。
- [x] P0.5-N54：将 planner action execution turn payload 的 auth profile 输入继续收敛为 runtime `AgentLoopAuthorization`，让 executor payload 不再直接持有 legacy `[]agentHTTPAuthProfile`，只在 legacy tool execution adapter 边界转换。
- [x] P0.5-N55：将 `runRuntimePlannerActionExecution` / `executeAgentPlannerActionList` 的 auth profile 参数继续收敛为 runtime `AgentLoopAuthorization`，让 action execution helper 内部不再传递 legacy auth profile slice。
- [x] P0.5-N56：将 `executeAgentPlannerActionList` / `runRuntimePlannerActionExecution` 的 services 参数继续收敛为 runtime `AgentLoopToolServiceBinding`，让 action execution helper 内部不再传递 legacy `routerServices`。
- [x] P0.5-N57：将 planner action execution turn result payload 继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，让 action execution turn result 不再直接持有 legacy `agentMessage` / `agentToolCall`。
- [x] P0.5-N58：将 `runRuntimePlannerActionExecution` / `executeAgentPlannerActionList` 的返回与 accumulator 继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，让 action execution helper 内部不再累积 legacy message/tool-call slice。
- [x] P0.5-N59：将 `DecideAgentAuthorization` 批准后 action execution result 的 phase/metadata merge 继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换推迟到 append/response 边界。
- [x] P0.5-N60：将 `DecideAgentAuthorization` approved-flow 的 response message/tool-call accumulator 继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换集中到 synthesis/follow-up adapter 与 final response 边界。
- [x] P0.5-N61：将 `runRuntimeAgentLLMToolSynthesis` 的 tool message 输入继续收敛为 runtime `AgentLoopMessage`，legacy `[]agentMessage` 只在兼容 wrapper 与 LLM stage adapter 边界转换。
- [x] P0.5-N62：将 `runRuntimeAgentLLMToolSynthesis` 的返回继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall` / `AgentLoopPlan`，legacy 转换推迟到 synthesis/follow-up adapter 边界。
- [x] P0.5-N63：将 `runRuntimeAgentLLMPlannerChat` 的返回继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall` / `AgentLoopPlan`，legacy 转换推迟到 LLM loop adapter 边界。
- [x] P0.5-N64：将 `runAgentLLMToolLoop` / `runRuntimeLLMToolLoop` 的返回继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换推迟到 `SendMessage` / LLMTurn adapter 边界。
- [x] P0.5-N65：将 `SendMessage` LLM success/error response merge 内部 accumulator 继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换推迟到 `buildAgentSendResponseFromAgentLoop` 与 warning/fallback adapter 边界。
- [x] P0.5-N66：将 `SendMessage` deterministic route / no-LLM route response merge accumulator 继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，减少底部 route/synthesis 分支 legacy slice 累积。
- [x] P0.5-N67：将 `SendMessage` control/error/authorization gate 简单响应分支继续收敛到 runtime response egress helper，消除 `runAgentSendMessageTurn` 内除 helper 外直接 `buildAgentSendResponse` 调用。
- [x] P0.5-N68：将 `runAgentReplanActionContinuation` / `runRuntimeReplanActionContinuation` 返回继续收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换推迟到 follow-up response merge 与 turn executor adapter 边界。
- [x] P0.5-N69：将 `daemonReplanActionContinuationTurnPayload` 的 `Req` / `History` / `MemorySummary` / `Plan` 输入继续收敛为 runtime `AgentLoopRequest` / `AgentLoopLLMRequest` / `AgentLoopMemorySummary` / `AgentLoopPlan`，legacy 转换推迟到 continuation turn executor adapter 边界。
- [x] P0.5-N70：抽出 runtime-native replan continuation request runner，让 `runAgentReplanActionContinuation` 成为 legacy wrapper，continuation turn executor adapter 不再把 `runtime.AgentLoopRequest` 转回散列 legacy 参数后再进入 loop。
- [x] P0.5-N71：抽出 runtime-native LLM tool loop request runner，让 `runAgentLLMToolLoop` 成为 legacy wrapper，LLMTurn runner adapter 不再把 runtime turn payload 转回散列 legacy 参数后再进入 loop。
- [x] P0.5-N72：将 `daemonLLMTurnPayload` 的 Loop/Services 进一步收敛为 `runtime.AgentLoopRequest`，复用 LLM tool loop request runner，减少 LLMTurn adapter 私有 payload wrapper。
- [x] P0.5-N73：将 LLM turn runner payload 进一步改为直接携带 `runtime.AgentLoopRequest`，移除 `daemonLLMTurnPayload` 单字段 compatibility wrapper。
- [x] P0.5-N74：清理已闲置的 `runtime.AgentLoopTurnPayload` DTO 与测试，LLM turn payload 契约统一到 `runtime.AgentLoopRequest` / `AgentLoopTurnResultPayload`。
- [x] P0.5-N75：将 `runRuntimeLLMToolLoop` 入口收敛为 `runtime.AgentLoopRequest`，让 SendMessage LLM branch 先构造 runtime loop request 后再进入 engine LLMTurn dispatch。
- [x] P0.5-N76：将 `runRuntimeReplanActionContinuation` 外层调用入口收敛为 `runtime.AgentLoopRequest`，让 follow-up continuation branch 先构造 runtime loop request 后再进入 turn executor dispatch。
- [x] P0.5-N77：将 replan continuation turn executor payload 进一步改为直接携带 `runtime.AgentLoopRequest`，移除 `daemonReplanActionContinuationTurnPayload` 单字段 compatibility wrapper。
- [x] P0.5-N78：将 replan continuation turn executor result payload 收敛为 `runtime.AgentLoopTurnResultPayload`，移除 `daemonReplanActionContinuationTurnResultPayload` daemon-private wrapper。
- [x] P0.5-N79：将 planner action execution turn executor result payload 收敛为 `runtime.AgentLoopTurnResultPayload`，移除 `daemonPlannerActionExecutionTurnResultPayload` daemon-private wrapper。
- [x] P0.5-N80：将 planner action execution turn executor payload 的 turn/language/operator/operation/services/authorization 输入收敛为 `runtime.AgentLoopRequest`，仅额外携带 `runtime.AgentLoopAction`。
- [x] P0.5-N81：将 deterministic route turn executor payload 的 session/prompt/language/operator/operation/services 输入收敛为 `runtime.AgentLoopRequest`，减少 deterministic route daemon-private payload 字段。
- [x] P0.5-N82：将 deterministic route turn executor result payload 收敛为 `runtime.AgentLoopTurnResultPayload`，移除 `daemonDeterministicRouteTurnResultPayload` daemon-private wrapper。
- [x] P0.5-N83：为 planner action execution 的 `runtime.AgentLoopRequest + runtime.AgentLoopAction` 组合新增 runtime-neutral payload DTO，移除最后一个 turn executor daemon-private payload wrapper。
- [x] P0.5-N84：将 `append_messages` session mutation payload 的 SessionID/Messages/Plan 输入收敛到 runtime turn/context + `runtime.AgentLoopMessage` / `runtime.AgentLoopPlan`，legacy 转换留在 mutator adapter 边界。
- [x] P0.5-N85：将 `record_user_message` session mutation payload 的 SessionID/Prompt/OperationLevel 输入收敛到 runtime turn/context，并新增 runtime-neutral payload 承载 Operator/Language，legacy 转换留在 mutator adapter 边界。
- [x] P0.5-N86：将 `request_authorization` session mutation payload 的 SessionID/OperatorPrompt/OperationLevel 输入收敛到 runtime turn/context，并新增 runtime-neutral payload 承载 Language/Action/Policy/Reason 等授权字段，legacy 转换留在 mutator adapter 边界。
- [x] P0.5-N87：将 `capability_blocked` session mutation payload 的 SessionID/OperatorPrompt/OperationLevel 输入收敛到 runtime turn/context，并新增 runtime-neutral payload 承载 Language/ActionName/IntentID/Reason/SuggestedPrompt，legacy 转换留在 mutator adapter 边界。
- [x] P0.5-N88：将 `control_session` session mutation payload 的 SessionID/Prompt/OperationLevel 输入收敛到 runtime turn/context，并新增 runtime-neutral payload 承载 Action/Reason/RunID/JobID/ClearInput/Operator/Language，legacy 转换留在 mutator adapter 边界。
- [x] P0.5-N89：将 `resolve_authorization` session mutation payload 的 SessionID/OperationLevel 输入收敛到 runtime turn/context，并新增 runtime-neutral payload 承载 Decision/AuthorizationID/Reason/Operator/Language/OperationLevelProvided，legacy 转换留在 mutator adapter 边界。
- [x] P0.5-N90：清理 session mutator runtime payload helper 重复 switch，抽出 typed clone helper 或合并 adapter 辅助逻辑，并审计 session mutation 是否还存在 daemon-private payload wrapper。
- [x] P0.5-N91：盘点并收敛 session mutation result payload 边界，优先为 record/control/resolve 等仍需返回 session snapshot 的结果定义 runtime-neutral result DTO 或明确 adapter 保留理由。
- [x] P0.5-N92：定义 runtime-neutral session mutation result DTO 的最小骨架（session snapshot / authorization / compaction 边界），并完成 record_user_message result adapter roundtrip。
- [x] P0.5-N93：将 control_session result 迁移到 runtime-neutral session mutation result DTO，复用 session snapshot/message/compaction adapter，并保持 legacy control response 语义。
- [x] P0.5-N94：将 resolve_authorization result 迁移到 runtime-neutral session mutation result DTO，复用 session snapshot/authorization/message/compaction adapter。
- [x] P0.5-N95：将 request_authorization result 迁移到 runtime-neutral session mutation result DTO，复用 session snapshot/authorization/message adapter。
- [x] P0.5-N96：将 capability_blocked result 迁移到 runtime-neutral session mutation result DTO，复用 session snapshot adapter 并保持 blocked response metadata。
- [x] P0.5-N97：将 append_messages result 迁移到 runtime-neutral session mutation result DTO，复用 session snapshot/compaction adapter，并收敛最后一个 session mutation result legacy payload。
- [x] P0.5-N98：审计并清理 session mutation result legacy fallback/helper 用法，确认 append/record/request_authorization/capability_blocked/control/resolve 的 daemon mutator 输出均为 runtime DTO，legacy result struct 只留作 adapter/caller DTO。
- [x] P0.5-N99：清理 session mutation result adapter 重复 switch，抽出 typed legacy/runtime egress helper 或按 payload family 分组，减少 `FromSessionMutationResult` boilerplate。
- [x] P0.5-N100：盘点 engine-native runner ownership 剩余 daemon-owned seam，确认下一批可迁移目标（planner JSON/native tool call 解析、authorization/capability gate 副作用、StepReplan 外层 precondition、follow-up continuation egress）与最小验证矩阵。
- [x] P0.5-N101：抽出 loop mode/default/precondition normalization helper，覆盖 LLM tool loop mode/max_loops/append metadata、plan-only no-action immediate-execution fallback、replan continuation max_loops/auto_continue stop branch；保持纯重构并补 focused parity 测试。
- [x] P0.5-N102：将 planner JSON / native tool-call parse/fallback/schedule override 收敛为 runtime-neutral parser result DTO/helper，`handleAgentLLMChat` / `synthesizeAgentToolResults` 仅保留 LLM provider 与 daemon message/tool adapter 边界。
- [x] P0.5-N103：将 authorization/capability gate decision + side-effect orchestration 标准化为 runtime-neutral gate/action-batch result DTO，先保持 mutation 副作用仍经 `SessionMutator` adapter，覆盖四类 gate parity。
- [x] P0.5-N104：标准化 StepReplan synthesis failure、auto-continue/max-loop stop 与 follow-up continuation egress metadata/stop reason，为后续真正 engine-owned loop runner 切换提供统一验收口径。
- [x] P0.5-N105：进行真正 engine-owned loop runner 切换与 completion audit，验证 runtime seam owns loop control 后 daemon adapter 只保留 legacy IO/side-effect compatibility。
- [x] P0.5-N106：清理 outer loop 旧 trampoline / side-effect compatibility 边界，优先盘点 `RunLLMTurn` / `ExecuteTurn` 是否还能进一步退化为调用方兼容层，并明确 planner/action/session 副作用后续下沉顺序。
- [x] P0.5-N107：按 N106 边界结论继续下沉 planner/provider、action execution 与 session mutation 副作用，优先选择一个可独立验证的 adapter family 做 engine-native DTO 收敛。
- [x] P0.5-N108：继续清理 planner/provider stage compatibility，优先把 provider request/stage result 边界的重复 fallback/legacy 转换再收敛一层，或转向 action execution/session mutation 剩余副作用 adapter。
- [x] P0.5-N109：继续按 N108 后边界收敛剩余 side-effect adapter，优先评估 action execution batch/queue metadata 或 session mutation 编排是否可再抽一层 runtime helper。
- [x] P0.5-N110：继续按 N109 后边界收敛 action execution / session mutation 剩余副作用 adapter，优先评估单 action execution error fallback 或 authorization approve action metadata 是否可抽为 runtime helper。
- [x] P0.5-N111：继续按 N110 后边界收敛 action execution / session mutation 剩余副作用 adapter，优先评估单 action execution error fallback result helper 或 authorization/session mutation request metadata helper。
- [x] P0.5-N112：继续按 N111 后边界收敛 action execution / session mutation 剩余副作用 adapter，优先评估 authorization/session mutation request metadata helper 或 action execution turn request helper。
- [x] P0.5-N113：继续按 N112 后边界收敛 action execution / session mutation 剩余副作用 adapter，优先评估 action execution turn request/context metadata helper 或 batch queue ID/max ownership helper。
- [ ] P0.5-N114：继续按 N113 后边界收敛 action execution / session mutation 剩余副作用 adapter，优先评估 batch queue ID/max ownership helper 或 deterministic/replan turn execution metadata helper。

当前盘点（2026-06-08）：

- 已经走 engine seam：`record_user_message`、`append_messages`、`control_session`、`resolve_authorization`、`request_authorization`、`capability_blocked`；deterministic route / planner action execution；planner chat 与 tool synthesis stage；`llm_tool_loop` / `replan_action_continuation` outer loop 现在经 `AgentLoopRunner` 接管，SendMessage LLM 主入口与 follow-up continuation 已直接进入 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`，内部 Action → StepReplan 通过 `planner_action_replan_cycle` 兼容 sub-mode 执行；`RunLLMTurn` 与 `ExecuteTurn` 的 replan kind 仅保留为兼容 trampoline。
- 已清理 session mutation 输入 payload 的 daemon-private wrapper：append/record/request_authorization/capability_blocked/control/resolve 均由 runtime DTO + TurnContext 承载；append_messages、record_user_message、request_authorization、capability_blocked、control_session 与 resolve_authorization result 已先经 runtime `AgentLoopAppendMessagesResultPayload` / `AgentLoopRecordUserMessageResultPayload` / `AgentLoopRequestAuthorizationResultPayload` / `AgentLoopCapabilityBlockedResultPayload` / `AgentLoopControlSessionResultPayload` / `AgentLoopResolveAuthorizationResultPayload` / `AgentLoopSessionSnapshot` roundtrip，legacy result struct 仅作为 daemon adapter/caller DTO 与兼容 fallback 保留。
- session mutation result payload 已整体迁入 runtime-neutral DTO：append/record/request_authorization/capability_blocked/control/resolve 均具备 runtime result DTO 与 daemon adapter roundtrip；legacy fallback helper 已收敛并重命名为 adapter-only compatibility 路径，daemon mutator 六类输出均有 runtime DTO 类型断言覆盖，`FromSessionMutationResult` 重复 switch 已收敛为 typed runtime/legacy egress helper。
- 已输出 engine-native runner 草案：建议将 `llm_tool_loop` 与 `replan_action_continuation` 统一为单个 loop runner 的两个 mode，明确请求/响应 payload、authorization gate、action queue metadata、synthesis、append/persist ownership 与迁移顺序。
- 已抽出 planner loop helper：`decideAgentPlannerActionGate` 负责无副作用计算 capability mismatch / explicit intent / operation level / manual authorization / allow 决策；`runtime.PrepareAgentLoopActionBatch` / `AgentLoopActionQueueMetadata` / `AgentLoopToolCallWithMetadata` / `AgentLoopAuthorizationActionMetadata` / `AgentLoopActionExecutionErrorResult` / `AgentLoopAuthorizationRequestMutationMetadata` / `AgentLoopCapabilityBlockedMutationMetadata` / `AgentLoopActionExecutionTurnMetadata` 负责统一 action batch 截断、queue/authorization/session mutation/action execution turn metadata 字段、error fallback result/details 展开、metadata merge 与 clone 隔离，daemon wrapper 只保留 queue ID 生成、legacy default tool metadata、turn kind/origin 选择与实际 action/session side effect 调度。
- action gate / action batch egress 已收敛到 runtime-neutral DTO：新增 `runtime.AgentLoopActionGateDecision` / `AgentLoopActionGateResult` / `AgentLoopActionBatchResult`，`executeAgentPlannerActionList` 降为 batch result wrapper；capability_blocked / request_authorization 副作用仍经 `SessionMutator` adapter，四类 gate parity 已补 focused 覆盖。
- loop egress 已标准化到 runtime-neutral DTO：新增 `runtime.AgentLoopEgressResult` 与统一 metadata key，`planner_error` / `step_replan_synthesis_error` / `no_actions` / `auto_continue_disabled` / `cycle_stopped` / `max_loops` 均能在 `AgentLoopResult`、LLM/replan turn payload 与 executor Data 中携带 `stop_reason`、`loop_count`、`max_loops`；StepReplan synthesis failure warning 仍保留原 message/tool 行为，但 egress stop reason 已可稳定断言。
- 已抽出 planner action/replan cycle helper：`runAgentPlannerActionReplanCycle` 统一 `runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 的 Action → action append → tool synthesis → synthesis append 骨架；两种入口仍保留各自前置条件、append origin 与 response/tool merge 语义；action execution 与 StepReplan synthesis 的 legacy adapter 调用已分别收敛到 cycle-private helper。
- 已把 shared cycle helper 的输入/输出整理为 daemon-private DTO：`agentPlannerActionReplanCycleRequest` 承载 turn/cycle-private LLM request/authorization/tool context/plan handle/options，`agentPlannerActionReplanCycleState` 承载 response/tool/history state，`agentPlannerActionReplanCycleResult` 承载新 state/final plan handle/continue decision；为后续替换 runtime-neutral DTO 做前置。
- 已将 cycle tool call DTO 迁到 runtime-neutral `runtime.AgentLoopToolCall` / `runtime.AgentLoopToolCallError`：cycle message/tool wrappers 不再持有 daemon-private tool call DTO，daemon 仅在 legacy `agentToolCall` ingress/egress adapter 边界转换。
- 已将 cycle message/link DTO 迁到 runtime-neutral `runtime.AgentLoopMessage` / `runtime.AgentLoopLink`：cycle message wrapper 不再持有 daemon-private message/link/recommendation DTO，daemon 仅在 legacy `agentMessage` ingress/egress adapter 边界转换。
- 已将 cycle state DTO 迁到 runtime-neutral `runtime.AgentLoopState`：daemon state wrapper 不再直接持有 daemon-private response/tool/history wrappers，只在 legacy response/tool/history ingress/egress adapter 边界转换，并在 cycle result 初始化时 clone state 避免请求状态共享可变 slice。
- LLM turn runner 输入已统一为 runtime-neutral `runtime.AgentLoopRequest`，输出统一为 `runtime.AgentLoopTurnResultPayload`：`runtime.AgentLoopTurnPayload` 与 `daemonLLMTurnPayload` 已清理，legacy `sendAgentMessageRequest` / history / memory / service binding 转换集中在 request ingress 与 runner egress adapter。
- 已将 LLM stage runner payload/result 迁到 runtime-neutral `runtime.AgentLoopLLMStagePayload` / `runtime.AgentLoopLLMStageResultPayload`：planner_chat 与 tool_synthesis 不再通过 daemon-private stage payload 携带 legacy request/history/memory/tool messages/plan，daemon stage runner 仅在 ingress/egress adapter 转换 legacy 类型，并通过 runtime DTO clone 隔离 mutable fields。
- 已将 tool service binding adapter compatibility 迁到 runtime-neutral `runtime.AgentLoopToolServiceBinding`：LLM tool loop request、deterministic route / replan continuation / planner action execution payload 与 `agentPlannerActionReplanCycleToolContext` 不再直接持有 `routerServices`；daemon adapter 通过 `agentLoopToolServiceBindingFromRouterServices` / `legacyRouterServicesFromAgentLoopToolServiceBinding` 解包，并保持 `jobs` 等服务引用不丢失。
- registry handler runtime 也已改为持有 runtime-neutral `runtime.AgentLoopToolServiceBinding`：`agentRegistryHandlerRuntime.Services` 不再暴露 daemon `routerServices`，builtin handler 只在调用 legacy handler adapter 时通过 `legacyRouterServices()` 解包，并保持 auth profiles slice clone 语义。
- engine-native loop runner 的顶层 DTO 已落到 runtime package：`runtime.AgentLoopMode` 覆盖 `llm_tool_loop` / `replan_action_continuation` 两种 outer mode，并新增 `planner_action_replan_cycle` 作为迁移期内部 sub-mode；`runtime.AgentLoopRequest` 汇总 Turn/LLM/history/memory/plan/state/authorization/tool services/metadata，`runtime.AgentLoopResult` 汇总 state/messages/tool calls/final plan/stop reason/queue IDs/metadata；`Engine.RunAgentLoop` / `AgentLoopRunner` seam 已接管 outer loop control；SendMessage/control+LLM 与 follow-up continuation 调用方已直接转调 `runRuntimeAgentLoopResult`，LLM turn runner 与 replan continuation executor 仅保留为外部/测试兼容 trampoline 并复用相同 egress payload helper。
- daemon shared cycle request/result 已具备 `runtime.AgentLoopRequest` / `runtime.AgentLoopResult` adapter 映射：覆盖 turn/LLM/auth/memory/plan/state/history/tool service/options metadata/result continue/stop reason，验证 roundtrip 与 clone 隔离；当前 `daemonAgentLoopRunner` 会按 mode 分派完整 `llm_tool_loop` outer loop、完整 `replan_action_continuation` outer loop，或单轮 `planner_action_replan_cycle` shared cycle；shared cycle 内部以 `runtime.AgentLoopState` 聚合 action execution / tool synthesis 输出，action execution 与 synthesis helper 也已改为接收 runtime `AgentLoopRequest` 并返回 runtime `AgentLoopResult`，daemon-private cycle request/state wrapper 主要保留为 legacy helper 兼容层。
- shared cycle request 已使用 runtime-native turn boundary：`agentPlannerActionReplanCycleRequest.Turn` 现在是 `runtime.TurnContext`，session/task/run/turn/prompt/origin/operation_level 不再作为散列 daemon 参数传入；仅在落回 append/action/synthesis legacy adapter 时转换为 `agentRuntimeTurnRef`。
- shared cycle request 已拆分 authorization 与 LLM synthesis request：action execution 只读取 runtime-neutral `runtime.AgentLoopAuthorization`，并通过 `runAgentPlannerActionReplanCycleActionExecutionAgentLoop` 在 helper 内临时转换 tool services / auth profiles / action snapshot；StepReplan synthesis 兼容路径已集中到 `runAgentPlannerActionReplanCycleSynthesisAgentLoop`，由 runtime DTO 临时转换到 legacy request。
- shared cycle request 的 LLM synthesis 输入已迁到 runtime-neutral `runtime.AgentLoopLLMRequest`：只保留 provider/model/API key/base URL/instructions/origin/operation_level/auth hints，并在调用 legacy synthesis adapter 时临时转换回 `sendAgentMessageRequest`。
- shared cycle request 的 auth profile 输入已迁到 runtime-neutral `runtime.AgentLoopAuthProfile`：authorization 与 LLM request 都不再直接持有 `agentHTTPAuthProfile`，legacy action/synthesis adapter 边界再转换回 daemon HTTP auth profile，并补 secret/enabled 深拷贝隔离测试。
- shared cycle request/result 的 plan 输入输出已封装为 `agentPlannerActionReplanCyclePlan` handle：cycle DTO 不再直接暴露 `*agentPlannerResponse` 字段，LLM stage/synthesis adapter 边界临时包装 legacy plan；handle 内部 plan/action 已迁到 runtime-neutral `runtime.AgentLoopPlan` / `runtime.AgentLoopAction`，不再直接持有 legacy planner response，并把 plan/action/todo/context/next_action 字段保真、深拷贝隔离、action list、action count 与 auto-continue 查询状态固化为构造时快照；shared cycle action execution helper 直接消费 `actionsSnapshot`，`runAgentLLMToolLoop` 轮次间也保持 plan handle。
- shared cycle request 的 tool/service 输入已封装为 `agentPlannerActionReplanCycleToolContext`：cycle DTO 不再直接暴露 `routerServices` 字段，tool context 内部已改为 runtime-neutral `runtime.AgentLoopToolServiceBinding`，`runAgentPlannerActionReplanCycleActionExecutionAgentLoop` 仅在 legacy action adapter 边界临时解包同一 services。
- shared cycle state 的 response/history messages 与 top-level tool calls 已封装为 `agentPlannerActionReplanCycleMessages` / `agentPlannerActionReplanCycleToolCalls`：cycle DTO 不再直接暴露 `[]agentMessage` / `[]agentToolCall` 字段；state 内部已迁到 runtime-neutral `runtime.AgentLoopState`，message wrapper 内部已迁到 runtime-neutral `runtime.AgentLoopMessage` / `runtime.AgentLoopLink`，tool wrapper 内部已迁到 runtime-neutral `runtime.AgentLoopToolCall` / `runtime.AgentLoopToolCallError`，不再持有 legacy slice；`runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 轮次间已改为直接持有 runtime `AgentLoopState` / `AgentLoopPlan` / `AgentLoopResult`，shared cycle runner 主流程已改为直接维护 `runtime.AgentLoopState` 并返回 `runtime.AgentLoopResult`，action execution / synthesis helper 已改为 runtime DTO 输入输出，只在入口/返回、append mutation 与 legacy adapter 内部显式转换，并补 state 深拷贝隔离、字段保真与多轮 history 顺序回归。
- shared cycle request 的 memory summary 已封装为 `agentPlannerActionReplanCycleMemorySummary`：cycle DTO 不再直接暴露 `*agentMemorySummary` 字段，wrapper 内部已迁到 runtime-neutral `runtime.AgentLoopMemorySummary`，不再持有 legacy memory 指针；LLM synthesis legacy adapter 边界显式转换，并补 LastMessageAt 字段保真与深拷贝隔离测试。
- `SendMessage` response merge regression matrix 已覆盖 control continue/revise、pending authorization gate、route-before-LLM synthesis，并复跑 no-LLM route、LLM fallback route/authorization、active-run continue parity focused tests。
- `SendMessage` Begin/Finish 已由 `runtime.Engine.RunTurnLifecycle` 包住 bounded turn body；turn_started → branch append/model/tool events → turn_finished/task terminal event 顺序已补回归。
- LLM tool loop 返回边界已收敛为 runtime DTO：`runAgentLLMToolLoop` / `runRuntimeLLMToolLoop` 现在返回 runtime `AgentLoopMessage` / `AgentLoopToolCall`，`daemonAgentLLMTurnRunner` result payload 直接 clone runtime DTO，legacy 转换推迟到 `SendMessage` response merge 与 LLMTurn adapter egress。
- `SendMessage` LLM merge accumulator 已收敛为 runtime DTO：control continue/revise + LLM、route-before-LLM synthesis、main LLM success 与 LLM failure fallback route/authorization 分支内部以 runtime `AgentLoopMessage` / `AgentLoopToolCall` 聚合 response；legacy 转换集中到 `buildAgentSendResponseFromAgentLoop`、append mutation 与 warning/fallback adapter 边界。
- `SendMessage` deterministic/no-LLM route merge accumulator 已收敛为 runtime DTO：active-run continue、auto-continue deterministic route、route-before-LLM route error、底部 no-LLM route success/error 与 optional synthesis branch 内部以 runtime `AgentLoopMessage` / `AgentLoopToolCall` 聚合 response；legacy 转换集中到 `buildAgentSendResponseFromAgentLoop`、append mutation 与 synthesis adapter 边界。
- `SendMessage` control/error/authorization gate 简单响应已收敛到 runtime response egress helper：control error、普通 control response 与 pending authorization gate 分支都先构造 runtime `AgentLoopMessage` / `AgentLoopToolCall`，`runAgentSendMessageTurn` 内除 `buildAgentSendResponseFromAgentLoop` helper 外不再直接调用 legacy `buildAgentSendResponse`。
- replan continuation 返回边界已收敛为 runtime DTO：`runAgentReplanActionContinuation` / `runRuntimeReplanActionContinuation` 现在返回 runtime `AgentLoopMessage` / `AgentLoopToolCall`，continuation turn executor result payload 已统一使用 `runtime.AgentLoopTurnResultPayload`；legacy 转换推迟到 follow-up response merge 与 turn executor adapter 边界。
- replan continuation turn executor payload 已去除 daemon-private wrapper：`runtime.TurnContext.Payload` 现在直接携带 `runtime.AgentLoopRequest`，daemon continuation executor 直接 clone request 并调用 `runAgentReplanActionContinuationAgentLoop`；legacy `sendAgentMessageRequest` / history / memory / plan / services 输入转换集中到 runtime request ingress。
- planner action execution turn executor result payload 已统一使用 `runtime.AgentLoopTurnResultPayload`，`runRuntimePlannerActionExecution` egress 直接 clone runtime messages/tool calls。
- replan continuation 外层 runner 已抽为 runtime-native request 主入口：`runAgentReplanActionContinuationAgentLoop` 直接消费 `runtime.AgentLoopRequest` 并串联 cycle state/plan，`runAgentReplanActionContinuation` 降为 legacy wrapper，continuation turn executor adapter 不再把 runtime request 转回散列 legacy 参数后再进入 loop。
- LLM tool loop 外层 runner 已抽为 runtime-native request 主入口：`runAgentLLMToolLoopAgentLoop` 直接消费 `runtime.AgentLoopRequest`，统一默认 mode/max loops/append metadata，并用 runtime history/state/memory/auth/service binding 串联 planner chat 与 shared cycle；`runAgentLLMToolLoop` 降为 legacy wrapper，LLMTurn runner adapter 不再把 runtime turn payload 展开回散列 legacy 参数后再进入 loop。
- LLM turn runner 私有 payload 已清理：Loop/history/state/memory/auth/service binding/max_loops/metadata 都在 `runtime.AgentLoopRequest` 内 clone 传递，`daemonAgentLLMTurnRunner` 直接复用 LLM tool loop request runner。
- LLM turn runner payload 已去除 daemon-private wrapper：`runtime.LLMTurnRequest.Payload` 现在直接携带 `runtime.AgentLoopRequest`，daemon runner 直接 clone request 并调用 `runAgentLLMToolLoopAgentLoop`。
- 闲置的 `runtime.AgentLoopTurnPayload` 已清理，runtime LLM turn 输入侧统一使用 `runtime.AgentLoopRequest`，输出侧保留 `runtime.AgentLoopTurnResultPayload`。
- `runRuntimeLLMToolLoop` 已新增 runtime request 主入口 `runRuntimeLLMToolLoopAgentLoop`；SendMessage LLM/control+LLM branch 先通过 `agentLLMToolLoopRequestFromLegacy` 构造 `runtime.AgentLoopRequest`，再进入 engine `RunAgentLoop` dispatch。
- `runRuntimeReplanActionContinuation` 已新增 runtime request 主入口 `runRuntimeReplanActionContinuationAgentLoop`；authorization approve follow-up 与 route-before-LLM synthesis follow-up branch 先通过 `agentReplanActionContinuationRequestFromLegacy` 构造 `runtime.AgentLoopRequest`，再进入 engine `RunAgentLoop` dispatch。
- P0.5-N105 runner ownership completion audit：
  - `daemonAgentLoopRunner.RunAgentLoop` 现在按 `llm_tool_loop`、`replan_action_continuation`、`planner_action_replan_cycle` 三种 mode 分派；前两者拥有完整 outer loop control，后者只执行单轮 Action → StepReplan cycle，避免 outer runner 递归。
  - `daemonAgentLLMTurnRunner` 与 replan continuation `daemonAgentTurnExecutor` 继续通过 `runRuntimeAgentLoopResult` 转调 `Engine.RunAgentLoop`，但 SendMessage LLM 主入口、control+LLM 分支与 follow-up continuation 已不再绕经 `RunLLMTurn` / `ExecuteTurn`，旧 seam 仅作为兼容 trampoline；stop_reason / loop_count / max_loops egress 继续透传并共用 helper。
  - LLM planner chat / StepReplan synthesis 的 provider 调用、planner JSON parse、plain-answer fallback、native tool-use action merge 与 schedule intent override 仍在 `handleAgentLLMChat` / `synthesizeAgentToolResults`。
  - `runAgentLLMToolLoopAgentLoop` 仍拥有 mode/default/max_loops、append metadata、planner chat append/apply、plan-only no-action 与 immediate-execution fallback、auto-continue stop 判断。
  - `runAgentReplanActionContinuationAgentLoop` 仍拥有 continuation max_loops、已有 plan action/auto_continue precondition 与循环停止条件；follow-up egress stop reason / loop_count / max_loops metadata 已按 N104 标准化。
  - `runAgentPlannerActionReplanCycleAgentLoop` 仍拥有 action message append、StepReplan append/apply 与 synthesis failure warning 生成；continue / stop reason egress 已按 N104 标准化，session mutation 副作用仍由 daemon 触发。
  - `executeAgentPlannerActionList` 仍拥有 action batch 上限、action queue metadata 与 per-action error fallback；authorization/capability gate decision/result 与 batch egress 已收敛到 runtime DTO，request_authorization/capability_blocked mutation orchestration 暂仍在 daemon，经 `SessionMutator` adapter 执行。
- P0.5-N100 最小验证矩阵：
  - planner chat parse error / plain answer fallback / native tool-use only / schedule override。
  - plan-only no actions 与 immediate-execution fallback actions 不混淆。
  - capability mismatch、explicit intent、operation level、manual authorization 四类 gate 的 session status、pending authorization / blocked state、message phase 与 append 顺序。
  - action queue metadata 同时落在 top-level tool calls 与 message 内 tool calls。
  - single action execution error fallback message/tool result 保持 details 展开语义。
  - StepReplan synthesis success/failure warning、append origin、planner apply 与 final plan egress。
  - replan continuation auto-continue / stop / max_loops 分支，以及 follow-up response merge。
  - `planner_loop` 与 `replan_action_continuation` 的 task/run/turn/prompt/operation_level metadata 对齐。
- P0.5-N101 loop default/precondition helper 已完成：`normalizeAgentLLMToolLoopRequest` / `normalizeAgentReplanActionContinuationRequest` 统一 mode、max_loops 与 append metadata 默认值，`agentLLMToolLoopStepPrecondition` / `agentLLMToolLoopNextPlanFromCycleResult` / `agentReplanActionContinuationStepPrecondition` 收敛 plan-only、immediate-execution fallback、auto_continue 与 max_loops 停止判断；focused parity 覆盖 metadata clone、显式 metadata 保留、fallback action 与 stop branch。
- P0.5-N102 planner parse result DTO/helper 已完成：新增 runtime-neutral `runtime.AgentLoopPlannerParseResult`，`agentLLMPlannerChatParseResult` / `agentLLMToolSynthesisParseResult` 统一承载 planner JSON parse、plain/prompt fallback、native tool-use action merge、schedule override、message phase 与 planner metadata；`handleAgentLLMChat` / `synthesizeAgentToolResults` 只负责 provider 调用、metadata merge 与 daemon message/tool/legacy plan adapter。
- P0.5-N105 outer loop runner 切换已完成：outer loop control 经 `Engine.RunAgentLoop` / `AgentLoopRunner`，单轮 shared cycle 使用 `planner_action_replan_cycle` sub-mode 保持 side-effect compatibility；focused 覆盖 LLM outer loop、continuation outer loop、LLMTurn trampoline 与 TurnExecutor trampoline。
- P0.5-N106 outer loop 旧 trampoline 清理已完成：`runRuntimeLLMToolLoopAgentLoop` 与 `runRuntimeReplanActionContinuationAgentLoop` 直接调用 `runRuntimeAgentLoopResult`，`RunLLMTurn` 与 replan `ExecuteTurn` 仅保留兼容入口；response merge、LLM fallback、authorization approve follow-up 已补直接 AgentLoopRunner 覆盖。
- P0.5-N107 planner/provider stage egress helper 已完成：`handleAgentLLMChat` / `synthesizeAgentToolResults` 在 parse metadata merge 后统一通过 `agentLLMStageResultPayloadFromPlannerParse` 构造 runtime `AgentLoopLLMStageResultPayload`，legacy 返回值集中到 `agentLoopLLMStageResultPayloadToLegacy` 兼容边界；补 helper clone/phase parity 覆盖。
- P0.5-N108 planner/provider stage dispatch helper 已完成：新增 `runRuntimeAgentLLMStage`、`agentLLMStageDispatchInput` 与 stage turn/payload/metadata helper，planner_chat 与 tool_synthesis 共用 `RunLLMStage` dispatch、`ErrLLMStageRunnerNotAttached` fallback、invalid payload 与 runtime payload clone 逻辑；补 fallback、invalid payload、payload clone 与现有 stage dispatch 覆盖。
- P0.5-N109 action batch/queue metadata helper 已完成：新增 runtime `AgentLoopPreparedActionBatch`、`PrepareAgentLoopActionBatch`、queue/batch metadata key constants 与 tool/message metadata merge helper；daemon `executeAgentPlannerActionBatch` 复用 runtime batch setup，top-level tool calls 与 message 内 tool calls 的 queue metadata stamp 改为共用 helper，并覆盖截断 clone、queue metadata 一致性与 gate stop 不执行后续 action。
- P0.5-N110 authorization approve action metadata helper 已完成：新增 runtime `AgentLoopAuthorizationActionMetadata` 与 authorization metadata key constants，批准后 action execution 的 top-level/message tool metadata stamp 复用 runtime/daemon shared metadata helper，并覆盖 approval append metadata。
- P0.5-N111 single action execution error fallback result helper 已完成：新增 runtime `AgentLoopActionExecutionErrorResult` 与 result key constants，daemon action batch error fallback 复用 helper，保留 service error details 嵌套与 top-level 展开语义，并避免 details 覆盖 action name。
- P0.5-N112 authorization/capability session mutation metadata helper 已完成：新增 runtime session mutation metadata key/helper，request_authorization 与 capability_blocked mutation request metadata 构造复用 runtime helper，并覆盖 request/turn metadata。
- P0.5-N113 action execution turn metadata helper 已完成：新增 runtime action execution turn metadata key/helper，planner action execution turn context 复用 helper 构造 executor/kind/operation_level/action_name metadata，并覆盖 metadata merge 与 clone 隔离。
- 下一批迁移顺序：N114 继续按 N113 后边界收敛 action execution / session mutation 剩余副作用 adapter，优先评估 batch queue ID/max ownership helper 或 deterministic/replan turn execution metadata helper。

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

2026-06-08 本轮增量（P0.5-N113：action execution turn metadata helper）：

```bash
go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopActionExecution'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；planner action execution turn context 的 executor/kind/operation_level/action_name metadata 已迁到 runtime helper，保持原 request/action payload clone 与 turn metadata merge 语义。

2026-06-08 本轮增量（P0.5-N112：authorization/capability session mutation metadata helper）：

```bash
go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopSessionMutationMetadataHelpers'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerExecutePlannerActionBatchGateResultParity'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；request_authorization 与 capability_blocked session mutation request metadata key/map 构造已迁到 runtime helper，daemon gate side-effect request 保持 turn/session/action metadata 语义。

2026-06-08 本轮增量（P0.5-N111：single action execution error fallback result helper）：

```bash
go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopActionExecutionErrorResultExpandsServiceDetails|TestAgentLoopActionGateAndBatchResultCloneIsolatesMutableFields'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper|TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；单 action execution error fallback 的 tool result 构造已迁到 runtime helper，保持 `action`、`details`、service details 顶层展开与 queue metadata stamp 语义。

2026-06-08 本轮增量（P0.5-N110：authorization approve action metadata helper）：

```bash
go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopAuthorizationActionMetadata|TestAgentLoopAuthorizationCloneIsolatesAuthProfiles'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerDecideAgentAuthorizationApprovedActionAppendUsesExternalTurnRef|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；authorization approve 后 action execution 的 metadata key/helper 已迁到 runtime，批准后 top-level/message tool metadata stamp 复用 shared metadata helper，保留 action name、authorization id/status 与 external turn ref append 语义。

2026-06-08 本轮增量（P0.5-N109：action batch/queue metadata runtime helper）：

```bash
go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopActionGateAndBatchResultCloneIsolatesMutableFields|TestPrepareAgentLoopActionBatchTruncatesClonesAndSeedsMetadata|TestAgentLoopActionQueueAndToolMetadataHelpersClone'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchGateResultParity|TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchStopsAtGateBeforeFollowingActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；action batch 截断/metadata seed、queue metadata key 与 tool/message metadata stamp 已收敛到 runtime helper，daemon action batch 保持 gate stop、default tool metadata 与 legacy side-effect 顺序不变。

2026-06-08 本轮增量（P0.5-N108：planner/provider stage dispatch compatibility helper）：

```bash
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeAgentLLMStage|TestAgentLLMStagePayloadFromDispatchInputClonesMutableFields|TestAgentManagerRunRuntimeAgentLLMPlannerChatDispatchesEngineStageRunner|TestAgentManagerRunRuntimeAgentLLMToolSynthesisDispatchesEngineStageRunner'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；planner_chat 与 tool_synthesis 的 stage dispatch、NotAttached fallback、invalid payload 与 payload clone 逻辑已统一到 `runRuntimeAgentLLMStage` / `agentLLMStageDispatchInput` helper。

2026-06-08 本轮增量（P0.5-N107：planner/provider stage egress runtime payload helper）：

```bash
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLLMPlannerChatParseResult|TestAgentLLMToolSynthesisParseResult|TestAgentLLMStageResultPayloadFromPlannerParseBuildsRuntimeDTO|TestAgentLoopLLMStageResultPayloadToLegacyClonesRuntimePayload|TestAgentManagerRunRuntimeAgentLLMPlannerChatDispatchesEngineStageRunner|TestAgentManagerRunRuntimeAgentLLMToolSynthesisDispatchesEngineStageRunner'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；planner chat 与 tool synthesis 的 parse 后消息/工具/plan egress 已统一到 runtime `AgentLoopLLMStageResultPayload` helper，legacy message/tool/plan 返回只保留在兼容转换边界。

2026-06-08 本轮增量（P0.5-N106：outer loop 旧 trampoline / compatibility 边界清理）：

```bash
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoopDispatchesAgentLoopRunner|TestAgentManagerRunRuntimeReplanActionContinuationDispatchesAgentLoopRunner|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackAuthorizationUsesSessionMutations|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerSendMessageResponseMergeControlLLMBranches|TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；SendMessage LLM 主入口、control+LLM 与 follow-up continuation 已直接进入 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`；`RunLLMTurn` 与 replan `ExecuteTurn` 降为兼容 trampoline，egress payload/max_loops 映射共用 `agentLoopTurnResultPayloadFromAgentLoopResult`。

2026-06-08 本轮增量（P0.5-N105：engine-owned outer loop runner 切换与 completion audit）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunAgentLoop|TestAgentManagerRunRuntimeAgentPlannerActionReplanCycleDispatchesEngineRunner|TestAgentManagerRunAgentLLMToolLoopCarriesRuntimeLoopDTOBetweenCycles|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest|TestDaemonAgentLoopRunnerExecutesLLMToolOuterLoopFromRuntimeRequest|TestDaemonAgentLoopRunnerExecutesReplanContinuationOuterLoopFromRuntimeRequest|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineRunner|TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon focused 与包级测试通过；`daemonAgentLoopRunner` 现在按 `llm_tool_loop` / `replan_action_continuation` / `planner_action_replan_cycle` 分派，outer loop control 已通过 `Engine.RunAgentLoop` 接管；LLM turn runner 与 replan continuation executor 只保留兼容 trampoline 并透传标准 egress metadata。

2026-06-08 本轮增量（P0.5-N104：AgentLoop egress stop reason 标准化）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试与 diff check 通过；新增 `runtime.AgentLoopEgressResult`，StepReplan synthesis failure、no actions、auto_continue disabled、cycle stopped、max_loops 与 planner error 均有标准 `stop_reason` / `loop_count` / `max_loops` egress；LLM tool loop、follow-up replan continuation、turn runner 与 executor payload/Data 已补 focused 覆盖。

2026-06-08 本轮增量（P0.5-N102：planner parse/native tool-use runtime-neutral parser result）：

```bash
go test -count=1 ./internal/agent/runtime -run TestAgentLoopPlannerParseResultCloneIsolatesMutableFields
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLLMPlannerChatParseResultParsesJSONAndMetadata|TestAgentLLMPlannerChatParseResultMergesNativeToolUse|TestAgentLLMPlannerChatParseResultScheduleOverride|TestAgentLLMToolSynthesisParseResultPlainAndEmptyFallback'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused runtime/daemon parser tests、runtime/daemon 包测试与 diff check 通过；planner JSON parse、native tool-use only、schedule override、plain synthesis fallback 与 empty synthesis fallback 均通过 `runtime.AgentLoopPlannerParseResult` clone/parity 覆盖。

2026-06-08 本轮增量（P0.5-N101：loop mode/default/precondition normalization helper）：

```bash
go test -count=1 ./internal/interfaces/daemon -run 'TestNormalizeAgentLLMToolLoopRequestDefaultsAndPreservesExplicitMetadata|TestAgentLLMToolLoopStepPreconditionFallbackAndStopBranches|TestAgentLLMToolLoopNextPlanFromCycleResultStopsWithoutAutoContinue|TestNormalizeAgentReplanActionContinuationRequestDefaultsAndPreconditions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused daemon tests、runtime/daemon 包测试与 diff check 通过；LLM tool loop 与 replan continuation 的 mode/default/max_loops/append metadata、plan-only no-action、immediate-execution fallback、auto_continue/max_loops stop branch 已抽为纯 helper 并覆盖 clone/metadata parity。

2026-06-08 本轮增量（P0.5-N99：session mutation result adapter helper 去重）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试通过；session mutation result adapter 六个 `FromSessionMutationResult` 入口已统一走 `daemonSessionMutationResultPayloadFromRuntimeOrLegacy` typed helper，继续保留 legacy fallback、runtime value 与 runtime pointer DTO 三类兼容路径，并补 helper 覆盖；diff check 通过。

2026-06-08 本轮增量（P0.5-N98：session mutation result legacy fallback/helper 审计）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试通过；legacy result fallback helper 已重命名为 `daemonLegacySessionMutationResultPayload` 并标注 adapter-only compatibility，append/record/request_authorization/capability_blocked/control/resolve 的 `daemonSessionMutator.MutateSession` 输出均补 runtime `AgentLoop*ResultPayload` 类型断言覆盖；diff check 通过。

2026-06-08 本轮增量（P0.5-N97：append_messages result runtime DTO roundtrip）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试通过；append_messages session mutation result 已经由 daemon legacy result 转为 runtime `AgentLoopAppendMessagesResultPayload`，并补 daemon mutator 输出 runtime DTO 覆盖；diff check 通过。

2026-06-08 本轮增量（P0.5-N96：capability_blocked result runtime DTO roundtrip）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试通过；capability_blocked session mutation result 已经由 daemon legacy result 转为 runtime `AgentLoopCapabilityBlockedResultPayload`，再在 egress adapter 转回 legacy caller DTO；diff check 通过。

2026-06-08 本轮增量（P0.5-N95：request_authorization result runtime DTO roundtrip）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试通过；request_authorization session mutation result 已经由 daemon legacy result 转为 runtime `AgentLoopRequestAuthorizationResultPayload`，再在 egress adapter 转回 legacy caller DTO；diff check 通过。

2026-06-08 本轮增量（P0.5-N94：resolve_authorization result runtime DTO roundtrip）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试通过；resolve_authorization session mutation result 已经由 daemon legacy result 转为 runtime `AgentLoopResolveAuthorizationResultPayload`，再在 egress adapter 转回 legacy caller DTO；diff check 通过。

2026-06-08 本轮增量（P0.5-N93：control_session result runtime DTO roundtrip）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试通过；control_session session mutation result 已经由 daemon legacy result 转为 runtime `AgentLoopControlSessionResultPayload`，再在 egress adapter 转回 legacy caller DTO；diff check 通过。

2026-06-08 本轮增量（P0.5-N92：runtime-neutral session mutation result DTO 骨架 + record_user_message result roundtrip）：

```bash
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：runtime/daemon 包测试通过；record_user_message session mutation result 已经由 daemon legacy result 转为 runtime `AgentLoopRecordUserMessageResultPayload`，再在 egress adapter 转回 legacy caller DTO；diff check 通过。

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

本轮增量（P0.5-N17：planner action/replan cycle state message/tool wrappers）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCyclePlan|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle state 的 response/history messages 与 top-level tool calls 封装为 cycle-private `agentPlannerActionReplanCycleMessages` / `agentPlannerActionReplanCycleToolCalls` 后，cycle DTO 不再直接暴露 `[]agentMessage` / `[]agentToolCall` 字段；外层 loop、legacy synthesis 与返回边界显式转换，并新增 state 深拷贝隔离测试，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N18：planner action/replan cycle memory summary wrapper）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCyclePlan|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle request 的 memory summary 封装为 cycle-private `agentPlannerActionReplanCycleMemorySummary` 后，cycle DTO 不再直接暴露 `*agentMemorySummary` 字段；LLM synthesis legacy adapter 边界显式转换，并新增 memory summary / `LastMessageAt` 深拷贝隔离测试，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N19：planner action/replan cycle plan handle 深拷贝隔离）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：`agentPlannerActionReplanCyclePlan` 现在对 legacy planner response 做入口/出口深拷贝隔离，覆盖 SuggestedPrompts、RecommendationOptions、Task、Todos/ContextRefIDs、ContextRefs、NextAction、Actions/ToolCalls arguments；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N20：planner action/replan cycle plan handle 查询 helper）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：`agentPlannerActionReplanCyclePlan` 新增 `actionCount` / `hasActions` / `shouldAutoContinue` cycle-native 查询 helper，`llm_tool_loop` 与 `replan_action_continuation` 的循环判断优先使用 plan handle，不再为 actions/auto-continue 判断提前解包 legacy planner response；plan handle 测试覆盖 nil/no-action/manual-stop/auto-continue 与入口/出口 mutation 隔离，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N21：planner action/replan cycle plan handle 查询快照）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：`agentPlannerActionReplanCyclePlan` 将 action count / auto-continue 查询状态固化为构造时 `actionCountSnapshot` / `autoContinue` 快照；`actionCount` / `hasActions` / `shouldAutoContinue` 不再从内部 legacy plan 指针重新计算，并补内部 legacy plan mutation 不影响查询 helper 的隔离测试；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N22：planner action/replan cycle plan handle action snapshot）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：`agentPlannerActionReplanCyclePlan` 新增构造时 `actionsSnapshot` 并通过 `actions()` 深拷贝返回；`runAgentPlannerActionReplanCycle` 调用 `executeAgentPlannerActionList` 直接消费 snapshot actions，不再为 action execution 解包 legacy planner response。新增 shared cycle action snapshot 回归，覆盖内部 legacy plan actions 被清空后仍按 snapshot 执行动作；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N23：LLM planner loop 保持 cycle plan handle）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCyclePlan'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：`runAgentLLMToolLoop` 初始化后即把 planner chat/fallback plan 包装成 `agentPlannerActionReplanCyclePlan`，循环内向 shared cycle 传递 `cyclePlan`，auto-continue 下一轮直接使用 `cycle.Plan`，不再通过 `cycle.Plan.legacyPlannerResponse()` 回填 legacy plan；新增两轮 auto-continue 回归覆盖 first/second action 与 synthesis 顺序，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N24：planner loop 保持 cycle state handle）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleState'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：`runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 初始化后持有 `agentPlannerActionReplanCycleState`，每轮向 shared cycle 直接传递/接收 `cycleState`，不再把 response/tool/history state 解包回 legacy slice 后重新包装；只在函数返回与 `runRuntimeAgentLLMToolSynthesis` legacy adapter 边界转换。新增 continuation 多轮 auto-continue state/history 回归，并扩展 LLM loop synthesis history 快照断言；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N25：planner cycle synthesis adapter helper）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleLLMRequest'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：StepReplan tool synthesis 调用收敛到 `runAgentPlannerActionReplanCycleSynthesis`，该 helper 统一处理 LLM request、cycle history、memory summary 与 action messages 的 legacy adapter 边界，并以 cycle messages/tool calls/plan handle 返回；主 cycle 流程不再直接展开 synthesis payload 的 legacy 转换。新增 helper 覆盖验证 payload 来自 cycle wrappers/state、mutation 不污染输入、返回 wrappers 深拷贝隔离以及 planner apply 所需 legacy plan 指针兼容；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N26：planner cycle action execution adapter helper）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：shared cycle action execution 调用收敛到 `runAgentPlannerActionReplanCycleActionExecution`，该 helper 统一处理 turnRef、tool context、plan action snapshot 与 authorization profiles 的 legacy adapter 边界，并以 cycle messages/tool calls 返回；主 cycle 流程不再直接展开 action execution payload 的 legacy 转换。新增 helper 覆盖验证 payload 来自 cycle request/wrappers、services binding 透传、auth/action mutation 不污染输入、返回 wrappers 深拷贝隔离以及 action queue metadata parity；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N27：planner cycle wrapper phase/append helper）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：cycle message/tool wrappers 新增 `len` / `withPhase` 与 wrapper append helper，`agentPlannerActionReplanCycleState` 新增 response/history/tool append helper；主 cycle 对 action/synthesis result 的 phase tagging 与 state append 不再直接展开 legacy slice，仅在 append mutation 与 synthesis adapter 边界临时转换。扩展 state wrapper 测试覆盖 phase tagging 不污染原 wrapper、append 深拷贝隔离与主 cycle action snapshot parity；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。


本轮增量（P0.5-N28：planner cycle message/tool private DTO）

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleState'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：cycle message/tool wrappers 内部存储改为 cycle-private message/tool DTO，不再直接持有 legacy `agentMessage` / `agentToolCall` slice；legacy 转换集中在 wrapper ingress/egress adapter。扩展 state wrapper 测试覆盖 message/tool 字段保真、nested links/suggested prompts/recommendation options、tool call error/detail 与 `FinishedAt` 指针深拷贝隔离；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。


本轮增量（P0.5-N29：planner cycle memory private DTO）

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleMemory|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：cycle memory summary wrapper 内部存储改为 cycle-private memory DTO，不再直接持有 legacy `*agentMemorySummary`；legacy 转换集中在 wrapper ingress/egress adapter。扩展 memory wrapper 测试覆盖 Summary/MessageCount/LastMessageID/UpdatedAt 字段保真、LastMessageAt 入口与出口指针深拷贝隔离，以及 synthesis payload mutation 不污染 cycle request；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N30：planner cycle plan private DTO）

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：cycle plan handle 内部存储改为 cycle-private plan/action DTO，不再直接持有 legacy `*agentPlannerResponse`；legacy 转换集中在 plan handle ingress/egress adapter。扩展 plan wrapper 测试覆盖 SuggestedPrompt/SuggestPrompt、recommendation detail/command、task/todo/context/next_action/reasoning 字段保真，以及 action/tool call arguments 与返回 actions 的深拷贝隔离；focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N31：planner cycle plan runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_plan.go internal/agent/runtime/agent_loop_plan_test.go internal/interfaces/daemon/agent_planner_loop.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopPlan|TestAgentPlannerActionReplanCyclePlan'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCyclePlan|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentPlannerActionReplanCycleMemory|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentPlannerActionReplanCycleAuthorization|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager|TestAgentLoopPlan'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopPlan` / `runtime.AgentLoopAction` 与 clone helpers，`agentPlannerActionReplanCyclePlan` 现在直接持有 runtime DTO；daemon 只在 legacy planner response/action ingress/egress adapter 中转换。新增 runtime clone 隔离测试，并复跑 shared cycle plan/action/synthesis/state 回归，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N32：planner cycle memory runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_memory.go internal/agent/runtime/agent_loop_memory_test.go internal/interfaces/daemon/agent_planner_loop.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopMemorySummary|TestAgentPlannerActionReplanCycleMemory'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleMemory|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager|TestAgentLoopMemorySummary'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopMemorySummary` 与 clone helper，`agentPlannerActionReplanCycleMemorySummary` 现在直接持有 runtime DTO；daemon 只在 legacy memory summary ingress/egress adapter 中转换。新增 runtime `LastMessageAt` clone 隔离测试，并复跑 shared cycle memory/synthesis/continuation 回归，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N33：planner cycle authorization runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_auth.go internal/agent/runtime/agent_loop_auth_test.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopAuthorization|TestAgentPlannerActionReplanCycleAuthorization|TestAgentPlannerActionReplanCycleLLMRequest'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleAuthorization|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentPlannerActionReplanCycleAuthorizationFromMessageRequestUsesPrivateAuthProfileDTO|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager|TestAgentLoopAuthorization'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopAuthorization` / `runtime.AgentLoopAuthProfile` 与 clone helpers，shared cycle request 的 authorization 字段与 LLM request auth profiles 现在直接使用 runtime DTO；daemon 只在 legacy HTTP auth profile ingress/egress adapter 中转换。新增 runtime secret/enabled clone 隔离测试，并复跑 auth/LLM request/action execution/continuation 回归，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N34：planner cycle LLM request runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_llm_request.go internal/agent/runtime/agent_loop_llm_request_test.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopLLMRequest|TestAgentPlannerActionReplanCycleLLMRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleLLMRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunAgentReplanActionContinuation'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager|TestAgentLoopLLMRequest'
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopLLMRequest` 与 clone helper，shared cycle request 的 LLM synthesis 输入现在直接使用 runtime DTO；daemon 只在 legacy `sendAgentMessageRequest` 兼容 adapter 中转换。新增 runtime auth profile clone 隔离测试，并复跑 LLM request/synthesis/continuation 回归，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N35：planner cycle tool call runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_tool_call.go internal/agent/runtime/agent_loop_tool_call_test.go internal/interfaces/daemon/agent_planner_loop.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopToolCall|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCyclePlan'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager|TestAgentLoopToolCall'
go test -count=1 ./internal/agent/runtime
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopToolCall` / `runtime.AgentLoopToolCallError` 与 clone helpers，cycle message/tool wrappers 内部现在直接持有 runtime DTO；daemon 只在 legacy `agentToolCall` ingress/egress adapter 中转换，并保持 phase/agent_phase tagging、FinishedAt/Error/map 顶层 clone 语义不变。新增 runtime tool call clone 隔离测试，并复跑 cycle state/action execution/synthesis/continuation 回归，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N36：planner cycle message runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_message.go internal/agent/runtime/agent_loop_message_test.go internal/interfaces/daemon/agent_planner_loop.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopMessage|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCyclePlan'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager|TestAgentLoopMessage'
go test -count=1 ./internal/agent/runtime
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopMessage` / `runtime.AgentLoopLink` 与 clone helpers，cycle message wrapper 内部现在直接持有 runtime DTO；daemon 只在 legacy `agentMessage` ingress/egress adapter 中转换，并保持 message phase tagging、tool call phase tagging、links/prompts/recommendation clone 语义不变。新增 runtime message clone 隔离测试，并复跑 cycle state/action execution/synthesis/continuation 回归，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N37：planner cycle state runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_state.go internal/agent/runtime/agent_loop_state_test.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopState|TestAgentPlannerActionReplanCycleState|TestAgentPlannerActionReplanCyclePlan'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycleState|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisUsesCycleState|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager|TestAgentLoopState'
go test -count=1 ./internal/agent/runtime
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopState` 与 clone helper，daemon cycle state wrapper 内部现在直接持有 runtime state DTO，response/messages/tool/history 只在 legacy adapter 边界转换；cycle result 初始化时 clone request state，避免请求状态与返回状态共享可变 slice。新增 runtime state clone 隔离测试，并复跑 cycle state/action execution/synthesis/continuation 回归，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N38：daemon LLM turn runner payload runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_turn_payload.go internal/agent/runtime/agent_loop_turn_payload_test.go internal/interfaces/daemon/agent_llm_turn_runner.go internal/interfaces/daemon/agent_send_message_response_merge_test.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopTurn|TestAgentManagerRunRuntimeLLMToolLoop|TestSendMessageResponseMerge'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoop|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuation|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunLLMStage|TestRunLLMTurn|TestExecuteTurn|TestMutateSession|TestRunTurnLifecycle|TestAgentManager|TestAgentLoopTurn'
go test -count=1 ./internal/agent/runtime
go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopTurnPayload` / `runtime.AgentLoopTurnResultPayload` 与 clone helpers，daemon LLM turn runner payload 不再直接持有 legacy `sendAgentMessageRequest` / `[]agentMessage` / `*agentMemorySummary`，result 不再返回 legacy message/tool slice；daemon 仅在 runner ingress/egress adapter 做 legacy 转换并暂保留 `routerServices` service binding。新增 payload/result clone 隔离与 runner payload adapter 隔离覆盖，focused daemon/runtime tests、daemon 全量包测试与 diff check 均通过。

本轮增量（P0.5-N39：LLM stage payload/result runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_llm_stage_payload.go internal/agent/runtime/agent_loop_llm_stage_payload_test.go internal/agent/runtime/agent_loop_plan.go internal/interfaces/daemon/agent_llm_stage_runner.go internal/interfaces/daemon/agent_send_message_response_merge_test.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopLLMStagePayload|TestAgentManagerRunRuntimeAgentLLMPlannerChat|TestAgentManagerRunRuntimeAgentLLMToolSynthesis|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesis'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopLLMStagePayload` / `runtime.AgentLoopLLMStageResultPayload` 与 `CloneAgentLoopPlan` pointer helper，planner_chat 与 tool_synthesis stage payload 不再直接持有 daemon legacy request/history/memory/tool messages，stage result 不再直接返回 legacy message/tool/plan；daemon stage runner 与 `runRuntimeAgentLLM*` 边界集中做 legacy 转换。新增 stage payload/result clone 隔离覆盖，并复跑 daemon/runtime 全量包测试。

本轮增量（P0.5-N40：tool service binding runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_tool_service_binding.go internal/agent/runtime/agent_loop_tool_service_binding_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_llm_turn_runner.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 runtime-neutral `runtime.AgentLoopToolServiceBinding` 与 clone helper，daemon 使用 `agentLoopToolServiceBindingFromRouterServices` / `legacyRouterServicesFromAgentLoopToolServiceBinding` 在 compatibility 边界绑定/解包 legacy services；`daemonLLMTurnPayload`、deterministic route / replan continuation / planner action execution payload 与 shared cycle tool context 不再直接持有 `routerServices`。新增 runtime binding clone 隔离、daemon binding adapter、LLM turn payload jobs 引用与 planner action execution jobs 引用保真覆盖，并复跑 daemon/runtime 全量包测试。

本轮增量（P0.5-N41：registry handler runtime service binding 收敛）

```bash
gofmt -w internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_registry_dispatch_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandler(RuntimeUsesToolServiceBinding|Dispatch)|TestRoutePrompt|TestAgentManagerExecuteAgentPlannerActions'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：`agentRegistryHandlerRuntime.Services` 已从 daemon `routerServices` 改为 runtime-neutral `runtime.AgentLoopToolServiceBinding`，`executeAgentRegistryHandler` 在入口绑定 legacy services，builtin registry handlers 通过 `legacyRouterServices()` helper 在 adapter 边界解包。新增 registry handler runtime binding 覆盖，验证 binding kind/metadata、`jobs` 引用保真、legacy helper 解包与 auth profile slice clone 语义，并复跑 focused 与 daemon/runtime 包测试。

本轮增量（P0.5-N42：agent loop runner request/result runtime-neutral DTO）

```bash
gofmt -w internal/agent/runtime/agent_loop_runner_payload.go internal/agent/runtime/agent_loop_runner_payload_test.go
go test -count=1 ./internal/agent/runtime
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runtime.AgentLoopMode` / `runtime.AgentLoopRequest` / `runtime.AgentLoopResult` 与 clone helpers，把 engine-native `llm_tool_loop` / `replan_action_continuation` loop runner 的顶层请求/结果契约落到 runtime package。新增 request/result clone 隔离覆盖，验证 Turn.Metadata、LLM/Auth profile secret+Enabled、history/message/tool metadata、memory LastMessageAt、plan action arguments、state slices、service binding metadata clone + Handle 保真、ActionQueueIDs 与 Metadata clone；daemon 行为未接入、不改变现有执行路径。

本轮增量（P0.5-N43：daemon shared cycle ↔ agent loop DTO adapter）

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentPlannerActionReplanCycle(Request|Result)AgentLoopAdapter'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：daemon shared cycle request/result 新增 `runtime.AgentLoopRequest` / `runtime.AgentLoopResult` adapter 映射，覆盖 turn/LLM/auth/memory/plan/state/history/tool service/options metadata 与 result continue/stop reason；新增 roundtrip 与 clone 隔离测试，验证 service binding `jobs` 引用保真、Turn.Payload 继续按迁移边界保留引用、可变 slices/maps/pointers 不串改。现有 `runAgentPlannerActionReplanCycle` 执行路径未接入新 runner，行为不变。

本轮增量（P0.5-N44：runtime AgentLoopRunner seam + daemon legacy runner adapter）

```bash
gofmt -w internal/agent/runtime/agent_loop_runner.go internal/agent/runtime/runtime.go internal/agent/runtime/engine_test.go internal/interfaces/daemon/agent_loop_runner.go internal/interfaces/daemon/agent_runtime.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/agent/runtime -run 'TestRunAgentLoop'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManager.*AgentLoop|TestAgentManagerRunRuntimeAgentPlannerActionReplanCycleDispatchesEngineRunner|TestAgentPlannerActionReplanCycle(Request|Result)AgentLoopAdapter|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runtime.Engine.RunAgentLoop` / `AgentLoopRunner` seam、nil/missing runner stable error 与 request/result clone 隔离语义；daemon `AttachEngine` 现在挂接 `daemonAgentLoopRunner`，该 adapter 先把 `runtime.AgentLoopRequest` 转回 shared cycle request 并复用 `runAgentPlannerActionReplanCycle`，不迁移执行 ownership；`runAgentLLMToolLoop` / `runAgentReplanActionContinuation` 的 shared cycle 调用改为经 engine seam dispatch，runner 缺失/失败时回退 legacy direct cycle，行为保持不变。

本轮增量（P0.5-N45：shared cycle runtime DTO 主状态）

```bash
gofmt -w internal/interfaces/daemon/agent_loop_runner_adapter_test.go internal/interfaces/daemon/agent_loop_runner.go internal/interfaces/daemon/agent_planner_loop.go
go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest'
go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentPlannerActionReplanCycle(Request|Result)AgentLoopAdapter|TestAgentManagerRunRuntimeAgentPlannerActionReplanCycleDispatchesEngineRunner'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：`daemonAgentLoopRunner` 现在直接调用 shared cycle 的 runtime request 主流程；`runAgentPlannerActionReplanCycle` 退化为 legacy wrapper，内部 action execution / tool synthesis 主状态改用 `runtime.AgentLoopState` 聚合并最终返回 `runtime.AgentLoopResult`。daemon-private request/state wrapper 继续只在 action execution 与 synthesis legacy adapter 边界兼容转换；新增 runtime request 直跑 shared cycle 回归，覆盖 action execution、tool synthesis、append mutation、history/state merge、final plan 与 action snapshot parity。

本轮增量（P0.5-N46：shared cycle helper runtime DTO 输入输出）

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_planner_loop.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentPlannerActionReplanCycle(SynthesisUsesCycleState|ActionExecutionUsesCycleRequest)|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentPlannerActionReplanCycle(SynthesisUsesCycleState|ActionExecutionUsesCycleRequest|ExecutesActionSnapshot)|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentPlannerActionReplanCycle(Request|Result)AgentLoopAdapter|TestAgentManagerRunRuntimeAgentPlannerActionReplanCycleDispatchesEngineRunner'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runAgentPlannerActionReplanCycleActionExecutionAgentLoop` 与 `runAgentPlannerActionReplanCycleSynthesisAgentLoop` runtime helper，直接接收 `runtime.AgentLoopRequest` / `runtime.AgentLoopState` 并返回 `runtime.AgentLoopResult`；shared cycle 主流程不再把完整 `agentPlannerActionReplanCycleRequest` / `State` 传入 action execution / synthesis helper。原 helper 保留为 legacy wrapper，focused tests 已切到 runtime helper，继续覆盖 action snapshot、auth/service binding、memory/history clone、append/state merge 与 synthesis final plan parity。

本轮增量（P0.5-N47：外层 cycle dispatch runtime DTO 主入口）

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeAgentPlannerActionReplanCycleDispatchesEngineRunner|TestAgentManagerRunAgentLLMToolLoopCarriesRuntimeLoopDTOBetweenCycles|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentPlannerActionReplanCycle(SynthesisUsesCycleState|ActionExecutionUsesCycleRequest|ExecutesActionSnapshot)|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopCarriesRuntimeLoopDTOBetweenCycles|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentPlannerActionReplanCycle(Request|Result)AgentLoopAdapter|TestAgentManagerRunRuntimeAgentPlannerActionReplanCycleDispatchesEngineRunner'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runRuntimeAgentPlannerActionReplanCycleAgentLoop` runtime dispatch 主入口，直接接收/返回 `runtime.AgentLoopRequest` / `runtime.AgentLoopResult`，原 `runRuntimeAgentPlannerActionReplanCycle` 退化为 legacy wrapper；`runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 每轮直接构造 runtime loop request 并用 runtime state/plan/result 串联 auto-continue。新增 runtime loop DTO 跨轮回归，覆盖 planner chat 后 state/history/initial plan 进入 runner、runner 返回 final plan 继续下一轮、最终 response/tool egress 与 append planner mutation parity。

本轮增量（P0.5-N64：LLM tool loop 返回 runtime DTO）：

```bash
gofmt -w internal/interfaces/daemon/agent_llm_turn_runner.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_session_flow.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineTurnRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesLLMTurnRunner|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentLLMToolLoopThreadsRuntimeCycleDTOState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：`runAgentLLMToolLoop` / `runRuntimeLLMToolLoop` 返回已改为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，engine LLMTurn result payload 与 loop fallback 都直接 clone runtime DTO；`SendMessage` control/main LLM branch 仅在 response merge 边界转换 legacy message/tool-call，runner adapter egress 保持 clone 隔离；focused daemon tests、runtime/daemon 包测试与 diff check 通过。

本轮增量（P0.5-N65：SendMessage LLM response merge runtime accumulator）：

```bash
gofmt -w internal/interfaces/daemon/agent_session_flow.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageResponseMergeControlLLMBranches|TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor|TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `buildAgentSendResponseFromAgentLoop` 作为 SendMessage response egress adapter；control continue/revise + LLM、route-before-LLM synthesis、main LLM success 与 LLM failure fallback route/authorization 分支内部 accumulator 改为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，只在 append/warning/fallback 与 final response 边界转换 legacy；focused daemon tests、runtime/daemon 包测试与 diff check 通过。

本轮增量（P0.5-N66：SendMessage deterministic/no-LLM response merge runtime accumulator）：

```bash
gofmt -w internal/interfaces/daemon/agent_session_flow.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessage(NoLLMRouteUsesTurnExecutor|ActiveRunContinueUsesTurnExecutor|RouteBeforeLLMUsesTurnExecutor|LLMFallbackRouteUsesTurnExecutor)|TestAgentManagerSendMessageResponseMerge(RouteBeforeLLMSynthesis|ControlLLMBranches|PendingAuthorizationGate)|TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：active-run continue、control auto-continue deterministic route、route-before-LLM route error 与底部 no-LLM route success/error/synthesis branch 的 response accumulator 改为 runtime `AgentLoopMessage` / `AgentLoopToolCall`；legacy 转换保留在 append mutation、synthesis/follow-up adapter 与 `buildAgentSendResponseFromAgentLoop` egress；focused daemon tests、runtime/daemon 包测试与 diff check 通过。

本轮增量（P0.5-N67：SendMessage control/error/auth gate response egress runtime helper）：

```bash
gofmt -w internal/interfaces/daemon/agent_session_flow.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageResponseMerge(ControlLLMBranches|PendingAuthorizationGate|RouteBeforeLLMSynthesis)|TestAgentManagerSendMessage(NoLLMRouteUsesTurnExecutor|ActiveRunContinueUsesTurnExecutor|RouteBeforeLLMUsesTurnExecutor|LLMFallbackRouteUsesTurnExecutor)|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：control error、普通 control response 与 pending authorization gate 分支的 final response egress 改为 `buildAgentSendResponseFromAgentLoop`；`runAgentSendMessageTurn` 内除 helper 实现外不再直接调用 legacy `buildAgentSendResponse`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N68：replan continuation 返回 runtime DTO）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_session_flow.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：`runAgentReplanActionContinuation` / `runRuntimeReplanActionContinuation` 返回已改为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，replan continuation turn result payload 与 fallback egress 均 clone runtime DTO；follow-up response merge 直接累积 runtime DTO，legacy 转换继续保留在 final response、append/synthesis 与 compatibility adapter 边界；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N69：replan continuation turn payload 输入 runtime request）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：`daemonReplanActionContinuationTurnPayload` 改为只携带 `runtime.AgentLoopRequest`，`Req` / `History` / `MemorySummary` / `Plan` / service binding 输入通过 runtime request 克隆传递；continuation turn executor adapter 在调用 legacy loop wrapper 前集中完成兼容转换，并保持 external turnRef、history/memory/plan、max_loops 与 result payload runtime clone 语义；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N70：replan continuation runtime-native request runner）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runAgentReplanActionContinuationAgentLoop` runtime request 主入口，直接消费 `runtime.AgentLoopRequest`、runtime history/state/memory/plan/auth/service binding 并串联 auto-continue cycle；`runAgentReplanActionContinuation` 退为 legacy wrapper，continuation turn executor adapter 直接调用 runtime request runner，不再把 payload request 展开回 legacy 参数后进入 loop；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N71：LLM tool loop runtime-native request runner）：

```bash
gofmt -w internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_llm_turn_runner.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineTurnRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesLLMTurnRunner|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentLLMToolLoopThreadsRuntimeCycleDTOState|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runAgentLLMToolLoopAgentLoop` runtime request 主入口，直接消费 `runtime.AgentLoopRequest` 并默认补齐 `llm_tool_loop` mode、max loops 与 planner append metadata；`runAgentLLMToolLoop` 退为 legacy wrapper，LLMTurn runner adapter 直接构造 runtime loop request 调用新 runner，不再把 payload 展开回散列 legacy 参数后进入 loop；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N72：LLM turn payload runtime request 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_llm_turn_runner.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_send_message_response_merge_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineTurnRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesLLMTurnRunner|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentLLMToolLoopThreadsRuntimeCycleDTOState|TestAgentManagerSendMessageResponseMergeControlLLMBranches'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：`daemonLLMTurnPayload` 从 `AgentLoopTurnPayload + Services` 收敛为单个 `runtime.AgentLoopRequest`，service binding、authorization、history/state、max loops 与 planner append metadata 都随 request clone；`daemonAgentLLMTurnRunner` 直接调用 `runAgentLLMToolLoopAgentLoop`，不再在 adapter 内重新拼 runtime request；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N73：LLM turn payload 直接携带 runtime request）：

```bash
gofmt -w internal/interfaces/daemon/agent_llm_turn_runner.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_send_message_response_merge_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineTurnRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesLLMTurnRunner|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentLLMToolLoopThreadsRuntimeCycleDTOState|TestAgentManagerSendMessageResponseMergeControlLLMBranches'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：`runRuntimeLLMToolLoop` 现在把 `runtime.AgentLoopRequest` 直接放入 `runtime.LLMTurnRequest.Payload`，`daemonAgentLLMTurnRunner` 直接按该 runtime request clone/dispatch，不再需要 `daemonLLMTurnPayload` compatibility wrapper；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N74：清理闲置 AgentLoopTurnPayload DTO）：

```bash
gofmt -w internal/agent/runtime/agent_loop_turn_payload.go internal/agent/runtime/agent_loop_turn_payload_test.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：删除已无调用方的 `runtime.AgentLoopTurnPayload` / `CloneAgentLoopTurnPayload` 与对应 clone 隔离测试，LLM turn 输入契约统一为 `runtime.AgentLoopRequest`，保留 `runtime.AgentLoopTurnResultPayload` 作为结果 payload；runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N75：runRuntimeLLMToolLoop runtime request 入口）：

```bash
gofmt -w internal/interfaces/daemon/agent_llm_turn_runner.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_session_flow.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineTurnRunner|TestAgentManagerRunRuntimeLLMToolLoopDispatchesLLMTurnRunner|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentLLMToolLoopThreadsRuntimeCycleDTOState|TestAgentManagerSendMessageResponseMergeControlLLMBranches'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runRuntimeLLMToolLoopAgentLoop(ctx, runtime.AgentLoopRequest)` 与 `agentLLMToolLoopRequestFromLegacy`，legacy wrapper 只负责 ingress 转换；SendMessage 的 control+LLM 与 main LLM branch 先构造 runtime loop request，再进入 engine `RunLLMTurn` dispatch，fallback 仍复用 `runAgentLLMToolLoopAgentLoop`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N76：runRuntimeReplanActionContinuation runtime request 入口）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_session_flow.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runRuntimeReplanActionContinuationAgentLoop(ctx, runtime.AgentLoopRequest)`、`agentReplanActionContinuationRequestFromLegacy` 与 continuation turn context helper；legacy wrapper 只负责 ingress 转换，follow-up continuation 分支先构造 runtime loop request，再进入 engine `ExecuteTurn` dispatch，fallback 复用 `runAgentReplanActionContinuationAgentLoop`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N77：replan continuation turn payload 直接携带 runtime request）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：移除 `daemonReplanActionContinuationTurnPayload` 单字段 wrapper，continuation turn executor 的 `TurnContext.Payload` 直接携带 `runtime.AgentLoopRequest` 并 clone 后进入 `runAgentReplanActionContinuationAgentLoop`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N78：replan continuation result payload runtime DTO 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeReplanActionContinuationDispatchesTurnExecutor|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationDispatchesToolSynthesisStage|TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：移除 `daemonReplanActionContinuationTurnResultPayload`，continuation turn executor result payload 统一为 `runtime.AgentLoopTurnResultPayload` 并通过 clone 隔离 messages/tool calls；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N79：planner action execution result payload runtime DTO 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：移除 `daemonPlannerActionExecutionTurnResultPayload`，planner action execution turn executor result payload 统一为 `runtime.AgentLoopTurnResultPayload`，action execution dispatch/fallback 与 shared cycle adapter focused tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N80：planner action execution turn payload runtime request 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：`daemonPlannerActionExecutionTurnPayload` 改为 `runtime.AgentLoopRequest + runtime.AgentLoopAction`，planner action execution turn executor 从 runtime request clone 中读取 turn/language/operator/operation/tool services/authorization，dispatch/fallback 与 shared cycle adapter focused tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N81：deterministic route turn payload runtime request 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor|TestAgentManagerSendMessageActiveRunContinueUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：移除 `daemonDeterministicRouteTurnPayload`，deterministic route turn executor payload 改为直接携带 `runtime.AgentLoopRequest`，route executor 从 request clone 中读取 turn/prompt/language/operator/operation/tool services；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N82：deterministic route result payload runtime DTO 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_send_message_lifecycle_test.go internal/interfaces/daemon/agent_send_message_response_merge_test.go
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor|TestAgentManagerSendMessageActiveRunContinueUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor|TestAgentManagerSendMessageRunTurnLifecycle|TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：移除 `daemonDeterministicRouteTurnResultPayload`，deterministic route turn executor result payload 统一为 `runtime.AgentLoopTurnResultPayload`，legacy message/tool call 转换推迟到 route prompt adapter egress；focused daemon tests、runtime/daemon 包测试与 diff check 通过。


本轮增量（P0.5-N83：planner action execution runtime payload DTO）：

```bash
gofmt -w internal/agent/runtime/agent_loop_plan.go internal/agent/runtime/agent_loop_action_execution_payload.go internal/agent/runtime/agent_loop_action_execution_payload_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopActionExecutionPayloadCloneIsolatesMutableFields|TestAgentLoopPlanCloneIsolatesMutableFields|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：新增 `runtime.AgentLoopActionExecutionPayload` 与 action clone helper，planner action execution turn payload 不再使用 daemon-private wrapper；focused runtime/daemon tests、runtime/daemon 包测试与 diff check 通过。

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
Planner cycle tool call runtime-neutral DTO 小切片验证通过：cycle message/tool wrappers 现在持有 `runtime.AgentLoopToolCall` / `runtime.AgentLoopToolCallError`，legacy `agentToolCall` 转换集中在 daemon adapter，phase tagging 与 clone 语义保持不变。
```
