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
- [ ] 继续把剩余 session mutation 分批迁成 engine-native runner。
- [ ] 最后继续收敛 remaining SendMessage branches 为 engine turn lifecycle。

---

## 4. P1：核心体验增强

- [ ] Frontend route-level lazy / bundle splitting，降低初始 bundle。
- [ ] 移动端 Agent Cockpit 降噪，避免横向滚动和信息堆叠。
- [ ] 真实浏览器 E2E smoke。
- [ ] a11y 回归：icon-only button、dialog/drawer 语义、焦点管理。
- [ ] 视觉回归：毛玻璃背景、半透明卡片、approval pending、破坏性操作状态。

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

## 7. 最新本地验证记录

2026-06-06 当前工作树验证通过：

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
```

结果摘要：

```text
Go test / vet / build 通过。
gofmt / diff whitespace check 通过。
Frontend vitest 6 files / 25 tests 通过。
Frontend production build 通过。
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
Control session mutation 小切片验证通过：`ControlSession` 的 bounded session 状态写入已改为 `SessionMutationControlSession`；continue/revise/pause/cancel/stop/new_task 的 session/task/todo/next_action/message、persist、memory compaction 与 message runtime events 仍复用 daemon legacy helper，stop/cancel 的 daemon job cancel 副作用继续保留在外层 control flow。
Deterministic route turn executor 小切片验证通过：`SendMessage` 的 no-LLM 本地规则 route 与 control auto-continue route 已改为 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` payload dispatch；实际 `routePrompt` 执行、local route source metadata、外层 appendMessages 与 response merge 仍保持原语义。
LLM fallback route 小切片验证通过：LLM provider failure 后的本地 fallback route 已复用 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` adapter；AI failure warning message、fallback messages prepend、tool calls merge 与 session append 顺序保持不变。
Route-before-LLM turn executor 小切片验证通过：LLM configured + deterministic pre-route 分支已复用 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` adapter；step phase tagging、toolCalls phase tagging、后续 LLM synthesis 入口与 response merge 仍保持原语义。
Replan continuation turn executor 小切片验证通过：authorization approve resume 与 route-before-LLM 后续 auto-continue 的 `runAgentReplanActionContinuation` 已经 `runtime.Engine.ExecuteTurn` dispatch 到 daemon legacy continuation adapter；实际 planner action execution、StepReplan synthesis、append 与 response merge 仍保持原语义。
Planner action execution turn executor 小切片验证通过：authorization approve 后的单个 planner action 已经 `runtime.Engine.ExecuteTurn` dispatch 到 daemon legacy `executeAgentPlannerAction` adapter；错误 fallback message、step phase tagging、authorization metadata、append/synthesis/auto-continue 语义保持原路径。
Planner loop action execution seam 小切片验证通过：LLM planner loop 与 replan continuation 内部的 `executeAgentPlannerActions` 已经按 action 复用 `planner_action_execution` adapter；authorization gate、action queue metadata、错误 fallback、phase tagging 与 StepReplan synthesis 语义保持原路径。
Turn-aware append session mutation 小切片验证通过：planner loop / replan continuation 的 action/warning append 已使用携带 task/run/turn/prompt/origin/operation_level 的 `SessionMutationAppendMessages` wrapper；默认 mutation 仍落到 daemon legacy append/persist/compaction/runtime events helper。
Turn-aware planner append mutation 小切片验证通过：LLM planner chat 初始 plan append 与 tool synthesis planner apply 已使用携带 planner_chat/tool_synthesis origin 的 turn-aware `SessionMutationAppendMessages` wrapper；planner apply、message append、response merge 与 auto-continue 语义保持不变。
```
