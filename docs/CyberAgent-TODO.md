# CyberAgent TODO List

日期：2026-06-06  
状态：动态维护中  
当前分支：`feat/production-agent-runtime`  
当前基线：本分支当前 HEAD（持续滚动，见 `git log -1 --oneline`）

---

## 1. 当前结论

CyberAgent 已完成 MVP safe scan、Local Daemon / Web API、Agent Cockpit、v0.1.3-v0.1.8 Agent-native 主链路，以及 Production Agent Runtime 的小切片收敛。当前重点从“补功能”转向：

1. 保持当前分支小切片提交与验证记录清晰。
2. 已完成 L1 Desktop/Wails 技术 spike 与 L2 Local API / Daemon 稳定版（config、token、runtime DB audit、compatibility strategy）。
3. 完成发布前验证与远程 release 流程。
4. 继续推进 L3 Web 多用户平台、L4 Remote Runner、L5 Memory、L6 LLM Gateway 等后置大能力。

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
- [x] 将 `beginRuntimeTurn` / `finishRuntimeTurn` 状态迁移到 engine API，并在 `SendMessage` 全面切到 `runtime.Engine.RunTurnLifecycle` 后删除 daemon begin/finish compatibility adapters。
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
- [x] 继续把真正 engine-owned loop runner 切换、completion audit 与剩余副作用 ownership 分批从 daemon adapter 迁到 runtime seam；P0.5-N238 completion audit 确认剩余 runtime seam 边界已收敛到 manual stage/commit/tag authorization gate。
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

- [x] Run List benchmark。
- [x] 可重建 run index。
- [x] Tool Registry / MCP / Scheduler observability view。
- [x] Timeline 大量事件渲染优化。
- [x] Trace viewer JSON 分段、搜索、复制、下载增强。

---

## 6. 后置大能力

- [x] L1：Desktop / Wails 技术 spike：完成 embedded daemon `Serve(ctx, net.Listener)`、loopback ephemeral bootstrap、RuntimeContract 与 WebView bootstrap seam；详见 `docs/CyberAgent-Desktop-Wails-Spike.md`。
- [x] L2：Local API / Daemon 稳定版：完成 config defaults、token diagnostics/redaction、runtime DB migration audit 与 compatibility strategy doc/tests；详见 `docs/CyberAgent-Local-Daemon-Stability.md`。
- [ ] L3：Web 多用户平台：architecture/design slice、no-op local tenant skeleton、router context middleware、service context consumption、显式 multi-user storage gate/resolver skeleton、frontend session/workspace UX skeleton、audit/admin/rate-limit/hardening config skeleton、audit JSONL sink skeleton、admin bootstrap readiness skeleton、provider/web-search/API auth secret status-only/server-side storage skeleton、rate-limit/deployment hardening runtime 接入、identity/tenant/workspace API skeleton、frontend server-side scope hydrate skeleton、auth/session/RBAC model skeleton、server-side session store skeleton、session cookie resolver middleware skeleton、Web login/logout/refresh endpoint skeleton、RBAC middleware/enforcer skeleton、membership store skeleton、RBAC policy loading/evaluation skeleton、显式 RBAC deny/audit enforcement gate、Web session principal resolver 与只读 membership admin API skeleton、membership write/admin mutation gate skeleton、membership upsert timestamp ownership skeleton、membership timestamp monotonic/client timestamp boundary skeleton 已完成；待做真实 Web account adapter、真实 login/logout/refresh 语义、CSRF/secure cookie hardening、完整 workspace isolation；详见 `docs/CyberAgent-Web-Multi-User-Platform.md`。
- [ ] L4：Remote Runner：待做 runner protocol、job lifecycle、artifact sync、credential boundary 与 failure recovery。
- [ ] L5：长期 Target / User / Team Memory：待做 memory schema、scoping/privacy、retrieval policy、compaction 与 export/delete 边界。
- [ ] L6：LLM Gateway 作为可选增强稳定化：待做 provider routing、quota/rate limit、request/response audit、fallback 与 cost visibility。

---

## 7. 未完成项执行清单（滚动）

推荐顺序：先完成 P0.5 engine-native 收敛与语义保护测试，再补 P1 前端/QA smoke，最后推进 P2 生产可观测与大规模性能。

### 7.0 后置大能力滚动计划

- [x] L1-D1：Desktop / Wails 技术 spike，完成 embedded daemon / loopback / WebView bootstrap seam 文档化。
- [x] L2-D1：daemon config file defaults。
- [x] L2-D2：token diagnostics / redaction。
- [x] L2-D3：runtime DB migration audit。
- [x] L2-D4：compatibility strategy doc/tests，固化 `CYBERAGENT_RUNTIME` whole-engine rollback、默认 runtime engine 与 default compatibility attachments。
- [x] L3-A1：Web 多用户平台 architecture/design slice，已定义 auth/session/tenant/workspace/API/storage/audit 边界和最小迁移计划；详见 `docs/CyberAgent-Web-Multi-User-Platform.md`。
- [x] L3-A2：新增 `Principal` / `TenantContext` / `TenantResolver` no-op local skeleton，默认返回 `local/default`，不改变 bearer auth、daemon routes、Desktop/bootstrap 或 runsDir 行为。
- [x] L3-A3：给 daemon router 增加 local principal/tenant context middleware；focused tests 断言 request context 存在 `local/default` scope，且 bearer auth、未授权响应、现有 `/api/v1/healthz` 行为不变。
- [x] L3-A4：让 agent/runs/jobs/schedules/providers 关键 service entrypoints 可读取 `TenantContext`；local mode 仍使用现有 `runsDir` / provider store / runtime DB，不改变 API 响应。
- [x] L3-A5：设计并实现 multi-user mode 显式 config gate 与 per-workspace storage resolver skeleton；默认 local mode 不启用、不迁移历史 runs。
- [x] L3-A6：前端新增 login/session/workspace UX skeleton，同时保留 local daemon settings 与 bearer auth 请求链路。
- [x] L3-A7：补 audit log、admin bootstrap、secret server-side storage、rate limit 与 deployment hardening。
  - [x] L3-A7a：daemon audit/admin/rate-limit/hardening 配置骨架与 config-file merge/CLI mapping/normalize 覆盖；默认 local no-op。
  - [x] L3-A7b：audit event JSONL sink skeleton，保证 token/API key/secret 不进入 audit JSON；enabled 时写入 `daemon.startup` readiness event。
  - [x] L3-A7c：admin bootstrap token loading 与 readiness/diagnostic endpoint，仍不改变 bearer auth。
  - [x] L3-A7d：server-side provider/API auth secret storage skeleton，frontend 只显示 secret 状态。
    - [x] L3-A7d-1：`GET /api/provider-profiles` 返回 provider/web-search secret 状态并脱敏 raw `api_key`；`PUT` 空 `api_key` 不覆盖同 scope 既有 secret，Agent runtime 内部 raw store 仍可读取。
    - [x] L3-A7d-2：Settings 前端 provider/web-search secret status-only UX，不把 provider/web-search API key 写入 localStorage；保存后只保留 configured 状态。
    - [x] L3-A7d-3：API auth profile server-side storage skeleton；Settings/Agent 只保留 profile metadata/configured 状态，不把 token/password/header_value/cookie 写入 localStorage。
  - [x] L3-A7e：rate-limit middleware 与 deployment hardening gates 的最小可验证接入；默认 no-op，启用后 API chain 执行 loopback gate 与 per-client token-bucket rate limit。
  - [x] L3-A8：server-side identity/tenant/workspace API skeleton；新增 `/api/v1/me`、`/api/v1/tenants`、`/api/v1/workspaces` 与 local/default workspace select no-op，仍不启用真实 Web login 或前端伪隔离。
  - [x] L3-A9：frontend server-side identity/workspace scope hydrate skeleton；Console 从 `/api/v1/me` 同步 local/default display，失败回退 `defaultConsoleSession`，不改变 bearer token/localStorage 请求链路。
  - [x] L3-A10：server-side auth/session/RBAC model skeleton；新增 future User / Session / Role / TenantMembership / WorkspaceMembership 类型与 `daemon.auth.web_sessions` / `daemon.auth.rbac` config gate，默认关闭且不改变 local bearer 行为。
  - [x] L3-A11：server-side session store skeleton；新增 `SessionStore` interface、disabled default store 与 in-memory server-side store，作为后续 cookie resolver/login 切片基础。
  - [x] L3-A12：session cookie resolver middleware skeleton；显式 `web_sessions.enabled` 时可解析 server-side session cookie 并写入 request context，但仍由 bearer auth 保护 `/api/`，不创建/刷新 cookie、不替换 `local_bearer`。
  - [x] L3-A13：Web login/logout/session refresh endpoint skeleton；默认关闭，先定义 contract、CSRF/secure-cookie 边界与测试，不接入真实账号体系。
  - [x] L3-A14：RBAC middleware/enforcer skeleton；默认关闭，先定义 route/action role requirements 与 no-op compatibility tests，不改变 local bearer。
  - [x] L3-A15：tenant/workspace membership store skeleton；默认 local-only/no-op，先固化 membership persistence 边界与 clone/role validation。
  - [x] L3-A16：RBAC policy loading/evaluation skeleton；默认关闭，连接 membership store 与 route requirements 但仍不 deny local bearer。
  - [x] L3-A17：RBAC deny/audit enforcement gate skeleton；默认关闭，local bearer 继续不受 Web RBAC deny 影响。
  - [x] L3-A18：Web session principal/tenant resolver skeleton；默认关闭，cookie-only 仍不替代 bearer，local bearer 默认行为不变。
  - [x] L3-A19：membership admin API skeleton；默认关闭/只读先行，避免破坏 local bearer 兼容。
  - [x] L3-A20：membership write/admin mutation gate skeleton；继续默认关闭，分离 bootstrap/owner/admin 权限与 audit。
  - [x] L3-A21：membership upsert timestamp ownership skeleton；MemoryMembershipStore 在 upsert 时填充 created_at/updated_at，并在重复 upsert 时保留 created_at。
  - [x] L3-A22：membership timestamp monotonic/client timestamp boundary skeleton；store 统一 UTC 化并夹紧 updated_at >= created_at，admin mutation 忽略 client-provided timestamps。
- [ ] L4-A1：Remote Runner protocol/design slice。
- [ ] L5-A1：长期 Memory schema/scoping design slice。
- [ ] L6-A1：LLM Gateway provider-routing/cost-audit design slice。

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
- [x] P0.5-N114：继续按 N113 后边界收敛 action execution / session mutation 剩余副作用 adapter，优先评估 batch queue ID/max ownership helper 或 deterministic/replan turn execution metadata helper。
- [x] P0.5-N115：继续按 N114 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 batch queue ID/max ownership helper。
- [x] P0.5-N116：继续按 N115 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 action batch result assembly helper。
- [x] P0.5-N117：继续按 N116 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 action execution loop body helper。
- [x] P0.5-N118：继续按 N117 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 action queue metadata append runtime-neutral helper。
- [x] P0.5-N119：继续按 N118 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 action gate session mutation request helper。
- [x] P0.5-N120：继续按 N119 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 control/resolve session mutation request helper。
- [x] P0.5-N121：继续按 N120 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 append/record session mutation request helper。
- [x] P0.5-N122：继续按 N121 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 session mutation orchestration dispatch helper。
- [x] P0.5-N123：继续按 N122 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 action batch step executor seam helper。
- [x] P0.5-N124：继续按 N123 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 action execution turn result dispatch helper。
- [x] P0.5-N125：继续按 N124 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 session mutation payload/result dispatch table helper。
- [x] P0.5-N126：继续按 N125 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 turn executor shared dispatch helper。
- [x] P0.5-N127：继续按 N126 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 session mutation caller result egress helper。
- [x] P0.5-N128：继续按 N127 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 session mutation result adapter table/helper。
- [x] P0.5-N129：继续按 N128 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 append mutation bool egress helper。
- [x] P0.5-N130：继续按 N129 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 session mutation caller egress DTO/helper 边界整理。
- [x] P0.5-N131：继续按 N130 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 session mutation caller invalid-payload/fallback audit table/helper。
- [x] P0.5-N132：继续按 N131 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 action execution invalid-payload/fallback audit table/helper。
- [x] P0.5-N133：继续按 N132 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 session mutation caller policy/table cleanup。
- [x] P0.5-N134：继续按 N133 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 turn execution caller policy/table cleanup。
- [x] P0.5-N135：继续按 N134 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 action/session mutation adapter residual audit。
- [x] P0.5-N136：继续按 N135 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 legacy fallback ownership cleanup。
- [x] P0.5-N137：继续按 N136 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 runtime compatibility trampoline audit。
- [x] P0.5-N138：继续按 N137 后边界收敛 action execution / session mutation 剩余副作用 adapter，删除 legacy signature wrapper candidates。
- [x] P0.5-N139：继续按 N138 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 active engine seam compatibility ownership audit。
- [x] P0.5-N140：继续按 N139 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 RunLLMTurn / ExecuteTurn active trampoline removal gate plan。
- [x] P0.5-N141：继续按 N140 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 active trampoline caller audit 与 runtime API deprecation 证据落地。
- [x] P0.5-N142：继续按 N141 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 runtime API compatibility comment/deprecation marker 落地。
- [x] P0.5-N143：继续按 N142 后边界收敛 action execution / session mutation 剩余副作用 adapter，清理仅测试使用的 replan continuation turn context helper。
- [x] P0.5-N144：继续按 N143 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 active compatibility seam post-cleanup caller audit。
- [x] P0.5-N145：继续按 N144 后边界收敛 action execution / session mutation 剩余副作用 adapter，评估 TurnExecutor deterministic_route / planner_action_execution 内部 caller 的迁移边界。
- [x] P0.5-N146：继续按 N145 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 deterministic_route / planner_action_execution TurnContext runtime helper 下沉。
- [x] P0.5-N147：继续按 N146 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 deterministic_route / planner_action_execution runtime request helper 下沉。
- [x] P0.5-N148：继续按 N147 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 replan_action_continuation turn boundary helper 下沉。
- [x] P0.5-N149：继续按 N148 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 TurnExecutor helper ownership audit 与内部 caller 下一步评估。
- [x] P0.5-N150：继续按 N149 后边界收敛 action execution / session mutation 剩余副作用 adapter，完成 deterministic_route 内部 ExecuteTurn caller 的 AgentLoop 迁移 precondition 设计。
- [x] P0.5-N151：继续按 N150 后边界收敛 action execution / session mutation 剩余副作用 adapter，新增 deterministic_route AgentLoop mode/runner branch，并在不改变 routePrompt 语义下替换内部 ExecuteTurn caller。
- [x] P0.5-N152：继续按 N151 后边界收敛 action execution / session mutation 剩余副作用 adapter，评估 planner_action_execution AgentLoop migration precondition 与 action side-effect runtime ownership 边界。
- [x] P0.5-N153：继续按 N152 后边界收敛 action execution / session mutation 剩余副作用 adapter，抽取 planner_action_execution side-effect boundary / action executor ownership plan。
- [x] P0.5-N154：继续按 N153 后边界收敛 action execution / session mutation 剩余副作用 adapter，评估 planner_action_execution runtime action executor contract / typed service binding 迁移前置。
- [x] P0.5-N155：继续按 N154 后边界收敛 action execution / session mutation 剩余副作用 adapter，抽取 planner_action_execution executor adapter seam 并减少 direct `executeAgentPlannerAction` caller surface。
- [x] P0.5-N156：继续按 N155 后边界收敛 action execution / session mutation 剩余副作用 adapter，评估 heuristic route direct action execution caller 是否收敛到 executor adapter seam。
- [x] P0.5-N157：继续按 N156 后边界收敛 action execution / session mutation 剩余副作用 adapter，评估 legacy `executeAgentPlannerAction` 是否归档为 executor adapter implementation-only helper。
- [x] P0.5-N158：继续按 N157 后边界收敛 action execution / session mutation 剩余副作用 adapter，评估 typed tool service binding / runtime action executor service contract 的最小迁移落点。
- [x] P0.5-N159：继续按 N158 后边界收敛 action execution / session mutation 剩余副作用 adapter，将 daemon legacy planner action executor 包成 runtime `AgentLoopActionExecutor` adapter 并评估 `runRuntimePlannerActionExecution` 是否可先走 `Engine.ExecuteAgentLoopAction`。
- [x] P0.5-N160：继续按 N159 后边界收敛 action execution / session mutation 剩余副作用 adapter，将 `runRuntimePlannerActionExecution` 优先切到 `Engine.ExecuteAgentLoopAction`，保留 `ExecuteTurn` compatibility fallback 并验证既有 planner action 语义。
- [x] P0.5-N161：继续按 N160 后边界收敛 action execution / session mutation 剩余副作用 adapter，新增 planner_action_execution AgentLoop mode/runner branch，并确认 primary caller 尚需 typed service binding / authorization/result egress blocker 收敛后再切换。
- [x] P0.5-N162：继续按 N161 后边界收敛 action execution / session mutation 剩余副作用 adapter，将 `runRuntimePlannerActionExecution` 主 caller 从 `Engine.ExecuteAgentLoopAction` 切到 `Engine.RunAgentLoop` 的 planner_action_execution mode，并保留 action executor / TurnExecutor / direct legacy fallback 语义。
- [x] P0.5-N163：继续按 N162 后边界收敛 action execution / session mutation 剩余副作用 adapter，拆解 planner_action_execution runner branch 剩余 typed tool service binding / authorization/result egress blocker，完成 result-to-loop composition runtime helper 下沉并评估下一处可 runtime-owned 的最小边界。
- [x] P0.5-N164：继续按 N163 后边界收敛 action execution / session mutation 剩余副作用 adapter，推进 planner_action_execution legacy result egress adapter 收敛，完成 executor seam caller-facing result DTO runtime 化并评估 `executeAgentPlannerAction` / action executor 返回 runtime DTO 的最小边界。
- [x] P0.5-N165：继续按 N164 后边界收敛 action execution / session mutation 剩余副作用 adapter，拆解 planner_action_execution typed tool service binding blocker，评估替代 `AgentLoopToolServiceBinding.Handle` / legacy `routerServices` unwrap 的最小 typed service provider contract。
- [x] P0.5-N166：继续按 N165 后边界收敛 action execution / session mutation 剩余副作用 adapter，迁移 `registry_handler_service_consumers` 第一批低风险 handler 从 `agentRegistryHandlerRuntime.legacyRouterServices()` 切到 typed service provider accessors，并保留 legacy `routerServices` reconstruction fallback。
- [x] P0.5-N167：继续按 N166 后边界收敛 `registry_handler_service_consumers`，迁移 scan / run-context / approval 剩余 handler 组到 typed service provider accessors，并继续保留 legacy `routerServices` reconstruction fallback。
- [x] P0.5-N168：继续按 N167 后边界收敛 planner_action_execution 剩余 blocker，评估并拆解 `authorization_context`：减少 `AgentLoopAuthorization` → daemon `[]agentHTTPAuthProfile` 回转适配，设计 runtime-native auth resolver / registry runtime context 最小迁移落点。
- [x] P0.5-N169：继续按 N168 后边界收敛 `authorization_context`：将 `executeAgentPlannerAction` / `executeAgentRegistryHandler` 签名改为直接传递 `runtime.AgentLoopAuthorization`，去掉 planner action execution side-effect boundary 中的 legacy auth profile slice 回转。
- [x] P0.5-N170：继续按 N169 后边界收敛 `executor_result_egress`：让 `executeAgentPlannerAction` 或 wrapper 直接返回 `runtime.AgentLoopActionExecutorResult`，减少 legacy `agentMessage` / `agentToolCall` egress adapter。
- [x] P0.5-N171：继续按 N170 后边界收敛 planner_action_execution legacy compatibility surface：迁移 direct `executeAgentPlannerAction` 测试/调用点到 `executeAgentPlannerActionResult` 或 executor seam，准备移除 legacy `[]agentMessage` / `[]agentToolCall` compatibility return。
- [x] P0.5-N172：继续按 N171 后边界收敛 registry handler result egress：将 `registryHandlerOutputToActionExecutorResult` 内部从 `registryHandlerOutputToMessages` legacy parser 迁到 runtime-native result construction，仅保留 legacy parser 作为 handler compatibility input adapter。
- [x] P0.5-N173：继续按 N172 后边界收敛 `tool_service_dispatch` / typed provider：评估 runtime-native action service interfaces，并先将 planner action side-effect boundary 从提前回转 `routerServices` 改为携带 runtime `AgentLoopToolServiceBinding`，把 legacy reconstruction 限制到 daemon legacy executor implementation fallback。
- [x] P0.5-N174：继续按 N173 后边界收敛 runtime action service interfaces：从 daemon-private `daemonAgentLoopToolServiceProvider` 抽取 registry/action handler 所需最小 runtime service capability contract，并逐步减少 `AgentLoopToolServiceBinding.Handle` opaque provider 依赖。
- [x] P0.5-N175：继续按 N174 后边界收敛 action service consumers：选择一组低风险 registry/action handler 从 `daemonAgentLoopToolServiceProvider` typed accessor 改为 runtime `AgentLoopActionServiceSet` capability resolver，保留 daemon provider fallback。
- [x] P0.5-N176：继续按 N175 后边界迁移下一批 action service consumers：将 schedule handler 组从 daemon-private provider accessors 改为 runtime capability resolver，并保持 typed provider fallback 覆盖。
- [x] P0.5-N177：继续按 N176 后边界迁移下一批 action service consumers：将 web.search / provider-store consumer 从 `daemonAgentLoopToolServiceProvider.ProviderStore()` 改为 runtime capability resolver，并保持 typed provider fallback 覆盖。
- [x] P0.5-N178：继续按 N177 后边界迁移 scan action service consumers：将 `scan.web.safe` / `JobManager` consumer 从 daemon-private provider accessors 改为 runtime capability resolver，并保持 typed provider fallback 覆盖。
- [x] P0.5-N179：继续按 N178 后边界迁移 run-context / approval action service consumers：将 `runs.context` / `reports.draft` / approval handlers 的 `RunService` / `ApprovalService` consumer 从 daemon-private provider accessors 改为 runtime capability resolver，并保持 typed provider fallback 覆盖。
- [x] P0.5-N180：继续按 N179 后边界收敛 action service compatibility fallback：审计剩余 `agentRegistryHandlerRuntime.toolServiceProvider()` / `legacyRouterServices()` 非测试引用，将 builtin registry handler 路径固化为 runtime capability-only，并明确下一步是否收敛 `Handle` / `daemonAgentLoopToolServiceProvider` fallback。
- [x] P0.5-N181：继续按 N180 后边界收敛 action service compatibility fallback：拆解剩余 `AgentLoopToolServiceBinding.Handle` / `daemonAgentLoopToolServiceProvider` fallback 非测试使用，优先评估 daemon legacy executor 的 `routerServices` reconstruction 是否可改为 runtime action service resolver / service bundle 直传。
- [x] P0.5-N182：继续按 N181 后边界收敛 daemon legacy executor / deterministic route 的 `routerServices` reconstruction：新增 `executeAgentPlannerActionResultWithToolServices` / `executeAgentRegistryHandlerResultWithToolServices` / `routePromptForTurnWithToolServices` 直传 runtime service binding，移除生产路径 `legacyRouterServicesFromAgentLoopToolServiceBinding` 调用。
- [x] P0.5-N183：继续按 N182 后边界收敛 explicit `Handle` / `daemonAgentLoopToolServiceProvider` / `routerServicesFromAgentLoopToolServiceBinding` compatibility helper：registry production runtime 固化为 ActionServices-only resolver，Handle fallback 仅保留在 explicit compatibility helper / 测试边界。
- [x] P0.5-N184：继续按 N183 后边界收敛剩余 provider/routerServices reconstruction helper：将 `routerServicesFromAgentLoopToolServiceBinding` / provider-from-binding / Handle compatibility reconstruction 移到 test-only adapter，并用源码扫描拒绝生产 runtime 文件保留这些 helper。
- [x] P0.5-N185：继续按 N184 后边界收敛历史 `daemonAgentLoopToolServiceProvider` typed-provider contract / compatibility 测试面，将 contract table/interface 与 provider contract metadata 从生产面移出，固化为 test-only fixture。
- [x] P0.5-N186：继续按 N185 后边界收敛 test-only explicit Handle / provider / routerServices roundtrip assertions，评估哪些兼容测试可改为 ActionServices-only 或退休，进一步压缩 legacy fixture 面。
- [x] P0.5-N187：继续按 N186 后边界收敛最后的 test-only `routerServicesFromAgentLoopToolServiceBinding` roundtrip helper 调用点，优先将 loop runner / turn executor 测试改为直接断言 `ActionServices` capabilities，评估 helper 是否可进一步缩小或删除。
- [x] P0.5-N188：继续按 N187 后边界收敛 runtime action service contract 的剩余 daemon-private `routerServices` ingress：删除 `executeAgentPlannerActionResult` / `executeAgentRegistryHandlerResult` legacy `routerServices` ingress wrappers，将 direct registry dispatch 测试/调用面改为 `*WithToolServices` runtime binding 入口。
- [x] P0.5-N189：继续按 N188 后边界审计剩余生产 `routerServices` ingress：删除 heuristic route / `routePromptForTurn` / deterministic route legacy `routerServices` wrappers，生产 route caller 改为 `*WithToolServices` runtime binding 入口。
- [x] P0.5-N190：继续按 N189 后边界审计剩余生产 `routerServices` ingress：将 `agentLLMToolLoopRequestFromLegacy` / `agentReplanActionContinuationRequestFromLegacy` / `agentPlannerActionReplanCycleToolContextFromRouterServices` 迁为 `*WithToolServices` runtime binding variants，并删除 legacy `routerServices` ingress。
- [x] P0.5-N191：继续按 N190 后边界收敛 planner action list compatibility wrapper：将 test-facing `executeAgentPlannerActions` 迁为 `executeAgentPlannerActionsWithToolServices`，删除剩余 planner action list legacy `routerServices` ingress wrapper。
- [x] P0.5-N192：继续按 N191 后边界审计剩余生产 `routerServices` ingress，优先评估 `handleAgentActiveRunContinueForTurn` 是否可改为 `*WithToolServices` + runtime `RunService` capability resolver，减少 active-run continue 对 daemon `routerServices` 的直接依赖。
- [x] P0.5-N193：继续按 N192 后边界审计剩余生产 `routerServices` ingress，优先评估 `ControlSession` / `DecideAgentAuthorization` / `runAgentSendMessageTurn` 是否可拆出更窄 runtime service binding 或 ActionServices-only 子入口，同时保留 HTTP router handler 对 `routerServices` 的边界职责。
- [x] P0.5-N194：继续按 N193 后边界收敛 `SendMessage` 顶层入口：评估是否新增 `SendMessageWithToolServices`，让 HTTP/router wrapper 独占 `routerServices` → runtime binding 转换，内部 lifecycle / turn dispatch 全程消费 runtime `AgentLoopToolServiceBinding`。
- [x] P0.5-N195：对本轮 `routerServices` ingress retirement 做 completion audit：用 source audit 固化 `agentManager` 只保留 `ControlSession` / `DecideAgentAuthorization` / `SendMessage` 三个 HTTP-facing wrapper，盘点剩余 `routerServices` 是否仅属于 router/server wiring 与 binding adapter，并据此决定下一批 engine-owned loop runner / completion audit 迁移点。
- [x] P0.5-N196：回到 engine-owned loop runner completion audit：盘点 `daemonAgentLoopRunner` / `runRuntimeAgentLoopResult` / shared cycle 中仍由 daemon adapter 持有的 append/persist、tool synthesis、action execution 与 fallback ownership，选择下一处可下沉到 runtime seam 的最小边界。
- [x] P0.5-N197：继续按 N196 选定边界下沉 shared cycle append/persist：将 `planner_action_replan_cycle` 中 action / warning / synthesis append placement 抽为 runtime-owned decision/egress helper，daemon adapter 仅保留 `SessionMutator` dispatch/fallback 与 legacy DTO 转换。
- [x] P0.5-N198：继续按 N197 后边界收敛 engine-owned loop runner fallback ownership：审计 `runRuntimeAgentLLMStage` / `runRuntimeAgentLLMToolSynthesis` 的 daemon legacy fallback reason/owner，并优先固化 planner_action_replan_cycle tool_synthesis fallback 的兼容边界。
- [x] P0.5-N199：继续按 N198 后边界收敛 engine-owned loop runner fallback ownership：审计 `runRuntimeAgentLoopResult` / `runDaemonAgentLoopDirectResult` 的 direct daemon fallback reason/owner，优先固化 AgentLoopRunner 未挂接与 `ErrAgentLoopRunnerNotAttached` 的兼容边界。
- [x] P0.5-N200：继续按 N199 后边界收敛 engine-owned loop runner action execution fallback ownership：审计 `planner_action_execution` runner branch 内 `ActionExecutor` / `TurnExecutor` / direct legacy executor fallback chain 的 reason/owner 与兼容边界，优先固化 action executor 未挂接与 `ErrAgentLoopActionExecutorNotAttached` 的兼容语义。
- [x] P0.5-N201：继续按 N200 后边界收敛 planner_action_execution runtime action service ownership：审计 direct legacy executor / registry dispatch 中剩余 daemon-private action service side effects，优先选择可下沉为 runtime-native action service interface 的最小 capability。
- [x] P0.5-N202：继续按 N201 选定边界下沉 workspace action service capability：为 workspace registry handlers 抽取 runtime `AgentLoopActionServiceWorkspace` capability 与 daemon adapter，减少 direct `rt.Manager.handleAgentWorkspace*` / `handleAgentCommand` side effects。
- [x] P0.5-N203：继续按 N202 后边界下沉 network request action service capability：为 `http.request` / `web.fetch` 抽取 runtime network request capability 与 daemon adapter，减少 direct `rt.Manager.handleAgentHTTPRequest` / `handleAgentWebFetch` side effects。
- [x] P0.5-N204：继续按 N203 后边界下沉 port scanner action service capability：为 `ports.scan.safe` 抽取 runtime port scanner capability 与 daemon adapter，减少 direct `rt.Manager.handleAgentPortScan` side effect。
- [x] P0.5-N205：继续按 N204 后 completion audit 结果收敛剩余 registry runtime capability consumers：审计 `scan.web.safe`、`daemon.health` / `plugins.list` / `runs.list`、`schedule.*`、`web.search`、run-context/approval 组的 direct `rt.Manager.handleAgent*` orchestration 调用，选择下一处可拆为更窄 runtime action service adapter 的最小边界。
- [x] P0.5-N206：继续按 N205 选定边界下沉 web search action service capability：为 `web.search` 抽取 runtime web search capability 与 daemon adapter，减少 direct `rt.Manager.handleAgentWebSearch` orchestration 调用。
- [x] P0.5-N207：继续按 N206 后选定边界下沉 web scan action service capability：为 `scan.web.safe` 抽取 runtime web scan capability 与 daemon adapter，减少 direct `rt.Manager.handleAgentScan` orchestration 调用。
- [x] P0.5-N208：继续按 N207 后选定边界下沉 schedule action service capability：为 `schedule.create` / `schedule.list` / `schedule.delete` 抽取 runtime schedule action capability 与 daemon adapter，减少 direct `rt.Manager.handleAgentSchedule*` orchestration 调用。
- [x] P0.5-N209：继续按 N208 后选定边界下沉 daemon status action service capability：为 `daemon.health` / `plugins.list` / `runs.list` 抽取 runtime daemon status action capability 与 daemon adapter，减少 direct `rt.Manager.handleAgentHealth` / `handleAgentPlugins` / `handleAgentRuns` orchestration 调用。
- [x] P0.5-N210：继续按 N209 后选定边界下沉 run-context/approval action service capability：为 `runs.context` / `reports.draft` / `runs.approvals.list` / `runs.approvals.approve` / `runs.approvals.deny` / `runs.resume_approved` 抽取 runtime run-context/approval action capability 与 daemon adapter，减少 direct `rt.Manager.handleAgentRunContextTool` / `handleAgentReportDraft` / `handleAgentApprovals` / `handleAgentRunContextAnalysis` / `handleAgentResumeApproved` / `handleAgentApprovalDecision` orchestration 调用。
- [x] P0.5-N211：继续按 N210 后 completion audit 结果收敛 planner action executor fallback：审计 registry action-service adapter 迁移完成状态，确认 `agent_registry_handlers.go` 已无 direct `rt.Manager.handleAgent*` orchestration 调用，并选择下一处可收敛的 `ActionExecutor` / `TurnExecutor` / direct legacy executor fallback retirement 边界。
- [x] P0.5-N212：继续按 N211 后选定边界收敛 planner action execution TurnExecutor compatibility fallback：审计并迁移仍依赖 `runRuntimePlannerActionExecutionTurn` 的 planner action execution fallback/caller 测试，评估是否可让 `ErrAgentLoopActionExecutorNotAttached` 直接进入 direct legacy executor 或更窄 runtime action executor adapter fallback。
- [x] P0.5-N213：继续按 N212 后选定边界收敛 planner action execution direct legacy executor fallback：审计 `daemonAgentLoopActionExecutor.ExecuteAgentLoopAction` / `daemonPlannerActionExecutionExecutor` 兼容 fallback 的调用面、side-effect ownership 与 runtime action service adapter 覆盖，评估是否可让 runtime `AgentLoopActionExecutor` 成为唯一 planner action execution 执行入口或进一步缩窄 direct legacy fallback。
- [x] P0.5-N214：继续按 N213 后选定边界收敛 planner action execution missing/detached ActionExecutor fallback：审计 `runAgentPlannerActionExecutionAgentLoopResult` 在 runtime engine / `AgentLoopActionExecutor` 缺失或 `ErrAgentLoopActionExecutorNotAttached` 时直接调用 `daemonAgentLoopActionExecutor` 的兼容语义；attached-engine missing/detached ActionExecutor 已转为 blocked/error，仅保留 no-engine AgentLoop direct fallback。
- [x] P0.5-N215：继续按 N214 后选定边界收敛 planner action execution no-engine AgentLoop direct fallback：审计无 runtime.Engine 时 `runRuntimeAgentLoopResult` / `runAgentPlannerActionExecutionAgentLoopResult` 进入 `daemonAgentLoopActionExecutor` 的兼容调用面；确认 production `NewServer` 默认先 `AttachEngine`，但 no-engine direct fallback 仍需为显式 `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0` 回退开关保留。
- [x] P0.5-N216：继续按 N215 后选定边界收敛 attached-engine AgentLoopRunner missing/detached fallback：审计 `runRuntimeAgentLoopResult` 在 `Engine.AgentLoopRunner()==nil` 或 `ErrAgentLoopRunnerNotAttached` 时进入 `runDaemonAgentLoopDirectResult` 的兼容语义；attached-engine missing/detached AgentLoopRunner 已转为 blocked/error，同时保留 no-engine legacy opt-out fallback。
- [x] P0.5-N217：继续按 N216 后选定边界收敛 attached-engine LLMStageRunner missing/detached fallback：审计 `runRuntimeAgentLLMStage` 在 `Engine.LLMStageRunner()==nil` 或 `ErrLLMStageRunnerNotAttached` 时进入 legacy planner_chat/tool_synthesis fallback 的兼容语义；attached-engine missing/detached LLMStageRunner 已转为 blocked/error，同时保留 no-engine legacy opt-out fallback。
- [x] P0.5-N218：继续按 N217 后选定边界收敛 deprecated LLMTurnRunner compatibility trampoline：审计 `runtime.Engine.RunLLMTurn` / `daemonAgentLLMTurnRunner.RunLLMTurn` 的非测试调用面与 compatibility audit；确认 LLMTurnRunner 已缩窄为 external engine seam only，内部 LLM tool loop 主路径保持 `Engine.RunAgentLoop` / `AgentLoopRunner`，默认 `AttachLLMTurnRunner` 仅为 deprecated API 兼容保留。
- [x] P0.5-N219：继续按 N218 后选定边界收敛 TurnExecutor compatibility trampolines：审计 `runtime.Engine.ExecuteTurn` / `daemonAgentTurnExecutor` 中 deterministic_route 与 replan_action_continuation 的非测试调用面、compatibility audit 与默认 attachment 语义，确认 deterministic/replan TurnExecutor trampoline 已缩窄为 external engine seam only，默认 `AttachTurnExecutor` 仅为 `ExecuteTurn` 外部兼容保留，内部 deterministic/replan 主路径保持 `Engine.RunAgentLoop` / `AgentLoopRunner`。
- [x] P0.5-N220：继续按 N219 后选定边界收敛 planner_action_execution TurnExecutor explicit compatibility trampoline：审计 `daemonAgentTurnExecutor.ExecuteTurn(kind=planner_action_execution)` 的非测试调用面、compatibility audit 与默认 attachment 语义，确认 planner_action_execution TurnExecutor branch 也缩窄为 external engine seam only，内部 planner action execution 主路径保持 `Engine.RunAgentLoop` / `AgentLoopRunner` 并通过 `Engine.ExecuteAgentLoopAction` / `ActionExecutor` 执行 action。
- [x] P0.5-N221：继续按 N220 后选定边界审计 `runtime.Engine.ExecuteTurn` / `TurnExecutor` API 整体 external compatibility removal gate：确认 deterministic_route、replan_action_continuation、planner_action_execution 三个 branch 均已 external-only 后，评估是否可标记 Deprecated、拆出更窄 compatibility seam，或将默认 `AttachTurnExecutor` 转入删除候选计划。
- [x] P0.5-N222：继续按 N221 后选定边界统一 deprecated `RunLLMTurn` / `ExecuteTurn` runtime API policy：文档化外部 migration window，评估 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认兼容挂接是否转 opt-in，或保持到 release gate。
- [x] P0.5-N223：继续按 N222 后选定边界补 deprecated runtime API release/API docs 与 opt-in compatibility flag audit：将 `RunLLMTurn` / `ExecuteTurn` 迁移窗口、替代 API 与默认 attachment 保留策略同步到发布说明/API 文档，评估是否新增 opt-in compatibility flag 或继续保持默认挂接到 release gate。
- [x] P0.5-N224：继续按 N223 后选定边界推进 post-release-gate deprecated runtime API opt-in compatibility flag decision：在 release/API docs 已覆盖后，评估是否为 `AttachLLMTurnRunner` / `AttachTurnExecutor` 增加显式 opt-in compatibility flag，或继续保持默认挂接到下一 release gate；已固化配置/API 边界与回归审计，不改变当前默认行为。
- [x] P0.5-N225：继续按 N224 后选定边界推进 next-release-gate deprecated runtime API opt-in compatibility flag reassessment：在 current release gate 完成后，再评估是否为 `AttachLLMTurnRunner` / `AttachTurnExecutor` 引入显式 opt-in compatibility flag、继续默认挂接，或进入更窄 removal/readiness 计划；已确认 release gate 完成证据前保持 N224 默认行为不变。
- [x] P0.5-N226：继续按 N225 后选定边界推进 current release gate completion evidence / release readiness audit：收集并固化 release checklist、API/release docs、source audit、回归测试与当前工作树状态证据，确认 deprecated runtime API compatibility policy 是否具备进入下一 release gate flag/reduction 决策的前置条件；已新增 release-readiness evidence audit，确认本切片不改变 runtime 默认行为。
- [x] P0.5-N227：继续按 N226 后选定边界推进 release candidate stabilization and deprecated runtime API flag/reduction decision：基于已固化的 release evidence、全量回归与 dirty working tree 状态，先稳定/归档当前 release-candidate diff，再决定是否进入 deprecated runtime API opt-in flag、默认 attachment reduction 或继续保持兼容默认挂接；已新增 release-candidate flag/reduction decision audit，当前 rolling diff 尚未 tag-ready，因此不新增 dedicated opt-in flag，不减少默认 attachment。
- [x] P0.5-N228：继续按 N227 后选定边界推进 release_candidate_diff_archive_and_tag_readiness_audit：归档当前 release-candidate diff inventory，明确 tracked/untracked 变更清单、review/stage/commit/tag-readiness gate 与 deprecated runtime API 默认挂接保持策略，之后再重新评估 opt-in flag 或 attachment reduction；已新增 `docs/CyberAgent-v0.1.2-Release-Candidate-Diff-Inventory.md` 与 archive/tag-readiness audit，tag-ready 仍阻塞。
- [x] P0.5-N229：继续按 N228 后选定边界推进 release_candidate_stage_commit_and_full_regression_gate：基于已归档 diff inventory，核对 intended stage/commit set、确认 untracked audit/adapters 是否全部纳入 release candidate，并在最终 staging 前后 rerun full release regression；未通过该 gate 前继续禁止 deprecated runtime API opt-in flag/default attachment reduction；已新增 stage/commit/full-regression gate audit，验证通过但未执行 git add/commit/tag。
- [x] P0.5-N230：继续按 N229 后选定边界推进 manual_stage_commit_and_tag_authorization：等待用户明确授权后，才允许执行 git add / git commit / git tag 或任何 deprecated runtime API 默认 attachment reduction；未获授权前只保留审计结果与兼容默认行为。已新增 manual authorization gate audit，当前边界为 `await_explicit_user_stage_commit_tag_authorization`；本轮不执行 git add / git commit / git tag，不新增 dedicated deprecated runtime API opt-in flag，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接。
- [x] P0.5-N231：继续在未获 git/tag 授权前推进 engine-owned loop runner completion audit：将 nested `planner_action_replan_cycle` dispatch 改为复用 `runRuntimeAgentLoopResult`，使 attached-engine `AgentLoopRunner` missing/detached 或 hard error 不再绕过统一 fallback/blocked 语义直接落回 daemon cycle；同步更新 completion audit 的下一边界为 `planner_action_replan_cycle_nested_dispatch_fallback`，发布 stage/commit/tag 与 deprecated runtime API 默认行为仍等待用户明确授权。
- [x] P0.5-N232：继续按 N231 后边界做 nested `planner_action_replan_cycle` source audit：固化生产 outer LLM/replan continuation 只经 `runRuntimeAgentPlannerActionReplanCycleAgentLoop` → `runRuntimeAgentLoopResult` 进入 cycle，direct `runAgentPlannerActionReplanCycleAgentLoop` 仅保留在 AgentLoopRunner branch、no-engine direct fallback 与 legacy wrapper 边界；completion audit 下一边界推进到 `shared_cycle_session_mutation_dispatch_fallback`，发布 stage/commit/tag 与 deprecated runtime API 默认行为仍等待用户明确授权。
- [x] P0.5-N233：继续按 N232 后边界审计 shared-cycle append SessionMutation dispatch/fallback：固化 runtime `AgentLoopReplanCycleApply*Append` 只拥有 append placement/state egress，daemon `appendAgentPlannerActionReplanCycleDecisionForTurn` 仅负责把 runtime append decision 派发到 `appendMessages*ForTurn`，而 `appendMessagesWithPlannerForTurn` 仍经 `runRuntimeAppendMessagesMutationForTurn` 后保留 legacy append fallback；completion audit 下一边界推进到 `shared_cycle_attached_session_mutator_fallback_retirement_readiness`，发布 stage/commit/tag 与 deprecated runtime API 默认行为仍等待用户明确授权。
- [x] P0.5-N234：继续按 N233 后边界收敛 attached-engine SessionMutator missing/detached append fallback：审计并固化 no-engine SessionMutation legacy fallback 仅为 `CYBERAGENT_RUNTIME` legacy opt-out 保留，attached-engine `SessionMutator()==nil` 或 `ErrSessionMutatorNotAttached` 规范化为 blocked/error，shared-cycle append 不再落回 `appendMessagesWithPlannerLegacyForTurn`；completion audit 下一边界推进到 `session_mutation_blocked_or_error_fallback_retirement_readiness`，发布 stage/commit/tag 与 deprecated runtime API 默认行为仍等待用户明确授权。
- [x] P0.5-N235：继续按 N234 后边界收敛 attached SessionMutator blocked/hard-error append fallback：审计并固化 no-engine append legacy fallback 仅为 `CYBERAGENT_RUNTIME` legacy opt-out 保留，attached `SessionMutator` 返回 generic `TurnResultBlocked` 或 hard error 时 `appendMessagesWithPlannerForTurn` 不再 silent legacy append；completion audit 下一边界推进到 `session_mutation_optional_result_fallback_retirement_readiness`，发布 stage/commit/tag 与 deprecated runtime API 默认行为仍等待用户明确授权。
- [x] P0.5-N236：继续按 N235 后边界收敛 request_authorization / capability_blocked optional-result SessionMutation fallback：审计并固化 no-engine optional-result legacy fallback 仅为 `CYBERAGENT_RUNTIME` legacy opt-out 保留，attached `SessionMutator` 返回 blocked、hard error 或 invalid payload 时不再让 request/capability caller 落回 legacy session state write；completion audit 下一边界推进到 `session_mutation_no_engine_legacy_fallback_retirement_readiness`，发布 stage/commit/tag 与 deprecated runtime API 默认行为仍等待用户明确授权。
- [x] P0.5-N237：继续按 N236 后边界审计剩余 no-engine SessionMutation legacy fallback：确认 production default runtime engine attached，append_messages / record_user_message / request_authorization / capability_blocked / control_session / resolve_authorization 六类 no-engine SessionMutation caller fallback 仅作为 `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0` whole-engine rollback switch 保留；completion audit 下一边界回到 `await_explicit_user_stage_commit_tag_authorization`，发布 stage/commit/tag 与 deprecated runtime API 默认行为仍等待用户明确授权。
- [x] P0.5-N238：完成 P0.5 engine-owned loop runner / runtime seam 总括项 completion audit：将旧总括 unchecked 项与 N105-N237 工件、completion audit table、no-engine SessionMutation fallback audit、default attachments 与验证命令对齐；确认当前 P0.5 已无可继续自动推进的非 git runtime seam hardening 子项，剩余边界为 `await_explicit_user_stage_commit_tag_authorization`。
- [x] P0.5-N239：继续按 engine-native turn lifecycle 下沉边界删除已无生产调用的 daemon `beginRuntimeTurn` / `finishRuntimeTurn` compatibility adapters；测试改为直接覆盖 `runRuntimeTurnLifecycle` terminal event parity，不改变 `SendMessage` bounded lifecycle 语义。

当前阻塞项：

- 等待用户明确授权是否执行 `git add` / `git commit` / `git tag`，以及是否进入任何后续 deprecated runtime API 默认 attachment reduction 设计；在授权前不执行 staging/commit/tag，也不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接，非 git 的 runtime seam hardening 仍按 TODO 小步推进。

当前盘点（2026-06-09）：

- 已经走 engine seam：`record_user_message`、`append_messages`、`control_session`、`resolve_authorization`、`request_authorization`、`capability_blocked`；deterministic route 已走 `AgentLoopModeDeterministicRoute`，planner action execution 主 caller 已走 `Engine.RunAgentLoop` / `AgentLoopModePlannerActionExecution`（runner branch 内部优先 `Engine.ExecuteAgentLoopAction`；attached-engine ActionExecutor missing / `ErrAgentLoopActionExecutorNotAttached` 已绕过内部 TurnExecutor compatibility hop 并转为 blocked/error，仅 runtime.Engine 缺失时保留 no-engine `daemonAgentLoopActionExecutor.ExecuteAgentLoopAction` direct fallback，外部 `daemonAgentTurnExecutor.ExecuteTurn(kind=planner_action_execution)` 仅作为 explicit compatibility trampoline 暂留；ActionExecutorResult / legacy TurnPayload → AgentLoopResult composition 已下沉到 runtime helper；`daemonPlannerActionExecutionExecutor` seam callers 现在消费 `runtime.AgentLoopActionExecutorResult`；`AgentLoopToolServiceBinding.Handle` 已从直接携带 `routerServices` 收敛为 explicit compatibility input，legacy `legacyRouterServicesFromAgentLoopToolServiceBinding` helper 已移除，planner action executor 与 deterministic route AgentLoop 分支会把 runtime `AgentLoopToolServiceBinding` 直传到 registry dispatch；builtin registry handlers 已脱离 `legacyRouterServices()`，其中 `daemon.health` / `plugins.list` / `runs.list`、`schedule.*`、`web.search`、`scan.web.safe`、run-context/approval 组与 workspace registry handlers 已进一步切到 `agentRegistryHandlerRuntime.actionServiceResolver()` runtime capability 路径，其中 workspace 组通过 `AgentLoopActionServiceWorkspace` 与 daemon workspace adapter 承接 side effects，network request 组通过 `AgentLoopActionServiceNetworkRequest` 与 daemon network request adapter 承接 `http.request` / `web.fetch` side effects，port scanner 组通过 `AgentLoopActionServicePortScanner` 与 daemon port scanner adapter 承接 `ports.scan.safe` side effect，web search 组通过 `AgentLoopActionServiceWebSearch` 与 daemon web search adapter 承接 `web.search` side effect，web scan 组通过 `AgentLoopActionServiceWebScan` 与 daemon web scan adapter 承接 `scan.web.safe` side effect，schedule 组通过 `AgentLoopActionServiceScheduleAction` 与 daemon schedule action adapter 承接 `schedule.create` / `schedule.list` / `schedule.delete` orchestration，daemon status 组通过 `AgentLoopActionServiceDaemonStatusAction` 与 daemon status action adapter 承接 `daemon.health` / `plugins.list` / `runs.list` orchestration，run-context/approval 组通过 `AgentLoopActionServiceRunContextApproval` 与 daemon run-context approval action adapter 承接 `runs.context` / `reports.draft` / `runs.approvals.*` / `runs.resume_approved` orchestration；production `agentRegistryHandlerRuntime` 已不再暴露 `toolServiceProvider()` / `legacyRouterServices()` helper，且 `actionServiceResolver()` 已固化为 `ActionServices`-only，不再经 `AgentLoopToolServiceBinding.Handle` fallback；explicit Handle/provider/routerServices reconstruction helper 已从生产面与测试面移除，loop runner / turn executor 等测试已改为直接断言 runtime `ActionServices` capabilities，`agent_tool_service_binding_compat_test.go` 仅保留迁移说明注释，生产 runtime 文件不再保留 provider/routerServices reconstruction helper）；planner chat 与 tool synthesis stage（P0.5-N198 已新增 LLMStage caller fallback ownership/boundary 表；P0.5-N217 已将 attached-engine LLMStageRunner missing/detached 转为 blocked/error，legacy `handleAgentLLMChat` / `synthesizeAgentToolResults` fallback 仅保留给 no-engine legacy opt-out 路径）；`llm_tool_loop` / `replan_action_continuation` outer loop 现在经 `AgentLoopRunner` 接管，SendMessage LLM 主入口、deterministic route 与 follow-up continuation 已直接进入 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`，内部 Action → StepReplan 通过 `planner_action_replan_cycle` 兼容 sub-mode 执行；attached-engine AgentLoopRunner missing/detached 已转 blocked/error，仅 no-engine legacy opt-out 保留 direct daemon fallback；P0.5-N218 已确认 `RunLLMTurn` / `daemonAgentLLMTurnRunner` 无内部生产 caller，缩窄为 deprecated external engine seam only；`AttachLLMTurnRunner` 仅为外部兼容保留，LLM tool loop 主路径保持 `Engine.RunAgentLoop` / `AgentLoopRunner`；P0.5-N219 已确认 `ExecuteTurn` deterministic_route / replan_action_continuation 非测试直接调用面只剩 runtime API dispatch 与 `AttachEngine` 默认 compatibility attachment，内部 deterministic route 与 follow-up replan continuation 主路径均经 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`，两个 TurnExecutor branch 缩窄为 external engine seam only 且暂不进入 removal candidate；P0.5-N220 已将 planner_action_execution TurnExecutor explicit branch 纳入 active compatibility audit，确认内部 planner action execution 主路径经 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`，branch 本身仅为 external `ExecuteTurn` compatibility 并委托 `daemonAgentLoopActionExecutor.ExecuteAgentLoopAction`；P0.5-N221 已将 `runtime.Engine.ExecuteTurn`、`TurnExecutor`、`TurnExecutorFunc`、`AttachTurnExecutor`、`Engine.TurnExecutor` 标记为 Deprecated，并新增 API 级 retirement gate：三个 TurnExecutor branch 均为 external-only active compatibility，但默认 `AttachTurnExecutor` 仍保留，当前不进入 deletion candidate；P0.5-N222 已新增 unified deprecated runtime API policy：`RunLLMTurn` / `ExecuteTurn` 均为 Deprecated + external engine seam only，migration window 固化为 `current_release_gate`，默认 `AttachLLMTurnRunner` / `AttachTurnExecutor` 保持到 release gate，opt-in/removal 推迟到发布/API 文档补齐后；P0.5-N223 已将 deprecated runtime API migration window、replacement API、default attachment 与 opt-in deferred policy 同步到 release notes、API contract、release checklist 和 release plan，并新增 release-docs/opt-in audit，当前不新增 opt-in compatibility flag；下一步边界推进到 `post_release_gate_deprecated_runtime_api_opt_in_flag_decision`。P0.5-N224 已新增 post-release-gate opt-in flag decision audit，确认 current release gate 未完成前不引入显式 deprecated runtime API opt-in flag，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接，现有 `CYBERAGENT_RUNTIME` 仅作为 whole-engine rollback switch；下一步边界推进到 `next_release_gate_deprecated_runtime_api_opt_in_flag_reassessment`。P0.5-N225 已新增 next-release-gate reassessment audit，确认当前 release gate 完成证据未固化前不进入 opt-in flag/config/API 变更，继续保留默认 compatibility attachments；下一步边界推进到 `current_release_gate_completion_evidence_and_release_readiness_audit`。
- P0.5-N226 已新增 current release gate completion evidence / release readiness audit，固化 release notes/API contract、release checklist/plan、source audit、回归命令与 dirty working tree 状态证据；本切片只记录证据，不新增 dedicated deprecated runtime API opt-in flag，不改变 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。下一步边界推进到 `release_candidate_stabilization_and_deprecated_runtime_api_flag_reduction_decision`。
- P0.5-N227 已新增 release candidate stabilization and deprecated runtime API flag/reduction decision audit：接收 N226 release-readiness evidence，并记录当前 release-candidate diff inventory（`git status --short` 显示 48 个 tracked modified、32 个 untracked、合计 80 项；`git diff --stat` 当前 tracked diff 为 48 files changed, 11505 insertions(+), 1413 deletions(-)）。由于 rolling diff 尚未归档为 tag-ready 清单，本轮决定继续阻塞 dedicated opt-in flag / 默认 attachment reduction，不新增 `CYBERAGENT_RUNTIME_COMPAT` / `CYBERAGENT_DEPRECATED_RUNTIME_API`，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接。下一步边界推进到 `release_candidate_diff_archive_and_tag_readiness_audit`。
- P0.5-N228 已新增 release candidate diff archive / tag-readiness audit：`docs/CyberAgent-v0.1.2-Release-Candidate-Diff-Inventory.md` 归档了 N228 前快照命令、tracked modified paths、untracked paths、tracked diff stat 与 tag-readiness gate；`daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessTable` 固化结论：当前 release candidate 仍 not tag-ready，下一步必须进入 `release_candidate_stage_commit_and_full_regression_gate`，在 stage/commit/full regression 通过前继续禁止 dedicated opt-in flag / 默认 attachment reduction，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接。
- P0.5-N229 已新增 release candidate stage/commit/full-regression gate audit：接收 N228 diff archive，记录当前工作树仍 dirty/not tag-ready，且本轮未执行 `git add` / `git commit` / `git tag`；focused/full daemon tests、runtime package test、全量 Go、vet、frontend vitest/build、CLI build 与 diff whitespace check 通过后，仍需要用户明确授权进入 `manual_stage_commit_and_tag_authorization`。未获授权前继续禁止 dedicated opt-in flag / 默认 attachment reduction，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接。
- P0.5-N230 已新增 manual stage/commit/tag authorization gate audit：`daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationTable` 固化当前自动化工作已到 `await_explicit_user_stage_commit_tag_authorization`，不运行 `git add` / `git commit` / `git tag`，不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。剩余动作需要用户明确授权。
- P0.5-N231 已推进 engine-owned loop runner completion audit：`runRuntimeAgentPlannerActionReplanCycleAgentLoop` 现在复用 `runRuntimeAgentLoopResult`，nested `planner_action_replan_cycle` 与顶层 AgentLoop dispatch 共享 no-engine direct fallback、attached-engine missing/detached runner blocked、hard error 不 fallback 的语义；`daemonAgentLoopRunnerCompletionAuditTable` 的下一运行时边界从已完成的 action-service ownership 更新为 `planner_action_replan_cycle_nested_dispatch_fallback`。
- P0.5-N232 已新增 nested cycle dispatch source audit：`TestAgentManagerRunRuntimePlannerActionReplanCycleSourceAuditUsesAgentLoopFallbackPolicy` 读取生产源码，确认 `runRuntimeAgentPlannerActionReplanCycleAgentLoop` 只调用 `runRuntimeAgentLoopResult`、`daemonAgentLoopNestedDispatchErrorResult` 与 normalization，不直接调用 `runAgentPlannerActionReplanCycleAgentLoop` / `runDaemonAgentLoopDirectResult` / `runtimeEngine()`；outer LLM/replan continuation 的两个 `cycleReq` caller 继续走 runtime dispatch，direct daemon cycle branch 仅保留在 `daemonAgentLoopRunner.RunAgentLoop` 与 no-engine `runDaemonAgentLoopDirectResult` scope。completion audit 下一运行时边界推进到 `shared_cycle_session_mutation_dispatch_fallback`。
- P0.5-N233 已新增 shared-cycle SessionMutation fallback audit：`daemonSharedCycleSessionMutationFallbackTable` 将 runtime append decision、daemon append mutation dispatch、legacy append fallback 与下一边界拆开；`TestDaemonSharedCycleSessionMutationFallbackSourceAuditScopesAppendFallback` 读取生产源码确认 `appendAgentPlannerActionReplanCycleDecisionForTurn` 不直接调用 legacy fallback / `runRuntimeSessionMutation`，而是经 `appendMessages*ForTurn` 进入 `runRuntimeAppendMessagesMutationForTurn`，失败或未处理时才由 append helper fallback 到 `appendMessagesWithPlannerLegacyForTurn`。completion audit 下一运行时边界推进到 `shared_cycle_attached_session_mutator_fallback_retirement_readiness`。
- P0.5-N234 已新增 attached SessionMutator not-attached retirement audit：`daemonSessionMutatorNotAttachedRetirementTable` 区分 no-engine legacy opt-out fallback、attached-engine missing mutator blocked、detached mutator result blocked 与下一边界；`runRuntimeSessionMutation` 现在对 attached-engine `SessionMutator()==nil` / `ErrSessionMutatorNotAttached` 返回 blocked/error，`appendMessagesWithPlannerForTurn` 的 append caller 将该类 egress 视为已处理以阻止 legacy append fallback；no-engine manager 仍保留 legacy append fallback。completion audit 下一运行时边界推进到 `session_mutation_blocked_or_error_fallback_retirement_readiness`。
- P0.5-N235 已新增 append SessionMutation blocked/error retirement audit：`daemonAppendSessionMutationBlockedOrErrorRetirementTable` 区分 no-engine append legacy opt-out fallback、attached SessionMutator blocked result、attached SessionMutator hard error 与下一边界；`daemonSessionMutationCallerHandledForKind` 现在让 append bool-only caller 在 attached mutator blocked/hard-error egress 下视为 handled，从而阻止 `appendMessagesWithPlannerForTurn` 落回 legacy append；no-engine manager 仍保留 legacy append fallback。completion audit 下一运行时边界推进到 `session_mutation_optional_result_fallback_retirement_readiness`。
- P0.5-N236 已新增 optional-result SessionMutation fallback retirement audit：`daemonOptionalSessionMutationFallbackRetirementTable` 区分 request_authorization/capability_blocked no-engine legacy opt-out fallback、attached mutator failure suppression 与下一边界；`daemonSessionMutationCallerOptionalResultPayloadForKind` 现在让 optional-result caller 在 attached mutator blocked/hard-error/invalid-payload egress 下视为 handled，从而阻止 request_authorization / capability_blocked 落回 legacy session state write；no-engine manager 仍保留 legacy fallback。completion audit 下一运行时边界推进到 `session_mutation_no_engine_legacy_fallback_retirement_readiness`。
- P0.5-N237 已新增 no-engine SessionMutation fallback retirement audit：`daemonNoEngineSessionMutationFallbackRetirementTable` 覆盖 production default engine attach、legacy runtime opt-out switch、append_messages / record_user_message / request_authorization / capability_blocked / control_session / resolve_authorization 六类 no-engine fallback 与 selected boundary；结论是 no-engine fallback 不退休，继续仅作为 `CYBERAGENT_RUNTIME` whole-engine rollback path 保留，默认 `NewServer` 仍 attach runtime engine + `daemonSessionMutator`。completion audit 下一运行时边界已回到 `await_explicit_user_stage_commit_tag_authorization`。本轮不执行 `git add` / `git commit` / `git tag`，不新增 `CYBERAGENT_RUNTIME_COMPAT` / `CYBERAGENT_DEPRECATED_RUNTIME_API`，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。
- P0.5-N238 已完成 P0.5 总括项 completion audit：逐项对齐 objective deliverables（engine-owned loop runner 切换、completion audit、剩余 side-effect ownership、SessionMutation fallback、manual stage/commit/tag gate）与实际工件；`daemonAgentLoopRunnerCompletionAuditTable` 五个 mode 的 next seam 均为 `await_explicit_user_stage_commit_tag_authorization`，旧 shared-cycle/action-service/session-mutation fallback seam entries 均为空；`daemonNoEngineSessionMutationFallbackRetirementTable` 明确 no-engine fallback 仅为 `CYBERAGENT_RUNTIME` whole-engine rollback 保留；`agent_runtime.go` 仍默认挂接 `AttachLLMTurnRunner` / `AttachTurnExecutor` / `AttachAgentLoopRunner` / `AttachAgentLoopActionExecutor` / `AttachSessionMutator`。因此 P0.5 runtime seam hardening 当前已闭合到 manual authorization gate，未获授权前不执行 git stage/commit/tag，也不做默认 attachment reduction。
- planner_action_execution 的 `authorization_context` 已在 P0.5-N169 解除 blocker：side-effect boundary 不再回转成 legacy `[]agentHTTPAuthProfile`；当前 runtime path 由 `daemonPlannerActionExecutionSideEffectBoundary`、`executeAgentPlannerActionResultWithToolServices` 与 `executeAgentRegistryHandlerResultWithToolServices` 端到端传递 `runtime.AgentLoopAuthorization`。`executor_result_egress` 已在 P0.5-N170 通过 runtime result wrapper 收敛；P0.5-N171 已迁移 direct registry dispatch 测试到 runtime result wrapper，并移除 legacy `executeAgentPlannerAction` / `executeAgentRegistryHandler` 的 `[]agentMessage` / `[]agentToolCall` compatibility return；P0.5-N172 已将 `registryHandlerOutputToActionExecutorResult` 内部迁到 runtime-native `AgentLoopMessage` / `AgentLoopToolCall` result construction，并保留 `registryHandlerOutputToMessages` 作为 legacy compatibility adapter；P0.5-N173 已将 planner action side-effect boundary 改为 clone/preserve runtime `AgentLoopToolServiceBinding`，legacy `routerServices` reconstruction 仅保留在 `daemonLegacyPlannerActionExecutionExecutor` implementation fallback 内；P0.5-N174 已新增 runtime `AgentLoopActionServiceSet` / capability names，并让 daemon binding 优先通过 `ActionServices` 解析服务，opaque `Handle` 仅作为 compatibility fallback；P0.5-N175 已将 `daemon.health` / `plugins.list` / `runs.list` 第一批低风险 registry consumers 改为 `actionServiceResolver()`；P0.5-N176 已将 `schedule.create` / `schedule.list` / `schedule.delete` schedule 组改为 runtime `ScheduleManager` capability resolver；P0.5-N177 已将 `web.search` / provider-store consumer 改为 runtime `ProviderStore` capability resolver；P0.5-N178 已将 `scan.web.safe` / JobManager consumer 改为 runtime `JobManager` capability resolver；P0.5-N179 已将 `runs.context` / `reports.draft` / approval handler 组改为 runtime `RunService` / `ApprovalService` capability resolver；P0.5-N180 已移除 production `agentRegistryHandlerRuntime.toolServiceProvider()` / `legacyRouterServices()` helper 与 cycle tool context legacy helper，并用源码扫描测试固化 builtin registry handler runtime capability-only 路径；P0.5-N181 已让 `agentLoopToolServiceBindingFromRouterServices` 默认产出 ActionServices-only / nil `Handle` binding，provider/routerServices reconstruction 仍可从 runtime capabilities 完成，explicit `Handle` 仅保留为 compatibility input；P0.5-N182 已让 daemon legacy executor 与 deterministic route 通过 `*WithToolServices` 直传 runtime binding，移除生产/测试对 `legacyRouterServicesFromAgentLoopToolServiceBinding` helper 的依赖并删除该 helper；P0.5-N183 已将 registry production runtime 的 service resolver 固化为 `AgentLoopActionServiceSet` capability-only，Handle-only binding 会被拒绝；P0.5-N184 已将 explicit Handle/provider/routerServices reconstruction helper 从 production `agent_tool_service_binding.go` 移到 test-only `agent_tool_service_binding_compat_test.go`，并用源码扫描确保所有非测试 daemon `.go` 文件不再保留这些 helper；P0.5-N185 已将历史 typed-provider contract table/interface 同步移入 test-only fixture，production binding metadata 不再暴露 `provider_contract` / `provider_adapter`，源码扫描也拒绝 production daemon 文件重新出现 provider contract 名称或 provider metadata；P0.5-N186 已退休 test-only typed-provider / explicit Handle fixture 与 provider reconstruction assertions；P0.5-N187 已将剩余 test-only `routerServicesFromAgentLoopToolServiceBinding` roundtrip 调用改为 direct `ActionServices` capability assertions，并删除 helper implementation；P0.5-N188 已删除 planner action executor / registry dispatch 的 legacy `routerServices` ingress wrappers，direct tests/callers 改为 `executeAgentPlannerActionResultWithToolServices` / `executeAgentRegistryHandlerResultWithToolServices`；P0.5-N189 已将 deterministic/heuristic route direct wrappers 切到 runtime `AgentLoopToolServiceBinding`，删除 `routePrompt` / `routePromptForTurn` / `routeHeuristicRegistryActions` / `runRuntimeDeterministicRoute` / `routePromptWithRuntimeDeterministicRoute` 的 legacy `routerServices` ingress wrappers；P0.5-N190 已将 LLM/replan request adapter 迁为 `agentLLMToolLoopRequestFromLegacyWithToolServices` / `agentReplanActionContinuationRequestFromLegacyWithToolServices` / `agentPlannerActionReplanCycleToolContextFromToolServices`，删除对应 legacy `routerServices` ingress；P0.5-N191 已将 planner action list wrapper 迁为 `executeAgentPlannerActionsWithToolServices`；P0.5-N192 已将 active-run continue helper 迁为 `handleAgentActiveRunContinueForTurnWithToolServices`，通过 runtime `AgentLoopActionServiceSet` 的 `RunService` capability resolver 读取 run context，并继续直传 runtime `AgentLoopToolServiceBinding` 到 deterministic route；P0.5-N193 已新增 `ControlSessionWithToolServices` / `DecideAgentAuthorizationWithToolServices`，`ControlSession` job cancel 改为通过 runtime `JobManager` capability resolver，`runAgentSendMessageTurn` 已迁为 `runAgentSendMessageTurnWithToolServices` 并删除 legacy `routerServices` ingress，HTTP-facing `ControlSession` / `DecideAgentAuthorization` 继续作为 router boundary wrapper；P0.5-N194 已新增 `SendMessageWithToolServices`，`SendMessage` wrapper 仅负责 HTTP/router `routerServices` → runtime binding 转换，内部 lifecycle / send-turn dispatch 全程消费 `runtime.AgentLoopToolServiceBinding`；P0.5-N195 已完成 `routerServices` ingress retirement completion audit，source audit 固化 `agentManager` production methods 只有 `ControlSession` / `DecideAgentAuthorization` / `SendMessage` 三个 HTTP-facing wrapper 可直接接收 `routerServices`，剩余 production `routerServices` 函数入口限定在 `router.go` HTTP routing/session/run/job/workspace/schedule handlers 与 `agent_tool_service_binding.go` runtime binding adapter；P0.5-N196 已新增 `daemonAgentLoopRunnerCompletionAuditTable`，覆盖 `deterministic_route` / `llm_tool_loop` / `replan_action_continuation` / `planner_action_replan_cycle` / `planner_action_execution` 五个 AgentLoop mode，并明确剩余 daemon-owned surface：shared cycle 的 action/warning/synthesis append placement、tool synthesis 的 daemon fallback、action execution fallback chain 与 direct daemon loop fallback；P0.5-N197 已新增 runtime `AgentLoopReplanCycleApplyActionAppend` / `ApplyWarningAppend` / `ApplySynthesisAppend` decision helper，`planner_action_replan_cycle` 的 action/warning/synthesis append placement 与 state egress 由 runtime DTO 决定，daemon 仅执行 `SessionMutationAppendMessages` dispatch/fallback 与 legacy message/plan conversion；P0.5-N198 已新增 `daemonLLMStageCallerFallbackOwnershipTable` / `daemonLLMStageCallerFallbackBoundaryTable`，`runRuntimeAgentLLMStage` 现在通过 ownership helper 处理 engine/stage-runner 未挂接与 `ErrLLMStageRunnerNotAttached` 回落，并用 tool_synthesis focused test 固化 `planner_action_replan_cycle` synthesis 兼容边界；P0.5-N199 已新增 `daemonAgentLoopCallerFallbackOwnershipTable` / `daemonAgentLoopCallerFallbackBoundaryTable`，`runRuntimeAgentLoopResult` 现在通过 ownership helper 处理 engine/AgentLoopRunner 未挂接与 `ErrAgentLoopRunnerNotAttached` 回落，并用 focused test 固化 direct daemon fallback 仅作为五个 AgentLoop mode 的兼容边界；P0.5-N200 已新增 `daemonPlannerActionExecutionFallbackOwnershipTable` / `daemonPlannerActionExecutionFallbackBoundaryTable`，显式登记 ActionExecutor、TurnExecutor compatibility 与 direct legacy executor 三段 fallback chain 的 reason/owner，并固化 `ErrAgentLoopActionExecutorNotAttached` 仅回落到 TurnExecutor compatibility，hard error 不会被吞掉；P0.5-N201 已新增 runtime action service ownership audit，覆盖 registry dispatch 中全部 direct `rt.Manager.handleAgent*` 调用，确认已 capability-owned 的 registry 批次与仍由 daemon manager 持有的 workspace/network/port-scan 侧效应，并选定下一处最小 capability 为 `AgentLoopActionServiceWorkspace`；P0.5-N202 已新增 runtime `AgentLoopActionServiceWorkspace` capability 的 daemon adapter 与 binding 注入，workspace.command / workspace.files.* / workspace.search handlers 改为通过 `workspaceActionServiceFromRegistryRuntime()` 解析 workspace capability，source audit 固化 `agent_registry_handlers.go` 不再直接调用 `rt.Manager.handleAgentCommand` / `rt.Manager.handleAgentWorkspace*`，下一处最小下沉边界转向 network request action service capability / daemon adapter；P0.5-N203 已新增 runtime `AgentLoopActionServiceNetworkRequest` capability 的 daemon adapter 与 binding 注入，`http.request` / `web.fetch` handlers 改为通过 `networkRequestActionServiceFromRegistryRuntime()` 解析 network request capability，source audit 固化 `agent_registry_handlers.go` 不再直接调用 `rt.Manager.handleAgentHTTPRequest` / `rt.Manager.handleAgentWebFetch`，ownership audit 将 selected next capability 推进到 `port_scanner` / `ports.scan.safe`；P0.5-N204 已新增 runtime `AgentLoopActionServicePortScanner` capability 的 daemon adapter 与 binding 注入，`ports.scan.safe` handler 改为通过 `portScannerActionServiceFromRegistryRuntime()` 解析 port scanner capability，source audit 固化 `agent_registry_handlers.go` 不再直接调用 `rt.Manager.handleAgentPortScan`，当前 workspace/network/port-scan side-effect groups 均为 runtime capability adapter attached，下一步转向剩余 registry runtime capability consumers 的 direct manager orchestration completion audit；P0.5-N205 已将剩余 direct registry manager orchestration 拆成 `scan_registry_handler`、`health_plugins_runs_registry_handlers`、`schedule_registry_handlers`、`web_search_registry_handler`、`run_context_approval_registry_handlers` 五组 ownership audit，保持已有 runtime service capability consumers 覆盖，并选定下一处最小 adapter 边界为 `web.search` / `handleAgentWebSearch` → runtime `web_search` capability；P0.5-N206 已新增 runtime `AgentLoopActionServiceWebSearch` capability 的 daemon adapter 与 binding 注入，`web.search` handler 改为通过 `webSearchActionServiceFromRegistryRuntime()` 解析 web search capability，source audit 固化 `agent_registry_handlers.go` 不再直接调用 `rt.Manager.handleAgentWebSearch`，ownership audit 将 selected next capability 推进到 `web_scan` / `scan.web.safe`；P0.5-N207 已新增 runtime `AgentLoopActionServiceWebScan` capability 的 daemon adapter 与 binding 注入，`scan.web.safe` handler 改为通过 `webScanActionServiceFromRegistryRuntime()` 解析 web scan capability，source audit 固化 `agent_registry_handlers.go` 不再直接调用 `rt.Manager.handleAgentScan`，ownership audit 将 selected next capability 推进到 `schedule_action` / `schedule.*`。P0.5-N208 已新增 runtime `AgentLoopActionServiceScheduleAction` capability 的 daemon adapter 与 binding 注入，`schedule.create` / `schedule.list` / `schedule.delete` handler 改为通过 `scheduleActionServiceFromRegistryRuntime()` 解析 schedule action capability，source audit 固化 `agent_registry_handlers.go` 不再直接调用 `rt.Manager.handleAgentSchedule*`，ownership audit 将 selected next capability 推进到 `daemon_status_action` / `daemon.health`、`plugins.list`、`runs.list`。P0.5-N209 已新增 runtime `AgentLoopActionServiceDaemonStatusAction` capability 的 daemon adapter 与 binding 注入，`daemon.health` / `plugins.list` / `runs.list` handlers 改为通过 `daemonStatusActionServiceFromRegistryRuntime()` 解析 daemon status action capability，adapter 继续通过 runtime resolver 读取 VersionService/PluginService/RunService，source audit 固化 `agent_registry_handlers.go` 不再直接调用 `rt.Manager.handleAgentHealth` / `handleAgentPlugins` / `handleAgentRuns`，ownership audit 将 selected next capability 推进到 `run_context_approval_action` / run-context 与 approvals 组。P0.5-N210 已新增 runtime `AgentLoopActionServiceRunContextApproval` capability 的 daemon adapter 与 binding 注入，`runs.context` / `reports.draft` / `runs.approvals.list` / `runs.approvals.approve` / `runs.approvals.deny` / `runs.resume_approved` handlers 改为通过 `runContextApprovalActionServiceFromRegistryRuntime()` 解析 run-context approval action capability，adapter 继续通过 runtime resolver 读取 RunService/ApprovalService，source audit 固化 `agent_registry_handlers.go` 不再直接调用任何 `rt.Manager.handleAgent*` orchestration，ownership audit 中所有 registry action side-effect groups 均为 runtime capability adapter attached，下一步转向 planner action executor fallback completion audit。P0.5-N211 已新增 registry action-service adapter completion audit 表，确认 ownership table 只剩 8 个 `runtime_capability_adapter_attached` registry side-effect groups、`selected_next_runtime_capability` / `daemon_manager_owned` 均为空，`agent_registry_handlers.go` direct `rt.Manager.handleAgent*` orchestration 为 0，并把下一步边界明确为 TurnExecutor compatibility fallback retirement readiness。P0.5-N212 已删除内部 `runRuntimePlannerActionExecutionTurn` fallback helper 与 payload/context adapter，ActionExecutor missing / `ErrAgentLoopActionExecutorNotAttached` 现在直接进入 `daemonAgentLoopActionExecutor.ExecuteAgentLoopAction`，同时将原依赖内部 TurnExecutor fallback 的 planner action/cycle 测试迁到 runtime `AgentLoopActionExecutor` test adapter；外部 `daemonAgentTurnExecutor.ExecuteTurn(kind=planner_action_execution)` 作为 explicit compatibility trampoline 暂留，下一步边界推进到 direct legacy executor fallback retirement readiness。P0.5-N213 已将 heuristic route 与外部 planner_action_execution TurnExecutor compatibility trampoline 的直接执行面改经 `daemonAgentLoopActionExecutor.ExecuteAgentLoopAction`，production `daemonPlannerActionExecutionExecutorForManager(...).ExecutePlannerAction` 仅剩 `daemonAgentLoopActionExecutor` implementation-only 调用；新增 direct legacy retirement audit，确认 registry side-effect ownership 仍由 runtime action-service adapters 覆盖，下一步边界推进到 action_executor_missing_fallback_retirement_readiness。P0.5-N214 已将 attached-engine missing/detached ActionExecutor fallback 收敛为 `runtime.ErrAgentLoopActionExecutorNotAttached` blocked/error，仅保留 no-engine AgentLoop direct fallback，下一步边界推进到 no_engine_agent_loop_direct_fallback_retirement_readiness。P0.5-N215 已新增 no-engine direct fallback retirement audit，确认 production `NewServer` 默认启用 runtime engine 并调用 `AttachEngine`，但显式 legacy runtime opt-out 仍需要保留 no-engine direct fallback；下一步边界推进到 agent_loop_runner_not_attached_fallback_retirement_readiness。P0.5-N216 已新增 `daemonAgentLoopRunnerNotAttachedRetirementTable`，attached-engine `AgentLoopRunner()==nil` 与 `ErrAgentLoopRunnerNotAttached` result 现在规范化为 `runtime.ErrAgentLoopRunnerNotAttached` blocked/error，不再进入 `runDaemonAgentLoopDirectResult`；下一步边界推进到 attached-engine LLMStageRunner missing/detached fallback。P0.5-N217 已新增 `daemonLLMStageRunnerNotAttachedRetirementTable`，attached-engine `LLMStageRunner()==nil` 与 `ErrLLMStageRunnerNotAttached` result 现在规范化为 `runtime.ErrLLMStageRunnerNotAttached` blocked/error，不再进入 planner_chat/tool_synthesis legacy fallback；P0.5-N218 已新增 `daemonLLMTurnRunnerCompatibilityRetirementTable` 与 source audit，确认 `RunLLMTurn` / `daemonAgentLLMTurnRunner` 无内部生产 caller，LLMTurnRunner 缩窄为 deprecated external engine seam only，下一步边界推进到 TurnExecutor deterministic/replan compatibility trampoline readiness。P0.5-N219 已新增 `daemonTurnExecutorCompatibilityRetirementTable` 与 source audit，确认非测试 `ExecuteTurn` caller 仅剩 runtime API dispatch，`AttachTurnExecutor` 仅在 `agentManager.AttachEngine` 默认挂接 compatibility seam，deterministic_route / replan_action_continuation 内部主路径均保持 `Engine.RunAgentLoop`，下一步边界推进到 planner_action_execution TurnExecutor explicit compatibility trampoline readiness。P0.5-N220 已将 `planner_action_execution_turn_executor` 加入 runtime compatibility trampoline/ownership audit，并新增 planner action TurnExecutor compatibility retirement table，确认 planner_action_execution 内部主路径保持 `Engine.RunAgentLoop` / `Engine.ExecuteAgentLoopAction`，explicit `ExecuteTurn` branch 仅作为 external compatibility seam 保留，下一步边界推进到 ExecuteTurn API overall removal/deprecation gate。P0.5-N221 已新增 `daemonExecuteTurnAPICompatibilityRetirementTable` 与 source audit，将 `ExecuteTurn` / `TurnExecutor` API 明确标记为 Deprecated external compatibility；deterministic_route、replan_action_continuation、planner_action_execution 三个 branch 均 external-only，但由于外部 API policy 未定义，默认 `AttachTurnExecutor` 暂不转 deletion candidate，下一步边界推进到 deprecated API migration window。
- 已清理 session mutation 输入 payload 的 daemon-private wrapper：append/record/request_authorization/capability_blocked/control/resolve 均由 runtime DTO + TurnContext 承载；append_messages、record_user_message、request_authorization、capability_blocked、control_session 与 resolve_authorization result 已先经 runtime `AgentLoopAppendMessagesResultPayload` / `AgentLoopRecordUserMessageResultPayload` / `AgentLoopRequestAuthorizationResultPayload` / `AgentLoopCapabilityBlockedResultPayload` / `AgentLoopControlSessionResultPayload` / `AgentLoopResolveAuthorizationResultPayload` / `AgentLoopSessionSnapshot` roundtrip，legacy result struct 仅作为 daemon adapter/caller DTO 与兼容 fallback 保留。
- session mutation result payload 已整体迁入 runtime-neutral DTO：append/record/request_authorization/capability_blocked/control/resolve 均具备 runtime result DTO 与 daemon adapter roundtrip；legacy fallback helper 已收敛并重命名为 adapter-only compatibility 路径，daemon mutator 六类输出均有 runtime DTO 类型断言覆盖，`FromSessionMutationResult` 重复 switch 已收敛为 typed runtime/legacy egress helper。
- 已输出 engine-native runner 草案：建议将 `llm_tool_loop` 与 `replan_action_continuation` 统一为单个 loop runner 的两个 mode，明确请求/响应 payload、authorization gate、action queue metadata、synthesis、append/persist ownership 与迁移顺序。
- 已抽出 planner loop helper：`decideAgentPlannerActionGate` 负责无副作用计算 capability mismatch / explicit intent / operation level / manual authorization / allow 决策；`runtime.PrepareAgentLoopActionBatch` / `AppendAgentLoopActionBatchActionResult` / `StopAgentLoopActionBatchAtGate` / `AgentLoopActionBatchStepResult` / `ApplyAgentLoopActionBatchStepResult` / `AgentLoopActionQueueMetadata` / `AgentLoopToolCallWithMetadata` / `AgentLoopAuthorizationActionMetadata` / `AgentLoopActionExecutionErrorResult` / `AgentLoopAuthorizationRequestMutationMetadata` / `AgentLoopCapabilityBlockedMutationMetadata` / `AgentLoopControlSessionMutationMetadata` / `AgentLoopResolveAuthorizationMutationMetadata` / `AgentLoopTurnExecutionMetadata` / `AgentLoopActionExecutionTurnMetadata` 负责统一 action batch 截断、queue/authorization/session mutation/turn execution metadata 字段、error fallback result/details 展开、step result apply、metadata merge 与 clone 隔离，daemon wrapper 只保留 job ID source 注入、legacy default tool metadata、turn kind/origin 选择与实际 action/session side effect 调度；batch max 与 queue ID 形状已迁到 runtime helper。
- action gate / action batch egress 已收敛到 runtime-neutral DTO：新增 `runtime.AgentLoopActionGateDecision` / `AgentLoopActionGateResult` / `AgentLoopActionBatchResult`，`executeAgentPlannerActionList` 降为 batch result wrapper；capability_blocked / request_authorization 副作用仍经 `SessionMutator` adapter，四类 gate parity 已补 focused 覆盖。
- P0.5-N119 action gate session mutation request helper 已完成：新增 runtime session mutation turn/request helper，request_authorization 与 capability_blocked gate mutation request 构造由 runtime 统一负责 turn/payload/metadata clone；daemon 仅保留 mutator dispatch、fallback 与 legacy adapter。
- P0.5-N120 control/resolve session mutation request helper 已完成：新增 runtime operator-origin turn/request helper 与 control_session、resolve_authorization metadata/request helper，external operator control 与 authorization decision 的 request 构造由 runtime 统一负责 turn/payload/metadata clone；daemon 仅保留 turnRef source、trim/normalize、mutator dispatch、fallback 与 legacy adapter。
- P0.5-N121 append/record session mutation request helper 已完成：新增 runtime append/record mutation metadata、turn 与 request helper，append_messages 与 record_user_message request 构造由 runtime 统一负责 turn/payload/metadata clone；daemon 仅保留 legacy DTO 转换、mutator dispatch/fallback 与 compatibility。
- P0.5-N122 session mutation orchestration dispatch helper 已完成：新增 daemon `runRuntimeSessionMutation` 统一处理 engine/mutator 缺失、nil ctx、`ErrSessionMutatorNotAttached`、blocked 与 error 语义；append/record/request_authorization/capability_blocked/control/resolve 六类 runtime mutation caller 统一复用该 dispatch helper，减少重复 `Eng.MutateSession` fallback 分支。
- P0.5-N123 action batch step executor seam helper 已完成：新增 runtime `AgentLoopActionBatchStepResult` / execution/gate step constructors / `ApplyAgentLoopActionBatchStepResult`，daemon 单步 executor 只返回 step egress，batch accumulator mutation、queue metadata append、gate stop apply 统一由 runtime helper 执行；daemon wrapper 仅保留 legacy default tool metadata/risk stamping。
- P0.5-N124 action execution turn result dispatch helper 已完成：新增 daemon `runRuntimePlannerActionExecutionTurn` / `agentPlannerActionExecutionTurnResultPayload` / shared `agentLoopTurnResultPayloadFromTurnResult`，统一处理 engine/executor 缺失、nil ctx、`ErrTurnExecutorNotAttached`、blocked/running-without-payload fallback、invalid payload/error 语义与 payload clone；`runRuntimePlannerActionExecution` 仅保留 legacy action fallback adapter。
- P0.5-N125 session mutation payload/result dispatch table helper 已完成：新增 daemon session mutation kind→handler dispatch table，`MutateSession` 不再维护六类 mutation 巨型 switch；payload clone/invalid payload error 统一经 typed helper，finished result skeleton 统一经 helper clone `Data`，六类 mutation 仍保留 runtime result DTO 与 legacy side-effect adapter 语义。
- P0.5-N126 turn executor shared dispatch helper 已完成：新增 daemon `agentTurnExecutionResultPayload`，deterministic route 与 planner action execution 共用 `ErrTurnExecutorNotAttached`、blocked、running-without-payload fallback、executor error、invalid payload error 与 runtime turn result payload clone 分支；两处 caller 仅保留各自 turn/request 构造和不同 invalid payload error 文案。
- P0.5-N127 session mutation caller result egress helper 已完成：新增 daemon `daemonSessionMutationCallerResultPayload` / `daemonSessionMutationCallerOptionalResultPayload`，record/control/resolve 三类 error-return caller 与 request_authorization/capability_blocked 两类 bool-only caller 共用 handled/error/payload conversion 逻辑，并保持 invalid payload、fallback 与 legacy adapter 语义不变。
- P0.5-N128 session mutation result adapter table/helper 已完成：新增 daemon session mutation result adapter table，append/record/request_authorization/capability_blocked/control/resolve 六类 `FromSessionMutationResult` 入口统一经 kind→adapter helper 分派，并保留 runtime value、runtime pointer、legacy payload fallback 与 clone 隔离语义。
- P0.5-N129 append mutation bool egress helper 已完成：新增 daemon `daemonSessionMutationCallerHandled`，append_messages runtime caller 不再手写 `handled && err == nil`，与其他 session mutation caller 共用 handled/error egress 语义并保持 legacy fallback 行为。
- P0.5-N130 session mutation caller egress DTO/helper 边界整理已完成：新增 daemon `daemonSessionMutationCallerEgress` 与 `runRuntimeSessionMutationCallerEgress`，append/record/request_authorization/capability_blocked/control/resolve runtime caller 不再各自传递 result/handled/error 三元组，caller helper 统一消费 egress DTO。
- P0.5-N131 session mutation caller invalid-payload/fallback audit table/helper 已完成：新增 daemon invalid payload kind→message audit table 与 `ForKind` result/optional caller helper，record/control/resolve strict error-return caller、request_authorization/capability_blocked optional fallback caller、append_messages bool-only caller 的 invalid-payload/fallback 语义均有 focused 覆盖。
- P0.5-N132 action execution invalid-payload/fallback audit table/helper 已完成：新增 daemon turn execution caller invalid payload kind→message audit table 与 `agentTurnExecutionResultPayloadForKind` helper，deterministic_route 与 planner_action_execution invalid payload 文案统一由 audit table 管理，missing executor、blocked、running-without-payload 与 executor error fallback 语义保持不变并有 focused 覆盖。
- P0.5-N133 session mutation caller policy/table cleanup 已完成：新增 daemon session mutation caller policy table，显式审计 append_messages bool-only、record/control/resolve strict-result、request_authorization/capability_blocked optional-result 三类 caller 策略，并将 `ForKind` strict/optional/bool helper 接入 policy guard；focused 覆盖 policy table、mismatch guard 与既有 fallback/invalid payload 语义。
- P0.5-N134 turn execution caller policy/table cleanup 已完成：新增 daemon turn execution caller policy table，显式审计 deterministic_route 与 planner_action_execution strict-result caller 策略，并将 `agentTurnExecutionResultPayloadForKind` 接入 policy guard；focused 覆盖 policy table、invalid payload table 与既有 fallback/handled/error 语义。
- P0.5-N135 action/session mutation adapter residual audit 已完成：新增 session mutation dispatch/result adapter/invalid payload/policy table cross-audit 与 turn execution caller invalid payload/policy table cross-audit，确保多张 audit table 覆盖集合一致且 replan continuation 不误归入 caller fallback 表。
- P0.5-N136 legacy fallback ownership cleanup 已完成：新增 turn execution caller fallback ownership table 与 session mutation caller fallback ownership table，显式标记 missing executor/mutator、blocked、running-without-payload、runtime engine 未挂接等 fallback 状态由 legacy caller owning fallback，`agentTurnExecutionResultPayload` 与 `runRuntimeSessionMutation` 统一通过 ownership helper 判断回落。
- P0.5-N137 runtime compatibility trampoline audit 已完成：新增 daemon runtime compatibility trampoline table，显式登记 `daemonAgentLLMTurnRunner.RunLLMTurn`、`daemonAgentTurnExecutor.executeReplanActionContinuationTurn` 与 legacy signature wrapper 候选的 mode、target 与 scope，并明确 `runRuntimeLLMToolLoopAgentLoop` / `runRuntimeReplanActionContinuationAgentLoop` 是直接 AgentLoop path，不应归入 compatibility trampoline。
- P0.5-N138 legacy signature wrapper deletion candidates 已完成：删除 `agentManager.runRuntimeLLMToolLoop` 与 `agentManager.runRuntimeReplanActionContinuation` 两个仅测试/旧签名使用的薄 wrapper，相关测试改为显式构造 `runtime.AgentLoopRequest` 后调用 direct AgentLoop path；当前 compatibility trampoline audit table 仅保留仍 active 的 engine seam entrypoint。
- P0.5-N139 active engine seam compatibility ownership audit 已完成：新增 compatibility trampoline ownership table，显式标记 `RunLLMTurn` 与 `ExecuteTurn` replan trampoline 当前由 external engine seam compatibility 持有，并记录各自 removal gate；当前不删除 active runtime API seam，避免破坏外部/测试兼容调用。
- P0.5-N140 active trampoline removal gate plan 已完成：compatibility trampoline ownership entry 增加 `replacement=runtime.Engine.RunAgentLoop`、`stage=active_compatibility` 与 `ReadyForRemoval` gate；当前两个 active trampoline 均明确未到 removal_candidate 阶段，只有完成 RunAgentLoop caller migration 后才能进入删除切片。
- P0.5-N141 active trampoline caller audit 已完成：`rg` 证据显示非测试代码中的 `RunLLMTurn` 仅剩 runtime API/interface/func adapter 定义与 daemon trampoline 实现，`agentReplanActionContinuationTurnContextFromRequest` 仅剩定义；daemon 非测试 attachment 仍在 `AttachEngine` 中挂接 `TurnExecutor` / `LLMTurnRunner`，因此当前 active trampoline 属于外部 runtime API compatibility，不属于内部生产主链路依赖。
- P0.5-N142 runtime API compatibility comment/deprecation marker 已完成：`LLMTurnRunner` / `LLMTurnRunnerFunc` / `AttachLLMTurnRunner` / `LLMTurnRunner` getter / `RunLLMTurn` 已标注 Deprecated 并指向 `Engine.RunAgentLoop`；`TurnExecutor` / `ExecuteTurn` 与 daemon replan continuation trampoline 增加 compatibility notice；compatibility ownership table 增加 runtime API marker/note 并补 focused 覆盖，仍保持 active seam 不删除。
- P0.5-N143 test-only replan continuation turn context helper cleanup 已完成：生产侧 `agentReplanActionContinuationTurnContextFromRequest` 已删除，保留同名 `_test.go` helper 供 replan trampoline focused tests 构造 `TurnContext`；非测试 caller audit 确认该 helper 已无生产引用。
- P0.5-N144 active compatibility seam post-cleanup caller audit 已完成：compatibility ownership entry 增加 `callerSurface=external_engine_seam_only` 与 caller audit note，固化 N143 后证据：`RunLLMTurn` 无内部生产主链路 caller，replan continuation turn context helper 已无非测试引用；两个 active trampoline 仍由 external engine seam 持有且未进入 removal candidate。
- P0.5-N145 TurnExecutor internal caller migration boundary audit 已完成：新增 deterministic_route / planner_action_execution migration boundary table，记录当前 seam 为 `runtime.Engine.ExecuteTurn`、stage 为 `active_turn_executor` 与各自 removal gate；audit table 与 invalid-payload/policy 表保持同集合，并明确 replan_action_continuation 不属于内部 ExecuteTurn caller migration 表。
- P0.5-N146 deterministic_route / planner_action_execution TurnContext runtime helper 下沉已完成：新增 runtime `AgentLoopTurnExecutionTurnFromRequest` / `AgentLoopTurnExecutionTurnWithPayload` / `AgentLoopActionExecutionTurn`，daemon deterministic route 与 planner action execution TurnContext 构造改为复用 runtime helper；replan continuation 测试 helper 也复用同一 runtime turn builder，metadata/payload clone 边界由 runtime focused tests 固化。
- P0.5-N147 deterministic_route / planner_action_execution runtime request helper 下沉已完成：新增 runtime `AgentLoopTurnExecutionRequest` / `AgentLoopActionExecutionRequest`，统一克隆 turn、tool service binding、authorization 与 request metadata；daemon request builder 只保留 daemon turnRef→`runtime.TurnContext` 适配，request DTO 组装交由 runtime helper。
- P0.5-N148 replan_action_continuation turn boundary helper 下沉已完成：新增 runtime `AgentLoopTurnExecutionBoundaryFromRequest`，生产侧 `agentReplanActionContinuationTurnBoundaryFromRequest` 不再手写 origin/index/metadata stamp，而是复用 runtime boundary builder 并保留原 payload；focused tests 覆盖 max_loops metadata 与 payload preservation。
- P0.5-N149 TurnExecutor helper ownership audit 已完成：新增 helper ownership table，确认 deterministic_route / planner_action_execution 的 request/turn/payload helper 均为 runtime-owned，剩余 daemon scope 只剩 turnRef adapter 与 legacy route/action side effects；replan_action_continuation 归档为 external compatibility trampoline，不属于内部 ExecuteTurn caller migration 集合。
- P0.5-N150 deterministic_route AgentLoop migration precondition 已完成：新增 deterministic_route AgentLoop migration precondition table，明确当前 caller、拟议 `AgentLoopModeDeterministicRoute`、所需 runtime helpers、daemon runner adapter parity、剩余 daemon routePrompt side effects 与下一步 swap 顺序；当前仅 deterministic_route 标记 `precondition_ready`，planner_action_execution 尚未进入该 precondition。
- P0.5-N151 deterministic_route AgentLoop caller migration 已完成：新增 `runtime.AgentLoopModeDeterministicRoute` 与 daemon AgentLoop runner branch，`runRuntimeDeterministicRoute` 已从内部 `Engine.ExecuteTurn` caller 切到 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`；`executeDeterministicRouteTurn` 保留为 external `ExecuteTurn` compatibility trampoline 并复用同一 routePrompt adapter，audit/compatibility table 已标记 internal caller live 于 AgentLoop、TurnExecutor branch 仅作外部兼容。
- P0.5-N152 planner_action_execution AgentLoop migration precondition / blocker audit 已完成：precondition table 现在同时记录 deterministic route live 与 planner action execution blocked 状态，planner action execution 明确卡在 `executeAgentPlannerAction` side effects、legacy `routerServices` handle unwrap、HTTP auth-profile adapter 与 action request/result DTO ownership 边界；当前未误标为 AgentLoop-ready。
- P0.5-N153 planner_action_execution side-effect boundary / ownership plan 已完成：新增 daemon planner action execution side-effect boundary helper，统一 runtime `AgentLoopRequest` / `AgentLoopAction` / `AgentLoopAuthorization` 到 legacy `executeAgentPlannerAction` inputs 的克隆和 adapter 收口；新增 boundary ownership plan table，明确 DTO 已 runtime-owned，剩余 `routerServices` unwrap、HTTP auth-profile adapter 与 action result egress 仍是 daemon legacy side effect，下一步迁到 typed runtime action executor contract。
- P0.5-N154 planner_action_execution runtime action executor contract / typed service binding precondition 已完成：新增 action executor contract precondition table，明确 `action_request_dto` 已 ready，`typed_tool_service_binding`、`authorization_context`、`executor_result_egress` 仍 blocked；helper 固化当前尚不能切 planner_action_execution AgentLoop mode，下一步先抽 executor adapter seam 降低 direct legacy caller surface。
- P0.5-N155 planner_action_execution executor adapter seam 已完成：新增 daemon-private `daemonPlannerActionExecutionExecutor` / legacy adapter，`executePlannerActionExecutionTurn` 与 `runRuntimePlannerActionExecution` fallback 不再直接调用 `executeAgentPlannerAction`，而是通过 side-effect boundary + executor seam 调度；直接 legacy caller surface 已收缩到 adapter 与 heuristic route 路径，focused tests 覆盖 seam dispatch 与 nil-manager guard。
- P0.5-N156 heuristic route action execution caller seam 收敛已完成：`routeHeuristicRegistryActions` 已改经 `daemonPlannerActionExecutionExecutor` seam 调度，caller audit 确认 `executeAgentPlannerAction` 非测试引用只剩函数定义与 legacy adapter implementation 调用；heuristic route focused test 覆盖 seam 后仍保持 registry handler dispatch 语义。
- P0.5-N157 legacy executor helper implementation-only audit 已完成：新增 `daemonPlannerActionExecutionLegacySurfaceTable` / helper，显式把 `executeAgentPlannerAction` 归档为 `executor_adapter_implementation_only`；新增注释要求新内部 caller 走 `daemonPlannerActionExecutionExecutor` seam，caller audit 仍确认非测试 `executeAgentPlannerAction(` 只剩函数定义与 legacy adapter implementation 调用。
- P0.5-N158 runtime action executor service contract 最小迁移落点已完成：新增 runtime `AgentLoopActionExecutor` / `Engine.ExecuteAgentLoopAction` seam 与 request/result clone 隔离覆盖；daemon 侧新增 service contract migration table，明确 runtime seam 已落地，下一步需要把 daemon legacy executor adapter 接到 Engine，同时 `AgentLoopToolServiceBinding.Handle`、authorization adapter 与 result egress 仍是 AgentLoop mode 切换 blocker。
- P0.5-N159 daemon legacy action executor runtime adapter 已完成：新增 `daemonAgentLoopActionExecutor` 实现 `runtime.AgentLoopActionExecutor` 并在 `AttachEngine` 中挂接；adapter 复用 `daemonPlannerActionExecutionExecutor` / side-effect boundary，返回 runtime `AgentLoopActionExecutorResult`，service contract migration table 已从 `daemon_adapter_pending` 更新为 `daemon_adapter_attached`，下一步可切 `runRuntimePlannerActionExecution` 优先走 `Engine.ExecuteAgentLoopAction`。
- P0.5-N160 planner action execution 优先 runtime action executor 已完成：`runRuntimePlannerActionExecution` 已先经 `Engine.ExecuteAgentLoopAction` 调度，action executor 缺失 / `ErrAgentLoopActionExecutorNotAttached` 时才回落到 `ExecuteTurn` compatibility fallback，最终 direct legacy executor adapter 仅作兜底；TurnExecutor-oriented tests 已显式 detach action executor 以覆盖兼容语义。
- P0.5-N161 planner_action_execution AgentLoop mode/runner branch 已完成：runtime 新增 `AgentLoopModePlannerActionExecution` 与 `AgentLoopPlannerActionExecutionRequest`，`AgentLoopRequest` 显式携带 cloned `Action`；daemon `AgentLoopRunner` / direct fallback 新增 planner_action_execution branch，并通过 action executor result 转成 `AgentLoopResult` metadata / state / messages / tool calls。当前 primary caller 仍优先 `Engine.ExecuteAgentLoopAction`，下一步再评估 `Engine.RunAgentLoop` 切换或继续拆 typed tool service binding / authorization / result egress blocker。
- P0.5-N162 planner_action_execution primary caller 切到 AgentLoop 已完成：`runRuntimePlannerActionExecution` 现在构造 `AgentLoopModePlannerActionExecution` + cloned `Action` 并调用 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`；runner branch 仍按 `Engine.ExecuteAgentLoopAction` → `ExecuteTurn` compatibility → direct legacy executor 顺序保留既有 fallback 语义。outer loop 与 external authorization focused tests 已更新为记录 planner_action_execution nested AgentLoop call，audit table 已标记 internal caller 为 AgentLoop。
- P0.5-N163 planner_action_execution blocker 拆解 / result composition runtime helper 已完成：新增 runtime `AgentLoopPlannerActionExecutionResultFromActionExecutorResult` / `FromTurnPayload`，daemon runner branch 的 ActionExecutorResult / TurnPayload → AgentLoopResult envelope 已下沉到 runtime；side-effect/service contract audit table 拆出 `runtime_result_composition` 已 landed，与仍 blocked 的 `typed_tool_service_binding`、`authorization_context`、`legacy_result_egress` 分离。
- P0.5-N164 legacy result egress adapter 收敛已完成：`daemonPlannerActionExecutionExecutor` seam 现在 caller-facing 返回 `runtime.AgentLoopActionExecutorResult`，daemon AgentLoop action executor、TurnExecutor compatibility branch 与 heuristic route caller 都消费 runtime result DTO；legacy `agentMessage` / `agentToolCall` → runtime DTO 的转换集中在 `daemonLegacyPlannerActionExecutionExecutor` 内部，audit table 拆出 `legacy_result_adapter_centralized` landed，剩余 blocker 降为 adapter implementation 内部仍由 `executeAgentPlannerAction` 返回 daemon-private result types。
- P0.5-N165 typed service provider contract 已完成：新增 daemon-private `daemonAgentLoopToolServiceProvider` contract 与 `daemonAgentLoopToolServiceProviderContractTable`，逐项映射 `routerServices.version/plugins/runs/approvals/jobs/agent/scanManager/schedules/providers` 到 typed accessor；`agentLoopToolServiceBindingFromRouterServices` 现在把 provider 放入 `AgentLoopToolServiceBinding.Handle`，`routerServices` value/pointer 仅作为 compatibility input 支持；`agentRegistryHandlerRuntime.toolServiceProvider()` 与 cycle tool context 已暴露 typed provider accessor，service contract audit 从 `typed_tool_service_binding` blocker 拆出 `registry_handler_service_consumers` blocker。N167 后 registry handler consumers 已全部改用 typed provider accessors；剩余 blocker 是 authorization context 仍转 daemon HTTP auth profile、legacy executor implementation 内部仍需 runtime-native result DTO。
- P0.5-N166 registry handler typed provider 第一批迁移已完成：`daemon.health` / `plugins.list` / `runs.list`、`schedule.create` / `schedule.list` / `schedule.delete` 与 `web.search` 均改为从 `agentRegistryHandlerRuntime.toolServiceProvider()` 获取 daemon-private typed provider；handler implementation 改用 `VersionService` / `PluginService` / `RunService` / `ScheduleManager` / `ProviderStore` typed accessor。新增 focused tests 使用“不允许 legacy reconstruction”的 provider stub 验证上述 handler 不再走 `legacyRouterServices()`；service contract audit 新增 first batch、schedule batch 与 web.search batch landed 记录。剩余 `registry_handler_service_consumers` blocker 缩小为 scan / run-context / approval handler 组仍调用 `legacyRouterServices()`。
- P0.5-N167 registry handler typed provider 剩余组迁移已完成：`scan.web.safe` 改用 `JobManager()` typed accessor；`runs.context` / `reports.draft` / `runs.approvals.list` / `runs.resume_approved` / `runs.approvals.approve` / `runs.approvals.deny` 改用 `RunService()` / `ApprovalService()` typed accessors；`rg \"rt\\.legacyRouterServices\\(\\)\" internal/interfaces/daemon/agent_registry_handlers.go` 已无结果。新增 scan batch 与 run-context/approval batch focused tests 使用“不允许 legacy reconstruction”的 provider stub 覆盖剩余 handler，service contract audit 将 `registry_handler_service_consumers` 标记为 typed provider landed。剩余 planner_action_execution blocker 缩小为 `authorization_context` 与 `executor_result_egress`。
- P0.5-N168 registry runtime authorization context 最小切片已完成：`agentRegistryHandlerRuntime` 新增 `runtime.AgentLoopAuthorization`，`http.request` handler 改走 `agentResolveAgentLoopAuthProfileHeaders` 直接解析 runtime auth profile，不再从 handler 调用面读取 legacy `rt.AuthProfiles`；`executeAgentRegistryHandler` 会同时填充 runtime auth context 与 legacy compatibility fields，并深拷贝 secret/enabled 指针。service contract audit 新增 `registry_handler_runtime_authorization_context` landed 记录；`authorization_context` 仍 blocked 于 `executeAgentPlannerAction` / `executeAgentRegistryHandler` 签名和 side-effect boundary 仍保留 legacy auth profile compatibility 参数，下一步改签名端到端传递 `AgentLoopAuthorization`。
- P0.5-N169 executor/registry authorization context 端到端传递已完成：`executeAgentPlannerAction` 与 `executeAgentRegistryHandler` 签名改为接收 `runtime.AgentLoopAuthorization`，`daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop` 直接 clone/preserve runtime authorization，不再转换为 daemon HTTP auth profile slice；registry runtime 仍填充 legacy `AuthProfiles` / `DefaultAuthProfileID` compatibility fields 供旧测试/调用面兼容，但 `authorization_context` 在 contract/audit table 已标记 ready/landed，planner_action_execution 剩余 blocker 收敛为 `executor_result_egress`。
- loop egress 已标准化到 runtime-neutral DTO：新增 `runtime.AgentLoopEgressResult` 与统一 metadata key，`planner_error` / `step_replan_synthesis_error` / `no_actions` / `auto_continue_disabled` / `cycle_stopped` / `max_loops` 均能在 `AgentLoopResult`、LLM/replan turn payload 与 executor Data 中携带 `stop_reason`、`loop_count`、`max_loops`；StepReplan synthesis failure warning 仍保留原 message/tool 行为，但 egress stop reason 已可稳定断言。
- 已抽出 planner action/replan cycle helper：`runAgentPlannerActionReplanCycle` 统一 `runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 的 Action → action append → tool synthesis → synthesis append 骨架；两种入口仍保留各自前置条件、append origin 与 response/tool merge 语义；action execution 与 StepReplan synthesis 的 legacy adapter 调用已分别收敛到 cycle-private helper。
- 已把 shared cycle helper 的输入/输出整理为 daemon-private DTO：`agentPlannerActionReplanCycleRequest` 承载 turn/cycle-private LLM request/authorization/tool context/plan handle/options，`agentPlannerActionReplanCycleState` 承载 response/tool/history state，`agentPlannerActionReplanCycleResult` 承载新 state/final plan handle/continue decision；为后续替换 runtime-neutral DTO 做前置。
- 已将 cycle tool call DTO 迁到 runtime-neutral `runtime.AgentLoopToolCall` / `runtime.AgentLoopToolCallError`：cycle message/tool wrappers 不再持有 daemon-private tool call DTO，daemon 仅在 legacy `agentToolCall` ingress/egress adapter 边界转换。
- 已将 cycle message/link DTO 迁到 runtime-neutral `runtime.AgentLoopMessage` / `runtime.AgentLoopLink`：cycle message wrapper 不再持有 daemon-private message/link/recommendation DTO，daemon 仅在 legacy `agentMessage` ingress/egress adapter 边界转换。
- 已将 cycle state DTO 迁到 runtime-neutral `runtime.AgentLoopState`：daemon state wrapper 不再直接持有 daemon-private response/tool/history wrappers，只在 legacy response/tool/history ingress/egress adapter 边界转换，并在 cycle result 初始化时 clone state 避免请求状态共享可变 slice。
- LLM turn runner 输入已统一为 runtime-neutral `runtime.AgentLoopRequest`，输出统一为 `runtime.AgentLoopTurnResultPayload`：`runtime.AgentLoopTurnPayload` 与 `daemonLLMTurnPayload` 已清理，legacy `sendAgentMessageRequest` / history / memory / service binding 转换集中在 request ingress 与 runner egress adapter。
- 已将 LLM stage runner payload/result 迁到 runtime-neutral `runtime.AgentLoopLLMStagePayload` / `runtime.AgentLoopLLMStageResultPayload`：planner_chat 与 tool_synthesis 不再通过 daemon-private stage payload 携带 legacy request/history/memory/tool messages/plan，daemon stage runner 仅在 ingress/egress adapter 转换 legacy 类型，并通过 runtime DTO clone 隔离 mutable fields。
- 已将 tool service binding adapter compatibility 迁到 runtime-neutral `runtime.AgentLoopToolServiceBinding`：LLM tool loop request、deterministic route / replan continuation / planner action execution payload 与 `agentPlannerActionReplanCycleToolContext` 不再直接持有 `routerServices`；daemon adapter 通过 `agentLoopToolServiceBindingFromRouterServices` 携带 runtime `ActionServices`，生产 dispatch 直传 binding。N183 后 registry production runtime 不再从 binding `Handle` fallback 恢复服务；N184 后 `routerServicesFromAgentLoopToolServiceBinding` / provider-from-binding / provider-from-Handle reconstruction helper 已移到 test-only compatibility adapter。
- registry handler runtime 也已改为持有 runtime-neutral `runtime.AgentLoopToolServiceBinding`：`agentRegistryHandlerRuntime.Services` 不再暴露 daemon `routerServices`；builtin handler 已全部通过 `agentRegistryHandlerRuntime.actionServiceResolver()` 消费 runtime `AgentLoopActionServiceSet` capabilities；production registry runtime 已不再暴露 `toolServiceProvider()` / `legacyRouterServices()` helper，Handle-only binding 仅由 test-only compatibility helper 覆盖。
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
- P0.5-N114 deterministic/replan turn execution metadata helper 已完成：新增 runtime generic turn execution metadata helper，deterministic route 与 replan continuation turn boundary 复用 helper 构造 executor/kind/operation_level/max_loops metadata。
- P0.5-N115 batch queue ID/max ownership helper 已完成：新增 runtime 默认 batch 上限、queue ID generator wrapper 与 job→queue ID shape helper；daemon action batch 不再持有本地 batch max 常量/queue ID 转换 wrapper，只注入 runs job ID source，empty batch 不生成 queue ID。
- P0.5-N116 action batch result assembly helper 已完成：新增 runtime action-result append 与 gate-stop helper，统一 clone 隔离、ProcessedActionCount、AuthorizationRequired 与 StopGate 写入；daemon batch loop 不再手写 result append/stop assembly。
- P0.5-N117 action execution loop body helper 已完成：抽出 daemon-private batch step context/helper 与 queue metadata append helper，`executeAgentPlannerActionBatch` 只负责 batch prepare 与逐项调度；metadata append helper 继续复用 runtime batch result assembly，同时保留 daemon-owned default tool metadata/risk stamping。
- P0.5-N118 action queue metadata append runtime-neutral helper 已完成：新增 `runtime.AppendAgentLoopActionBatchActionResultWithQueueMetadata`，runtime 统一负责 queue metadata stamp + result append + clone 隔离；daemon wrapper 退化为只补 legacy default tool metadata/risk，再调用 runtime helper。
- P0.5-N119 action gate session mutation request helper 已完成：新增 runtime session mutation turn/request helper，request_authorization 与 capability_blocked gate mutation request 构造由 runtime 统一负责 turn/payload/metadata clone；daemon 只保留 mutator dispatch/fallback 与 legacy compatibility。
- P0.5-N120 control/resolve session mutation request helper 已完成：新增 runtime operator-origin turn/request helper 与 control_session、resolve_authorization metadata/request helper，daemon control/resolve mutation literal 下沉到 runtime helper。
- P0.5-N121 append/record session mutation request helper 已完成：新增 runtime append/record mutation metadata、turn 与 request helper，append_messages 与 record_user_message request 构造由 runtime 统一负责 turn/payload/metadata clone。
- P0.5-N122 session mutation orchestration dispatch helper 已完成：daemon runtime session mutation caller 统一经 `runRuntimeSessionMutation` 处理 dispatch/fallback/error/blocked 语义。
- P0.5-N123 action batch step executor seam helper 已完成：新增 runtime step result DTO 与 apply helper，daemon `executeAgentPlannerActionBatchStep` 不再直接 mutate batch result，而是返回 execution/gate step egress 后统一 apply。
- P0.5-N124 action execution turn result dispatch helper 已完成：planner action execution 的 `ExecuteTurn` result dispatch/fallback/invalid payload 处理已收敛到独立 helper，并复用 shared turn-result payload cloner。
- P0.5-N125 session mutation payload/result dispatch table helper 已完成：daemon `MutateSession` 经 kind→handler table 分派，payload clone/invalid payload error 与 finished result skeleton helper 化，保留六类 runtime result DTO 输出类型。
- P0.5-N126 turn executor shared dispatch helper 已完成：deterministic route 与 planner action execution 复用 `agentTurnExecutionResultPayload` 处理 missing executor、blocked/running fallback、executor error、payload clone 与 invalid payload error 语义。
- P0.5-N127 session mutation caller result egress helper 已完成：session mutation runtime caller 复用 shared helper 处理 handled/error/invalid payload/payload egress，error-return 与 optional bool-only 两类调用方保持原 fallback 表现。
- P0.5-N128 session mutation result adapter table/helper 已完成：session mutation result egress adapter 统一经 kind table 分派，六类 typed helper 不再各自维护 runtime/legacy payload extraction boilerplate。
- P0.5-N129 append mutation bool egress helper 已完成：append_messages runtime caller 复用 `daemonSessionMutationCallerHandled` 判断 handled/error，保留 mutator missing、blocked、error 时回落 legacy append 的原行为。
- P0.5-N130 session mutation caller egress DTO/helper 边界整理已完成：session mutation runtime caller 统一由 `runRuntimeSessionMutationCallerEgress` 返回 result/handled/error 聚合 DTO，result/optional/bool 三类 caller helper 共用该边界。
- P0.5-N131 session mutation caller invalid-payload/fallback audit table/helper 已完成：invalid payload 文案改为由 mutation kind audit table 统一管理，record/control/resolve 保持 handled+error，request_authorization/capability_blocked 保持 optional fallback，append_messages 保持 bool-only handled/error 判断。
- P0.5-N132 action execution invalid-payload/fallback audit table/helper 已完成：turn execution caller invalid payload 文案改为由 kind audit table 统一管理，deterministic_route 与 planner_action_execution 均通过 `agentTurnExecutionResultPayloadForKind` 消费；missing executor、blocked、running-without-payload fallback 与 executor error handled 语义保持不变。
- P0.5-N133 session mutation caller policy/table cleanup 已完成：append_messages bool-only、record/control/resolve strict-result、request_authorization/capability_blocked optional-result 三类 runtime caller 策略由 policy table 显式管理，`ForKind` helper 加入 policy guard，避免调用方语义继续隐式散落。
- P0.5-N134 turn execution caller policy/table cleanup 已完成：deterministic_route 与 planner_action_execution strict-result runtime caller 策略由 policy table 显式管理，`agentTurnExecutionResultPayloadForKind` 加入 policy guard。
- P0.5-N135 action/session mutation adapter residual audit 已完成：session mutation dispatch/result adapter/invalid payload/policy table 与 turn execution caller invalid payload/policy table 现有 cross-audit 覆盖，减少后续表项漂移风险。
- P0.5-N136 legacy fallback ownership cleanup 已完成：turn execution 与 session mutation caller fallback 状态现在由 ownership table/helper 显式管理，legacy caller fallback 不再散落在条件判断中。
- 下一批迁移顺序：N142 继续按 N141 后边界收敛 action execution / session mutation 剩余副作用 adapter，优先评估 runtime API compatibility comment/deprecation marker 或剩余 daemon adapter cleanup。

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

- [x] P2-R1：Run List benchmark：构造大量 runs/events 数据，记录 API 与前端渲染基线。
- [x] P2-R2：实现可重建 run index 的恢复/校验流程，明确 SQLite/runtime store 与 runs dir 的 source of truth。
- [x] P2-R3：Tool Registry / MCP / Scheduler observability view：展示 registry source、operation level gate、scheduler job 状态。
- [x] P2-R4：Timeline 大量事件渲染优化：虚拟列表/分组/懒加载候选方案与回归测试。
- [x] P2-R5：Trace viewer JSON 增强：分段渲染、搜索、复制、下载、错误定位。

当前盘点（2026-06-09，P2-R1）：

- 已新增 `scripts/run-list-benchmark.mjs`：脚本在 `.tmp/bench/run-list/...` 下生成隔离的 synthetic run workspaces（不读取真实 `runs/`），启动 daemon 指向该 runs dir，重复请求 `/api/v1/runs?limit=...` 记录 API baseline；默认还启动 Vite + Playwright，把同一页 run summaries 转成当前 `/history` 页面使用的 scan-list 形态，拦截 `/api/scans` 后记录大量列表项的浏览器渲染 baseline。
- 默认 benchmark 场景为 1000 runs × 20 events/run、API page limit 1000、5 次 API measured iteration、`/history` 渲染 500 条；支持 `--runs`、`--events`、`--page-limit`、`--api-iterations`、`--render-limit` 与 `--skip-browser` 调整规模。
- 本地 baseline（2026-06-09）：fixture 生成 1.23s；`/api/v1/runs?limit=1000` 返回 1000 items / 20000 counted events，API samples `[228.83, 226.75, 229.34, 233.94, 228.44] ms`，mean 229.46ms、p50 228.83ms、p95 233.94ms；`/history` 500 items route-to-items 1026.17ms、after `/api/scans` fulfill 705.17ms。该结果为后续 P2-R2 run index 与 P2-R4 timeline/list virtualization 的对照基线。

当前盘点（2026-06-09，P2-R2）：

- 已新增 service-level 可重建 run index cache：`internal/core/services/runs_index.go` 写入 `<RunsDir>/.index/runs.json`，schema 为 `cyberagent.run_index.v1`，内容包含 `generated_at`、`source=runs_dir`、`runs_dir`、`count` 与完整 `RunSummary` items。
- `RunService.ListRuns` 已抽出 `scanRunSummaries`；当前列表读路径仍直接扫描 filesystem run workspaces，`runs/` 目录仍是 persisted run list 的 source of truth，`.index/runs.json` 只是可删除、可重建 cache。
- 新增 `RebuildIndex` / `LoadIndex` / `ValidateIndex`：rebuild 从 runs dir 重扫并以临时文件 + rename 写入 index；validate 将 index 与当前扫描结果对比，输出 `missing_from_index`、`extra_in_index` 与 `stale` summary diff，不做自动修复。
- 明确 SQLite/runtime store（`internal/runtime/state/sqlite/store.go`）仍属于 agent runtime Task/Run/Turn/Event store，不作为当前 filesystem run workspace listing 的 source of truth；如果后续要让 UI/API 读取 index，需要先保留 scan fallback 与 validate/rebuild 操作入口。
- 已补 `internal/core/services/runs_test.go` 覆盖：index 从 runs dir 重建与持久化、validate 发现新增/删除/stale、删除 `.index` 后 `ListRuns` 仍以 runs dir 工作并可再次 rebuild。

当前盘点（2026-06-09，P2-R4）：

- 已新增 `web/frontend/src/agent/components/timeline-windowing.ts`：提供 `windowTimelineItems`、`nextTimelineLimit`、`buildEventLookup`，默认支持 tail/head 窗口、focusIndex 保留定位项、hiddenBefore/hiddenAfter 与加载更多状态。
- Agent Cockpit `TurnTimeline` 默认只渲染最近 30 个 turns，并支持按 30 个逐步加载；选中或 running turn 会被保留在窗口内。`TurnGroup` 改为通过 event id lookup 读取审批/记忆事件，避免每个 turn 重复扫描完整 `state.events`。
- Scan detail 的执行链、全局事件流与 Step detail drawer 已改为有界渲染：执行链默认 120 个 step、全局事件流默认 300 条、单 step 详情默认 200 条，并提供“显示更早/更多”入口；focus step/focus event 会保持在窗口中。
- 已补 `web/frontend/src/agent/components/timeline-windowing.test.ts` 覆盖 tail/head 窗口、focus window、边界 clamp、limit 递增、max cap 与 event lookup stale 覆盖行为。

当前盘点（2026-06-09，P2-R5）：

- 已新增 `web/frontend/src/agent/components/trace-json.ts`：把 trace JSON 拆成 overview、summary、active_task、messages、tool_calls、timeline、todos、context_refs、pending_changes、approval/ui/memory/run state 与 replay_hints 等逻辑 section；大数组默认按 50 条分块，避免一次渲染完整大 JSON。
- Trace drawer 新增 JSON 分段视图：支持按 path/key/value/error 搜索，显示匹配 section 数、行数、大小、item 数，并在有搜索时展示命中行预览。
- Trace drawer 新增复制与下载：支持复制完整 trace、下载完整 trace（复用原入口）、复制单个 JSON section、下载单个 section 文件。
- Trace drawer 新增错误定位：自动标出包含 `error` 字段、failed/error/blocked/denied 等错误态的 section，点击错误 chip 会展开并滚动到对应 JSON section。
- 已补 `web/frontend/src/agent/components/trace-json.test.ts` 覆盖大数组分块、搜索匹配、错误 marker、命中行定位与 JSON 序列化失败兜底。

### 7.4 后置大能力拆分

- [x] L1：Desktop / Wails 技术 spike：daemon 嵌入、日志/更新、权限模型。
- [x] L2：Local API / Daemon 稳定版：配置、token、runtime DB migration、兼容策略。
- [ ] L3：Web 多用户平台：L3-A1~L3-A22 architecture/context/storage/auth/session/RBAC/session-store/cookie-resolver/auth-endpoint/RBAC-middleware/membership-store/policy-evaluator/deny-audit gate/web-session principal resolver/read-only membership admin/membership write gate/membership upsert timestamp ownership/timestamp hardening skeleton 已完成；待做真实 Web account adapter、真实 login/logout/refresh、CSRF/secure cookie hardening、完整 workspace isolation 与配额。
- [ ] L4：Remote Runner：远端 workspace、队列、日志流、artifact 回收。
- [ ] L5：长期 Target / User / Team Memory：作用域、过期、可解释检索与删除。
- [ ] L6：LLM Gateway 稳定化：provider fallback、model capability registry、prompt/version trace。

当前盘点（2026-06-09，L1-D1）：

- daemon `Server` 新增 `Serve(ctx, net.Listener)` 嵌入入口，`ListenAndServe` 复用同一入口；Wails/desktop shell 或测试 harness 可使用 `127.0.0.1:0` caller-owned listener 绑定 ephemeral loopback port，并从 listener 读取实际 URL 注入 WebView。
- 已补 focused 回归测试覆盖 ephemeral loopback listener、Bearer auth `/api/v1/healthz` 请求与 context cancellation shutdown；该切片只打开 desktop embed seam，不改变 daemon API 路由与 token 行为。

当前盘点（2026-06-09，L1-D2）：

- 新增 `internal/interfaces/desktop` dependency-free bootstrap seam：`StartEmbeddedDaemon(ctx, BootstrapConfig)` 负责 loopback ephemeral daemon 启动，`RuntimeContract` 提供 safe-to-log API URL/token file/runs/log/permissions 合约，`WebViewBootstrap` 明确敏感 token 注入 payload。
- 新增 focused tests 覆盖 Desktop bootstrap healthz、runtime contract 不序列化 bearer token、非 loopback listen host 拒绝；权限模型固定为 loopback-only、WebView-only token、configured dirs、无 desktop bypass、shell-owned explicit updates。
- 新增 `docs/CyberAgent-Desktop-Wails-Spike.md` 并加入 docs index，记录 Wails bootstrap 流程、runtime contract、权限边界、日志/更新模型与非目标。L1 技术 spike 已完成；完整 Desktop/Wails 产品仍保留在后置大能力总项中。

当前盘点（2026-06-09，L2-D1）：

- Local daemon config file defaults 已落地：`internal/core/config.Config` 新增 `DaemonConfig`，支持 `daemon.listen`、`daemon.runs_dir`、`daemon.token_file`、`daemon.max_concurrent_jobs`；merge 规则保持“显式配置覆盖默认值”。
- `cyberagent daemon` 新增 `--config`，默认也读取 user/project config；`applyDaemonConfigFile` 保持 CLI flags 最高优先级，显式 `--listen` / `--runs-dir` / `--token-file` / `--max-concurrent-jobs` 不会被 config 覆盖。
- 新增 `docs/CyberAgent-Local-Daemon-Stability.md` 并加入 docs index，记录 L2 成功标准、L2-D1 YAML 示例、验证命令与后续 token/runtime DB/compat 切片。

当前盘点（2026-06-09，L2-D2）：

- 新增 `internal/interfaces/daemon/token_inspection.go`：`InspectTokenFile` 提供 safe-to-log token file diagnostics，覆盖 missing、empty、readable、length、mode、secure permissions 与 error，且不返回 token value。
- 新增 `RedactBearerTokenForLog`，只输出 `<redacted:len=N>` 或 `<redacted:empty>`；focused tests 确认 secret 不进入 redaction 输出，loose POSIX mode 会被诊断为 insecure。
- `docs/CyberAgent-Local-Daemon-Stability.md` 已补 L2-D2 token stability audit；本切片不改变 token auth 行为、不强制 chmod 既有 token file，先提供稳定诊断和日志红线。L2 尚未整体完成，下一步进入 runtime DB migration audit。

当前盘点（2026-06-09，L2-D3）：

- Runtime SQLite store 新增 `CurrentSchemaVersion = 1` 与 `Store.AuditSchema(ctx)` safe-to-log schema audit，输出 DB path、`PRAGMA user_version`、expected version、当前 tables/indexes 与 missing required objects。
- `Open(path)` 初始化 schema 后会把既有 `user_version=0` 的 runtime DB 非破坏性标记为版本 1；不会重写已经带版本的 DB。
- 已补 focused tests 覆盖完整 schema audit、既有 DB user_version metadata migration、partial schema missing table/index 报告。
- `docs/CyberAgent-Local-Daemon-Stability.md` 已补 L2-D3 runtime DB migration audit；本切片不做破坏性 migration/repair，后续只剩 L2-D4 compatibility strategy doc/tests。L2 尚未整体完成。

当前盘点（2026-06-09，L2-D4）：

- Local daemon runtime 兼容策略已固化：`CYBERAGENT_RUNTIME` 继续只作为 whole-engine rollback switch；默认启用 runtime engine；`legacy/off/disabled/disable/false/0/none` 才关闭 whole engine。
- 默认 compatibility attachments 继续保留 `AttachLLMTurnRunner`、`AttachTurnExecutor`、`AttachAgentLoopRunner`、`AttachAgentLoopActionExecutor`；当前不新增 `CYBERAGENT_RUNTIME_COMPAT` 或 `CYBERAGENT_DEPRECATED_RUNTIME_API`。
- `docs/CyberAgent-Local-Daemon-Stability.md` 状态已更新为 L2 已完成，下一步进入 L3 Web 多用户平台 architecture/design slice。

当前盘点（2026-06-09，L3-A1）：

- 新增 `docs/CyberAgent-Web-Multi-User-Platform.md`，明确当前单用户 daemon bearer token / single `routerServices` / localStorage token-secret 模型不应直接升级为多用户 Web 认证边界。
- 定义 L3 最小目标模型：`Principal`、`TenantContext`、`TenantResolver`、User/Tenant/Workspace、local/default 兼容主体，以及 AuthN/AuthZ、HTTP session、Agent session、runtime session 的边界。
- 定义最小迁移计划：先做 no-op local skeleton 与 router context middleware，再逐步推进 service context、per-workspace storage resolver、frontend login/session/workspace UX、audit/admin/deployment hardening。

当前盘点（2026-06-09，L3-A2）：

- 新增 `internal/core/tenant/context.go`，提供 dependency-light `Principal`、`TenantContext`、`TenantResolver`、`LocalPrincipal()`、`LocalContext()`、`LocalResolver` 与 context roundtrip helper。
- `LocalResolver.ResolveTenant` 默认返回 `tenant=local` / `workspace=default` / `mode=local`；不会读取 header、cookie、URL query 或前端传入 tenant/workspace，因此不改变现有 bearer auth、daemon routes、Desktop/bootstrap、runsDir 或 runtime DB 行为。
- 新增 `internal/core/tenant/context_test.go` 覆盖 local/default scope、provided principal clone、request query 不影响 scope、context roundtrip clone 隔离。下一步进入 L3-A3 router middleware，把 no-op scope 注入 daemon request context 并补 bearer auth parity tests。

当前盘点（2026-06-09，L3-A3）：

- 新增 `internal/interfaces/daemon/tenant_middleware.go`，在 bearer auth 通过后调用 `withLocalTenantContext` 注入 cloned `TenantContext` 到 request context。
- `internal/interfaces/daemon/router.go` 的 L3-A3 API chain 当时调整为 `corsMiddleware(bearerAuth(token, withLocalTenantContext(mux)))`；L3-A7e 后扩展为 CORS → optional hardening → optional rate-limit → bearer auth → tenant context → mux，gates 默认 no-op，未授权请求仍不会执行 tenant context middleware。
- 新增 `internal/interfaces/daemon/tenant_middleware_test.go` 覆盖 valid bearer 后 `local/default` scope 注入、unauthorized request 不运行 middleware，并复跑 health/version 与 CORS preflight parity。下一步进入 L3-A4 service entrypoint context consumption。

当前盘点（2026-06-09，L3-A4）：

- 新增 `coretenant.FromContextOrLocal` 与 `services.tenantContextForServiceEntry`，让 runs / run index / approvals service entrypoint 在缺失 scope 时稳定回退 `local/default`，在带 future multi-user scope 时只读取 context、不改路径。
- daemon 侧新增 `tenant_scope.go`，并让 agent session、SendMessage/control/auth decision、jobs、schedules、provider store 入口读取 `TenantContext`；job runner 会继承 cloned tenant context，但 `RunDir` 仍来自配置的 `runsDir`。
- `web.ContextProviderStore` 作为可选 provider store extension，让 `/api/provider-profiles` 在 daemon store 下可消费 request context，同时保持原 `ProviderStore` contract 兼容。
- 新增 `internal/core/services/tenant_context_test.go` 与 `internal/interfaces/daemon/tenant_context_entrypoints_test.go`，覆盖带 non-local scope 时 runsDir、schedule store、provider JSON path、agent session store 均不变。下一步进入 L3-A5：显式 multi-user config gate 与 per-workspace storage resolver。

---

## 8. 最新本地验证记录

2026-06-09 本轮增量（L3-A1：Web 多用户平台 architecture/design slice）:

```bash
test -f docs/CyberAgent-Web-Multi-User-Platform.md
rg -n "Principal|TenantContext|TenantResolver|local/default|L3-A2" docs/CyberAgent-Web-Multi-User-Platform.md docs/CyberAgent-TODO.md
git diff --check
```

结果：Web 多用户平台 L3-A1 文档切片已完成；当前只定义 auth/session/tenant/workspace/API/storage/audit 边界与最小迁移计划，不改变 bearer token、daemon routes、runsDir、runtime DB、Desktop/bootstrap 或 runtime compatibility 策略。下一步进入 L3-A2：新增 no-op local `Principal` / `TenantContext` / `TenantResolver` skeleton。

2026-06-09 本轮增量（L3-A2：no-op local tenant skeleton）:

```bash
gofmt -w internal/core/tenant/context.go internal/core/tenant/context_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant
git diff --check
```

结果：新增 `internal/core/tenant` dependency-light skeleton；`LocalResolver` 稳定返回 `local/default` scope，context helper 深拷贝 `Principal.Roles`，focused tests 与 diff whitespace check 通过。本切片不接入 router，不改变现有 daemon/API 行为；下一步 L3-A3 将以 middleware 形式注入 request context 并验证 bearer auth parity。

2026-06-09 本轮增量（L3-A3：daemon local tenant context middleware）:

```bash
gofmt -w internal/interfaces/daemon/tenant_middleware.go internal/interfaces/daemon/tenant_middleware_test.go internal/interfaces/daemon/router.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestLocalTenantContextMiddleware|TestHTTPAuthEnvelopeHealthAndVersion|TestHTTPCORSPreflightBypassesAuthForLoopbackOrigin'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant
git diff --check
```

结果：daemon `/api/` route chain 已在 valid bearer 后注入 local/default tenant context；unauthorized request 不运行 tenant middleware；health/version auth envelope 与 CORS preflight parity 通过。本切片不改变 API 响应、runsDir、provider store、runtime DB 或 Desktop/bootstrap 行为。下一步 L3-A4 将让关键 service entrypoints 读取 `TenantContext` 并保持 local mode no-op。

2026-06-09 本轮增量（L3-A4：service entrypoint tenant context read）:

```bash
gofmt -w internal/core/tenant/context.go internal/core/tenant/context_test.go internal/core/services/tenant_context.go internal/core/services/tenant_context_test.go internal/core/services/runs.go internal/core/services/runs_index.go internal/core/services/approvals.go internal/interfaces/daemon/tenant_scope.go internal/interfaces/daemon/tenant_context_entrypoints_test.go internal/interfaces/daemon/jobs.go internal/interfaces/daemon/scheduler.go internal/interfaces/daemon/providers.go internal/interfaces/daemon/agent.go internal/interfaces/daemon/agent_session_flow.go internal/interfaces/daemon/agent_schedule_action_service.go internal/interfaces/daemon/agent_schedule_action_service_test.go internal/interfaces/daemon/agent_schedule_web.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/router.go internal/interfaces/daemon/server.go internal/interfaces/web/api.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/services -run 'Test.*TenantContext|TestRunIndexTenantContext'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemon.*TenantContext|TestScheduleManagerTenantContext|TestProviderStoreTenantContext|TestAgentManagerTenantContext|TestAgentLoopToolServiceBindingWithScheduleActionService|TestDaemonScheduleActionService'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/web
```

结果：agent/runs/jobs/schedules/providers 关键入口已可读取 `TenantContext`；local mode 仍使用配置的 `runsDir`、schedule store、provider JSON path 与 agent session store，API response 不新增 tenant/workspace 字段。本切片不启用 multi-user storage resolver、不迁移历史 runs、不改变 `runtime.db` 路径。下一步 L3-A5 将新增显式 multi-user config gate 与 per-workspace storage resolver 设计/实现。

2026-06-09 本轮增量（L3-A5：multi-user config gate 与 storage resolver skeleton）:

```bash
gofmt -w internal/core/config/config.go internal/core/config/config_test.go internal/core/tenant/storage.go internal/core/tenant/storage_test.go internal/interfaces/cli/daemon.go internal/interfaces/cli/root_test.go internal/interfaces/daemon/config.go internal/interfaces/daemon/storage_resolver.go internal/interfaces/daemon/storage_resolver_test.go internal/interfaces/daemon/server.go internal/interfaces/daemon/providers.go internal/interfaces/daemon/jobs.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/core/config ./internal/interfaces/cli ./internal/interfaces/daemon -run 'TestStorageResolver|TestDaemonStorageResolver|TestProviderStoreMultiUserStorageResolver|TestJobManagerMultiUserStorageResolver|TestApplyDaemonConfigFileUsesDefaultsAndPreservesChangedFlags|TestLoadFromFile|TestMergeOverlayWins|TestNewServerCreatesTokenFileAndNormalizesDefaults|TestNewServerDefaultRuntimeEngineCanOptOutToLegacy|TestDaemon.*TenantContext|TestScheduleManagerTenantContext|TestProviderStoreTenantContext|TestAgentManagerTenantContext'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/core/config ./internal/interfaces/cli ./internal/interfaces/web ./internal/interfaces/daemon
git diff --check
```

结果：新增 `daemon.multi_user.enabled` / `daemon.multi_user.storage_root` YAML gate 与 CLI `--multi-user` / `--multi-user-storage-root`；新增 `coretenant.StorageResolver` / `PathStorageResolver`，gate 关闭时即使带 future multi-user context 也返回 local paths，gate 开启时解析 `<storage_root>/tenants/<tenant>/workspaces/<workspace>/...`；daemon `NewServer` 使用 resolver 生成 local startup paths，provider store context load/save 与 web scan job run dir 已支持 scoped path。core/tenant、core/config、interfaces/cli、interfaces/web、interfaces/daemon 包级测试与 diff whitespace check 通过。默认 local mode 不启用 multi-user、不迁移历史 runs。下一步 L3-A6：frontend login/session/workspace UX，同时保留 local daemon settings。

2026-06-09 本轮增量（L3-A6：frontend session/workspace UX skeleton）:

```bash
npm test --prefix web/frontend -- --run session
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
git diff --check
```

结果：新增 `web/frontend/src/lib/session.ts` 与 focused Vitest，规范化 local/default session、future multi-user scope 与安全 segment；`ConsoleSettings` 增加 `session` 默认值但仍保持 local daemon bearer token/base URL/localStorage 认证链路；Layout 顶部新增 identity/workspace switcher，Settings 新增 Session & Workspace panel、disabled multi-user login placeholder 与 local/default scope 说明。当前不接入真实 Web login、不新增 workspace select API、不迁移 runs、不把前端 workspace 当作服务端隔离边界。下一步 L3-A7：audit log、admin bootstrap、secret server-side storage、rate limit 与 deployment hardening。

2026-06-09 本轮增量（L3-A9：frontend server-side identity/workspace scope hydrate skeleton）:

```bash
npm test --prefix web/frontend -- --run session
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
git diff --check
```

结果：前端 API client 新增 `/api/v1/me`、`/api/v1/tenants`、`/api/v1/workspaces` 与 `POST /api/v1/workspaces/{workspace_id}/select` 类型/方法；`ConsoleProvider` 会在本地 daemon token 可用后调用 `GET /api/v1/me` hydrate 当前 local/default session display，失败或未配置 token 时回退 `defaultConsoleSession`；Layout 与 Settings 增加 server scope sync/fallback 状态显示。仍保留 localStorage settings 与 bearer token 请求链路，不启用真实 Web login、不允许前端任意 workspace 作为隔离边界、不迁移 runs。

2026-06-09 本轮增量（L3-A10：server-side auth/session/RBAC model skeleton）:

```bash
gofmt -w internal/core/tenant/auth_model.go internal/core/tenant/auth_model_test.go internal/core/config/config.go internal/core/config/config_test.go internal/interfaces/cli/daemon.go internal/interfaces/cli/root_test.go internal/interfaces/daemon/config.go internal/interfaces/daemon/config_test.go internal/interfaces/daemon/identity_scope_test.go internal/interfaces/daemon/server.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/core/config ./internal/interfaces/cli ./internal/interfaces/daemon -run 'TestL3AuthModel|TestLoadFromFile|TestMergeOverlayWins|TestApplyDaemonConfigFileUsesDefaultsAndPreservesChangedFlags|TestConfigNormalize.*L3|TestIdentityMeKeepsLocalBearerWhenL3AuthSkeletonConfigured|TestIdentityMeReturnsLocalTenantContext'
git diff --check
```

结果：新增 `internal/core/tenant/auth_model.go`，定义 future `User` / `Session` / `Role` / `TenantMembership` / `WorkspaceMembership` 与 clone/role normalize 边界；新增 `daemon.auth.web_sessions` 与 `daemon.auth.rbac` YAML config gate、daemon normalize 与 CLI config-file mapping。默认 local mode 下 web session/RBAC gate 关闭，不创建 cookie session，不替换 bearer auth；即使配置 skeleton gate，`GET /api/v1/me` 仍返回 `local/default` + `local_bearer`，且不设置 `Set-Cookie`。真实 Web login handler、server-side session store、RBAC enforcement 与完整 workspace isolation 仍是后续 L3 切片。

2026-06-09 本轮增量（L3-A11：server-side session store skeleton）:

```bash
gofmt -w internal/core/tenant/session_store.go internal/core/tenant/session_store_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant -run 'Test.*SessionStore|TestL3AuthModel'
git diff --check
```

结果：新增 `internal/core/tenant/session_store.go`，定义 `SessionStore` interface、默认 `DisabledSessionStore` 与 `MemorySessionStore`，并提供 `Session.ActiveAt()` 供后续 Web auth middleware 判断 expiry/revocation。测试覆盖默认 disabled store 拒绝使用、memory store server-side clone 隔离、revoke、expiry 与 unknown RBAC role 拒绝。该切片本身未接入 `/api/` router、未设置 cookie、不替换 bearer auth；真实 login/logout/session refresh endpoint 与安全 cookie/CSRF hardening 留给后续 L3 切片。

2026-06-09 本轮增量（L3-A12：session cookie resolver middleware skeleton）:

```bash
gofmt -w internal/interfaces/daemon/router.go internal/interfaces/daemon/server.go internal/interfaces/daemon/web_session_middleware.go internal/interfaces/daemon/web_session_middleware_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestWebSession|TestNewRouterWebSessions|TestIdentityMeKeepsLocalBearerWhenL3AuthSkeletonConfigured|TestIdentityMeReturnsLocalTenantContext|TestIdentityScopeEndpointsRequireBearer'
git diff --check
```

结果：新增 `internal/interfaces/daemon/web_session_middleware.go`，提供默认 no-op 的 session cookie resolver skeleton；`web_sessions.enabled=false` 时不读取 cookie、不访问 session store，显式启用时仅把 active `web_session` 写入 request context。`NewServer` 在显式启用时挂接空的 in-memory session store，但 `/api/` 仍必须通过 bearer auth；cookie-only 请求仍返回 unauthorized，`GET /api/v1/me` 仍是 `local/default` + `local_bearer`，且不设置 `Set-Cookie`。真实 login/logout/session refresh、CSRF/secure cookie hardening、RBAC enforcement 与 workspace isolation 仍待后续切片。

2026-06-09 本轮增量（L3-A13：Web login/logout/session refresh endpoint skeleton）:

```bash
gofmt -w internal/interfaces/daemon/router.go internal/interfaces/daemon/web_auth_handlers.go internal/interfaces/daemon/web_auth_handlers_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestWebAuthSkeleton|TestWebSession|TestNewRouterWebSessions|TestIdentityMeKeepsLocalBearerWhenL3AuthSkeletonConfigured'
git diff --check
```

结果：新增 `internal/interfaces/daemon/web_auth_handlers.go` 与 `/api/v1/auth/login`、`/api/v1/auth/logout`、`/api/v1/auth/session/refresh` skeleton route。默认 `web_sessions.enabled=false` 时这些 endpoint 在 bearer auth 后返回 `not_found`，显式启用时返回 `unsupported` 并只暴露 CSRF/header、HttpOnly/SameSite/Secure cookie policy 与 account-adapter 未配置状态；所有路径都不创建 session、不设置 `Set-Cookie`、不接受 cookie-only 请求、不替换 `local_bearer`。真实账号体系、真实 login/logout/refresh、CSRF token issuance/validation、secure cookie hardening、RBAC enforcement 与 workspace isolation 仍待后续切片。

2026-06-09 本轮增量（L3-A14：RBAC middleware/enforcer skeleton）:

```bash
gofmt -w internal/interfaces/daemon/router.go internal/interfaces/daemon/rbac_middleware.go internal/interfaces/daemon/rbac_middleware_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRBAC|TestIdentityMeKeepsLocalBearerWhenL3AuthSkeletonConfigured|TestWebAuthSkeleton|TestWebSession'
git diff --check
```

结果：新增 `internal/interfaces/daemon/rbac_middleware.go`，定义 route/action/scope/role requirement table 与 request-scoped RBAC decision context。默认 `rbac.enabled=false` 时完全 no-op；显式启用时只记录 `skeleton_not_enforced` decision，不阻断请求、不把 `LocalPrincipal().Roles == ["local"]` 解释成 Web RBAC role、不改变 `/api/v1/me` 的 `local_bearer` 行为。真实 deny/audit enforcement 与 workspace isolation 仍待后续切片。

2026-06-09 本轮增量（L3-A15：tenant/workspace membership store skeleton）:

```bash
gofmt -w internal/core/tenant/membership_store.go internal/core/tenant/membership_store_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant -run 'Test.*MembershipStore|TestL3AuthModel|Test.*SessionStore'
git diff --check
```

结果：新增 `internal/core/tenant/membership_store.go`，定义 `MembershipStore` interface、默认 `DisabledMembershipStore` 与 in-memory tenant/workspace membership store。测试覆盖默认 disabled store 拒绝使用、tenant/workspace membership clone 隔离、required id validation、unknown role 拒绝、tenant/workspace role scope validation 与 context cancellation。当前 membership store 已由 L3-A16 接入 RBAC evaluation skeleton，但仍未接入 RBAC deny、login/session 或 workspace storage resolver；真实 enforcement 与 isolation 留给后续切片。

2026-06-09 本轮增量（L3-A16：RBAC policy loading/evaluation skeleton）:

```bash
gofmt -w internal/interfaces/daemon/rbac_middleware.go internal/interfaces/daemon/rbac_policy.go internal/interfaces/daemon/rbac_middleware_test.go internal/interfaces/daemon/router.go internal/interfaces/daemon/server.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRBAC|TestLoadDaemonRBACPolicy|TestNewServerLoadsExplicitRBACPolicyPath|TestIdentityMeKeepsLocalBearerWhenL3AuthSkeletonConfigured'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/core/config ./internal/interfaces/cli ./internal/interfaces/daemon -run 'Test.*MembershipStore|TestRBAC|TestLoadDaemonRBACPolicy|TestNewServerLoadsExplicitRBACPolicyPath|TestWebAuthSkeleton|Test.*Session|Test.*Auth|TestIdentity|TestConfigNormalize|TestLoadFromFile|TestMergeOverlayWins'
git diff --check
```

结果：新增 `internal/interfaces/daemon/rbac_policy.go`，并将 RBAC middleware 从纯 annotation skeleton 扩展为 policy evaluator skeleton。默认 `rbac.enabled=false` 时仍 no-op、不加载 policy path、不读取 membership store；显式启用时使用内置/显式 YAML policy requirement，并按 tenant/workspace scope 查询 `MembershipStore` 计算 allowed/missing-role decision。`Enforced=false` 保持不变，local bearer 请求不会因 Web RBAC policy 或 membership 缺失被 deny；显式 `policy_path` 缺失/非法会在 `NewServer` fail fast。真实 deny/audit enforcement、membership admin、真实 Web principal resolver 与 workspace isolation 仍待后续切片。


2026-06-09 本轮增量（L3-A17：RBAC deny/audit enforcement gate skeleton）:

```bash
gofmt -w internal/interfaces/daemon/rbac_middleware.go internal/interfaces/daemon/rbac_middleware_test.go internal/interfaces/daemon/router.go internal/interfaces/daemon/server.go internal/interfaces/daemon/config.go internal/interfaces/daemon/config_test.go internal/core/config/config.go internal/core/config/config_test.go internal/interfaces/cli/daemon.go internal/interfaces/cli/root_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRBAC|TestNewRouterRBAC|TestNewServerLoadsExplicitRBACPolicyPath|TestNewServerRBACPolicyPathIsIgnoredWhenDisabledAndFailsWhenEnabled|TestIdentityMeKeepsLocalBearerWhenL3AuthSkeletonConfigured'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/config ./internal/interfaces/cli -run 'TestLoadFrom|TestMergeOverlayWins|TestConfigFile|TestApply|TestDaemon'
```

结果：新增 `daemon.auth.rbac.audit_enabled` 与 `daemon.auth.rbac.enforcement_enabled` config gate，并在 daemon router 中把 L3-A16 decision 接入 `withRBACEnforcementGate`。两个 gate 默认关闭；`audit_enabled=true` 时仅写入 `rbac.decision` 非敏感 metadata（action/scope/required roles/evaluation reason/policy source type），不写 bearer token、request path 或 policy file path；`enforcement_enabled=true` 时会对非 local bearer principal 的 missing membership/missing role 返回 `policy_denied`，但 local bearer 继续豁免，不把 `local` role 当作 Web RBAC grant。下一步 L3-A18：Web session principal/tenant resolver skeleton，仍保持 cookie-only 不替代 bearer 与 local daemon 默认不变。


2026-06-09 本轮增量（L3-A18：Web session principal/tenant resolver skeleton）:

```bash
gofmt -w internal/interfaces/daemon/tenant_middleware.go internal/interfaces/daemon/router.go internal/interfaces/daemon/config.go internal/interfaces/daemon/config_test.go internal/interfaces/daemon/web_session_middleware_test.go internal/core/config/config.go internal/core/config/config_test.go internal/interfaces/cli/daemon.go internal/interfaces/cli/root_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestWebSession|TestNewRouterWebSession|TestIdentityMeKeepsLocalBearerWhenL3AuthSkeletonConfigured|TestRBAC|TestNewRouterRBAC'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/config ./internal/interfaces/cli -run 'TestLoadFrom|TestMergeOverlayWins|TestConfigFile|TestApply|TestDaemon'
```

结果：新增 `daemon.auth.web_sessions.principal_resolver_enabled` config gate，并把 daemon router 的 tenant middleware 替换为 `withRequestTenantContext`。默认仍走 `local/default` + `local_bearer`；cookie-only 请求继续 unauthorized；显式开启后，只有 bearer 已通过且 cookie resolver 找到 active server-side `web_session` 时，才把 session subject/tenant/workspace hydrate 为 request `TenantContext`。新增集成测试验证 Web session principal 可喂给 L3-A17 RBAC enforcement：缺 membership 返回 `policy_denied`，补齐 membership 后允许。下一步 L3-A19：membership admin API skeleton。

2026-06-09 本轮增量（L3-A19：membership admin API skeleton）:

```bash
gofmt -w internal/interfaces/daemon/membership_admin_handlers.go internal/interfaces/daemon/membership_admin_handlers_test.go internal/interfaces/daemon/router.go internal/interfaces/daemon/rbac_middleware.go internal/interfaces/daemon/admin_bootstrap.go internal/interfaces/daemon/server.go internal/interfaces/daemon/config.go internal/interfaces/daemon/config_test.go internal/core/config/config.go internal/core/config/config_test.go internal/interfaces/cli/daemon.go internal/interfaces/cli/root_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestMembershipAdmin|TestRBACRequirementTable|TestAdminBootstrap|TestConfigNormalize'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/config ./internal/interfaces/cli -run 'TestLoadFrom|TestMergeOverlayWins|TestConfigFile|TestApply|TestDaemon'
```

结果：新增 `daemon.admin.membership_api_enabled` config gate 与 `GET /api/v1/admin/memberships` 只读 skeleton。默认关闭时 bearer 通过后返回 `not_found`；显式开启后可按 `scope=tenant|workspace`、`tenant_id`、`workspace_id`、`subject_id` 查询 membership store，只返回 roles 与非敏感 metadata。POST/PUT/DELETE 不写 store，响应中固定 `write_endpoints_enabled=false`；RBAC route table 新增 `admin.membership.read`，local bearer 默认行为不变。下一步 L3-A20：membership write/admin mutation gate skeleton。

2026-06-13 本轮增量（P0.5-N239：engine-native turn lifecycle adapter cleanup）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime.go internal/interfaces/daemon/agent_subscribe_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestRunTurnLifecycle|TestBeginTurn|TestFinishTurn|TestAgentManagerRunRuntimeTurnLifecyclePublishesTerminalEvents|TestAgentManagerSendMessageRunsInsideRuntimeTurnLifecycle'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestMembershipAdmin|TestRBACRequirementTable|TestConfigNormalize|TestAdminBootstrap|TestNewServerWritesStartupAudit'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/config ./internal/interfaces/cli -run 'TestLoadFrom|TestMergeOverlayWins|TestApplyDaemonConfigFileUsesDefaultsAndPreservesChangedFlags'
```

结果：删除已无生产调用的 daemon `beginRuntimeTurn` / `finishRuntimeTurn` compatibility adapters，`SendMessage` 与 terminal event parity 覆盖统一落到 `runRuntimeTurnLifecycle` / `runtime.Engine.RunTurnLifecycle`；不改变 Begin/Finish lifecycle、response completion、membership gate 或 local bearer 默认行为。

2026-06-09 本轮增量（L3-A20：membership write/admin mutation gate skeleton）:

```bash
gofmt -w internal/interfaces/daemon/membership_admin_handlers.go internal/interfaces/daemon/membership_admin_handlers_test.go internal/interfaces/daemon/router.go internal/interfaces/daemon/rbac_middleware.go internal/interfaces/daemon/server.go internal/interfaces/daemon/config.go internal/interfaces/daemon/config_test.go internal/core/config/config.go internal/core/config/config_test.go internal/interfaces/cli/daemon.go internal/interfaces/cli/root_test.go docs/CyberAgent-Web-Multi-User-Platform.md docs/CyberAgent-TODO.md docs/api/v0.1.2-api-contract.md
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestMembershipAdmin|TestRBACRequirementTable|TestConfigNormalize|TestAdminBootstrap|TestNewServerWritesStartupAudit'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/config ./internal/interfaces/cli -run 'TestLoadFrom|TestMergeOverlayWins|TestApplyDaemonConfigFileUsesDefaultsAndPreservesChangedFlags'
```

结果：新增默认关闭的 `daemon.admin.membership_write_enabled` gate 与 `POST /api/v1/admin/memberships` mutation skeleton；写入仍要求 `membership_api_enabled=true`，并区分 bootstrap token 与 Web session principal。local bearer 默认继续不能写 membership；audit 只记录 scope/subject/role_count/authorized_by/comment_present 等非敏感 metadata。下一步可继续收敛 membership write 语义或转向 engine-native turn lifecycle 下沉。

2026-06-13 本轮增量（L3-A21：membership upsert timestamp ownership skeleton）:

```bash
gofmt -w internal/core/tenant/membership_store.go internal/core/tenant/membership_store_test.go internal/interfaces/daemon/membership_admin_handlers_test.go docs/CyberAgent-Web-Multi-User-Platform.md docs/CyberAgent-TODO.md docs/api/v0.1.2-api-contract.md
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/interfaces/daemon -run 'TestMemoryMembershipStore|TestMembershipAdmin'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/interfaces/daemon -run 'TestMemoryMembershipStore|TestMembershipAdmin|TestRBACRequirementTable|TestConfigNormalize|TestAdminBootstrap|TestNewServerWritesStartupAudit'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/config ./internal/interfaces/cli -run 'TestLoadFrom|TestMergeOverlayWins|TestApplyDaemonConfigFileUsesDefaultsAndPreservesChangedFlags'
git diff --check
```

结果：`MemoryMembershipStore` 现在在 tenant/workspace membership upsert 时填充 UTC `created_at` / `updated_at`，重复 upsert 未传 `created_at` 时保留原创建时间；admin POST 响应返回 store-owned timestamps 并验证 roles 替换。该切片不改变 local bearer 默认行为，不引入 membership delete、persistent backend、真实 account/login/CSRF、workspace mutation 或 approval 写动作。

2026-06-13 本轮增量（L3-A22：membership timestamp monotonic/client timestamp boundary skeleton）:

```bash
gofmt -w internal/core/tenant/membership_store.go internal/core/tenant/membership_store_test.go internal/interfaces/daemon/membership_admin_handlers_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/interfaces/daemon -run 'TestMemoryMembershipStore|TestMembershipAdmin'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/interfaces/daemon -run 'TestMemoryMembershipStore|TestMembershipAdmin|TestRBACRequirementTable|TestConfigNormalize|TestAdminBootstrap|TestNewServerWritesStartupAudit'
git diff --check
```

结果：`MemoryMembershipStore` 统一把 membership timestamps 规范为 UTC，并在输入或既有数据出现时钟偏斜时夹紧 `updated_at >= created_at`；admin membership POST 继续只接受 scope/tenant/workspace/subject/roles/comment，client-provided `created_at` / `updated_at` 字段不会进入 store 或响应。该切片仍不改变 local bearer 默认行为，不引入 membership delete、persistent backend、真实 account/login/CSRF、workspace mutation 或 approval 写动作。

2026-06-09 本轮增量（L2-D4：Compatibility strategy doc/tests）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonLocalRuntimeCompatibilityStrategy|TestAgentRuntimeEngineEnabledFromEnv|TestNewServerDefaultRuntimeEngineCanOptOutToLegacy'
git diff --check
```

结果：Local daemon runtime 兼容策略已由 focused tests 固化：`CYBERAGENT_RUNTIME` 只作为 whole-engine rollback switch；默认启用 runtime engine；默认继续挂接 `AttachLLMTurnRunner` / `AttachTurnExecutor` / `AttachAgentLoopRunner` / `AttachAgentLoopActionExecutor`；不新增 `CYBERAGENT_RUNTIME_COMPAT` 或 `CYBERAGENT_DEPRECATED_RUNTIME_API`。L2 已完成，下一步进入 L3。

2026-06-09 本轮增量（L2-D3：Runtime DB migration audit）:

```bash
gofmt -w internal/runtime/state/sqlite/store.go internal/runtime/state/sqlite/store_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/runtime/state/sqlite -run 'TestStoreAuditSchema|TestStoreOpenSetsUserVersion'
git diff --check
```

结果：SQLite runtime store 已有 safe-to-log schema audit；`PRAGMA user_version` 从 legacy `0` 非破坏性推进到 `CurrentSchemaVersion=1`；完整 schema tables/indexes 与 partial schema missing objects 均有 focused 覆盖。L2-D3 已完成，下一步补 compatibility strategy doc/tests。

2026-06-09 本轮增量（L2-D2：Token stability diagnostics / redaction）:

```bash
gofmt -w internal/interfaces/daemon/token_inspection.go internal/interfaces/daemon/token_inspection_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestInspectTokenFile|TestRedactBearerToken'
git diff --check
```

结果：token inspection 可安全诊断 missing/empty/readable/length/mode/permission 状态且不暴露 token value；redaction helper 不泄露 secret；POSIX loose token mode 可被标记为 insecure；diff whitespace check 通过。L2-D2 已完成，下一步补 runtime DB migration audit。

2026-06-09 本轮增量（L2-D1：Local daemon config file defaults）:

```bash
gofmt -w internal/core/config/config.go internal/core/config/config_test.go internal/interfaces/cli/daemon.go internal/interfaces/cli/root_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/config ./internal/interfaces/cli -run 'TestMergeOverlayWins|TestLoadFromFile|TestDaemonCommandHelp|TestApplyDaemonConfigFileUsesDefaultsAndPreservesChangedFlags'
git diff --check
```

结果：config merge 支持 daemon section；daemon command help 暴露 `--config`；config file defaults 可填充 daemon listen/runs/token/max jobs，且 changed CLI flags 保持最高优先级；diff whitespace check 通过。L2-D1 已完成，下一步补 token stability audit。

2026-06-09 本轮增量（L1-D2：Desktop / Wails runtime contract 与 bootstrap seam）:

```bash
gofmt -w internal/interfaces/daemon/server.go internal/interfaces/daemon/server_test.go internal/interfaces/desktop/bootstrap.go internal/interfaces/desktop/bootstrap_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestServerServeSupportsEmbeddedEphemeralLoopbackListener|TestNewServerCreatesTokenFileAndNormalizesDefaults'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/desktop
git diff --check
```

结果：Desktop bootstrap seam 可启动 embedded daemon 并用 WebView bearer token 请求 `/api/v1/healthz`；`RuntimeContract` JSON 不泄露 token；非 loopback bind 被拒绝；Wails spike 文档覆盖 bootstrap、权限、日志与更新边界。L1 技术 spike 已标记完成，下一步转入 L2 Local API / Daemon 稳定版。

2026-06-09 本轮增量（L1-D1：Desktop / Wails daemon 嵌入 seam）:

```bash
gofmt -w internal/interfaces/daemon/server.go internal/interfaces/daemon/server_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestServerServeSupportsEmbeddedEphemeralLoopbackListener|TestNewServerCreatesTokenFileAndNormalizesDefaults'
git diff --check
```

结果：daemon `Server.Serve(ctx, net.Listener)` 支持 caller-owned listener 与 `127.0.0.1:0` ephemeral loopback 嵌入启动；focused 测试验证带 token 的 healthz 请求成功、context cancel 后 server 可正常 shutdown。focused daemon tests 与 diff whitespace check 通过。L1-D1 已落地；当时剩余 runtime contract / Wails bootstrap / 日志更新权限边界，已在 L1-D2 补齐。

2026-06-09 本轮增量（P2-R5：Trace viewer JSON 增强）:

```bash
npm test --prefix web/frontend
npm run build --prefix web/frontend
git diff --check
```

结果：新增 `trace-json` helper 与 5 个回归测试；Trace drawer 支持 JSON 逻辑分段/大数组分块、搜索与命中行预览、完整/分段复制、完整/分段下载、错误 section 自动定位与滚动展开。前端 production build 与 8 个 frontend test files / 37 tests 通过，diff whitespace check 通过；P2-R5 已标记完成，P2 Production / Observability 清单全部完成。

2026-06-09 本轮增量（P2-R4：Timeline 大量事件渲染优化）:

```bash
npm test --prefix web/frontend
npm run build --prefix web/frontend
git diff --check
```

结果：新增 timeline windowing helper 与 7 个回归测试；Agent Cockpit TurnTimeline 默认按 30 个 turn 窗口渲染并用 event lookup 避免 turn × events 重复扫描；Scan detail 执行链/全局事件流/Step detail drawer 分别按 120/300/200 条有界渲染，支持保留 focus step/event 与逐步加载更多。前端 production build 与 7 个 frontend test files / 32 tests 通过，diff whitespace check 通过；P2-R4 已标记完成，下一步转入 P2-R5 Trace viewer JSON 增强。

2026-06-09 本轮增量（P2-R3：Tool Registry / MCP / Scheduler observability view）:

```bash
npm run build --prefix web/frontend
npm test --prefix web/frontend
git diff --check
```

结果：新增 `/ops` 综合观测页并接入侧边栏/移动底栏导航；页面从 `/api/v1/agent/skills`、`/api/v1/agent/mcp`、`/api/v1/schedules`、`/api/v1/agent/capabilities` 汇总 Tool Registry source、operation level gate 预检、MCP server/注入能力与 Scheduler job 状态，并每 15 秒自动刷新。前端 production build 与 6 个 frontend test files / 25 tests 通过，diff whitespace check 通过；P2-R3 已标记完成，下一步转入 P2-R4 Timeline 大量事件渲染优化。

2026-06-09 本轮增量（P2-R2：可重建 run index cache / validate / rebuild）:

```bash
gofmt -w internal/core/services/runs.go internal/core/services/runs_index.go internal/core/services/runs_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/services -run 'TestRunService.*Index|TestRunServiceReadsGeneratedRun|TestRunServiceIgnoresInternalAgentStateDirectory'
node --check scripts/run-list-benchmark.mjs
git diff --check
```

结果：`RunService` 新增 filesystem-backed rebuildable index cache，路径为 `<RunsDir>/.index/runs.json`；`ListRuns` 仍扫描 runs dir，index 可删除后重建；`ValidateIndex` 可发现 source-of-truth runs dir 与 cache 之间的新增、删除和 stale summary 差异。focused service tests、benchmark 脚本语法检查与 diff whitespace check 通过。P2-R2 已标记完成；下一步转入 P2-R3 Tool Registry / MCP / Scheduler observability view。

2026-06-09 本轮增量（P2-R1：Run List benchmark baseline）:

```bash
node --check scripts/run-list-benchmark.mjs
node scripts/run-list-benchmark.mjs --runs 50 --events 3 --api-iterations 2 --render-limit 20
node scripts/run-list-benchmark.mjs --runs 1000 --events 20 --api-iterations 5 --render-limit 500
git diff --check
```

结果：新增可重复 benchmark 脚本，synthetic fixtures 全部写入 `.tmp/bench/run-list/...`，不会污染真实 runs dir。小规模 smoke 通过；1000 runs × 20 events/run 基线为 API mean 229.46ms / p50 228.83ms / p95 233.94ms，`/history` 500 items route-to-items 1026.17ms / after `/api/scans` fulfill 705.17ms。P2-R1 已标记完成；下一步转入 P2-R2 可重建 run index。

2026-06-09 本轮增量（P0.5-N238：P0.5 runtime seam 总括 completion audit）:

```bash
rg -n "^- \\[ \\]" docs/CyberAgent-TODO.md
rg -n "daemonAgentLoopRunnerCompletionNextRuntimeSeam|daemonAgentLoopRunnerCompletionAuditTable|ManualStageCommitTagGate|SessionMutationNoEngineFallback|await_explicit_user_stage_commit_tag_authorization" internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
rg -n "daemonNoEngineSessionMutationFallbackRetirement|CYBERAGENT_RUNTIME|SessionMutationKind|ErrSessionMutatorNotAttached|runRuntimeSessionMutation" internal/interfaces/daemon/agent_session_mutator.go internal/interfaces/daemon/agent_session_mutator_test.go internal/interfaces/daemon/server.go internal/interfaces/daemon/agent_runtime.go
rg -n "AttachLLMTurnRunner|AttachTurnExecutor|AttachEngine|RunAgentLoop|AgentLoopRunner|ExecuteAgentLoopAction|SessionMutator" internal/interfaces/daemon/agent_runtime.go internal/agent/runtime/*.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes|TestDaemonNoEngineSessionMutationFallback'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestRunAgentLoop|TestExecuteAgentLoopAction|TestMutateSession'
git diff --check
```

结果：P0.5 旧总括 unchecked 项已完成真实 completion audit 并标记完成；证据显示 AgentLoop completion audit 五个 mode 的下一边界均为 `await_explicit_user_stage_commit_tag_authorization`，旧 shared-cycle/action-service/session-mutation fallback seam entries 为空；no-engine SessionMutation fallback 仅作为 `CYBERAGENT_RUNTIME` whole-engine rollback 保留；runtime 默认 attachment 未减少。focused daemon/runtime tests 与 diff whitespace check 通过。当前剩余未完成项转入 P2/P后置大能力；git stage/commit/tag 仍等待明确授权。

2026-06-09 本轮增量（P0.5-N237：no-engine SessionMutation fallback retention audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_session_mutator.go internal/interfaces/daemon/agent_session_mutator_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonNoEngineSessionMutationFallback|TestDaemonOptionalSessionMutationFallbackRetirement|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes|TestAgentRuntimeEngineEnabledFromEnv|TestNewServerDefaultRuntimeEngineCanOptOutToLegacy'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：no-engine `SessionMutation` fallback 已审计并保留为 `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0` whole-engine rollback switch；production default 仍 attach runtime engine + `daemonSessionMutator`，六类 caller fallback（append_messages / record_user_message / request_authorization / capability_blocked / control_session / resolve_authorization）不作为默认生产路径。completion audit 下一边界回到 `await_explicit_user_stage_commit_tag_authorization`。daemon/runtime 包测试与 diff whitespace check 通过；本轮不执行 `git add`、`git commit` 或 `git tag`，不新增 `CYBERAGENT_RUNTIME_COMPAT` / `CYBERAGENT_DEPRECATED_RUNTIME_API`，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。

2026-06-09 本轮增量（P0.5-N236：optional-result SessionMutation fallback narrowing）:

```bash
gofmt -w internal/interfaces/daemon/agent_session_mutator.go internal/interfaces/daemon/agent_session_mutator_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonOptionalSessionMutationFallbackRetirement|TestDaemonSessionMutationCallerOptionalResultPayloadSuppressesFailures|TestAgentManagerSessionMutationCallerInvalidPayloadFallbackAudit|TestAgentManagerRequestAuthorizationOptionalMutationFailureBlocksLegacyFallback|TestAgentManagerCapabilityBlockedOptionalMutationFailureBlocksLegacyFallback|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：optional-result `SessionMutator` fallback 已收窄：`request_authorization` / `capability_blocked` 在 attached `SessionMutator` 返回 blocked、hard error 或 invalid payload egress 时视为 handled，阻止 caller 落回 legacy session state write；no-engine legacy opt-out fallback 继续保留。completion audit 下一边界推进到 `session_mutation_no_engine_legacy_fallback_retirement_readiness`。本轮不执行 `git add`、`git commit` 或 `git tag`，不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。

2026-06-09 本轮增量（P0.5-N235：attached SessionMutator blocked/error append fallback narrowing）:

```bash
gofmt -w internal/interfaces/daemon/agent_session_mutator.go internal/interfaces/daemon/agent_session_mutator_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAppendSessionMutationBlockedOrErrorRetirement|TestAgentManagerAppendMessagesWithPlannerForTurnSessionMutator|TestDaemonSessionMutationCallerHandledSuppressesFailures|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：attached-engine `SessionMutator` blocked/hard-error append fallback 已收窄：no-engine legacy opt-out append fallback 继续保留，attached `SessionMutator` 返回 generic `TurnResultBlocked` 或 hard error 时阻止 `appendMessagesWithPlannerForTurn` 落回 legacy append；completion audit 下一边界推进到 `session_mutation_optional_result_fallback_retirement_readiness`。本轮不执行 `git add`、`git commit` 或 `git tag`，不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。

2026-06-09 本轮增量（P0.5-N234：attached SessionMutator not-attached append fallback narrowing）:

```bash
gofmt -w internal/interfaces/daemon/agent_session_mutator.go internal/interfaces/daemon/agent_session_mutator_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationCallerFallbackOwnershipTableCoversReasons|TestDaemonSessionMutatorNotAttachedRetirementAuditCoversAppendBoundary|TestDaemonSharedCycleSessionMutationFallback|TestAgentManagerRunRuntimeSessionMutationDispatchHelper|TestAgentManagerAppendMessagesWithPlannerForTurnSessionMutatorNotAttachedBlocksLegacyFallback|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：attached-engine `SessionMutator` missing/detached fallback 已收窄：no-engine legacy opt-out append fallback 继续保留，attached-engine `SessionMutator()==nil` 与 `ErrSessionMutatorNotAttached` result 规范化为 blocked/error，并阻止 `appendMessagesWithPlannerForTurn` 落回 legacy append；completion audit 下一边界推进到 `session_mutation_blocked_or_error_fallback_retirement_readiness`。本轮不执行 `git add`、`git commit` 或 `git tag`，不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。

2026-06-09 本轮增量（P0.5-N233：shared-cycle SessionMutation fallback audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_session_mutator.go internal/interfaces/daemon/agent_session_mutator_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSharedCycleSessionMutationFallback|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：shared-cycle append SessionMutation fallback audit 已固化：runtime append decision 仍只负责 placement/state egress，daemon append helper 负责 SessionMutation dispatch，legacy append fallback 仍保留在 `appendMessagesWithPlannerForTurn` → `appendMessagesWithPlannerLegacyForTurn` 边界；completion audit 下一边界推进到 `shared_cycle_attached_session_mutator_fallback_retirement_readiness`。本轮不执行 `git add`、`git commit` 或 `git tag`，不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。

2026-06-09 本轮增量（P0.5-N232：nested planner_action_replan_cycle source audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_loop_runner_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionReplanCycleSourceAuditUsesAgentLoopFallbackPolicy|TestAgentManagerRunRuntimePlannerActionReplanCycleUsesAgentLoopFallbackPolicy|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused nested dispatch / completion audit 测试、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；nested `planner_action_replan_cycle` source audit 已固化：runtime wrapper 不再直接调 daemon cycle / direct fallback，而是统一进入 `runRuntimeAgentLoopResult`；outer LLM/replan continuation 的 production `cycleReq` caller 继续走 runtime dispatch，direct daemon cycle 仅保留在 AgentLoopRunner branch 与 no-engine direct fallback scope。completion audit 下一边界推进到 `shared_cycle_session_mutation_dispatch_fallback`。本轮不执行 `git add`、`git commit` 或 `git tag`，不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。

2026-06-09 本轮增量（P0.5-N231：engine-owned loop runner nested cycle dispatch fallback audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_loop_runner.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_loop_runner_test.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionReplanCycleUsesAgentLoopFallbackPolicy|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes|TestAgentManagerRunRuntimeAgentLoopResult'
```

结果：focused AgentLoop runner fallback / completion audit 测试通过；nested `planner_action_replan_cycle` dispatch 已改为复用 `runRuntimeAgentLoopResult`，attached-engine missing/detached `AgentLoopRunner` 会返回 `runtime.ErrAgentLoopRunnerNotAttached` blocked/error，runner hard error 不再直接 fallback 到 daemon cycle；completion audit 下一边界更新为 `planner_action_replan_cycle_nested_dispatch_fallback`。本轮不执行 `git add`、`git commit` 或 `git tag`，不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。

2026-06-09 本轮增量（P0.5-N230：manual_stage_commit_and_tag_authorization）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonDeprecatedRuntimeAPI'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused deprecated runtime API audit、full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonDeprecatedRuntimeAPIManualStageCommitTagAuthorizationTable` 与 source/docs audit，确认当前自动化边界为 `await_explicit_user_stage_commit_tag_authorization`。验证后当前工作树仍为 48 个 tracked modified、33 个 untracked、合计 81 项，tracked diff stat 为 48 files changed, 11575 insertions(+), 1413 deletions(-)。本轮不执行 `git add`、`git commit` 或 `git tag`；未获明确授权前不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。

2026-06-09 本轮增量（P0.5-N229：release_candidate_stage_commit_and_full_regression_gate）:

```bash
git status --short
git diff --stat
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonDeprecatedRuntimeAPI'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test ./...
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go vet ./...
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go build ./cmd/cyberagent
git diff --check
```

结果：focused deprecated runtime API audit、full daemon tests、runtime package test、全量 `go test ./...`、`go vet ./...`、frontend vitest/build、CLI build 与 diff whitespace check 通过；新增 `daemonDeprecatedRuntimeAPIReleaseCandidateStageCommitFullRegressionGateTable` 与 source/docs audit，接收 N228 diff archive 并固化 stage/commit/full-regression gate。验证后当前工作树仍为 48 个 tracked modified、33 个 untracked、合计 81 项，tracked diff stat 为 48 files changed, 11558 insertions(+), 1413 deletions(-)，因此仍 dirty/not tag-ready；本轮不执行 `git add`、`git commit` 或 `git tag`。未获明确授权前不新增 dedicated deprecated runtime API opt-in flag，不减少 `AttachLLMTurnRunner` / `AttachTurnExecutor` 默认挂接。下一步 P0.5-N230 转向 `manual_stage_commit_and_tag_authorization`。

2026-06-09 本轮增量（P0.5-N228：release_candidate_diff_archive_and_tag_readiness_audit）:

```bash
git status --short
git diff --stat
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonDeprecatedRuntimeAPI'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused deprecated runtime API audit、full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `docs/CyberAgent-v0.1.2-Release-Candidate-Diff-Inventory.md`、`daemonDeprecatedRuntimeAPIReleaseCandidateDiffArchiveTagReadinessTable` 与 source/docs audit，归档 N228 前 release-candidate diff inventory。归档快照显示 48 个 tracked modified、32 个 untracked、合计 80 项，tracked diff stat 为 48 files changed, 11521 insertions(+), 1413 deletions(-)；新增 archive 文档本身也需纳入后续 review/stage/commit set。验证后当前工作树为 48 个 tracked modified、33 个 untracked、合计 81 项，tracked diff stat 为 48 files changed, 11537 insertions(+), 1413 deletions(-)，因此 release candidate 仍 not tag-ready；本切片不改变 runtime 默认行为，不新增 dedicated deprecated runtime API opt-in flag，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接。下一步 P0.5-N229 转向 `release_candidate_stage_commit_and_full_regression_gate`。

2026-06-09 本轮增量（P0.5-N227：release candidate stabilization and deprecated runtime API flag/reduction decision）:

```bash
git status --short
git diff --stat
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonDeprecatedRuntimeAPI'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused deprecated runtime API audit、full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonDeprecatedRuntimeAPIReleaseCandidateFlagReductionDecisionTable` 与 source/docs audit，接收 N226 release-readiness evidence，并记录当前 release-candidate diff inventory。当前 `git status --short` 为 48 个 tracked modified、32 个 untracked、合计 80 项，`git diff --stat` tracked diff 为 48 files changed, 11505 insertions(+), 1413 deletions(-)，因此 release candidate 尚未 tag-ready；本切片不改变 runtime 默认行为，不新增 dedicated deprecated runtime API opt-in flag，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接。下一步 P0.5-N228 转向 `release_candidate_diff_archive_and_tag_readiness_audit`。

2026-06-09 本轮增量（P0.5-N226：current release gate completion evidence / release readiness audit）:

```bash
rg -n "RunLLMTurn|ExecuteTurn|Deprecated|current_release_gate|RunAgentLoop|ExecuteAgentLoopAction|AttachLLMTurnRunner|AttachTurnExecutor|retained_until_release_gate|opt-in" docs/release-notes docs/api docs/CyberAgent-Release-Checklist.md docs/CyberAgent-Release-Plan.md
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonDeprecatedRuntimeAPI'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test ./...
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go vet ./...
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go build ./cmd/cyberagent
git status --short
git diff --check
```

结果：release/API docs audit、focused/full daemon tests、runtime package test、全量 `go test ./...`、`go vet ./...`、frontend vitest/build、CLI build 与 diff whitespace check 通过；新增 `daemonDeprecatedRuntimeAPICurrentReleaseGateCompletionEvidenceTable` 与 source/docs audit，固化 release checklist、API/release docs、source audit、回归测试与当前 dirty working tree 证据。当前工作树仍包含滚动 tracked/untracked 变更，因此 release tag-ready 仍阻塞；本切片不改变 runtime 默认行为，不新增 dedicated deprecated runtime API opt-in flag，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接。下一步 P0.5-N227 转向 `release_candidate_stabilization_and_deprecated_runtime_api_flag_reduction_decision`。

2026-06-09 本轮增量（P0.5-N225：next-release-gate deprecated runtime API opt-in flag reassessment audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonDeprecatedRuntimeAPI'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonDeprecatedRuntimeAPINextReleaseGateReassessmentTable` 与 source audit，确认 current release gate completion evidence 未固化前不进入 dedicated opt-in flag / config / API 变更，`AttachLLMTurnRunner` / `AttachTurnExecutor` 继续默认挂接，`CYBERAGENT_RUNTIME` 仍仅作为 whole-engine rollback switch。下一步 P0.5-N226 转向 `current_release_gate_completion_evidence_and_release_readiness_audit`。

2026-06-09 本轮增量（P0.5-N224：post-release-gate deprecated runtime API opt-in flag decision audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonDeprecatedRuntimeAPI'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonDeprecatedRuntimeAPIPostReleaseGateOptInFlagDecisionTable` 与 source audit，确认 current release gate 未完成前不新增显式 deprecated runtime API opt-in compatibility flag，`agentManager.AttachEngine` 仍默认挂接 `AttachLLMTurnRunner` / `AttachTurnExecutor`，现有 `CYBERAGENT_RUNTIME` 仅保持 whole-engine rollback switch。下一步 P0.5-N225 转向 `next_release_gate_deprecated_runtime_api_opt_in_flag_reassessment`。

2026-06-09 本轮增量（P0.5-N223：deprecated runtime API release/API docs + opt-in flag audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonDeprecatedRuntimeAPIReleaseDocs|TestDaemonDeprecatedRuntimeAPIPolicy'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonDeprecatedRuntimeAPIReleaseDocsOptInAuditTable` 与 release-docs source audit，确认 `runtime.Engine.RunLLMTurn` / `runtime.Engine.ExecuteTurn` 的 Deprecated、`current_release_gate` migration window、`RunAgentLoop` / `ExecuteAgentLoopAction` replacement、`AttachLLMTurnRunner` / `AttachTurnExecutor` 默认 `retained_until_release_gate` policy 已同步到 release notes、API contract、release checklist 与 release plan。当前不新增 opt-in compatibility flag，默认 compatibility attachment 保持到 release gate；下一步 P0.5-N224 转向 `post_release_gate_deprecated_runtime_api_opt_in_flag_decision`。

2026-06-09 本轮增量（P0.5-N222：deprecated runtime API policy / migration window audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus|TestDaemonRuntimeCompatibilityTrampolineCallerSurfacesDocumentPostCleanupState|TestDaemonLLMTurnRunnerCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonLLMTurnRunnerCompatibilitySourceAuditKeepsMainPathOnAgentLoopRunner|TestDaemonTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonTurnExecutorCompatibilitySourceAuditKeepsMainPathsOnAgentLoopRunner|TestDaemonPlannerActionExecutionTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonPlannerActionExecutionTurnExecutorCompatibilitySourceAuditKeepsMainPathOnAgentLoopRunner|TestDaemonExecuteTurnAPICompatibilityRetirementAuditMarksDeprecatedExternalSeam|TestDaemonExecuteTurnAPICompatibilitySourceAuditMarksDeprecatedExternalOnly|TestDaemonDeprecatedRuntimeAPIPolicyUnifiesRunLLMTurnAndExecuteTurn|TestDaemonDeprecatedRuntimeAPIPolicySourceAuditKeepsRuntimeAPIsExternalOnly'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonDeprecatedRuntimeAPIPolicyTable` 与统一 source audit，确认 `RunLLMTurn` / `ExecuteTurn` 均为 Deprecated external engine seam only，非测试 direct call 仅剩 runtime API dispatch，`AttachLLMTurnRunner` / `AttachTurnExecutor` 仅在 `agentManager.AttachEngine` 默认 compatibility 挂接。policy 将 migration window 固化为 `current_release_gate`，默认 attachment 保持到 release gate，opt-in/removal 推迟到 release/API docs 补齐之后。下一步 P0.5-N223 转向 `deprecated_runtime_api_release_docs_and_opt_in_flag_audit`。

2026-06-09 本轮增量（P0.5-N221：ExecuteTurn / TurnExecutor API deprecation/removal gate audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go internal/agent/runtime/turn_executor.go
rg -n "Deprecated:|ExecuteTurn|AttachTurnExecutor|daemonExecuteTurnAPICompatibility" internal/agent/runtime/turn_executor.go internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go internal/interfaces/daemon/agent_runtime.go internal/interfaces/daemon/agent_turn_executor.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus|TestDaemonRuntimeCompatibilityTrampolineCallerSurfacesDocumentPostCleanupState|TestDaemonTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonTurnExecutorCompatibilitySourceAuditKeepsMainPathsOnAgentLoopRunner|TestDaemonPlannerActionExecutionTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonPlannerActionExecutionTurnExecutorCompatibilitySourceAuditKeepsMainPathOnAgentLoopRunner|TestDaemonExecuteTurnAPICompatibilityRetirementAuditMarksDeprecatedExternalSeam|TestDaemonExecuteTurnAPICompatibilitySourceAuditMarksDeprecatedExternalOnly'
git diff --check
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonExecuteTurnAPICompatibilityRetirementTable` 与 source audit，确认非测试 `ExecuteTurn` direct call 仍仅剩 runtime API dispatch，`AttachTurnExecutor` 仅在 `agentManager.AttachEngine` 默认挂接；`TurnExecutor` / `TurnExecutorFunc` / `AttachTurnExecutor` / `Engine.TurnExecutor` / `Engine.ExecuteTurn` 已标记 Deprecated，三个 active TurnExecutor trampoline 均 external-only active compatibility，但由于外部 API migration policy 尚未定义，默认 attachment 不进入删除候选。下一步 P0.5-N222 转向 `execute_turn_api_deprecation_marker_and_external_migration_window`，统一 deprecated `RunLLMTurn` / `ExecuteTurn` policy 与迁移窗口。

2026-06-09 本轮增量（P0.5-N220：planner_action_execution TurnExecutor explicit compatibility trampoline audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go internal/agent/runtime/turn_executor.go
rg -n "ExecuteTurn|AttachTurnExecutor|executePlannerActionExecutionTurn|runRuntimePlannerActionExecution|AgentLoopModePlannerActionExecution" internal --glob '*.go' --glob '!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus|TestDaemonRuntimeCompatibilityTrampolineCallerSurfacesDocumentPostCleanupState|TestDaemonTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonTurnExecutorCompatibilitySourceAuditKeepsMainPathsOnAgentLoopRunner|TestDaemonPlannerActionExecutionTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonPlannerActionExecutionTurnExecutorCompatibilitySourceAuditKeepsMainPathOnAgentLoopRunner|TestDaemonPlannerActionExecutionTurnCompatibilityRetirementAuditSelectsDirectLegacyBoundary'
git diff --check
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；`planner_action_execution_turn_executor` 已加入 active compatibility trampoline / ownership audit，新增 planner action TurnExecutor compatibility retirement table 与 source audit，确认非测试 `ExecuteTurn` direct call 仍仅剩 runtime API dispatch，`AttachTurnExecutor` 仅在 `agentManager.AttachEngine` 默认挂接；内部 `runRuntimePlannerActionExecution` 主路径构造 `AgentLoopModePlannerActionExecution` 并进入 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`，`executePlannerActionExecutionTurn` 仅作为 external `ExecuteTurn` compatibility branch 保留，且目标已收敛为 `daemonAgentLoopActionExecutor.ExecuteAgentLoopAction`。下一步 P0.5-N221 转向 `execute_turn_api_external_compatibility_removal_readiness`，审计 `ExecuteTurn` / `TurnExecutor` API 整体 deprecation/removal gate。

2026-06-09 本轮增量（P0.5-N219：TurnExecutor deterministic/replan compatibility trampoline retirement audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
rg -n "ExecuteTurn|AttachTurnExecutor|daemonAgentTurnExecutor|runRuntimeDeterministicRouteWithToolServices|runRuntimeReplanActionContinuationAgentLoop" internal --glob '*.go' --glob '!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus|TestDaemonRuntimeCompatibilityTrampolineCallerSurfacesDocumentPostCleanupState|TestDaemonLLMTurnRunnerCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonLLMTurnRunnerCompatibilitySourceAuditKeepsMainPathOnAgentLoopRunner|TestDaemonTurnExecutorCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonTurnExecutorCompatibilitySourceAuditKeepsMainPathsOnAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner'
git diff --check
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonTurnExecutorCompatibilityRetirementTable` 与 source audit，确认非测试 `ExecuteTurn` direct call 仅剩 `runtime.Engine.ExecuteTurn` API 内部 dispatch，`AttachTurnExecutor` 仅在 `agentManager.AttachEngine` 默认挂接 compatibility seam；`SendMessage` / deterministic route / follow-up replan continuation 主路径不再直接调用 `ExecuteTurn`，继续经 `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`。deterministic_route 与 replan_action_continuation TurnExecutor branch 保留为 external engine seam only，当前不进入 removal candidate；下一步 P0.5-N220 转向 `planner_action_execution_turn_executor_compatibility_trampoline_retirement_readiness`，审计 explicit planner_action_execution `ExecuteTurn` compatibility trampoline。

2026-06-09 本轮增量（P0.5-N218：deprecated LLMTurnRunner compatibility trampoline retirement audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
rg -n "RunLLMTurn|AttachLLMTurnRunner|daemonAgentLLMTurnRunner" internal --glob '*.go' --glob '!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus|TestDaemonRuntimeCompatibilityTrampolineCallerSurfacesDocumentPostCleanupState|TestDaemonLLMTurnRunnerCompatibilityRetirementAuditNarrowsToExternalSeam|TestDaemonLLMTurnRunnerCompatibilitySourceAuditKeepsMainPathOnAgentLoopRunner|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner'
git diff --check
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonLLMTurnRunnerCompatibilityRetirementTable` 与 source audit，确认非测试 `RunLLMTurn` caller 仅剩 runtime API dispatch，`AttachLLMTurnRunner` 仅在 `agentManager.AttachEngine` 默认挂接 deprecated compatibility seam，`SendMessage` LLM 主路径不调用 `RunLLMTurn` 而继续经 `runRuntimeLLMToolLoopAgentLoop` → `runRuntimeAgentLoopResult` / `Engine.RunAgentLoop`。`daemonAgentLLMTurnRunner` 保留为 external engine seam only，当前不进入 removal candidate；下一步 P0.5-N219 转向 `turn_executor_compatibility_trampoline_retirement_readiness`，审计 deterministic/replan `ExecuteTurn` compatibility trampolines。

2026-06-09 本轮增量（P0.5-N217：attached-engine LLMStageRunner missing/detached fallback narrowing）:

```bash
gofmt -w internal/interfaces/daemon/agent_llm_stage_runner.go internal/interfaces/daemon/agent_llm_stage_runner_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonLLMStageCallerFallbackOwnershipTableCoversReasons|TestDaemonLLMStageCallerFallbackBoundaryCoversStages|TestDaemonLLMStageRunnerNotAttachedRetirementAuditNarrowsFallback|TestAgentManagerRunRuntimeAgentLLMStageToolSynthesisFallbackBoundary|TestAgentManagerRunRuntimeAgentLLMStageRunnerNotAttachedBlocksWithoutLegacyFallback|TestAgentManagerRunRuntimeAgentLLMStageInvalidPayloadDoesNotFallback'
git diff --check
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；`runRuntimeAgentLLMStage` 现在仅在 runtime.Engine 缺失时允许 legacy planner_chat/tool_synthesis fallback，attached-engine `LLMStageRunner()==nil` 与 `ErrLLMStageRunnerNotAttached` result 均规范化为 `runtime.ErrLLMStageRunnerNotAttached` blocked/error，不再触发 legacy `handleAgentLLMChat` / `synthesizeAgentToolResults` fallback；新增 `daemonLLMStageRunnerNotAttachedRetirementTable` 与 focused test 固化 no-engine legacy opt-out fallback retained / missing runner blocked / detached result blocked 三段语义，并将下一步边界推进到 `llm_turn_runner_compatibility_trampoline_retirement_readiness`。`P0.5-N217` 标记完成，下一步 P0.5-N218 审计 deprecated LLMTurnRunner compatibility trampoline。

2026-06-09 本轮增量（P0.5-N216：attached-engine AgentLoopRunner missing/detached fallback narrowing）:

```bash
gofmt -w internal/interfaces/daemon/agent_loop_runner.go internal/interfaces/daemon/agent_loop_runner_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopCallerFallbackOwnershipTableCoversReasons|TestDaemonAgentLoopCallerFallbackBoundaryCoversModes|TestDaemonAgentLoopNoEngineDirectFallbackRetirementAuditScopesFallback|TestDaemonAgentLoopRunnerNotAttachedRetirementAuditNarrowsFallback|TestAgentManagerRunRuntimeAgentLoopResultDirectFallbackBoundary|TestAgentManagerRunRuntimeAgentLoopResultRunnerNotAttachedBlocksWithoutDirectFallback|TestAgentManagerRunRuntimeAgentLoopResultUnsupportedModeStaysDirectFallbackError|TestAgentManagerRunRuntimeAgentLoopResultDoesNotFallbackForRunnerHardError'
git diff --check
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；`runRuntimeAgentLoopResult` 现在仅在 runtime.Engine 缺失时允许 no-engine direct daemon fallback，attached-engine `AgentLoopRunner()==nil` 与 `ErrAgentLoopRunnerNotAttached` result 均规范化为 `runtime.ErrAgentLoopRunnerNotAttached` blocked/error，不再触发 `runDaemonAgentLoopDirectResult`；新增 `daemonAgentLoopRunnerNotAttachedRetirementTable` 与 focused test 固化 no-engine legacy opt-out fallback retained / missing runner blocked / detached result blocked 三段语义，并将下一步边界推进到 `llm_stage_runner_not_attached_fallback_retirement_readiness`。`P0.5-N216` 标记完成，下一步 P0.5-N217 收敛 attached-engine LLMStageRunner missing / `ErrLLMStageRunnerNotAttached` fallback。

2026-06-09 本轮增量（P0.5-N215：planner action execution no-engine AgentLoop direct fallback retirement audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_loop_runner.go internal/interfaces/daemon/agent_loop_runner_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopCallerFallbackOwnershipTableCoversReasons|TestDaemonAgentLoopCallerFallbackBoundaryCoversModes|TestDaemonAgentLoopNoEngineDirectFallbackRetirementAuditScopesFallback|TestAgentManagerRunRuntimeAgentLoopResultDirectFallbackBoundary|TestAgentManagerRunRuntimePlannerActionExecutionNoEngineKeepsDirectLegacyFallback'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonAgentLoopNoEngineDirectFallbackRetirementTable` 与 helper/test，固化 production `NewServer` 默认启用 runtime engine 并调用 `AttachEngine`，`daemonAgentLoopRunner` / `daemonAgentLoopActionExecutor` 会在默认生产路径挂接；显式 `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0` 仍是 documented rollback path，因此 no-engine direct fallback 暂不转 blocked/error，planner_action_execution 仅在 runtime.Engine 缺失时经 `runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult -> runAgentPlannerActionExecutionAgentLoopResult -> daemonAgentLoopActionExecutor` 进入 direct legacy fallback。`P0.5-N215` 标记完成，下一步 P0.5-N216 收敛 attached-engine `AgentLoopRunner` missing / `ErrAgentLoopRunnerNotAttached` fallback。

2026-06-09 本轮增量（P0.5-N214：planner action execution missing/detached ActionExecutor fallback narrowing）:

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionFallbackOwnershipTableCoversChain|TestDaemonPlannerActionExecutionFallbackBoundaryCoversChain|TestDaemonPlannerActionExecutionDirectLegacyRetirementAuditScopesExecutorEntrypoint|TestDaemonPlannerActionExecutionActionExecutorMissingRetirementAuditNarrowsFallback|TestAgentManagerRunRuntimePlannerActionExecutionActionExecutorFallbackBoundary|TestAgentManagerRunRuntimePlannerActionExecutionNoEngineKeepsDirectLegacyFallback|TestAgentManagerRunRuntimePlannerActionExecutionMissingActionExecutorBlocksWithoutTurnExecutorFallback|TestAgentManagerRunRuntimePlannerActionExecutionActionExecutorNotAttachedBlocksWithoutDirectLegacyFallback'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；`runRuntimePlannerActionExecutionActionExecutor` 仅在 runtime.Engine 缺失时允许 `daemonAgentLoopActionExecutor` direct legacy fallback，attached-engine missing ActionExecutor 与 `ErrAgentLoopActionExecutorNotAttached` result 均规范化为 blocked/error，不再触发 direct legacy side effects；新增 `daemonPlannerActionExecutionActionExecutorMissingRetirementTable` 固化 no-engine fallback retained / missing executor blocked / detached result blocked 三段语义，并将下一步边界推进到 no-engine AgentLoop direct fallback retirement readiness。`P0.5-N214` 标记完成，下一步 P0.5-N215 审计 no-engine AgentLoop direct fallback 是否可继续收窄。

2026-06-09 本轮增量（P0.5-N213：planner action execution direct legacy executor fallback scope audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_action_executor.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionDirectLegacyRetirementAuditScopesExecutorEntrypoint|TestDaemonPlannerActionExecutionTurnCompatibilityRetirementAuditSelectsDirectLegacyBoundary|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonAgentLoopActionExecutorCoversHeuristicRoute|TestAgentManagerRunRuntimePlannerActionExecutionMissingActionExecutorBypassesTurnExecutor|TestAgentManagerRunRuntimePlannerActionExecutionActionExecutorNotAttachedBypassesTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；heuristic route 与外部 `ExecuteTurn(kind=planner_action_execution)` compatibility trampoline 均改为经 `daemonAgentLoopActionExecutor.ExecuteAgentLoopAction` 进入 runtime `AgentLoopActionExecutor` contract，production `daemonPlannerActionExecutionExecutorForManager(...).ExecutePlannerAction` 直接调用面缩到 `daemonAgentLoopActionExecutor` implementation-only；新增 `daemonPlannerActionExecutionDirectLegacyRetirementTable` 与 focused source audit，确认 registry action side effects 仍由 8 个 runtime action-service adapters 覆盖。`P0.5-N213` 标记完成，下一步 P0.5-N214 审计 missing/detached ActionExecutor fallback 是否可进一步缩窄。

2026-06-09 本轮增量（P0.5-N212：planner action execution TurnExecutor compatibility fallback retirement）:

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_action_gate_result_test.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionFallbackOwnershipTableCoversChain|TestDaemonPlannerActionExecutionFallbackBoundaryCoversChain|TestDaemonPlannerActionExecutionRegistryAdapterCompletionAuditSelectsNextFallbackBoundary|TestDaemonPlannerActionExecutionTurnCompatibilityRetirementAuditSelectsDirectLegacyBoundary|TestAgentManagerRunRuntimePlannerActionExecutionMissingActionExecutorBypassesTurnExecutor|TestAgentManagerRunRuntimePlannerActionExecutionActionExecutorNotAttachedBypassesTurnExecutor|TestAgentManagerTurnExecutionCallerInvalidPayloadAudit'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisFailureEgress|TestAgentManagerRunAgentLLMToolLoopResultStopsAtMaxLoops|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest|TestDaemonAgentLoopRunnerExecutesLLMToolOuterLoopFromRuntimeRequest|TestDaemonAgentLoopRunnerExecutesReplanContinuationOuterLoopFromRuntimeRequest|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerExecuteAgentPlannerActionsDispatchesPlannerActionsThroughTurnExecutor|TestAgentManagerRunAgentPlannerActionReplanCycleExecutesActionSnapshot|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopKeepsCyclePlanHandleBetweenAutoContinueSteps|TestAgentManagerRunAgentReplanActionContinuationKeepsCycleStateBetweenAutoContinueSteps'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；删除内部 `runRuntimePlannerActionExecutionTurn` compatibility fallback helper，`runAgentPlannerActionExecutionAgentLoopResult` 在 ActionExecutor missing / `ErrAgentLoopActionExecutorNotAttached` 时直接进入 `daemonAgentLoopActionExecutor.ExecuteAgentLoopAction`，并保留 `daemonAgentTurnExecutor.ExecuteTurn(kind=planner_action_execution)` 作为外部显式兼容 trampoline；迁移仍依赖 planner action TurnExecutor fallback 的 action batch / shared cycle / loop runner 测试到 runtime `AgentLoopActionExecutor` test adapter。`P0.5-N212` 标记完成，下一步 P0.5-N213 审计 direct legacy executor fallback 是否可进一步缩窄或退休。

2026-06-09 本轮增量（P0.5-N211：registry action-service adapter completion audit / planner action executor fallback 边界选择）:

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutionRegistryAdapterCompletionAuditSelectsNextFallbackBoundary|TestDaemonPlannerActionExecutionFallbackBoundaryCoversChain|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonPlannerActionExecutionRegistryAdapterCompletionTable` 与 helper/test，固化 registry action-service adapter completion：所有 registry side-effect groups 均为 runtime capability adapter attached，`agent_registry_handlers.go` 已无 direct `rt.Manager.handleAgent*` orchestration，下一处可收敛边界选为 `turn_executor_compatibility_fallback_retirement_readiness`。`P0.5-N211` 标记完成，下一步 P0.5-N212 审计 `runRuntimePlannerActionExecutionTurn` 兼容 fallback 是否可旁路或缩窄。

2026-06-09 本轮增量（P0.5-N210：run-context/approval action service capability 下沉）:

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/interfaces/daemon/agent_run_context_approval_action_service.go internal/interfaces/daemon/agent_run_context_approval_action_service_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingWithRunContextApprovalActionServiceInjectsRuntimeCapability|TestExecuteRegistryHandlerAttachesRunContextApprovalActionServiceCapability|TestRunContextApprovalRegistryHandlersUseRuntimeActionServiceCapability|TestRunContextApprovalRegistryHandlerSourceAvoidsDirectManagerRunContextApprovalCalls|TestRegistryHandlerRunContextApprovalBatchUsesRuntimeActionServiceCapabilities|TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 runtime `AgentLoopActionServiceRunContextApproval` capability、`daemonRunContextApprovalActionService` / `daemonRunContextApprovalManagerActionService` adapter 与 binding 注入，registry dispatch 在 handler 调用前注入 run-context approval action capability；`runs.context` / `reports.draft` / `runs.approvals.list` / `runs.approvals.approve` / `runs.approvals.deny` / `runs.resume_approved` handlers 统一经 `runContextApprovalActionServiceFromRegistryRuntime()` 获取服务并把 runtime resolver 传给 adapter，以保留 RunService/ApprovalService capability 读取；新增 binding 注入、adapter 注入、run-context/approval handlers fake capability 与 source audit 覆盖，ownership audit 将 run-context/approval 组标记为 runtime capability adapter attached，并确认 `agent_registry_handlers.go` 已无 direct `rt.Manager.handleAgent*` orchestration 调用。`P0.5-N210` 标记完成。下一步 P0.5-N211 做 registry action-service adapter completion audit 并选择 planner action executor fallback retirement 边界。

2026-06-09 本轮增量（P0.5-N209：daemon status action service capability 下沉）:

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/interfaces/daemon/agent_daemon_status_action_service.go internal/interfaces/daemon/agent_daemon_status_action_service_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingWithDaemonStatusActionServiceInjectsRuntimeCapability|TestExecuteRegistryHandlerAttachesDaemonStatusActionServiceCapability|TestDaemonStatusRegistryHandlersUseRuntimeActionServiceCapability|TestDaemonStatusRegistryHandlerSourceAvoidsDirectManagerStatusCalls|TestRegistryHandlerFirstBatchUsesRuntimeActionServiceCapabilities|TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 runtime `AgentLoopActionServiceDaemonStatusAction` capability、`daemonStatusActionService` / `daemonStatusManagerActionService` adapter 与 binding 注入，registry dispatch 在 handler 调用前注入 daemon status action capability；`daemon.health` / `plugins.list` / `runs.list` handlers 统一经 `daemonStatusActionServiceFromRegistryRuntime()` 获取服务并把 runtime resolver 传给 adapter，以保留 VersionService/PluginService/RunService capability 读取；新增 binding 注入、adapter 注入、daemon status handlers fake capability 与 source audit 覆盖，ownership audit 将 daemon status 组标记为 runtime capability adapter attached，并把下一处 selected capability 推进到 `run_context_approval_action` / run-context 与 approvals 组。`P0.5-N209` 标记完成。下一步 P0.5-N210 下沉 run-context/approval action service capability。

2026-06-09 本轮增量（P0.5-N208：schedule action service capability 下沉）:

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/interfaces/daemon/agent_schedule_action_service.go internal/interfaces/daemon/agent_schedule_action_service_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingWithScheduleActionServiceInjectsRuntimeCapability|TestExecuteRegistryHandlerAttachesScheduleActionServiceCapability|TestScheduleRegistryHandlersUseRuntimeActionServiceCapability|TestScheduleRegistryHandlerSourceAvoidsDirectManagerScheduleCalls|TestRegistryHandlerScheduleBatchUsesRuntimeActionServiceCapabilities|TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 runtime `AgentLoopActionServiceScheduleAction` capability、`daemonScheduleActionService` / `daemonScheduleManagerActionService` adapter 与 binding 注入，registry dispatch 在 handler 调用前注入 schedule action capability；`schedule.create` / `schedule.list` / `schedule.delete` handler 统一经 `scheduleActionServiceFromRegistryRuntime()` 获取服务并把 runtime resolver 传给 adapter，以保留 ScheduleManager capability 读写；新增 binding 注入、adapter 注入、schedule.* handler fake capability 与 source audit 覆盖，ownership audit 将 schedule 组标记为 runtime capability adapter attached，并把下一处 selected capability 推进到 `daemon_status_action` / `daemon.health`、`plugins.list`、`runs.list`。`P0.5-N208` 标记完成。下一步 P0.5-N209 下沉 daemon status action service capability。

2026-06-09 本轮增量（P0.5-N207：web scan action service capability 下沉）:

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_web_scan_action_service.go internal/interfaces/daemon/agent_web_scan_action_service_test.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingWithWebScanActionServiceInjectsRuntimeCapability|TestExecuteRegistryHandlerAttachesWebScanActionServiceCapability|TestWebScanRegistryHandlerUsesRuntimeActionServiceCapability|TestWebScanRegistryHandlerSourceAvoidsDirectManagerScanCalls|TestRegistryHandlerScanBatchUsesRuntimeActionServiceCapabilities|TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 runtime `AgentLoopActionServiceWebScan` capability、`daemonWebScanActionService` / `daemonWebScanManagerActionService` adapter 与 binding 注入，registry dispatch 在 handler 调用前注入 web scan capability；`scan.web.safe` handler 统一经 `webScanActionServiceFromRegistryRuntime()` 获取服务并把 runtime resolver 传给 adapter，以保留 JobManager capability 扫描任务创建；新增 binding 注入、adapter 注入、scan.web.safe handler fake capability 与 source audit 覆盖，ownership audit 将 scan 组标记为 runtime capability adapter attached，并把下一处 selected capability 推进到 `schedule_action` / `schedule.*`。`P0.5-N207` 标记完成。下一步 P0.5-N208 下沉 `schedule.*` schedule action service capability。

2026-06-09 本轮增量（P0.5-N206：web search action service capability 下沉）:

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/interfaces/daemon/agent_web_search_action_service.go internal/interfaces/daemon/agent_web_search_action_service_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestAgentLoopToolServiceBindingWithWebSearchActionServiceInjectsRuntimeCapability|TestExecuteRegistryHandlerAttachesWebSearchActionServiceCapability|TestWebSearchRegistryHandlerUsesRuntimeActionServiceCapability|TestWebSearchRegistryHandlerSourceAvoidsDirectManagerWebSearchCalls|TestRegistryHandlerWebSearchUsesRuntimeActionServiceCapabilities'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 runtime `AgentLoopActionServiceWebSearch` capability、`daemonWebSearchActionService` / `daemonWebSearchManagerActionService` adapter 与 binding 注入，registry dispatch 在 handler 调用前注入 web search capability；`web.search` handler 统一经 `webSearchActionServiceFromRegistryRuntime()` 获取服务并把 runtime resolver 传给 adapter，以保留 ProviderStore capability 配置读取；新增 binding 注入、adapter 注入、web.search handler fake capability 与 source audit 覆盖，ownership audit 将 web search 组标记为 runtime capability adapter attached，并把下一处 selected capability 推进到 `web_scan` / `scan.web.safe`。`P0.5-N206` 标记完成。下一步 P0.5-N207 下沉 `scan.web.safe` web scan action service capability。

2026-06-09 本轮增量（P0.5-N205：registry runtime capability consumers completion audit）:

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；`daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable` 已把剩余 direct registry manager orchestration 调用拆为 `scan_registry_handler`、`health_plugins_runs_registry_handlers`、`schedule_registry_handlers`、`web_search_registry_handler`、`run_context_approval_registry_handlers` 五组，并继续覆盖 `agent_registry_handlers.go` 中所有 `rt.Manager.handleAgent*` 调用；workspace/network/port-scan 三组保持 runtime capability adapter attached，下一处 selected capability 明确为 `web_search`。`P0.5-N205` 标记完成。下一步 P0.5-N206 下沉 `web.search` web search action service capability。

2026-06-09 本轮增量（P0.5-N204：port scanner action service capability 下沉）:

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/interfaces/daemon/agent_port_scanner_action_service.go internal/interfaces/daemon/agent_port_scanner_action_service_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestAgentLoopToolServiceBindingWithPortScannerActionServiceInjectsRuntimeCapability|TestExecuteRegistryHandlerAttachesPortScannerActionServiceCapability|TestPortScannerRegistryHandlerUsesRuntimeActionServiceCapability|TestPortScannerRegistryHandlerSourceAvoidsDirectManagerPortScanCalls|TestRoutePromptSafePortScanUsesHeuristicRegistryDispatch'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 runtime `AgentLoopActionServicePortScanner` capability、`daemonPortScannerActionService` / `daemonPortScannerManagerActionService` adapter 与 binding 注入，registry dispatch 在 handler 调用前注入 port scanner capability；`ports.scan.safe` handler 统一经 `portScannerActionServiceFromRegistryRuntime()` 获取服务；新增 binding 注入、adapter 注入、port scanner handler fake capability 与 source audit 覆盖，ownership audit 将 port scan 组标记为 runtime capability adapter attached，且当前 workspace/network/port-scan side-effect groups 均已 capability-adapter-backed。`P0.5-N204` 标记完成。下一步 P0.5-N205 对剩余 registry runtime capability consumers 的 direct manager orchestration 调用做 completion audit 并选择下一处最小 adapter 边界。

2026-06-09 本轮增量（P0.5-N203：network request action service capability 下沉）:

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/interfaces/daemon/agent_network_request_action_service.go internal/interfaces/daemon/agent_network_request_action_service_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_registry_dispatch_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestAgentLoopToolServiceBindingWithNetworkRequestActionServiceInjectsRuntimeCapability|TestExecuteRegistryHandlerAttachesNetworkRequestActionServiceCapability|TestNetworkRequestRegistryHandlersUseRuntimeActionServiceCapability|TestNetworkRequestRegistryHandlerSourceAvoidsDirectManagerNetworkCalls|TestRegistryHandlerHTTPRequestUsesRuntimeAuthorizationContext'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 runtime `AgentLoopActionServiceNetworkRequest` capability、`daemonNetworkRequestActionService` / `daemonNetworkRequestManagerActionService` adapter 与 binding 注入，registry dispatch 在 handler 调用前注入 network request capability；`http.request` / `web.fetch` handlers 统一经 `networkRequestActionServiceFromRegistryRuntime()` 获取服务；新增 binding 注入、adapter 注入、network request handler fake capability 与 source audit 覆盖，ownership audit 将 network request 组标记为 runtime capability adapter attached，并把下一处 selected capability 推进到 `port_scanner` / `ports.scan.safe`。`P0.5-N203` 标记完成。下一步 P0.5-N204 下沉 `ports.scan.safe` port scanner action service capability。

2026-06-09 本轮增量（P0.5-N202：workspace action service capability 下沉）:

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_workspace_action_service_test.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_workspace_action_service.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestAgentLoopToolServiceBindingWithWorkspaceActionServiceInjectsRuntimeCapability|TestExecuteRegistryHandlerAttachesWorkspaceActionServiceCapability|TestWorkspaceRegistryHandlersUseRuntimeWorkspaceActionServiceCapability|TestWorkspaceRegistryHandlerSourceAvoidsDirectManagerWorkspaceCalls|TestRegistryHandlerRuntimeActionServiceResolverRequiresRuntimeCapabilities|TestRegistryHandlerDispatch|TestRoutePromptGenericWorkspaceUsesHeuristicRegistryDispatch'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 `daemonWorkspaceActionService` / `daemonWorkspaceManagerActionService` adapter，registry dispatch 在 handler 调用前注入 runtime `AgentLoopActionServiceWorkspace` capability，workspace handlers 统一经 `workspaceActionServiceFromRegistryRuntime()` 获取服务；新增 binding 注入、adapter 注入、workspace handler fake capability 与 source audit 覆盖，ownership audit 将 workspace 组标记为 runtime capability adapter attached，并把下一处 selected capability 推进到 `network_request`。`P0.5-N202` 标记完成。下一步 P0.5-N203 下沉 `http.request` / `web.fetch` network request action service capability。

2026-06-09 本轮增量（P0.5-N201：planner_action_execution runtime action service ownership 审计）：

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionRuntimeActionServiceOwnershipAuditCoversRegistryDispatch|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused/full daemon tests、runtime package test 与 diff whitespace check 通过；新增 runtime `AgentLoopActionServiceWorkspace` capability 名称，新增 `daemonPlannerActionExecutionRuntimeActionServiceOwnershipTable`，把 registry dispatch 中 direct `rt.Manager.handleAgent*` 调用分为已 runtime capability-owned 的 registry 批次、选定下一步下沉的 workspace 批次，以及后续 network request / port scan 批次；source audit 覆盖 `agent_registry_handlers.go` 中全部 direct manager handler calls，确保剩余 daemon-private side effects 显式登记。`P0.5-N201` 标记完成。下一步 P0.5-N202 下沉 workspace action service capability 与 daemon adapter。

2026-06-09 本轮增量（P0.5-N200：planner_action_execution action fallback ownership 固化）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionFallback|TestAgentManagerRunRuntimePlannerActionExecutionActionExecutor|TestAgentManagerRunRuntimePlannerActionExecutionTurnFallback|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes|TestDaemonTurnExecutionCallerFallbackOwnershipTableCoversReasons'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：focused/full daemon tests 与 diff whitespace check 通过；新增 `daemonPlannerActionExecutionFallbackOwnershipTable` / `daemonPlannerActionExecutionFallbackBoundaryTable`，显式登记 `runtime_engine_not_attached`、`agent_loop_action_executor_not_attached`、`turn_executor_compatibility_fallback` 与 `direct_legacy_executor` 均由 daemon legacy caller 兼容边界持有；`runRuntimePlannerActionExecutionActionExecutor` 现在通过 ownership helper 处理 engine/action executor 未挂接与 `ErrAgentLoopActionExecutorNotAttached` result fallback；`runRuntimePlannerActionExecutionTurn` 复用 pre-dispatch fallback helper 固化 missing TurnExecutor 语义；新增 focused 覆盖 action executor missing/not-attached 回落到 TurnExecutor、TurnExecutor missing 回落到 direct legacy executor、hard error 不被吞掉；completion audit 下一 seam 已更新为 `planner_action_execution_runtime_action_service_ownership`。`P0.5-N200` 标记完成。下一步 P0.5-N201 审计 planner_action_execution runtime action service ownership。

2026-06-09 本轮增量（P0.5-N199：AgentLoop direct daemon fallback ownership 固化）：

```bash
gofmt -w internal/interfaces/daemon/agent_loop_runner.go internal/interfaces/daemon/agent_loop_runner_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopCallerFallback|TestAgentManagerRunRuntimeAgentLoopResult|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：focused/full daemon tests 与 diff whitespace check 通过；新增 `daemonAgentLoopCallerFallbackOwnershipTable` / `daemonAgentLoopCallerFallbackBoundaryTable`，显式登记 `runtime_engine_not_attached` 与 `agent_loop_runner_not_attached` 两类 fallback reason 均由 daemon legacy caller 兼容边界持有；`runRuntimeAgentLoopResult` 现在通过 ownership helper 处理 pre-dispatch 与 `ErrAgentLoopRunnerNotAttached` result fallback；新增 direct fallback focused 覆盖，固化 `runRuntimeAgentLoopResult -> runDaemonAgentLoopDirectResult` 是五个 AgentLoop mode 的兼容 fallback 边界，非 runner-not-attached hard error 不会被吞掉；completion audit 下一 seam 已更新为 `planner_action_execution_fallback_chain_ownership`。`P0.5-N199` 标记完成。下一步 P0.5-N200 审计 `planner_action_execution` action execution fallback chain ownership。

2026-06-09 本轮增量（P0.5-N198：LLMStage tool_synthesis fallback ownership 固化）：

```bash
gofmt -w internal/interfaces/daemon/agent_llm_stage_runner.go internal/interfaces/daemon/agent_llm_stage_runner_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonLLMStageCallerFallback|TestAgentManagerRunRuntimeAgentLLMStage|TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：focused/full daemon tests 与 diff whitespace check 通过；新增 `daemonLLMStageCallerFallbackOwnershipTable` / `daemonLLMStageCallerFallbackBoundaryTable`，显式登记 `runtime_engine_not_attached` 与 `llm_stage_runner_not_attached` 两类 fallback reason 均由 daemon legacy caller 兼容边界持有；`runRuntimeAgentLLMStage` 现在通过 ownership helper 处理 pre-dispatch 与 `ErrLLMStageRunnerNotAttached` result fallback；新增 tool_synthesis focused 覆盖，固化 `runRuntimeAgentLLMToolSynthesis -> synthesizeAgentToolResults` 是 `planner_action_replan_cycle` synthesis 的兼容 fallback 边界；completion audit 下一 seam 已更新为 `agent_loop_direct_fallback_ownership`。`P0.5-N198` 标记完成。下一步 P0.5-N199 审计 `runRuntimeAgentLoopResult` direct daemon fallback ownership。

2026-06-09 本轮增量（P0.5-N197：shared cycle append/persist decision 下沉）：

```bash
gofmt -w internal/agent/runtime/agent_loop_replan_cycle_append.go internal/agent/runtime/agent_loop_replan_cycle_append_test.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopReplanCycle'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes|TestAgentManagerRunAgentPlannerActionReplanCycleSynthesisFailureEgress|TestDaemonAgentLoopRunnerExecutesSharedCycleFromRuntimeRequest'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：focused/full runtime 与 daemon tests 通过，diff whitespace check 通过；新增 runtime `AgentLoopReplanCycleAppendDecision` 与 action/warning/synthesis apply helpers，覆盖 action message/history/tool-call timing、empty-message tool-call timing、warning response-only append、synthesis planner append 与 clone isolation；`runAgentPlannerActionReplanCycleAgentLoop` 已改为消费 runtime decision，daemon adapter 只保留 append mutation dispatch/fallback 与 legacy DTO 转换；completion audit 的下一 seam 已更新为 `tool_synthesis_fallback_ownership`。`P0.5-N197` 标记完成。下一步 P0.5-N198 审计 tool_synthesis fallback ownership。

2026-06-09 本轮增量（P0.5-N196：engine-owned loop runner completion audit）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime'
```

结果：focused audit tests 通过；新增 `daemonAgentLoopRunnerCompletionAuditTable` 与 `TestDaemonAgentLoopRunnerCompletionAuditCoversEngineOwnedModes`，固化五个 AgentLoop mode 的 append/persist、tool synthesis、action execution 与 fallback ownership，并将下一处 runtime seam 明确为 `shared_cycle_append_persist`。`P0.5-N196` 标记完成。下一步 P0.5-N197 下沉 shared cycle append/persist decision。

2026-06-09 本轮增量（P0.5-N195：routerServices ingress retirement completion audit）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRouterServicesProductionIngressStaysAtWiringAndBindingBoundaries|TestAgentManagerRouterServicesIngressStaysAtHTTPWrappers|TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime'
```

结果：focused source-audit tests 通过；新增 `TestRouterServicesProductionIngressStaysAtWiringAndBindingBoundaries`，AST 扫描 production daemon `.go` 函数签名并固化 `routerServices` 参数只允许出现在 HTTP/router handlers、`agent_tool_service_binding.go` binding adapter，以及 `agentManager` 的 `ControlSession` / `DecideAgentAuthorization` / `SendMessage` 三个 HTTP-facing wrappers；配合 `TestAgentManagerRouterServicesIngressStaysAtHTTPWrappers` 证明本轮 internal helper/wrapper `routerServices` ingress retirement 已收口。`P0.5-N195` 标记完成。下一步 P0.5-N196 回到 engine-owned loop runner completion audit。

2026-06-09 本轮增量（P0.5-N194：SendMessage runtime tool service binding split）：

```bash
gofmt -w internal/interfaces/daemon/agent_session_flow.go internal/interfaces/daemon/agent_send_message_lifecycle_test.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageRunsInsideRuntimeTurnLifecycle|TestAgentManagerSendMessageNoLLMRouteUsesAgentLoopRunner|TestAgentManagerRouterServicesIngressStaysAtHTTPWrappers|TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
rg -n "routerServices" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
```

结果：focused daemon tests 与 production `routerServices` occurrence audit 通过；新增 `SendMessageWithToolServices`，HTTP-facing `SendMessage` 仅保留为 `routerServices` → `runtime.AgentLoopToolServiceBinding` wrapper，turn lifecycle callback 与 `runAgentSendMessageTurnWithToolServices` 全程复用同一 runtime binding。新增 `TestAgentManagerRouterServicesIngressStaysAtHTTPWrappers`，固化 `agentManager` production methods 中只有 `ControlSession` / `DecideAgentAuthorization` / `SendMessage` 三个 HTTP-facing wrapper 可直接接收 `routerServices`。`P0.5-N194` 标记完成。下一步 P0.5-N195 做剩余 `routerServices` ingress completion audit。

2026-06-09 本轮增量（P0.5-N193：control/auth/send-turn runtime tool service binding split）：

```bash
gofmt -w internal/interfaces/daemon/agent_session_flow.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerControlSessionJobCancelAppendUsesExternalTurnRef|TestAgentManagerDecideAgentAuthorizationApprovedActionAppendUsesExternalTurnRef|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestAgentManagerSendMessageNoLLMRouteUsesAgentLoopRunner|TestAgentManagerSendMessageActiveRunContinueUsesAgentLoopRunner|TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
! rg -n "func \(m \*agentManager\) runAgentSendMessageTurn\(" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
```

结果：focused daemon tests 与 send-turn removed-wrapper production 残留扫描通过；`ControlSessionWithToolServices` / `DecideAgentAuthorizationWithToolServices` 已作为 runtime binding 子入口落地，HTTP-facing wrapper 继续负责 `routerServices` → `AgentLoopToolServiceBinding` 转换；`ControlSession` job cancel 改为通过 runtime `JobManager` capability resolver；`runAgentSendMessageTurn` legacy `routerServices` ingress 已迁为 `runAgentSendMessageTurnWithToolServices`，send-turn 内部控制、active-run continue、deterministic route、LLM/replan request 均复用同一 runtime tool service binding。`P0.5-N193` 标记完成。下一步 P0.5-N194 评估 `SendMessage` 顶层是否也拆出 `SendMessageWithToolServices`。

2026-06-09 本轮增量（P0.5-N192：active-run continue routerServices ingress wrapper retirement）：

```bash
gofmt -w internal/interfaces/daemon/agent_run_handlers.go internal/interfaces/daemon/agent_session_flow.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
! rg -n "func \(m \*agentManager\) handleAgentActiveRunContinueForTurn\(" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、active-run continue removed-wrapper production 残留扫描、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`handleAgentActiveRunContinueForTurn` legacy `routerServices` ingress 已迁为 `handleAgentActiveRunContinueForTurnWithToolServices`，active-run continue 通过 runtime `RunService` capability resolver 读取 run context，并将同一 runtime `AgentLoopToolServiceBinding` 直传到 `routePromptForTurnWithToolServices`。`P0.5-N192` 标记完成。下一步 P0.5-N193 审计 `ControlSession` / `DecideAgentAuthorization` / `runAgentSendMessageTurn` 等剩余生产 `routerServices` ingress 是否可拆出更窄 runtime service binding 或 ActionServices-only 子入口。

2026-06-09 本轮增量（P0.5-N190/N191：LLM/replan/request adapter 与 planner action list routerServices ingress wrapper retirement）：

```bash
gofmt -w internal/interfaces/daemon/agent_llm_turn_runner.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_session_flow.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_session_mutator_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_tool_service_binding_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeLLMToolLoopDispatchesEngineRunner|TestAgentManagerRunRuntimeReplanActionContinuationDispatchesEngineRunner|TestAgentManagerRunAgentLLMToolLoopDispatchesSharedReplanCycle|TestAgentManagerRunAgentLLMToolLoopHandlesMultipleCycles|TestAgentManagerRunAgentReplanActionContinuationUsesRuntimeState|TestAgentManagerRunAgentReplanActionContinuationDispatchesSynthesisStage|TestAgentPlannerActionReplanCycleRequestRuntimeAdapterRoundTrip|TestDaemonAgentLoopRunnerUsesRuntimeLoopDTOForSharedCycle|TestExecuteAgentPlannerActionBatchDispatchesRuntimeTurnExecutor|TestExecuteAgentPlannerActionsRequestAuthorizationUsesRuntimeMutation|TestExecuteAgentPlannerActionsCapabilityBlockedUsesRuntimeMutation|TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
! rg -n "func agentLLMToolLoopRequestFromLegacy\\(|func agentReplanActionContinuationRequestFromLegacy\\(|func agentPlannerActionReplanCycleToolContextFromRouterServices\\(|func \\(m \\*agentManager\\) runAgentLLMToolLoop\\(|func \\(m \\*agentManager\\) runAgentReplanActionContinuation\\(|func \\(m \\*agentManager\\) executeAgentPlannerActions\\(" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
! rg -n "daemonAgentLoopToolServiceProvider|daemonToolServiceProviderContractName|daemonAgentLoopToolServiceProviderContract|\"provider_contract\"|\"provider_adapter\"" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试、production provider metadata 残留扫描、removed-wrapper production 残留扫描与 diff whitespace check 通过；`agentLLMToolLoopRequestFromLegacy` / `agentReplanActionContinuationRequestFromLegacy` / `agentPlannerActionReplanCycleToolContextFromRouterServices` 已迁为 `*WithToolServices` / runtime binding 入口并删除 legacy `routerServices` ingress；`runAgentLLMToolLoop` / `runAgentReplanActionContinuation` test-facing wrappers 与 `executeAgentPlannerActions` planner action list wrapper 也已迁到 `*WithToolServices`。`P0.5-N190` / `P0.5-N191` 标记完成。下一步 P0.5-N192 审计 active-run continue 的剩余 `routerServices` ingress。

2026-06-09 本轮增量（P0.5-N189：heuristic route / routePromptForTurn routerServices ingress wrapper retirement）：

```bash
gofmt -w internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_session_flow.go internal/interfaces/daemon/agent_run_handlers.go internal/interfaces/daemon/agent_helpers_test.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_tool_service_binding_test.go
rg -n "routePrompt\\(|routePromptForTurn\\(|routePromptWithRuntimeDeterministicRoute\\(|runRuntimeDeterministicRoute\\(|route[A-Za-z]+HeuristicActions\\(|routeHeuristicRegistryActions\\(" internal/interfaces/daemon -g'*.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionExecutorSeamCoversHeuristicRoute|TestRoutePromptUsesHeuristicRegistryDispatch|TestRoutePromptGenericWorkspaceUsesHeuristicRegistryDispatch|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionHelperOwnershipTableDocumentsRuntimeOwnedHelpers|TestDaemonTurnExecutionAgentLoopMigrationPreconditions|TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
! rg -n "func \\(m \\*agentManager\\) routePrompt\\(|func \\(m \\*agentManager\\) routePromptForTurn\\(|func \\(m \\*agentManager\\) routeHeuristicRegistryActions\\(|func \\(m \\*agentManager\\) runRuntimeDeterministicRoute\\(|func \\(m \\*agentManager\\) routePromptWithRuntimeDeterministicRoute\\(" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
! rg -n "daemonAgentLoopToolServiceProvider|daemonToolServiceProviderContractName|daemonAgentLoopToolServiceProviderContract|\"provider_contract\"|\"provider_adapter\"" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试、production provider metadata 残留扫描、removed-wrapper production 残留扫描与 diff whitespace check 通过；deterministic/heuristic route direct wrappers 已切到 runtime `AgentLoopToolServiceBinding`，production `routePrompt` / `routePromptForTurn` / `routeHeuristicRegistryActions` / `runRuntimeDeterministicRoute` / `routePromptWithRuntimeDeterministicRoute` legacy `routerServices` ingress wrappers 已删除，生产 caller 改为 `routePromptWithToolServices` / `routePromptForTurnWithToolServices` / `routePromptWithRuntimeDeterministicRouteWithToolServices` / `runRuntimeDeterministicRouteWithToolServices`。`P0.5-N189` 标记完成。下一步 P0.5-N190 审计 LLM/replan/request adapter 的剩余 `routerServices` ingress。

2026-06-09 本轮增量（P0.5-N188：planner action / registry dispatch routerServices ingress wrapper retirement）：

```bash
gofmt -w internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerDispatch|TestExecuteAgentPlannerActionResultReturnsRuntimeExecutorDTO|TestRegistryHandlerRuntimeUsesToolServiceBinding|TestRegistryHandlerRuntimeActionServiceResolverRequiresRuntimeCapabilities|TestRegistryBuiltinMissingHandlerFailsWithoutLegacyFallback|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
! rg -n "func \(m \*agentManager\) executeAgentPlannerActionResult\(|func \(m \*agentManager\) executeAgentRegistryHandlerResult\(" internal/interfaces/daemon -g'*.go'
! rg -n "executeAgentPlannerActionResult\(|executeAgentRegistryHandlerResult\(" internal/interfaces/daemon -g'*.go'
! rg -n "daemonAgentLoopToolServiceProvider|daemonToolServiceProviderContractName|daemonAgentLoopToolServiceProviderContract|\"provider_contract\"|\"provider_adapter\"" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试、production provider metadata 残留扫描、removed-wrapper 残留扫描与 diff whitespace check 通过；production `executeAgentPlannerActionResult` / `executeAgentRegistryHandlerResult` legacy `routerServices` ingress wrappers 已删除，direct registry dispatch 测试改为 `executeAgentPlannerActionResultWithToolServices` / `executeAgentRegistryHandlerResultWithToolServices` runtime binding 入口。audit / contract plan 文案同步为 `*WithToolServices` 端到端传递 runtime service binding 与 `AgentLoopAuthorization`，`P0.5-N188` 标记完成。下一步 P0.5-N189 审计 heuristic route / `routePromptForTurn` legacy callers 的剩余 `routerServices` ingress。

2026-06-09 本轮增量（P0.5-N187：routerServices roundtrip helper retirement）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding_compat_test.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingFromRouterServicesUsesActionServicesWithoutOpaqueHandle|TestAgentLoopToolServiceBindingRuntimeActionServicesTakePrecedenceOverOpaqueHandle|TestRegistryHandlerRuntimeActionServiceResolverRequiresRuntimeCapabilities|TestRegistryHandlerRuntimeUsesToolServiceBinding|TestAgentPlannerActionReplanCycleRequestRuntimeAdapterRoundTrip|TestAgentManagerRunRuntimeAgentPlannerActionReplanCycleDispatchesEngineRunner|TestDaemonPlannerActionExecutionSideEffectBoundaryClonesRuntimeInputs|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime'
rg -n "func routerServicesFromAgentLoopToolServiceBinding|routerServicesFromAgentLoopToolServiceBinding\\(" internal/interfaces/daemon -g'*.go'
! rg -n "daemonAgentLoopToolServiceProvider|daemonToolServiceProviderContractName|daemonAgentLoopToolServiceProviderContract|\"provider_contract\"|\"provider_adapter\"" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试、production provider metadata 残留扫描与 diff whitespace check 通过；loop runner / registry dispatch / turn executor / binding 测试已从 `routerServicesFromAgentLoopToolServiceBinding` roundtrip 改为 direct `ActionServices` capability assertions，helper implementation 已删除，源码中仅保留 source-audit forbidden string。audit / contract plan 文案同步为 provider/routerServices reconstruction retired from production and test runtime paths，`P0.5-N187` 标记完成。下一步 P0.5-N188 评估 planner action executor / registry dispatch 入口是否可直接接收 runtime service binding 或 ActionServices，继续减少 daemon-private `routerServices` ingress。

2026-06-09 本轮增量（P0.5-N186：test-only Handle/provider fixture retirement）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingFromRouterServicesUsesActionServicesWithoutOpaqueHandle|TestAgentLoopToolServiceBindingRuntimeActionServicesTakePrecedenceOverOpaqueHandle|TestRegistryHandlerRuntimeActionServiceResolverRequiresRuntimeCapabilities|TestRegistryHandlerFirstBatchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerScheduleBatchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerWebSearchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerScanBatchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerRunContextApprovalBatchUsesRuntimeActionServiceCapabilities|TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonPlannerActionExecutionSideEffectBoundaryDoesNotReconstructLegacyServices'
! rg -n "daemonAgentLoopToolServiceProvider|daemonToolServiceProviderContractName|daemonAgentLoopToolServiceProviderContract|\"provider_contract\"|\"provider_adapter\"" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试、production provider metadata 残留扫描与 diff whitespace check 通过；test-only typed-provider contract table/interface、provider reconstruction helper 与 explicit Handle fallback assertions 已退休，`agent_tool_service_binding_compat_test.go` 仅保留从 runtime `ActionServices` 回转 `routerServices` 的测试 roundtrip helper；Handle-only binding 在 registry runtime 与 test-only helper 均被拒绝。audit table 文案已同步为 ActionServices-only / Handle fallback retired，`P0.5-N186` 标记完成。下一步 P0.5-N187 收敛剩余 `routerServicesFromAgentLoopToolServiceBinding` 调用点，优先让 loop runner / turn executor 测试直接断言 `ActionServices` capabilities。

2026-06-09 本轮增量（P0.5-N185：historical typed-provider contract test-only fixture）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_tool_service_binding_compat_test.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime|TestAgentLoopToolServiceBindingFromRouterServicesUsesActionServicesWithoutOpaqueHandle|TestDaemonAgentLoopToolServiceProviderContractDocumentsRouterServiceFieldMapping|TestRegistryHandlerRuntimeActionServiceResolverRequiresRuntimeCapabilities|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
! rg -n "daemonAgentLoopToolServiceProvider|daemonToolServiceProviderContractName|daemonAgentLoopToolServiceProviderContract|\"provider_contract\"|\"provider_adapter\"" internal/interfaces/daemon -g'*.go' -g'!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；历史 `daemonAgentLoopToolServiceProvider` interface / contract table 已从 production `agent_tool_service_binding.go` 移入 `agent_tool_service_binding_compat_test.go` test-only fixture，production `agentLoopToolServiceBindingFromRouterServices` / `daemonActionServiceSetFromRouterServices` metadata 不再写入 `provider_contract` / `provider_adapter`，registry 测试也不再构造 provider contract metadata。源码扫描测试继续扫描所有非测试 daemon `.go` 文件，并新增拒绝 typed-provider contract 名称与 provider metadata 重新进入生产面。audit table 将 historical provider contract 归档为 `tool_service_provider_contract_test_fixture` runtime landed 项，runtime-contract landed 计数更新为 19，typed-provider landed stage 退休。下一步 P0.5-N186 收敛 test-only explicit Handle/provider/routerServices roundtrip assertions。

2026-06-09 本轮增量（P0.5-N184：tool service compatibility helpers test-only）:

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_tool_service_binding_compat_test.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestToolServiceCompatibilityHelpersStayOutOfProductionRuntime|TestAgentLoopToolServiceBindingFromRouterServicesUsesActionServicesWithoutOpaqueHandle|TestAgentLoopToolServiceBindingRuntimeActionServicesTakePrecedenceOverOpaqueHandle|TestRouterServicesFromAgentLoopToolServiceBindingSupportsPointerHandle|TestRegistryHandlerRuntimeActionServiceResolverRequiresRuntimeCapabilities|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`agent_tool_service_binding.go` 生产面删除 provider/routerServices/Handle binding reconstruction helper 与 `daemonAgentLoopActionServiceResolver` fallback 字段，explicit compatibility reconstruction 移入 `agent_tool_service_binding_compat_test.go` test-only adapter。源码扫描测试升级为扫描所有非测试 daemon `.go` 文件，拒绝 `routerServicesFromAgentLoopToolServiceBinding`、`agentLoopActionServiceResolverFromAgentLoopToolServiceBinding`、provider-from-Handle、provider-from-ActionServices 以及 routerServices/provider reconstruction helper 定义。audit table 新增 `tool_service_binding_provider_reconstruction_test_only` landed 项并将 runtime-contract landed 计数更新为 18。下一步 P0.5-N185 继续收敛历史 typed-provider contract / explicit Handle fallback 测试面。

2026-06-09 本轮增量（P0.5-N183：registry runtime ActionServices-only resolver）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestToolServiceCompatibilityHelpersStayInBindingAdapter|TestRegistryHandlerRuntimeActionServiceResolverRequiresRuntimeCapabilities|TestRegistryHandlerRuntimeProductionSurfaceDoesNotExposeLegacyServiceFallbackHelpers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；新增 `agentLoopActionServiceResolverFromActionServices`，production `agentRegistryHandlerRuntime.actionServiceResolver()` 改为 ActionServices-only，不再通过 `AgentLoopToolServiceBinding.Handle` fallback 恢复 daemon provider。新增覆盖断言 Handle-only binding 会被 registry runtime 拒绝，但 explicit compatibility helper 仍能接受；源码扫描固化 `agentLoopActionServiceResolverFromAgentLoopToolServiceBinding` / `agentLoopToolServiceProviderFromHandle` / `routerServicesFromAgentLoopToolServiceBinding` 只保留在 `agent_tool_service_binding.go` compatibility adapter 或测试边界。audit table 新增 `registry_runtime_action_services_only` landed 项并将 runtime-contract landed 计数更新为 17。下一步 P0.5-N184 继续收敛剩余 provider/routerServices reconstruction helper 的保留范围。

2026-06-09 本轮增量（P0.5-N182：runtime tool service direct dispatch）：

```bash
gofmt -w internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go internal/agent/runtime/agent_loop_tool_service_binding.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionExecutorPassesRuntimeToolServicesWithoutReconstruction|TestDeterministicRouteAgentLoopPassesRuntimeToolServicesWithoutReconstruction|TestRuntimeToolServiceDispatchProductionSurfaceDoesNotReconstructRouterServices|TestAgentPlannerActionReplanCycleRequestAgentLoopAdapterRoundTripClones|TestAgentLoopToolServiceBindingFromRouterServicesUsesActionServicesWithoutOpaqueHandle|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；新增 `executeAgentPlannerActionResultWithToolServices` / `executeAgentRegistryHandlerResultWithToolServices` / `routePromptForTurnWithToolServices`，daemon legacy planner action executor 与 deterministic route AgentLoop 分支不再把 runtime binding 回转成 `routerServices`。新增覆盖验证自定义 runtime binding metadata 可穿透 executor / deterministic route 到 registry handler runtime，并新增源码扫描拒绝生产 dispatch 文件调用 `legacyRouterServicesFromAgentLoopToolServiceBinding`；该 helper 已删除，adapter roundtrip 测试改为显式使用 `routerServicesFromAgentLoopToolServiceBinding` compatibility helper。audit table 新增 `runtime_tool_service_direct_dispatch` landed 项并将 runtime-contract landed 计数更新为 16。下一步 P0.5-N183 收敛 explicit Handle/provider/routerServices reconstruction helper 的保留范围。

2026-06-09 本轮增量（P0.5-N181：tool service binding Handle default removed）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingFromRouterServicesUsesActionServicesWithoutOpaqueHandle|TestAgentLoopToolServiceBindingRuntimeActionServicesTakePrecedenceOverOpaqueHandle|TestRouterServicesFromAgentLoopToolServiceBindingSupportsPointerHandle|TestDaemonPlannerActionExecutionSideEffectBoundaryFromAgentLoopClonesRuntimeInputs|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`agentLoopToolServiceBindingFromRouterServices` 不再默认填充 opaque `Handle` / daemon provider，而是只通过 `ActionServices` 携带 service capabilities。`routerServicesFromAgentLoopToolServiceBinding` 与 provider reconstruction 仍能从 ActionServices 恢复 jobs 等引用，explicit Handle pointer/provider compatibility test 继续保留。audit table 新增 `tool_service_binding_handle_default_surface` landed 项并将 runtime-contract landed 计数更新为 15。下一步 P0.5-N182 收敛 legacy executor / deterministic route 的 `legacyRouterServicesFromAgentLoopToolServiceBinding` 调用。

2026-06-09 本轮增量（P0.5-N180：registry runtime fallback surface audit）：

```bash
gofmt -w internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_planner_loop.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerRuntimeProductionSurfaceDoesNotExposeLegacyServiceFallbackHelpers|TestRegistryHandlerRuntimeUsesToolServiceBinding|TestAgentPlannerActionReplanCycleRequestAgentLoopAdapterRoundTripClones|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；production `agentRegistryHandlerRuntime` 已移除 `toolServiceProvider()` / `legacyRouterServices()` helper，registry binding test 改为验证 `actionServiceResolver()` runtime capability path，新增源码扫描测试拒绝 production registry dispatch/handler 文件重新暴露或调用这些 fallback helper；`agentPlannerActionReplanCycleToolContext` 的 legacy provider/routerServices helper 也已移除，测试改为在 explicit binding compatibility helper 边界断言 roundtrip。audit table 新增 `registry_handler_runtime_fallback_surface` landed 项并将 runtime-contract landed 计数更新为 14。下一步 P0.5-N181 拆解剩余 `Handle` / `daemonAgentLoopToolServiceProvider` fallback 与 daemon legacy executor `routerServices` reconstruction。

2026-06-09 本轮增量（P0.5-N179：run-context / approval registry consumers use runtime action service resolver）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_run_handlers.go internal/interfaces/daemon/agent_session_helpers.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerRunContextApprovalBatchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerRunContextApprovalBatchUsesTypedProviderAccessors|TestRegistryHandlerScanBatchUsesRuntimeActionServiceCapabilities|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`daemonAgentLoopActionServiceResolver` 新增 `ApprovalService()` capability resolver，`runs.context`、`reports.draft`、`runs.approvals.list`、`runs.resume_approved`、`runs.approvals.approve` 与 `runs.approvals.deny` 已从 `toolServiceProvider()` / daemon-private `RunService()` / `ApprovalService()` accessor 改为 `actionServiceResolver()` / runtime `runs` + `approvals` capability，typed provider/Handle 继续作为 fallback。新增覆盖使用所有 accessor 均 panic 的 Handle 验证 run-context/approval 组只走 runtime capabilities；audit table 将 run-context/approval batch 与 registry handler service consumers 标记为 runtime-contract landed。下一步 P0.5-N180 审计剩余 compatibility fallback。

2026-06-09 本轮增量（P0.5-N178：scan registry consumer uses runtime action service resolver）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_run_handlers.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerScanBatchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerScanBatchUsesTypedProviderAccessors|TestRegistryHandlerWebSearchUsesRuntimeActionServiceCapabilities|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`daemonAgentLoopActionServiceResolver` 新增 `JobManager()` capability resolver，`scan.web.safe` 已从 `toolServiceProvider()` / `daemonAgentLoopToolServiceProvider.JobManager()` 改为 `actionServiceResolver()` / runtime `AgentLoopActionServiceSet` jobs capability，typed provider/Handle 继续作为 fallback。新增覆盖使用所有 accessor 均 panic 的 Handle 验证 scan 组只走 runtime `jobs` capability；audit table 将 scan batch 标记为 runtime-contract landed。下一步 P0.5-N179 迁移 run-context / approval consumers。

2026-06-09 本轮增量（P0.5-N177：web.search provider-store consumer uses runtime action service resolver）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_schedule_web.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerWebSearchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerWebSearchUsesTypedProviderAccessors|TestRegistryHandlerScheduleBatchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerFirstBatchUsesRuntimeActionServiceCapabilities|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`daemonAgentLoopActionServiceResolver` 新增 `ProviderStore()` capability resolver，`web.search` 已从 `toolServiceProvider()` / `daemonAgentLoopToolServiceProvider.ProviderStore()` 改为 `actionServiceResolver()` / runtime `AgentLoopActionServiceSet` providers capability，typed provider/Handle 继续作为 fallback。新增覆盖使用所有 accessor 均 panic 的 Handle 验证 web.search 只走 runtime `providers` capability；audit table 将 web.search batch 标记为 runtime-contract landed。下一步 P0.5-N178 迁移 scan / JobManager consumer。

2026-06-09 本轮增量（P0.5-N176：schedule registry consumers use runtime action service resolver）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_schedule_web.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerScheduleBatchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerScheduleBatchUsesTypedProviderAccessors|TestRegistryHandlerFirstBatchUsesRuntimeActionServiceCapabilities|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`daemonAgentLoopActionServiceResolver` 新增 `ScheduleManager()` capability resolver，`schedule.create`、`schedule.list`、`schedule.delete` 已从 `toolServiceProvider()` / `daemonAgentLoopToolServiceProvider.ScheduleManager()` 改为 `actionServiceResolver()` / runtime `AgentLoopActionServiceSet`，typed provider/Handle 继续作为 fallback。新增覆盖使用所有 accessor 均 panic 的 Handle 验证 schedule 组只走 runtime `schedules` capability；audit table 将 schedule batch 标记为 runtime-contract landed。下一步 P0.5-N177 迁移 web.search / provider-store consumer。

2026-06-09 本轮增量（P0.5-N175：first registry consumers use runtime action service resolver）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_run_handlers.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerFirstBatchUsesRuntimeActionServiceCapabilities|TestRegistryHandlerFirstBatchUsesTypedProviderAccessors|TestAgentLoopToolServiceBindingRuntimeActionServicesTakePrecedenceOverOpaqueHandle|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；新增 `agentRegistryHandlerRuntime.actionServiceResolver()` / `daemonAgentLoopActionServiceResolver`，`daemon.health`、`plugins.list`、`runs.list` 第一批 handler 已从直接消费 `daemonAgentLoopToolServiceProvider` 改为优先通过 runtime `AgentLoopActionServiceSet` capability 解析 Version/Plugin/Run service，opaque Handle/provider 仅作为 fallback。新增覆盖使用“所有 provider accessor 都 panic”的 Handle 验证第一批 handler 只走 runtime capabilities；audit table 将 first batch 标记为 runtime-contract landed。下一步 P0.5-N176 继续迁移 schedule/web-search/scan 或 run-context/approval 组。

2026-06-09 本轮增量（P0.5-N174：runtime action service capability set）：

```bash
gofmt -w internal/agent/runtime/agent_loop_action_services.go internal/agent/runtime/agent_loop_action_services_test.go internal/agent/runtime/agent_loop_tool_service_binding.go internal/agent/runtime/agent_loop_tool_service_binding_test.go internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopActionServiceSetCloneIsolatesMapsAndPreservesRefs|TestAgentLoopToolServiceBindingCloneIsolatesMetadataAndPreservesHandle'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingFromRouterServicesPreservesLegacyReferences|TestAgentLoopToolServiceBindingRuntimeActionServicesTakePrecedenceOverOpaqueHandle|TestRouterServicesFromAgentLoopToolServiceBindingSupportsPointerHandle|TestDaemonAgentLoopToolServiceProviderMapsRouterServicesFields|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused runtime/daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；新增 runtime `AgentLoopActionServiceSet` 与 capability names（version/plugins/runs/approvals/jobs/agent/scan_manager/schedules/providers），`AgentLoopToolServiceBinding.Clone()` 现在 clone runtime action service capability maps。daemon binding 从 `routerServices` 同步写入 `ActionServices`，`agentLoopToolServiceProviderFromAgentLoopToolServiceBinding` 优先从 runtime capability set 构造 typed provider，`Handle` 仅保留为 daemon-private provider / legacy `routerServices` compatibility fallback。下一步 P0.5-N175 迁移低风险 registry/action handler consumers 到 runtime capability resolver。

2026-06-09 本轮增量（P0.5-N173：planner action tool service dispatch boundary runtime binding）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_prompt_dispatch.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionSideEffectBoundaryFromAgentLoopClonesRuntimeInputs|TestDaemonPlannerActionExecutionSideEffectBoundaryDoesNotReconstructLegacyServices|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonPlannerActionExecutionExecutorSeamDispatchesLegacyPlannerAction'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`daemonPlannerActionExecutionSideEffectBoundary` 不再保存 legacy `routerServices`，而是 clone/preserve runtime `AgentLoopToolServiceBinding`，新增覆盖证明 boundary 构造不会触发 typed provider 的 legacy reconstruction。legacy `routerServices` reconstruction 仅保留在 `daemonLegacyPlannerActionExecutionExecutor` 调用 `executeAgentPlannerActionResult` 前的 implementation fallback 内；audit table 与 TODO 已将下一步收敛点转为 P0.5-N174 runtime action service capability contract。

2026-06-09 本轮增量（P0.5-N172：registry handler result egress runtime-native construction）：

```bash
gofmt -w internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerOutputToActionExecutorResultBuildsRuntimeNativeDTO|TestRegistryHandlerDispatch|TestExecuteAgentPlannerActionResultReturnsRuntimeExecutorDTO|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`registryHandlerOutputToActionExecutorResult` 不再调用 `registryHandlerOutputToMessages`，而是直接构造 runtime `AgentLoopMessage` / `AgentLoopToolCall` DTO，并新增 runtime-native handler output 覆盖。`registryHandlerOutputToMessages` 仅保留为 legacy compatibility adapter；audit table 将 `legacy_result_egress` 证据更新为 runtime-native construction 已落地。下一步 P0.5-N173 继续收敛 `tool_service_dispatch` / typed provider，评估 runtime-native action service interfaces。

2026-06-09 本轮增量（P0.5-N171：planner action direct legacy compatibility return 移除）：

```bash
gofmt -w internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerDispatch|TestExecuteAgentPlannerActionResultReturnsRuntimeExecutorDTO|TestRegistryHandlerRuntimeUsesToolServiceBinding|TestRegistryBuiltinMissingHandlerFailsWithoutLegacyFallback|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutionLegacyCompatibilitySurfaceRetiredAudit|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；direct registry dispatch 测试已改为消费 `executeAgentPlannerActionResult` / `executeAgentRegistryHandlerResult`，并删除 legacy `executeAgentPlannerAction` / `executeAgentRegistryHandler` 的 `[]agentMessage` / `[]agentToolCall` compatibility wrappers。legacy surface audit 将 `executeAgentPlannerAction` 标记为 retired；下一步 P0.5-N172 继续把 `registryHandlerOutputToActionExecutorResult` 内部的 legacy output parser 收敛为 runtime-native result construction。

2026-06-09 本轮增量（P0.5-N170：executor result egress runtime result wrapper 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_registry_dispatch_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestExecuteAgentPlannerActionResultReturnsRuntimeExecutorDTO|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutionLegacyExecutorImplementationOnlyAudit|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonPlannerActionExecutionExecutorSeamDispatchesLegacyPlannerAction|TestRegistryHandlerDispatch|TestRegistryHandlerRuntimeUsesToolServiceBinding|TestRegistryBuiltinMissingHandlerFailsWithoutLegacyFallback'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；新增 `executeAgentPlannerActionResult` / `executeAgentRegistryHandlerResult`，让 planner action executor path 先产出 `runtime.AgentLoopActionExecutorResult`，`daemonLegacyPlannerActionExecutionExecutor` 直接消费 runtime result。legacy `executeAgentPlannerAction` / `executeAgentRegistryHandler` 现在只作为 compatibility wrapper 将 runtime result 反向适配回 `agentMessage` / `agentToolCall`。audit table 将 `executor_result_egress` 标记为 ready/runtime landed，`blocked_by_legacy_binding` 数量降为 0；下一步 P0.5-N171 收敛 direct legacy compatibility return 与相关测试/调用面。

2026-06-09 本轮增量（P0.5-N169：executor/registry authorization context 端到端 runtime 化）：

```bash
gofmt -w internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_registry_dispatch_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionHelperOwnershipTableDocumentsRuntimeOwnedHelpers|TestDaemonTurnExecutionAgentLoopMigrationPreconditions|TestDaemonTurnExecutionPlannerActionExecutionAgentLoopMigrationBlockers|TestDaemonPlannerActionExecutionSideEffectBoundaryFromAgentLoopClonesRuntimeInputs|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutionLegacyExecutorImplementationOnlyAudit|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestRegistryHandlerRuntimeUsesToolServiceBinding|TestRegistryHandlerHTTPRequestUsesRuntimeAuthorizationContext|TestRegistryHandlerDispatch|TestRegistryBuiltinMissingHandlerFailsWithoutLegacyFallback'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`daemonPlannerActionExecutionSideEffectBoundary` 现在保存 runtime `AgentLoopAuthorization`，legacy executor adapter 直接把 authorization 传给 `executeAgentPlannerAction`，registry dispatch 也直接接收 runtime authorization 并仅派生 legacy compatibility fields。audit table 将 `authorization_context` 标记为 ready/runtime landed，blocked-by-legacy-binding 数量降为 1（仅剩当时待迁移的 `executor_result_egress`）。下一步 P0.5-N170 继续收敛 legacy `agentMessage` / `agentToolCall` result egress adapter。

2026-06-09 本轮增量（P0.5-N168：registry runtime authorization context 最小切片）：

```bash
gofmt -w internal/interfaces/daemon/agent_common.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerRuntimeUsesToolServiceBinding|TestAgentLoopAuthProfileResolverUsesRuntimeAuthorization|TestRegistryHandlerHTTPRequestUsesRuntimeAuthorizationContext|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`agentRegistryHandlerRuntime` 已携带 `runtime.AgentLoopAuthorization`，`http.request` registry handler 改用 runtime auth resolver 直接读取 runtime auth profiles，focused test 使用 runtime-only auth profile + conflicting legacy auth profile 证明 handler 不再从 legacy `rt.AuthProfiles` 解析凭证。`authorization_context` blocker 已缩小为 executor/registry dispatch 签名与 side-effect boundary 仍保留 legacy `[]agentHTTPAuthProfile` compatibility 参数，下一步 N169 改签名端到端传递 `AgentLoopAuthorization`。

2026-06-09 本轮增量（P0.5-N167：registry handler typed provider 剩余组迁移）：

```bash
gofmt -w internal/interfaces/daemon/agent_run_handlers.go internal/interfaces/daemon/agent_session_helpers.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandler(FirstBatchUsesTypedProviderAccessors|ScheduleBatchUsesTypedProviderAccessors|WebSearchUsesTypedProviderAccessors|ScanBatchUsesTypedProviderAccessors|RunContextApprovalBatchUsesTypedProviderAccessors)|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutionLegacyExecutorImplementationOnlyAudit|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
rg -n "rt\\.legacyRouterServices\\(\\)" internal/interfaces/daemon/agent_registry_handlers.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过，`rg` 无剩余 `rt.legacyRouterServices()` 调用；`scan.web.safe` 已改走 `JobManager()` typed accessor，run-context/report/approval handler 组已改走 `RunService()` / `ApprovalService()` typed accessors。新增 scan batch 与 run-context/approval batch focused tests 使用“不允许 legacy reconstruction”的 provider stub 覆盖剩余 handler，audit table 将 `registry_handler_service_consumers` 从 blocked 调整为 typed provider landed；剩余 planner_action_execution blocker 为 `authorization_context` 与 `executor_result_egress`。

2026-06-09 本轮增量（P0.5-N166：registry handler typed provider 第一批迁移）：

```bash
gofmt -w internal/interfaces/daemon/agent_schedule_web.go internal/interfaces/daemon/agent_registry_handlers.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestRegistryHandlerFirstBatchUsesTypedProviderAccessors|TestRegistryHandlerScheduleBatchUsesTypedProviderAccessors|TestRegistryHandlerWebSearchUsesTypedProviderAccessors|TestRoutePromptSchedule|TestSchedule|TestRoutePromptAdvancedHTTP|TestRoutePromptSafeHTTP|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`daemon.health` / `plugins.list` / `runs.list`、`schedule.create` / `schedule.list` / `schedule.delete` 与 `web.search` 已从 `rt.legacyRouterServices()` 切到 `rt.toolServiceProvider()` typed accessor 路径，新增“不允许 legacy reconstruction”的 provider stub 覆盖 first batch、schedule batch 与 web.search batch。audit table 已新增 `registry_handler_typed_provider_web_search_batch` landed，并把剩余 `registry_handler_service_consumers` blocker 缩小为 scan / run-context / approval handler 组。

2026-06-09 本轮增量（P0.5-N165：planner_action_execution typed service provider contract）：

```bash
gofmt -w internal/interfaces/daemon/agent_tool_service_binding.go internal/interfaces/daemon/agent_tool_service_binding_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_registry_dispatch.go internal/interfaces/daemon/agent_registry_dispatch_test.go internal/interfaces/daemon/agent_planner_loop.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentLoopToolServiceBindingFromRouterServicesPreservesLegacyReferences|TestRouterServicesFromAgentLoopToolServiceBindingSupportsPointerHandle|TestDaemonAgentLoopToolServiceProviderMapsRouterServicesFields|TestDaemonAgentLoopToolServiceProviderContractDocumentsRouterServiceFieldMapping|TestRegistryHandlerRuntimeUsesToolServiceBinding|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutionLegacyExecutorImplementationOnlyAudit|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`AgentLoopToolServiceBinding.Handle` 不再直接携带 `routerServices`，而是携带 daemon-private `daemonAgentLoopToolServiceProvider`。新增 contract table 固化 `routerServices` 九个字段到 typed accessor 的映射，registry runtime / cycle tool context 已暴露 `toolServiceProvider()`，同时保留 `routerServices` value/pointer compatibility input 与 legacy reconstruction fallback。service contract audit 已把原 `typed_tool_service_binding` blocker 拆成已 landed 的 `typed_tool_service_provider_contract` 与仍 blocked 的 `registry_handler_service_consumers`。

2026-06-09 本轮增量（P0.5-N164：planner_action_execution legacy result egress adapter 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_action_executor.go internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionExecutorSeamDispatchesLegacyPlannerAction|TestDaemonPlannerActionExecutionExecutorSeamRequiresManager|TestDaemonPlannerActionExecutionExecutorSeamCoversHeuristicRoute|TestDaemonAgentLoopActionExecutorDispatchesLegacyPlannerAction|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutionLegacyExecutorImplementationOnlyAudit|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopPlannerActionExecutionResultFromActionExecutorResult|TestAgentLoopPlannerActionExecutionResultFromTurnPayload|TestExecuteAgentLoopAction'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：focused daemon/runtime tests、runtime 包级测试、daemon 包级测试与 diff whitespace check 通过；`daemonPlannerActionExecutionExecutor` seam 的 caller-facing 输出已改为 `runtime.AgentLoopActionExecutorResult`，daemon AgentLoop action executor、TurnExecutor compatibility branch 与 heuristic route caller 不再直接消费 legacy `agentMessage` / `agentToolCall` result。legacy result egress conversion 已集中到 `daemonLegacyPlannerActionExecutionExecutor` 内部，service contract audit table 新增 `legacy_result_adapter_centralized` landed；剩余 blocker 是 typed tool service binding、authorization context 与 legacy executor implementation 内部仍需 runtime-native result DTO。

2026-06-09 本轮增量（P0.5-N163：planner_action_execution blocker 拆解 / result composition runtime helper）：

```bash
gofmt -w internal/agent/runtime/agent_loop_planner_action_execution_result.go internal/agent/runtime/agent_loop_planner_action_execution_result_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopPlannerActionExecutionResultFromActionExecutorResult|TestAgentLoopPlannerActionExecutionResultFromTurnPayload|TestExecuteAgentLoopAction'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonAgentLoopRunnerExecutesPlannerActionExecutionMode|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesAgentLoopRunner|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesActionExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试、daemon 包级测试与 diff whitespace check 通过；`planner_action_execution` runner branch 已将 ActionExecutorResult / TurnPayload → AgentLoopResult composition 下沉到 runtime helper，daemon 只调用 runtime helper 组装 result envelope。audit table 已拆出 `runtime_result_composition` landed，并把剩余 blocker 明确为 `typed_tool_service_binding`、`authorization_context` 与 `legacy_result_egress`；下一步继续收敛 legacy result egress adapter，评估 `executeAgentPlannerAction` / action executor 返回 runtime DTO 的最小边界。

2026-06-09 本轮增量（P0.5-N162：planner_action_execution primary caller 切到 AgentLoop）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionExecutionDispatchesAgentLoopRunner|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesActionExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestDaemonAgentLoopRunnerExecutesLLMToolOuterLoopFromRuntimeRequest|TestDaemonAgentLoopRunnerExecutesReplanContinuationOuterLoopFromRuntimeRequest|TestAgentManagerDecideAgentAuthorizationApprovedFlowThreadsExternalTurnRefThroughRuntimeRunners|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionHelperOwnershipTableDocumentsRuntimeOwnedHelpers|TestDaemonTurnExecutionAgentLoopMigrationPreconditions|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`runRuntimePlannerActionExecution` 主 caller 已切到 `Engine.RunAgentLoop` / `AgentLoopModePlannerActionExecution`，runner branch 内继续保留 `Engine.ExecuteAgentLoopAction`、`ExecuteTurn` compatibility 与 direct legacy executor fallback 顺序；outer LLM/replan loop 与 external authorization flow 现在能观测到 nested `planner_action_execution` AgentLoop call，下一步进入 typed tool service binding / authorization / result egress blocker 拆解。

2026-06-09 本轮增量（P0.5-N161：planner_action_execution AgentLoop mode/runner branch）：

```bash
gofmt -w internal/agent/runtime/agent_loop_runner_payload.go internal/agent/runtime/agent_loop_runner_payload_test.go internal/agent/runtime/agent_loop_turn_execution_request.go internal/agent/runtime/agent_loop_turn_execution_request_test.go internal/interfaces/daemon/agent_loop_runner.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopRequestCloneIsolatesMutableFields|TestAgentLoopPlannerActionExecutionRequestBuildsModeAndClonedAction'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonAgentLoopRunnerExecutesPlannerActionExecutionMode|TestDaemonTurnExecutionAgentLoopMigrationPreconditions|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused runtime/daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`planner_action_execution` 已拥有 runtime request helper 与 AgentLoop mode/runner branch，runner branch 复用 `Engine.ExecuteAgentLoopAction` / daemon action executor adapter 并输出 `AgentLoopResult` state/messages/tool_calls/metadata；audit table 已更新为“mode branch 已落地但 primary caller 尚未切换”，下一步继续评估主 caller 是否切到 `Engine.RunAgentLoop` 或先拆 typed tool service binding / authorization / result egress blocker。

2026-06-09 本轮增量（P0.5-N160：planner action execution 优先走 runtime action executor）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_action_gate_result_test.go internal/interfaces/daemon/agent_loop_runner_adapter_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionExecutionDispatchesActionExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonTurnExecutionAgentLoopMigrationPreconditions'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`runRuntimePlannerActionExecution` 已先 dispatch 到 `Engine.ExecuteAgentLoopAction`，只有 action executor 缺失 / `ErrAgentLoopActionExecutorNotAttached` 时才回落到 `ExecuteTurn` compatibility fallback，最终仍保留 direct legacy executor adapter 兜底；既有 TurnExecutor-oriented 回归显式 detach action executor 以覆盖兼容路径，service contract / AgentLoop migration audit 已更新到 N160 后状态。

2026-06-09 本轮增量（P0.5-N159：daemon legacy action executor runtime adapter）：

```bash
gofmt -w internal/interfaces/daemon/agent_runtime.go internal/interfaces/daemon/agent_action_executor.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerAttachEngineWiresDaemonTurnExecutor|TestDaemonAgentLoopActionExecutorDispatchesLegacyPlannerAction|TestDaemonAgentLoopActionExecutorRequiresManager|TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`daemonAgentLoopActionExecutor` 已实现并由 `AttachEngine` 挂接到 `Engine.ExecuteAgentLoopAction`，adapter 复用 legacy planner action executor seam 且输出 runtime action executor result；migration table 确认 runtime contract 与 daemon adapter 均已就位，下一步切换 `runRuntimePlannerActionExecution` 的优先 dispatch 路径。

2026-06-09 本轮增量（P0.5-N158：runtime action executor service contract landing point）：

```bash
gofmt -w internal/agent/runtime/runtime.go internal/agent/runtime/agent_loop_action_executor.go internal/agent/runtime/agent_loop_action_executor_test.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestExecuteAgentLoopActionDispatchesAttachedExecutorWithCloneIsolation|TestExecuteAgentLoopActionWithoutExecutorReturnsStableError|TestExecuteAgentLoopActionNilEngineReturnsStableError'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutorServiceContractMigrationLandingPoint|TestDaemonPlannerActionExecutionLegacyExecutorImplementationOnlyAudit|TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused runtime/daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；runtime 侧已落地 `AgentLoopActionExecutor` / `Engine.ExecuteAgentLoopAction` 最小 contract 并覆盖 request/result clone 隔离、missing executor 与 nil engine；daemon migration table 标记 runtime contract 已 landed，但 legacy adapter 接入、typed tool service binding、authorization context 与 result egress 仍待后续切片收敛。

2026-06-09 本轮增量（P0.5-N157：legacy executeAgentPlannerAction implementation-only audit）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_prompt_dispatch.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionLegacyExecutorImplementationOnlyAudit|TestDaemonPlannerActionExecutionExecutorSeamCoversHeuristicRoute|TestDaemonPlannerActionExecutionExecutorSeamDispatchesLegacyPlannerAction|TestDaemonPlannerActionExecutionExecutorSeamRequiresManager'
rg -n "executeAgentPlannerAction\(" internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_planner_loop.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、caller surface audit、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`executeAgentPlannerAction` 已通过 legacy surface audit table 归档为 executor adapter implementation-only helper，新增注释约束新内部 caller 走 `daemonPlannerActionExecutionExecutor` seam；当前 planner_action_execution 仍 blocked 于 typed service binding、authorization context 与 executor result egress runtime contract。

2026-06-09 本轮增量（P0.5-N156：heuristic route action execution caller seam 收敛）：

```bash
gofmt -w internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionExecutorSeamCoversHeuristicRoute|TestDaemonPlannerActionExecutionExecutorSeamDispatchesLegacyPlannerAction|TestDaemonPlannerActionExecutionExecutorSeamRequiresManager|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor'
rg -n "executeAgentPlannerAction\(" internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_planner_loop.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、caller surface audit、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`routeHeuristicRegistryActions` 已改经 `daemonPlannerActionExecutionExecutor` seam 调度，`executeAgentPlannerAction` 非测试引用只剩函数定义与 legacy adapter implementation 调用。

2026-06-09 本轮增量（P0.5-N155：planner_action_execution executor adapter seam）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionExecutorSeamDispatchesLegacyPlannerAction|TestDaemonPlannerActionExecutionExecutorSeamRequiresManager|TestDaemonPlannerActionExecutionSideEffectBoundaryFromAgentLoopClonesRuntimeInputs|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor'
rg -n "executeAgentPlannerAction\\(" internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_prompt_dispatch.go internal/interfaces/daemon/agent_planner_loop.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、caller surface audit、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`executePlannerActionExecutionTurn` 与 `runRuntimePlannerActionExecution` fallback 已经改经 `daemonPlannerActionExecutionExecutor` seam 调度，`agent_turn_executor.go` 中的 direct `executeAgentPlannerAction` 仅剩 legacy adapter 实现本身；heuristic route direct caller 仍保留为下一步评估对象。

2026-06-09 本轮增量（P0.5-N154：planner_action_execution runtime action executor contract / typed service binding precondition）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutorContractPreconditionDocumentsTypedServiceBindingBlockers|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonPlannerActionExecutionSideEffectBoundaryFromAgentLoopClonesRuntimeInputs|TestDaemonTurnExecutionAgentLoopMigrationPreconditions|TestDaemonTurnExecutionPlannerActionExecutionAgentLoopMigrationBlockers'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；新增 planner action executor contract precondition table，确认 runtime DTO input 已 ready，但 typed tool-service binding、authorization context 与 executor result egress 仍 blocked，因此 `planner_action_execution` 还不能直接添加 AgentLoop mode/runner；下一步先抽 executor adapter seam，减少 direct `executeAgentPlannerAction` caller surface。

2026-06-09 本轮增量（P0.5-N153：planner_action_execution side-effect boundary / ownership plan）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonPlannerActionExecutionSideEffectBoundaryFromAgentLoopClonesRuntimeInputs|TestDaemonPlannerActionExecutionSideEffectBoundaryOwnershipPlanDocumentsCurrentBlockers|TestDaemonTurnExecutionAgentLoopMigrationPreconditions|TestDaemonTurnExecutionPlannerActionExecutionAgentLoopMigrationBlockers|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`planner_action_execution` 的 side-effect inputs 已收口到 `daemonPlannerActionExecutionSideEffectBoundaryFromAgentLoop`，统一克隆 runtime request/action/authorization 并集中 legacy `routerServices` unwrap 与 HTTP auth-profile adapter；ownership plan table 标记剩余 blocker 仍由 daemon legacy side effect 持有，下一步进入 runtime action executor contract / typed service binding 评估。

2026-06-09 本轮增量（P0.5-N152：planner_action_execution AgentLoop migration precondition / blocker audit）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutionAgentLoopMigrationPreconditions|TestDaemonTurnExecutionPlannerActionExecutionAgentLoopMigrationBlockers|TestDaemonTurnExecutionHelperOwnershipTableDocumentsRuntimeOwnedHelpers|TestDaemonTurnExecutionCallerAuditTablesStayAligned|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`planner_action_execution` AgentLoop precondition 已记录为 blocked-by-daemon-side-effect，blocker table 固化 `executeAgentPlannerAction` side effects、legacy `routerServices` unwrap、HTTP auth-profile adapter 与 action request/result DTO ownership 边界；deterministic route 仍保持 AgentLoop caller live，planner action execution 未被误标为 ready。

2026-06-09 本轮增量（P0.5-N151：deterministic_route AgentLoop caller migration）：

```bash
gofmt -w internal/agent/runtime/agent_loop_runner_payload.go internal/interfaces/daemon/agent_loop_runner.go internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go internal/interfaces/daemon/agent_send_message_lifecycle_test.go internal/interfaces/daemon/agent_send_message_response_merge_test.go internal/interfaces/daemon/agent_runtime_compatibility.go internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopRequestCloneIsolatesMutableFields|TestRunAgentLoopDispatchesAttachedRunnerWithCloneIsolation'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRouteDispatchesAgentLoopRunner|TestAgentManagerSendMessageNoLLMRouteUsesAgentLoopRunner|TestAgentManagerSendMessageActiveRunContinueUsesAgentLoopRunner|TestAgentManagerSendMessageLLMFallbackRouteUsesAgentLoopRunner|TestAgentManagerSendMessageRouteBeforeLLMUsesAgentLoopRunner|TestAgentManagerSendMessageRunsInsideRuntimeTurnLifecycle|TestAgentManagerSendMessageResponseMergeRouteBeforeLLMSynthesis|TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionDeterministicRouteAgentLoopMigrationPreconditions|TestDaemonTurnExecutionCallerAuditTablesStayAligned'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused runtime/daemon tests、daemon 包级测试、runtime 包级测试与 diff whitespace check 通过；`deterministic_route` 内部 caller 已改走 AgentLoop mode/runner，no-LLM route、active-run continue、LLM fallback route、route-before-LLM、turn lifecycle 与 response merge 回归均验证通过；`executeDeterministicRouteTurn` 继续作为 external `ExecuteTurn` compatibility trampoline 保留。

2026-06-09 本轮增量（P0.5-N150：deterministic_route AgentLoop migration precondition）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutionDeterministicRouteAgentLoopMigrationPreconditions|TestDaemonTurnExecutionHelperOwnershipTableDocumentsRuntimeOwnedHelpers|TestDaemonTurnExecutionCallerAuditTablesStayAligned|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 deterministic_route AgentLoop migration precondition table，明确拟议 AgentLoop mode、required runtime helpers、runner adapter parity、remaining daemon routePrompt side effects 与下一步 swap 顺序，并确认 planner_action_execution/replan_action_continuation 尚未被误标为 ready。

2026-06-09 本轮增量（P0.5-N149：TurnExecutor helper ownership audit）：

```bash
gofmt -w internal/interfaces/daemon/agent_turn_executor.go internal/interfaces/daemon/agent_turn_executor_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutionHelperOwnershipTableDocumentsRuntimeOwnedHelpers|TestDaemonTurnExecutionCallerAuditTablesStayAligned|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；TurnExecutor helper ownership table 固化三类 kind 的 helper owner/caller surface/remaining daemon scope/next migration，确认 deterministic_route 与 planner_action_execution 内部 caller 的 request/turn/payload helper 均已 runtime-owned，replan_action_continuation 仅作为 external compatibility trampoline 记录。

2026-06-09 本轮增量（P0.5-N148：replan_action_continuation turn boundary helper 下沉）：

```bash
gofmt -w internal/agent/runtime/agent_loop_turn_execution_metadata.go internal/agent/runtime/agent_loop_turn_execution_metadata_test.go internal/interfaces/daemon/agent_turn_executor.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopTurnExecutionMetadataMergesExtrasAndClones|TestAgentLoopTurnExecutionTurnFromRequestBuildsClonedBoundary|TestAgentLoopTurnExecutionBoundaryFromRequestPreservesPayload'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner|TestDaemonAgentTurnExecutorReplanContinuationReturnsEgressPayload|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner|TestDaemonAgentTurnExecutorReplanContinuationReturnsEgressPayload|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionCallerAuditTablesStayAligned'
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试、TurnExecutor caller migration focused daemon tests 与 diff whitespace check 通过；runtime 新增 turn execution boundary builder，replan_action_continuation 生产 turn boundary 构造改为 runtime helper，origin/index/max_loops metadata 与原 payload preservation 由 runtime focused test 覆盖。

2026-06-09 本轮增量（P0.5-N147：deterministic_route / planner_action_execution runtime request helper 下沉）：

```bash
gofmt -w internal/agent/runtime/agent_loop_turn_execution_request.go internal/agent/runtime/agent_loop_turn_execution_request_test.go internal/interfaces/daemon/agent_turn_executor.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopTurnExecutionRequestBuildsClonedBoundary|TestAgentLoopActionExecutionRequestBuildsClonedAuthorizationBoundary|TestAgentLoopTurnExecutionTurnFromRequestBuildsClonedBoundary|TestAgentLoopActionExecutionTurnBuildsPayloadAndMetadata'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionCallerAuditTablesStayAligned'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionCallerAuditTablesStayAligned'
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试、TurnExecutor caller migration focused daemon tests 与 diff whitespace check 通过；runtime 新增 turn/action execution request builders，daemon deterministic_route / planner_action_execution request 构造已从手写 `AgentLoopRequest` 字面量改为复用 runtime helper，turn/tool service/authorization/request metadata clone 隔离由 runtime tests 覆盖。

2026-06-09 本轮增量（P0.5-N146：deterministic_route / planner_action_execution TurnContext runtime helper 下沉）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopTurnExecutionMetadataMergesExtrasAndClones|TestAgentLoopTurnExecutionTurnFromRequestBuildsClonedBoundary|TestAgentLoopActionExecutionPayloadCloneIsolatesMutableFields|TestAgentLoopActionExecutionTurnBuildsPayloadAndMetadata|TestAgentLoopActionExecutionTurnMetadataMergesAndClones'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner|TestDaemonAgentTurnExecutorReplanContinuationReturnsEgressPayload'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionCallerAuditTablesStayAligned'
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试、TurnExecutor caller migration focused daemon tests 与 diff whitespace check 通过；runtime 新增 turn execution turn builders，daemon deterministic_route / planner_action_execution TurnContext 构造已从 daemon helper 下沉到 runtime helper，replan continuation 测试 helper 也复用 runtime turn builder，payload/metadata clone 隔离由 runtime tests 覆盖。

2026-06-09 本轮增量（P0.5-N145：TurnExecutor internal caller migration boundary audit）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutionCallerMigrationTableCoversInternalExecuteTurnCallers|TestDaemonTurnExecutionCallerAuditTablesStayAligned|TestDaemonTurnExecutionCallerInvalidPayloadTableCoversKinds|TestDaemonTurnExecutionCallerPolicyTableCoversKinds|TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 deterministic_route / planner_action_execution migration boundary audit，确认两类内部 ExecuteTurn caller 仍处于 `active_turn_executor`，replan_action_continuation 仍只作为 external compatibility seam，不进入内部 caller 迁移集合。

2026-06-09 本轮增量（P0.5-N144：active compatibility seam post-cleanup caller audit）：

```bash
rg -n "RunLLMTurn" internal --glob '*.go' --glob '!*_test.go'
if rg -n "agentReplanActionContinuationTurnContextFromRequest" internal --glob '*.go' --glob '!*_test.go'; then exit 1; else echo 'ok: no non-test helper refs'; fi
rg -n "ExecuteTurn" internal --glob '*.go' --glob '!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus|TestDaemonRuntimeCompatibilityTrampolineCallerSurfacesDocumentPostCleanupState|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：post-cleanup caller audit、focused daemon tests、runtime 包级测试与 diff whitespace check 通过；compatibility ownership table 记录两个 active trampoline 的 caller surface 均为 `external_engine_seam_only`，其中 replan helper 已无非测试引用，`ExecuteTurn` 的生产内部 caller 仅剩 deterministic_route 与 planner_action_execution 两类非 replan path。

2026-06-09 本轮增量（P0.5-N143：test-only replan continuation turn context helper cleanup）：

```bash
if rg -n "agentReplanActionContinuationTurnContextFromRequest" internal/interfaces/daemon --glob '*.go' --glob '!*_test.go'; then exit 1; else echo 'ok: no non-test references'; fi
rg -n "agentReplanActionContinuationTurnContextFromRequest" internal/interfaces/daemon --glob '*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner|TestDaemonAgentTurnExecutorReplanContinuationReturnsEgressPayload'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：production caller audit、focused daemon tests、runtime 包级测试与 diff whitespace check 通过；`agentReplanActionContinuationTurnContextFromRequest` 已从生产文件移除，仅在 `_test.go` helper 与两处 focused test 中出现，replan compatibility trampoline 行为保持覆盖。

2026-06-09 本轮增量（P0.5-N142：runtime API compatibility comment/deprecation marker）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonRuntimeCompatibilityTrampolineRuntimeAPIMarkersDocumentStatus|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；runtime LLMTurn seam 已用 Deprecated marker 指向 `Engine.RunAgentLoop`，`TurnExecutor` replan compatibility seam 已以 compatibility notice 保留，ownership audit table 增加 runtime API marker/note 且两个 active trampoline 仍未进入 removal candidate。

2026-06-09 本轮增量（P0.5-N141：active trampoline caller audit）：

```bash
rg -n "RunLLMTurn" internal --glob '*.go' --glob '!*_test.go'
rg -n "agentReplanActionContinuationTurnContextFromRequest" internal --glob '*.go' --glob '!*_test.go'
rg -n "Attach(TurnExecutor|LLMTurnRunner)" internal --glob '*.go' --glob '!*_test.go'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：caller audit、focused daemon tests、runtime 包级测试与 diff whitespace check 通过；非测试 `RunLLMTurn` 只剩 runtime API/interface/func adapter 定义、daemon trampoline 实现与 compatibility audit 证据，`agentReplanActionContinuationTurnContextFromRequest` 仅剩生产定义且只被测试调用，`AttachTurnExecutor` / `AttachLLMTurnRunner` 仍在 `AttachEngine` 中作为 external engine seam 挂接。

2026-06-09 本轮增量（P0.5-N140：active trampoline removal gate plan）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；active trampoline ownership entry 增加 replacement/stage/removal readiness gate，当前 `RunLLMTurn` 与 `ExecuteTurn` replan trampoline 均处于 `active_compatibility`，明确未进入可删除阶段。

2026-06-09 本轮增量（P0.5-N139：active engine seam compatibility ownership audit）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestDaemonRuntimeCompatibilityTrampolineOwnershipTableCoversActiveTrampolines|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 active compatibility trampoline ownership table，明确 `RunLLMTurn` 与 `ExecuteTurn` replan trampoline 仍由 external engine seam compatibility 持有，并记录迁移到 `Engine.RunAgentLoop` 后再移除的 removal gate。

2026-06-09 本轮增量（P0.5-N138：legacy signature wrapper deletion candidates）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityNonTrampolineEntrypointsAreNotAuditedAsActive|TestAgentManagerRunRuntimeLLMToolLoopAgentLoopDispatchesAgentLoopRunner|TestAgentManagerRunRuntimeReplanActionContinuationAgentLoopDispatchesAgentLoopRunner|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；删除 `agentManager.runRuntimeLLMToolLoop` / `agentManager.runRuntimeReplanActionContinuation` legacy signature wrapper，测试改为直接构造 `runtime.AgentLoopRequest` 并调用 AgentLoop path，compatibility trampoline table 仅保留仍 active 的 engine seam entrypoint。

2026-06-09 本轮增量（P0.5-N137：runtime compatibility trampoline audit）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonRuntimeCompatibilityTrampolineTableCoversEntrypoints|TestDaemonRuntimeCompatibilityDirectAgentLoopEntrypointsAreNotTrampolines|TestDaemonLLMTurnRunnerDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutorReplanContinuationDispatchesThroughRuntimeAgentLoopRunner|TestDaemonTurnExecutionCallerAuditTablesStayAligned|TestDaemonSessionMutationAdapterAuditTablesStayAligned'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 runtime compatibility trampoline audit table，覆盖 LLM turn runner、replan continuation turn executor 与两类 legacy signature wrapper，并明确 direct AgentLoop path 不属于 compatibility trampoline。

2026-06-09 本轮增量（P0.5-N136：legacy fallback ownership cleanup）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationCallerFallbackOwnershipTableCoversReasons|TestDaemonTurnExecutionCallerFallbackOwnershipTableCoversReasons|TestAgentManagerRunRuntimeSessionMutationDispatchHelper|TestAgentTurnExecutionResultPayloadHandlesDispatchStates|TestDaemonSessionMutationAdapterAuditTablesStayAligned|TestDaemonTurnExecutionCallerAuditTablesStayAligned'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 turn execution caller fallback ownership table 与 session mutation caller fallback ownership table，显式标记 legacy caller owns fallback 的 missing executor/mutator、blocked、running-without-payload 与 runtime engine 未挂接状态。

2026-06-09 本轮增量（P0.5-N135：action/session mutation adapter residual audit）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationAdapterAuditTablesStayAligned|TestDaemonTurnExecutionCallerAuditTablesStayAligned|TestDaemonSessionMutationCallerPolicyTableCoversKinds|TestDaemonTurnExecutionCallerPolicyTableCoversKinds|TestDaemonSessionMutationCallerInvalidPayloadTableCoversKinds|TestDaemonTurnExecutionCallerInvalidPayloadTableCoversKinds'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 session mutation dispatch/result adapter/invalid payload/policy table cross-audit 与 turn execution caller invalid payload/policy table cross-audit，确保 adapter/audit 表覆盖集合一致，并明确 replan_action_continuation 不属于 turn execution caller fallback 表。

2026-06-09 本轮增量（P0.5-N134：turn execution caller policy/table cleanup）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutionCallerPolicyTableCoversKinds|TestDaemonTurnExecutionCallerInvalidPayloadTableCoversKinds|TestAgentManagerTurnExecutionCallerInvalidPayloadAudit|TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentPlannerActionExecutionTurnResultPayloadHandlesDispatchStates|TestAgentTurnExecutionResultPayloadHandlesDispatchStates|TestAgentManagerRunRuntimePlannerActionExecutionTurnFallbackStates|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 turn execution caller policy table，显式区分 deterministic_route 与 planner_action_execution strict-result 策略；`agentTurnExecutionResultPayloadForKind` 已接入 policy guard，保持 missing executor、blocked、running-without-payload、invalid payload 与 executor error/fallback 语义不变。

2026-06-09 本轮增量（P0.5-N133：session mutation caller policy/table cleanup）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationCallerPolicyTableCoversKinds|TestDaemonSessionMutationCallerInvalidPayloadTableCoversKinds|TestAgentManagerSessionMutationCallerInvalidPayloadFallbackAudit|TestDaemonSessionMutationCallerHandledSuppressesFailures|TestDaemonSessionMutationCallerResultPayloadHandlesEgressStates|TestDaemonSessionMutationCallerOptionalResultPayloadSuppressesFailures|TestAgentManagerRunRuntimeSessionMutationDispatchHelper|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator|TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 session mutation caller policy table，显式区分 append_messages bool-only、record/control/resolve strict-result、request_authorization/capability_blocked optional-result 三类策略；`ForKind` strict/optional/bool helper 已接入 policy guard，保持 missing mutator、blocked、invalid payload、executor error/fallback 语义不变。

2026-06-09 本轮增量（P0.5-N132：action execution invalid-payload/fallback audit table/helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonTurnExecutionCallerInvalidPayloadTableCoversKinds|TestAgentManagerTurnExecutionCallerInvalidPayloadAudit|TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentPlannerActionExecutionTurnResultPayloadHandlesDispatchStates|TestAgentTurnExecutionResultPayloadHandlesDispatchStates|TestAgentManagerRunRuntimePlannerActionExecutionTurnFallbackStates|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 turn execution caller invalid-payload audit table 和 `agentTurnExecutionResultPayloadForKind` helper，deterministic_route / planner_action_execution invalid payload 文案统一由 kind table 管理；确认 invalid payload 仍作为 handled error 返回，missing executor、blocked、running-without-payload 与 executor error fallback 语义保持不变。

2026-06-09 本轮增量（P0.5-N131：session mutation caller invalid-payload/fallback audit table/helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationCallerInvalidPayloadTableCoversKinds|TestAgentManagerSessionMutationCallerInvalidPayloadFallbackAudit|TestDaemonSessionMutationCallerHandledSuppressesFailures|TestDaemonSessionMutationCallerResultPayloadHandlesEgressStates|TestDaemonSessionMutationCallerOptionalResultPayloadSuppressesFailures|TestAgentManagerRunRuntimeSessionMutationDispatchHelper|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator|TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 session mutation caller invalid-payload audit table 和 `ForKind` caller helper，统一 record/request_authorization/capability_blocked/control/resolve invalid payload 文案；补齐 direct caller audit，确认 record/control/resolve invalid payload 作为 handled error 返回，request_authorization/capability_blocked invalid payload 继续触发 legacy fallback，append_messages 仍只按 handled/error bool 判断。

2026-06-08 本轮增量（P0.5-N130：session mutation caller egress DTO/helper 边界整理）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationCallerHandledSuppressesFailures|TestDaemonSessionMutationCallerResultPayloadHandlesEgressStates|TestDaemonSessionMutationCallerOptionalResultPayloadSuppressesFailures|TestAgentManagerRunRuntimeSessionMutationDispatchHelper|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator|TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 session mutation caller egress DTO/helper，append/record/request_authorization/capability_blocked/control/resolve runtime caller 统一通过 `runRuntimeSessionMutationCallerEgress` 汇聚 result/handled/error，再由 result/optional/bool helper 消费，保持 fallback、invalid payload 与 legacy adapter 语义不变。

2026-06-08 本轮增量（P0.5-N129：append mutation bool egress helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationCallerHandledSuppressesFailures|TestDaemonSessionMutationCallerResultPayloadHandlesEgressStates|TestDaemonSessionMutationCallerOptionalResultPayloadSuppressesFailures|TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator|TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef|TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan|TestAgentManagerRunRuntimeSessionMutationDispatchHelper'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 append mutation bool egress helper，append_messages runtime caller 复用 shared handled/error 判断，保持 mutator missing、blocked、error 时 legacy append fallback 语义不变。

2026-06-08 本轮增量（P0.5-N128：session mutation result adapter table/helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationResultAdapterTableCoversKinds|TestDaemonSessionMutationResultPayloadForKindUsesAdapterTable|TestDaemonSessionMutationResultPayloadFromRuntimeOrLegacy|TestAgentLoopRecordUserMessageResultPayloadAdapterRoundTrip|TestAgentLoopControlSessionResultPayloadAdapterRoundTrip|TestAgentLoopResolveAuthorizationResultPayloadAdapterRoundTrip|TestAgentLoopRequestAuthorizationResultPayloadAdapterRoundTrip|TestAgentLoopCapabilityBlockedResultPayloadAdapterRoundTrip|TestAgentLoopAppendMessagesResultPayloadAdapterRoundTrip|TestDaemonSessionMutatorAppendMessagesReturnsAgentLoopResultPayload|TestDaemonSessionMutatorReturnsAgentLoopResultPayloads|TestDaemonSessionMutationCallerResultPayloadHandlesEgressStates|TestDaemonSessionMutationCallerOptionalResultPayloadSuppressesFailures|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 session mutation result adapter table/helper，append/record/request_authorization/capability_blocked/control/resolve 六类 `FromSessionMutationResult` 入口统一经 kind→adapter helper 分派，保留 runtime value、runtime pointer、legacy payload fallback、type mismatch 与 clone 隔离语义。

2026-06-08 本轮增量（P0.5-N127：session mutation caller result egress helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonSessionMutationCallerResultPayloadHandlesEgressStates|TestDaemonSessionMutationCallerOptionalResultPayloadSuppressesFailures|TestDaemonSessionMutationResultPayloadFromRuntimeOrLegacy|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 session mutation caller result egress helper，record/control/resolve 三类 error-return caller 与 request_authorization/capability_blocked 两类 bool-only caller 共用 handled/error/payload conversion 逻辑，保持 invalid payload、fallback 与 legacy adapter 语义不变。

2026-06-08 本轮增量（P0.5-N126：turn executor shared dispatch helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentTurnExecutionResultPayloadHandlesDispatchStates|TestAgentPlannerActionExecutionTurnResultPayloadHandlesDispatchStates|TestAgentManagerRunRuntimePlannerActionExecutionTurnFallbackStates|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 `agentTurnExecutionResultPayload` 作为 deterministic route 与 planner action execution 的 shared turn result dispatch helper，统一 missing executor、blocked/running fallback、executor error、payload clone 与 invalid payload error 处理，同时保留两类 caller 原有 error 文案和 legacy fallback 语义。

2026-06-08 本轮增量（P0.5-N125：session mutation payload/result dispatch table helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestCloneAgentLoopSessionMutationPayload|TestDaemonSessionMutationDispatchTableCoversKinds|TestDaemonSessionMutationPayloadHelperClonesAndReportsInvalid|TestDaemonSessionMutationFinishedResultClonesData|TestDaemonSessionMutatorAppendMessagesReturnsAgentLoopResultPayload|TestDaemonSessionMutatorReturnsAgentLoopResultPayloads|TestAgentManagerRunRuntimeSessionMutationDispatchHelper|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopActionExecutionPayloadCloneIsolatesMutableFields|TestAgentLoopActionExecutionTurnMetadataMergesAndClones|TestAgentLoopActionBatchStepResultHelpersCloneAndApply|TestAppendAgentLoopActionBatchActionResultWithQueueMetadata'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentPlannerActionExecutionTurnResultPayloadHandlesDispatchStates|TestAgentManagerRunRuntimePlannerActionExecutionTurnFallbackStates|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerExecuteAgentPlannerActionsDispatchesPlannerActionsThroughTurnExecutor|TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator|TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef|TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan|TestAgentManagerAppendMessagesForTurnCompactionUsesTurnRef|TestAgentManagerControlSessionCompactionUsesMutationTurnRef|TestAgentManagerResolveAuthorizationCompactionUsesMutationTurnRef'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon/runtime tests、runtime 包级测试与 diff whitespace check 通过；daemon `MutateSession` 已由 session mutation kind→handler table 分派，typed payload clone/invalid payload error 与 finished result skeleton 统一由 helper 处理，六类 mutation 保持 runtime result DTO 输出、legacy fallback adapter 与 metadata 语义不变。

2026-06-08 本轮增量（P0.5-N124：action execution turn result dispatch helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentPlannerActionExecutionTurnResultPayloadHandlesDispatchStates|TestAgentManagerRunRuntimePlannerActionExecutionTurnFallbackStates|TestAgentManagerRunAgentPlannerActionReplanCycleActionExecutionUsesCycleRequest|TestAgentManagerExecuteAgentPlannerActionsDispatchesPlannerActionsThroughTurnExecutor'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopActionExecutionPayloadCloneIsolatesMutableFields|TestAgentLoopActionExecutionTurnMetadataMergesAndClones|TestAgentLoopActionBatchStepResultHelpersCloneAndApply|TestAppendAgentLoopActionBatchActionResultWithQueueMetadata|TestAgentManagerRunRuntimePlannerActionExecutionDispatchesTurnExecutor|TestAgentPlannerActionExecutionTurnResultPayloadHandlesDispatchStates|TestAgentManagerRunRuntimePlannerActionExecutionTurnFallbackStates|TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper|TestAgentManagerExecutePlannerActionBatchStopsAtGateBeforeFollowingActions|TestAgentManagerExecutePlannerActionBatchGateResultParity'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused daemon/runtime tests、runtime 包级测试与 diff whitespace check 通过；planner action execution 的 turn result dispatch/fallback/invalid payload/clone 逻辑已收敛到独立 helper，`runRuntimePlannerActionExecution` 只负责构造 loop/action context 与 legacy fallback adapter；LLM turn result payload cloner 复用 shared helper，保持现有 `ErrTurnExecutorNotAttached`、blocked、running-without-payload 与 executor error 语义。

2026-06-08 本轮增量（P0.5-N123：action batch step executor seam helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopActionBatchStepResultHelpersCloneAndApply|TestAppendAgentLoopActionBatchActionResultWithQueueMetadata|TestApplyAgentPlannerActionBatchStepResultPreservesDefaultMetadata|TestAppendAgentPlannerActionBatchResultWithQueueMetadataPreservesDefaultMetadata|TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper|TestAgentManagerExecutePlannerActionBatchStopsAtGateBeforeFollowingActions|TestAgentManagerExecutePlannerActionBatchGateResultParity'
```

结果：focused runtime/daemon tests 通过；新增 runtime `AgentLoopActionBatchStepResult`、execution/gate step constructors 与 `ApplyAgentLoopActionBatchStepResult`，daemon `executeAgentPlannerActionBatchStep` 改为返回 step egress，由 runtime apply helper 统一处理 batch result mutation、queue metadata append 与 gate stop；daemon wrapper 仅保留 legacy default tool metadata/risk stamping。

2026-06-08 本轮增量（P0.5-N122：session mutation orchestration dispatch helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopIngressSessionMutationRequestHelpers|TestAgentLoopOperatorSessionMutationRequestHelpers|TestAgentLoopActionGateSessionMutationRequestHelpers|TestAgentManagerRunRuntimeSessionMutationDispatchHelper|TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator|TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef|TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeSessionMutationDispatchHelper|TestAgentManagerAppendMessagesWithPlannerDispatchesSessionMutator|TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef|TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerExecutePlannerActionRequestsAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator'
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 daemon `runRuntimeSessionMutation` dispatch helper，统一 engine/mutator 缺失、nil ctx、`ErrSessionMutatorNotAttached`、blocked、error 与 handled 语义；append/record/request_authorization/capability_blocked/control/resolve 六类 runtime session mutation caller 改为复用该 helper，保留原 fallback 与 payload validation 行为。

2026-06-08 本轮增量（P0.5-N121：append/record session mutation request helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopIngressSessionMutationRequestHelpers|TestAgentLoopOperatorSessionMutationRequestHelpers|TestAgentLoopActionGateSessionMutationRequestHelpers|TestAgentManagerAppendMessagesDispatchesSessionMutator|TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef|TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerAppendMessagesDispatchesSessionMutator|TestAgentManagerAppendMessagesForTurnDispatchesSessionMutatorWithTurnRef|TestAgentManagerAppendMessagesWithPlannerForTurnDispatchesSessionMutatorWithPlan|TestAgentManagerRunRuntimeRecordUserMessageDispatchesSessionMutator|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator'
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 runtime append/record mutation metadata、turn/request helper，append_messages 与 record_user_message mutation request 构造由 runtime 统一负责 turn boundary、payload、request metadata clone；daemon 不再手写生产路径 SessionMutationRequest literal，仅保留 DTO 转换、mutator dispatch/fallback 与 legacy compatibility。

2026-06-08 本轮增量（P0.5-N120：control/resolve session mutation request helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopOperatorSessionMutationRequestHelpers|TestAgentLoopActionGateSessionMutationRequestHelpers|TestAgentLoopSessionMutationMetadataHelpers|TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeControlSessionDispatchesSessionMutator|TestAgentManagerRunRuntimeResolveAuthorizationDispatchesSessionMutator|TestAgentManagerExecutePlannerActionAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator'
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 runtime operator-origin turn/request helper，control_session 与 resolve_authorization mutation request 构造由 runtime 统一负责 turn boundary、payload、request metadata clone；daemon 只保留 turnRef source、trim/normalize、mutator dispatch/fallback 与 legacy compatibility。

2026-06-08 本轮增量（P0.5-N119：action gate session mutation request helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopActionGateSessionMutationRequestHelpers|TestAgentLoopSessionMutationMetadataHelpers|TestAgentManagerExecutePlannerActionBatchGateResultParity|TestAgentManagerExecutePlannerActionAuthorizationThroughSessionMutator|TestAgentManagerExecutePlannerActionCapabilityBlockedThroughSessionMutator'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 runtime session mutation turn/request helper，统一 request_authorization 与 capability_blocked gate mutation 的 turn boundary、payload、request metadata clone；daemon 只保留 mutator dispatch/fallback 与 legacy compatibility。

2026-06-08 本轮增量（P0.5-N118：action queue metadata append runtime-neutral helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAppendAgentLoopActionBatchActionResultWithQueueMetadata|TestAppendAgentPlannerActionBatchResultWithQueueMetadataPreservesDefaultMetadata|TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper|TestAgentManagerExecutePlannerActionBatchStopsAtGateBeforeFollowingActions|TestAgentManagerExecutePlannerActionBatchGateResultParity'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 runtime `AppendAgentLoopActionBatchActionResultWithQueueMetadata`，统一 queue metadata stamp、top-level/message tool calls clone 与 batch append；daemon `appendAgentPlannerActionBatchResultWithQueueMetadata` 只保留 legacy default tool metadata/risk stamping，然后转调 runtime helper，保持 action queue metadata、error fallback、gate stop 与默认 risk 语义。

2026-06-08 本轮增量（P0.5-N117：action execution loop body helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAppendAgentPlannerActionBatchResultWithQueueMetadataPreservesDefaultMetadata|TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper|TestAgentManagerExecutePlannerActionBatchStopsAtGateBeforeFollowingActions|TestAgentManagerExecutePlannerActionBatchGateResultParity'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime
git diff --check
```

结果：focused runtime/daemon tests、runtime 包级测试与 diff whitespace check 通过；新增 daemon-private action batch step context/helper 与 queue metadata append helper，`executeAgentPlannerActionBatch` 只保留 batch prepare 与逐项调度，action 执行/error fallback/metadata stamp/append 收敛到单步 helper；queue metadata append 继续通过 runtime result assembly helper clone 隔离，同时保持 daemon default tool metadata/risk 语义。

2026-06-08 本轮增量（P0.5-N116：action batch result assembly helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestAgentLoopActionBatchAssemblyHelpersClone|TestPrepareAgentLoopActionBatch|TestAgentPlannerActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchGateResultParity|TestAgentManagerExecuteAgentPlannerActionListThreadsActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper|TestAgentManagerExecutePlannerActionBatchStopsAtGateBeforeFollowingActions'
git diff --check
```

结果：focused runtime/daemon tests 与 diff whitespace check 通过；新增 `AppendAgentLoopActionBatchActionResult` / `StopAgentLoopActionBatchAtGate`，daemon action batch loop 复用 runtime helper 写入 messages/tool calls、processed count、authorization_required 与 stop gate，保持 gate stop、queue metadata、error fallback 与 clone 隔离语义。

2026-06-08 本轮增量（P0.5-N115：batch queue ID/max ownership helper）：

```bash
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestPrepareAgentLoopActionBatch|TestAgentPlannerActionQueueMetadata|TestAgentManagerExecutePlannerActionBatchGateResultParity|TestAgentManagerExecutePlannerActionBatchErrorFallbackUsesRuntimeResultHelper|TestAgentManagerExecutePlannerActionBatchStopsAtGateBeforeFollowingActions'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused runtime/daemon tests 通过；runtime 包级测试通过；daemon 包级全量测试在当前网络受限沙箱中因 `httptest` 无法 bind `[::1]:0` 中断（非本轮代码失败）。`git diff --check` 通过（仅有 macOS `xcrun` cache warning）。本轮将 action batch 默认上限、queue ID generator wrapper 与 job→queue ID shape helper 下沉到 runtime；daemon action batch 删除本地 max/queue wrapper，只注入 `runs.NewJobID` 作为 ID source，保持 empty batch 不生成 queue ID、truncation 与 queue metadata 语义。

2026-06-08 本轮增量（P0.5-N114：deterministic/replan turn execution metadata helper）：

```bash
go test -count=1 ./internal/agent/runtime -run 'TestAgentLoopTurnExecutionMetadata|TestAgentLoopActionExecutionTurnMetadata'
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerRunRuntimeDeterministicRouteDispatchesTurnExecutor|TestAgentManagerRunRuntimeReplanActionContinuationDispatchesAgentLoopRunner'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon
git diff --check
```

结果：focused 与 runtime/daemon 包级测试通过；deterministic route 与 replan continuation turn metadata 已迁到 runtime generic turn execution helper，action execution turn metadata helper 改为复用同一底层 helper，保持 executor/kind/operation_level/max_loops 语义。

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
go test -count=1 ./internal/interfaces/daemon -run 'TestAgentManagerSendMessageRunsInsideRuntimeTurnLifecycle|TestAgentManagerSendMessageResponseMerge|TestAgentManagerSendMessageNoLLMRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackRouteUsesTurnExecutor|TestAgentManagerSendMessageLLMFallbackAuthorizationUsesSessionMutations|TestAgentManagerSendMessageRouteBeforeLLMUsesTurnExecutor|TestAgentManagerSendMessageActiveRunContinueUsesTurnExecutor|TestAgentManagerEmitsMemoryCompactedRuntimeEvent|TestAgentManagerRunRuntimeTurnLifecyclePublishesTerminalEvents'
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
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestExecuteTurn|TestAgentManagerAttachEngineWiresDaemonTurnExecutor|TestAgentManagerRunRuntimeTurnLifecyclePublishesTerminalEvents|TestHTTPAgentChatNativeApprovalDecisions|TestHTTPAgentChatNativeResumeApproved'
go test -count=1 ./internal/agent/runtime ./internal/interfaces/daemon -run 'TestBeginTurn|TestFinishTurn|TestAgentManagerRunRuntimeTurnLifecyclePublishesTerminalEvents|TestHTTPAgentChatNativeApprovalDecisions|TestHTTPAgentChatNativeResumeApproved'
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
Turn lifecycle API 小切片验证通过：`runtime.Engine.BeginTurn` / `FinishTurn` 已接管 task/run/turn state 与 `turn_started`、`turn_finished`、`turn_failed`、approval/run terminal events 发布；`SendMessage` 已经通过 `runRuntimeTurnLifecycle` 进入 `runtime.Engine.RunTurnLifecycle`，原 event order、status/reason/error data 与 approval/terminal run 语义保持不变。
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
