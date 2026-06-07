# CyberAgent 后续优化方案

状态：动态维护中  
维护人：Leader  
协作方式：后端、前端、QA 成员发现问题后反馈给 Leader，由 Leader 统一更新本文档  
约束：本文档仅使用优先级、切片顺序、依赖关系与验收口径，不使用排期式表达。

---

## 1. 目标

在 Phase 1–6 已完成的 Agent Runtime 基础上，继续推进 CyberAgent 从“功能完整”向“体验更现代、契约更稳定、测试保障更充分、生产可用性更强”的方向优化。

本轮优化核心目标：

1. **UI 毛玻璃重设计**
   - 渐变背景。
   - 半透明卡片。
   - 毛玻璃质感。
   - 现代感与深度感兼具。
   - 视觉层次丰富。

2. **Runtime event / UI state 契约稳定化**
   - 固定 runtime event catalog。
   - 增加 golden fixtures。
   - 为前端提供稳定状态派生字段，减少 UI 猜字段。

3. **Approval / Recovery 语义收敛**
   - 区分 run HITL approval 与 agent tool authorization。
   - 保护 recovery 中 awaiting approval 的语义。
   - 避免恢复、审批、事件流之间状态冲突。

4. **质量基线与回归矩阵补齐**
   - 固化 Go / frontend 测试命令。
   - 补齐 UI、SSE、approval、recovery、a11y、浏览器 smoke 验证。

5. **安全与配置说明一致性修正**
   - 设置页密钥存储文案必须与真实 localStorage 行为一致。
   - 避免用户误解敏感信息保存位置。

---

## 2. 当前基线

最新状态同步：

- 仓库实际 HEAD：本分支当前 HEAD（持续滚动，见 `git log -1 --oneline`）。
- P0 Slice A–H 已完成代码落地并通过本地验证。
- Runtime engine 默认路径已完成小切片切换：daemon 默认打开 SQLite runtime engine；如需回滚，可设置 `CYBERAGENT_RUNTIME=legacy`、`off`、`disabled`、`false` 或 `0`。
- 当前优化重点已从 P0 实施切换为：文档收敛、subagent / engine-native 收尾、P1/P2 体验与生产化增强。
- 生产级 Agent Runtime 六阶段中的主体能力已具备；仍需单独收尾的终态差距包括：减少 legacy planner/routing 依赖、subagent 实质实现、SSE/session polling 兼容层进一步收敛。
- 三个重拉 Codex 已完成只读调研并完成 P0 小切片实施反馈：
  - Backend-Codex-Optimize-2：runtime event catalog、UI state、approval canonical、recovery protection。
  - Frontend-Codex-Optimize-2：glass primitives、Cockpit 迁移、settings 文案、UI state 消费。
  - QA-Codex-Optimize-2：质量基线与回归矩阵。

最新本地验证：

```bash
go test ./...
go vet ./...
go build -ldflags "-X main.version=ci -X main.commit=$(git rev-parse --short HEAD)" -o bin/cyberagent ./cmd/cyberagent
./bin/cyberagent version
./bin/cyberagent --version
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
```

验证结果：

```text
Go test / vet / build 通过。
Frontend vitest 6 files / 25 tests 通过。
Frontend production build 通过。
```

---

## 3. 协作规则

### 3.1 Leader 职责

Leader 负责：

- 维护本文档。
- 汇总成员反馈。
- 将反馈转化为方案更新、任务拆分、依赖调整、验收口径调整。
- 控制前端、后端、QA 文件边界。
- 避免不同成员同时修改同一契约或同一文件造成冲突。
- 防止视觉改造破坏 runtime 功能。
- 防止后端 contract 改动缺少前端/QA 对齐。

### 3.2 成员反馈格式

成员发现问题时，统一反馈给 Leader，不直接修改总方案。反馈格式建议：

```text
问题：
证据文件/命令/现象：
影响范围：
建议调整：
是否阻塞：是/否
建议归属：前端/后端/QA/Leader
```

### 3.3 实施边界

- 先计划，后实施。
- 小切片、小提交、小范围验收。
- 不把 UI、approval、recovery、event catalog、机械重构混入同一提交。
- QA 可以提出业务 bug，但默认不直接大改业务实现。

---

## 4. 总体优先级

### P0：必须优先处理

当前状态：已完成。

1. Slice A：质量基线与回归矩阵计划。✅
2. Slice B：Glass Tokens + Primitives。✅
3. Slice C：Runtime Event Catalog + Golden Fixtures。✅
4. Slice D：Agent Cockpit 毛玻璃主界面重构。✅
5. Slice E：后端 UI State 派生层。✅
6. Slice F：Approval Canonical State + Event Bridge。✅
7. Slice G：Settings Secret Storage 文案与最小安全修正。✅
8. Slice H：Recovery 状态冲突保护。✅

### P1：核心体验增强

1. Route-level Lazy / Bundle Splitting。✅
2. 移动端 Cockpit 降噪。✅
3. 真实浏览器 E2E / a11y / 视觉回归。

### P2：生产可用性增强

1. Run List Benchmark + 可重建索引。
2. Tool Registry / MCP / Scheduler Observability View。
3. Timeline 大量事件渲染优化。
4. Trace viewer JSON 分段、搜索、复制、下载增强。

### P0.5：Legacy Planner / Routing 收敛评估

当前仍在执行链上的旧路径：

1. `internal/interfaces/daemon/agent_session_flow.go`
   - `SendMessage` 仍是 Agent 对话主入口。
   - runtime engine 已接管 SQLite 事件桥、SSE replay、recovery 与部分 lifecycle / turn / LLM / session mutation seam dispatch；尚未完整接管 `SendMessage` 调度和 planner loop。
   - no-LLM control continue 的 active-run 上下文读取已改为经 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` dispatch，不再直接调用 run context analysis handler；`active_run_continue` append 与 response merge 语义保持不变。
2. `internal/interfaces/daemon/agent_planner_loop.go`
   - LLM JSON planner / native tool calls 仍由 daemon 解析、回写 session state，并通过 `executeAgentPlannerActions` 执行。
   - operation level、explicit intent gate、manual authorization gate、action queue metadata 仍在 daemon 层。
3. `internal/interfaces/daemon/agent_prompt_dispatch.go`
   - `routePrompt` 仍是 no-LLM、本地 fallback、确定性 pre-route、auto-continue 的规则路由入口。
   - 本切片已删除 `executeAgentPlannerAction` 内部的 legacy per-tool switch fallback；builtin action 只能通过 Tool Registry handler 执行，handler 缺失会以 `internal_error` 失败。
   - 本切片已将 read-only/system、safe HTTP、non-safe HTTP、safe scan、workspace staged change、reports draft、workspace mutation、approval approve/deny/resume-approved、schedule create/list/delete 与 generic workspace fallback 路由分支收敛为 `agentHeuristicActionsFromPrompt` 产出 action，再通过 Tool Registry handler 执行。
   - generic workspace prompt 未命中具体 read/list/tree/search/write 类 intent 时，现在 fallback 为 `workspace.files.list path=.` registry route，不再走 legacy `workspace.prepare` direct handler。
   - 本切片已为 routePrompt 工具结果统一增加 metadata source：registry 分发为 `deterministic_route/registry_dispatch`，保留直连 handler 的分支为 `deterministic_route/direct_handler`。
   - 本切片已将 source 提升到后端 trace timeline 顶层，并在 Agent Cockpit Timeline 工具卡显示轻量 source badge。
4. `internal/interfaces/daemon/agent_intents.go` 与 `internal/interfaces/daemon/agent_prompt_extractors.go`
   - 启发式 intent / 参数抽取仍决定 local routing、manual authorization fallback 与 prompt/action 匹配。
5. `internal/interfaces/daemon/router.go`
   - daemon 仍挂载兼容 `/api/scans`、`/api/providers`、`/api/tools` 路由，供旧 Dashboard scan/API 面板使用；Agent Cockpit 主链路走 `/api/v1/agent/*`。
6. `web/frontend/src/pages/agent-cockpit.tsx`
   - Cockpit 以 Agent session SSE 为主，但仍保留 session bootstrap/getSession、legacy authorization gate fallback 与 send response merge 兼容层。

小切片顺序：

1. ✅ 删除 planner action 的 legacy per-tool dispatch fallback，只保留 registry handler 单一路径。
2. ✅ 将 `routePrompt` 的 read-only/system 分支改为生成 `agentHeuristicActionsFromPrompt` → registry dispatch，先不触碰 scan / workspace mutation / approval 写动作。
3. ✅ 把 local routing 的工具结果统一标记 source，便于 runtime event 与 trace viewer 区分 deterministic route、LLM planner、manual authorization resume。
4. ✅ 将 `routePrompt` 的 safe HTTP 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖 GET / HEAD / OPTIONS 与 Web 常见端口 HTTP fallback 这类 safe `http.request`。
   - POST / PUT / PATCH / DELETE 等 non-safe HTTP 已在后续切片改为 advanced HTTP registry route；继续保留 `advanced` operation-level gate。
   - route source metadata 继续标记为 `deterministic_route/registry_dispatch`。
5. ✅ 将 `routePrompt` 的 `ports.scan.safe` 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖有界 TCP connect 安全端口探测。
   - 保留 `safe_scan` operation-level gate、端口范围上限与 registry handler 内部执行语义。
   - route source metadata 继续标记为 `deterministic_route/registry_dispatch`。
6. ✅ 将 `routePrompt` 的 `scan.web.safe` 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖 safe Web/API 扫描，会继续通过 registry handler 启动 daemon job / run / evidence workflow。
   - 保留 `safe_scan` operation-level gate、scan intent/capability mismatch 判断与原 handler 的 job/run/context_refs 语义。
   - route source metadata 从 direct handler 更新为 `deterministic_route/registry_dispatch`。
7. ✅ 将 `routePrompt` 的 workspace staged-change 预览类本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖自然语言 `workspace.files.diff` 与 `workspace.files.rebase`。
   - 继续不触碰 `workspace.files.write/patch/apply/discard/delete/move/copy/mkdir` 等真实落盘或变更 resolution 动作。
   - `workspace.files.diff_many` 已纳入 staged-change registry route allowlist，但当前没有自然语言 diff_many extractor，仍主要由 HTTP API / LLM planner 结构化 action 使用。
   - 修正 heuristic action 顺序：带“预览/diff/stage”的 old/new 修改请求优先生成 `workspace.files.diff`，不误落到 `workspace.files.patch`。
8. ✅ 将 `routePrompt` 的 `reports.draft` 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖自然语言报告草稿生成，会继续通过 registry handler 写入当前 Agent workspace 的 markdown 草稿。
   - 保留 `workspace_write` operation-level gate、run_id / active run 解析与 review_required 结果语义。
   - route source metadata 继续标记为 `deterministic_route/registry_dispatch`。
9. ✅ 将 `routePrompt` 的 workspace mutation 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖自然语言 `workspace.files.write/patch/apply/discard/delete/move/copy/mkdir`，含 apply-all / discard-all。
   - 保留 `workspace_write` operation-level gate，registry handler 继续复用原 workspace handler，因此 write/patch/apply/discard/delete/move/copy/mkdir 的 workspace 安全、冲突处理、原子性与审计语义不变。
   - route source metadata 继续标记为 `deterministic_route/registry_dispatch`；`read_only` 不会调用 mutation registry handler。
10. ✅ 将 `routePrompt` 的 approval approve/deny/resume-approved 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖自然语言 `runs.approvals.approve`、`runs.approvals.deny` 与 `runs.resume_approved`。
   - 保留 `approval_ops` operation-level gate、最新用户显式指令匹配、approval comment 传递、active/current run fallback 与原 approval/resume handler 的 audit/event 语义。
   - route source metadata 继续标记为 `deterministic_route/registry_dispatch`；`read_only` 不会调用 approval write registry handler。
11. ✅ 将 `routePrompt` 的 schedule create/list/delete 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖自然语言 `schedule.create`、`schedule.list` 与 `schedule.delete`。
   - 保留 create/delete 的 `advanced` operation-level gate 与 list 的 `read_only` gate；权限不足时不会调用 registry handler。
   - registry handler 继续复用原 schedule create/list/delete handler，cron/prompt/id 抽取、scheduler persist 与返回链接语义保持不变。
   - route source metadata 继续标记为 `deterministic_route/registry_dispatch`。
12. ✅ 将 `routePrompt` 的 non-safe HTTP 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖自然语言 POST / PUT / PATCH / DELETE `http.request`。
   - 保留 `advanced` operation-level gate；`safe_scan` / `read_only` 权限不足时不会调用 registry handler。
   - registry handler 继续复用原 `handleAgentHTTPRequest`，method/url 校验、SSRF policy、响应截断与错误语义保持不变。
   - route source metadata 从 `deterministic_route/direct_handler` 更新为 `deterministic_route/registry_dispatch`。
13. ✅ 将 `routePrompt` 的 generic workspace fallback 本地路由小切片收敛为 `agentHeuristicActionsFromPrompt` → registry dispatch：
   - 覆盖只表达 `workspace` / `工作空间`、未命中具体 list/tree/search/read/write/mutation 的自然语言 fallback。
   - 现在映射到 `workspace.files.list path=.`，保留 `read_only` operation-level gate。
   - route source metadata 从 `deterministic_route/direct_handler` 更新为 `deterministic_route/registry_dispatch`；legacy `workspace.prepare` 直连 handler 暂不再作为该 fallback 的执行路径。
14. ✅ 将 no-LLM control continue 的 active-run 上下文读取小切片收敛为 `runtime.Engine.ExecuteTurn` 的 `deterministic_route` dispatch：
   - 覆盖用户输入 `continue` / `继续` 且当前 session 有 active run、未配置 LLM 时的自动上下文读取。
   - contextKind 选择仍按 pending approvals、report JSON、findings、artifacts、events 顺序；evidence action conclusion、`active_run_continue` append origin 与 response merge 保持原语义。
   - 实际工具执行继续落到 `routePromptForTurn` → `agentHeuristicActionsFromPrompt` → `runs.context` Tool Registry handler。
15. 当前收敛策略：不一次性搬迁 legacy turn，继续按 registry dispatch → lifecycle seam → LLM runner seam → engine-native runner 分层推进。
   - 原因：`SendMessage`、planner loop、授权 gate、session mutation 仍由 daemon 保证语义；一次性把 `runAgentLLMToolLoop` 完整下沉会同时牵动 session persist、authorization resume、tool synthesis、auto-continue 与 SSE replay，切片过大。
   - 已完成确定性工具 registry 化、`BeginTurn`/`FinishTurn` lifecycle API 与 `RunLLMTurn` dispatch seam。
   - 后续更稳妥路径是继续拆分 planner chat、tool synthesis、step replan、session mutation，再收敛 authorization resume / auto-continue 与剩余 SendMessage branches。

高风险 / 写动作 registry 化分批建议：

| 批次 | 动作 | 风险与状态 | 迁移建议 |
| --- | --- | --- | --- |
| A | read-only/system：daemon.health、plugins.list、runs.list、runs.context、runs.approvals.list、workspace list/tree/read/search、generic workspace fallback、allowlisted workspace.command | 已完成 | 保持 registry dispatch，并用 source metadata 观察回归；generic workspace fallback 现在映射为 `workspace.files.list path=.` |
| B | safe HTTP：GET / HEAD / OPTIONS `http.request` | 已完成本切片；仍会受 safe_scan 权限与 HTTP SSRF policy 约束 | 继续补 trace / auth profile 场景 |
| B2 | non-safe HTTP：POST / PUT / PATCH / DELETE `http.request` | 已完成自然语言本地路由 registry dispatch；会发起写语义 HTTP 请求但不改本地 workspace | 继续观察 `advanced` gate、SSRF policy、method/url/body/auth profile parity 与 source metadata |
| C | `ports.scan.safe` | 已完成本切片；网络探测但不落盘，参数边界较清晰 | 继续观察 port allowlist / source metadata parity |
| D | `scan.web.safe` | 已完成本切片；会启动 daemon job、创建 run/evidence/report；不是文件写动作但状态面大 | 继续观察 job/run/context refs/timeline 与 source metadata parity |
| E | workspace diff/rebase | 已完成自然语言 staged-change 本地路由小切片；diff/rebase 不直接改文件但会持久化 pending change state | 继续观察 staged state、conflict/rebase 与 source metadata parity；diff_many 暂无自然语言 extractor |
| E2 | `reports.draft` | 已完成本切片；会写当前 Agent workspace 的报告草稿，需要人工复核 | 继续观察 workspace draft path、review_required 与 source metadata parity |
| E3 | workspace diff_many | 结构化 state-only changeset，当前无自然语言 extractor | 后续独立切片，先补自然语言/结构化参数测试，再考虑 registry 化 local route |
| F | workspace write/patch/apply/discard/delete/move/copy/mkdir | 已完成自然语言本地路由 registry dispatch；真实 workspace mutation 或 staged change resolution，风险高 | 继续观察 `workspace_write` gate、explicit intent gate、source metadata 与既有 conflict/atomicity 测试 parity |
| G | runs.approvals.approve / deny / resume_approved | 已完成自然语言本地路由 registry dispatch；审批状态写动作，会改变 run 执行链 | 继续观察 `approval_ops` gate、最新用户显式指令匹配、comment/audit/event parity 与 source metadata |
| H | schedule.create / schedule.list / schedule.delete | 已完成自然语言本地路由 registry dispatch；create/delete 会改变 scheduler 持久状态，list 为只读 | 继续观察 `advanced`/`read_only` gate、cron/prompt/id 抽取、scheduler persist 与 source metadata parity |

engine-native turn lifecycle 下沉入口建议：

1. ✅ 先定义 engine 侧 `TurnExecutor`/`TurnContext` seam，只复用现有 daemon legacy executor，不改变行为。
2. ✅ 将 `beginRuntimeTurn` / `finishRuntimeTurn` 的状态迁移到 engine API，daemon 仍负责 session adapter。
3. 进行中：新增 `RunLLMTurn`/`LLMTurnRunner` seam、`RunLLMStage`/`LLMStageRunner` seam 与 `MutateSession`/`SessionMutator` seam；`SendMessage` 的 LLM 主入口已经 engine dispatch 到 daemon legacy `runAgentLLMToolLoop`，loop 内部 planner chat / tool synthesis、auto-continue StepReplan synthesis、authorization resume synthesis、route-before-LLM/standalone route synthesis 已经 engine stage dispatch 到 `handleAgentLLMChat` / `synthesizeAgentToolResults`；`appendMessagesWithPlanner`、`SendMessage` 开头 user message record、`DecideAgentAuthorization` approve/deny 状态写入、`ControlSession` 状态写入、planner/manual authorization request 与 capability mismatch blocked state 已经 engine session mutation dispatch，LLM fallback manual authorization branch 已补覆盖确认 provider failure warning + authorization append mutation 与 pending authorization request mutation 的 turn metadata 对齐；no-LLM deterministic route、control auto-continue route、active-run continue context fetch、LLM provider failure fallback route、route-before-LLM deterministic pre-route、后续 replan action continuation、authorization approved planner action execution，以及 LLM planner/replan loop 内部 action execution 已经 `ExecuteTurn` dispatch 到 daemon legacy turn adapters；planner/replan/LLM/SendMessage append 与外部 control/authorization append 已经改为携带 turnRef 的 `append_messages` dispatch；authorization approve 后的 planner action execution、tool synthesis 与 follow-up continuation runtime runner 也已贯穿同一个 external turnRef；append/control/resolve authorization 的 `memory_compacted` events 已补齐 mutation turnRef metadata；deterministic route 内部 capability-gap plan-only planner apply 已沿用外层 turnRef 进入 turn-aware append mutation；`llm_tool_loop` / `replan_action_continuation` engine-native loop runner 草案已补齐，`Engine.RunAgentLoop` / `AgentLoopRunner` seam 已挂接 daemon shared cycle adapter，shared cycle 内部主状态已推进到 runtime `AgentLoopRequest` / `AgentLoopResult` / `AgentLoopState`，action execution / tool synthesis helper 与外层 cycle dispatch 入口均已改为 runtime DTO 输入输出；planner loop action queue metadata 与 authorization/capability gate decision helper 已抽出并补 parity 覆盖；`SendMessage` response merge regression matrix 已补齐 control continue/revise、pending authorization gate、route-before-LLM synthesis，并复跑 no-LLM route、LLM fallback 与 active-run continue parity。
4. ✅ `SendMessage` Begin/Finish lifecycle 已收敛为 `runtime.Engine.RunTurnLifecycle` bounded runner；response merge branches 拆入 daemon turn body helper，继续作为迁移 adapter 保持 parity。

已完成的 seam 边界：

- `runtime.Engine` 新增可选 `TurnExecutor`，`ExecuteTurn` 只转发到 attached executor，不主动发 runtime lifecycle events。
- `TurnContext` 承载 session/task/run/turn ID、message ID、prompt/goal、origin、operation level、turn index、timestamp、metadata，以及迁移期 daemon-private `Payload`；daemon session mutation、planner loop、authorization gate 仍留在现有路径。
- daemon `AttachEngine` 会挂接 `daemon_legacy_adapter`；默认无 kind 时仍返回 `running` 占位结果，明确表示 legacy `SendMessage` 仍拥有 turn execution；`deterministic_route` kind 会执行现有 daemon `routePromptForTurn` 并返回 daemon-private messages/toolCalls payload，`replan_action_continuation` kind 会执行现有 daemon `runAgentReplanActionContinuation` 并返回 daemon-private messages/toolCalls payload，`planner_action_execution` kind 会执行现有 daemon `executeAgentPlannerAction` 并返回 daemon-private messages/toolCalls payload，因此 no-LLM route、control auto-continue route、LLM provider failure fallback route、route-before-LLM deterministic pre-route、后续 auto-continue continuation、authorization approved planner action execution 与 LLM planner/replan loop 内部 action execution 可先经 `ExecuteTurn` seam 调度且不改变 append/response/runtime event parity；deterministic route 内部 capability-gap plan-only planner apply 也会沿用当前 turnRef dispatch 到 append mutation。

已完成的 lifecycle API 边界：

- `runtime.Engine.BeginTurn` 接管 task/run/turn state 写入与 `task_created`、`run_started`、`turn_started` 事件发布。
- `runtime.Engine.FinishTurn` 接管 turn state 写入、`turn_finished`/`turn_failed` 发布，以及 approval required、task finished、task failed run 终态分派。
- `runtime.Engine.RunTurnLifecycle` 已提供 Begin → bounded body → Finish 包装；body 负责返回 `TurnResult`，engine 负责 lifecycle state 与 terminal events。
- daemon `beginRuntimeTurn` / `finishRuntimeTurn` 只保留外部兼容/测试 adapter；`SendMessage` 现在经 `runRuntimeTurnLifecycle` 映射 session snapshot、message ID 与 response completion，bounded body 内的 control/pending-auth/route-before-LLM/LLM fallback/no-LLM response merge 仍由 daemon helper 保持原语义。

已完成的 LLM runner seam 边界：

- `runtime.Engine` 新增可选 `LLMTurnRunner`，`RunLLMTurn` 只负责 dispatch，不发布 lifecycle events，不改变 Begin/Finish ownership。
- `LLMTurnRequest` 使用稳定 `TurnContext` 承载 runtime 标识；临时 `Payload any` 只作为 daemon-private planner/session 类型尚未迁出 daemon 前的迁移适配层。
- daemon `AttachEngine` 会挂接 `daemon_legacy_llm_tool_loop`；`SendMessage` 中 control continue/revise 与普通 LLM configured 两个主入口已改为调用 `runRuntimeLLMToolLoop`，实际执行仍复用原 `runAgentLLMToolLoop`，保持 model streaming、tool action、synthesis、authorization gate 与 session persist 语义不变。

已完成的 LLM stage runner seam 边界：

- `runtime.Engine` 新增可选 `LLMStageRunner`，`RunLLMStage` 只负责 dispatch 单个 bounded LLM stage，不发布 lifecycle events。
- 已接入 stage 为 `planner_chat` 与 `tool_synthesis`。
- daemon `AttachEngine` 会挂接 `daemon_legacy_llm_stage`；`runAgentLLMToolLoop` 首个 planner chat 已改为调用 `runRuntimeAgentLLMPlannerChat`，内部 tool synthesis、`runAgentReplanActionContinuation` 的 auto-continue synthesis、authorization approve resume synthesis、route-before-LLM/standalone route synthesis 已改为调用 `runRuntimeAgentLLMToolSynthesis`。
- 实际执行仍复用原 `handleAgentLLMChat` / `synthesizeAgentToolResults`，保持 provider fallback、model_token/model_reasoning streaming、planner JSON parsing、native tool call extraction、StepReplan/final_answer 解析与 planner/tool_synthesis metadata 语义不变。

已完成的 session mutation seam 边界：

- `runtime.Engine` 新增可选 `SessionMutator`，`MutateSession` 只负责 dispatch 单个 bounded session mutation，不发布 lifecycle events。
- 已接入 mutation 为 `append_messages`、`record_user_message`、`request_authorization`、`capability_blocked`、`control_session` 与 `resolve_authorization`，覆盖 legacy `appendMessagesWithPlanner` 的 messages append、tool/link/planner state apply、session persist、memory compaction 与 runtime events 发布，`SendMessage` 开头的 user message 记录、ActiveTask start/touch、history/memory snapshot 与 persist，planner/manual authorization request 的 `PendingAuthorization` 创建、session status/next_action 更新与 persist，capability mismatch 的 session/task/todo blocked 与 next_action 更新，`ControlSession` 的 continue/revise/pause/cancel/stop/new_task 状态更新、persist、compaction 与 runtime events 发布，以及 `DecideAgentAuthorization` approve/deny 的 pending 校验、授权状态更新、session/task/todo/next_action 更新、persist、compaction 与 runtime events 发布。
- daemon `AttachEngine` 会挂接 `daemon_legacy_session`；实际执行仍复用原 legacy session helpers，保持 session lock 顺序、planner apply、compaction、persist、`memory_compacted` 与 message runtime events 语义不变；append/control/resolve authorization 的 `memory_compacted` events 现在使用 mutation turnRef，legacy wrapper 无显式 turnRef 时按 session/message/auth seed 补齐 external turnRef。
- `record_user_message` 实际执行仍复用 daemon legacy helper，保持 user message timestamp、task start/touch、history/memory snapshot、persist best-effort、`BeginTurn` 先于 memory compaction event 的原有顺序。
- `request_authorization` 实际执行仍复用 daemon legacy helper，保持授权请求 ID、RequiredLevel/Policy/Reason/OriginalPrompt、PendingAuthorization、waiting_authorization/session next_action 与返回授权提示 message 文案/phase 不变；授权提示 message 仍由外层 planner/SendMessage append mutation 负责落入 messages。
- `capability_blocked` 实际执行仍复用 daemon legacy helper，保持 non-equivalent tool mismatch 的 blocked session/task/todo、blocked reason、next_action、persist 与外层 `step_replan` message/tool 返回顺序不变。
- `control_session` 实际执行仍复用 daemon legacy helper，保持 continue/revise/pause/cancel/stop/new_task 对 session/task/todo/next_action/message、persist、`memory_compacted` 与 message runtime events 的原有语义；stop/cancel 的 daemon job cancel 副作用仍留在 `ControlSession` 外层 control flow。
- `resolve_authorization` 实际执行仍复用 daemon legacy helper，保持 pending authorization ID 校验、operation level uplift、approve/deny message 文案与 phase、ActiveTask/Todos/NextAction、`approval_resolved`、`memory_compacted` 与 message runtime events 语义不变；approve 后挂起 action 执行、tool message append、synthesis 与 auto-continue 仍留在原 `DecideAgentAuthorization` 后续路径，但 runtime runner dispatch 与 append mutation 已共享同一个 authorization external turnRef。

### 4.1 Engine-native LLM tool/replan runner 设计草案（P0.5-N5）

目标：把当前 daemon-owned `runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 后续收敛为一个 engine-native loop runner，同时不改变现有消息顺序、授权 gate、planner apply、tool call metadata、session persist、SSE/runtime event 与 `SendMessage` response merge 语义。

推荐形态：先设计为单个 loop runner、两个 mode，而不是继续维护两套循环：

- `llm_tool_loop`：从 planner chat 开始，负责 Plan → Action → StepReplan 循环。
- `replan_action_continuation`：从已有 `agentPlannerResponse` 开始，只在 `NextAction.AutoContinue=true` 时继续 Action → StepReplan 循环。

已落地 runtime-neutral 请求/结果 DTO（P0.5-N42 已下沉到 `internal/agent/runtime`；迁移期 daemon adapter 仍保持现有执行路径）：

```go
type AgentLoopMode string

const (
    AgentLoopModeLLMToolLoop        AgentLoopMode = "llm_tool_loop"
    AgentLoopModeReplanContinuation AgentLoopMode = "replan_action_continuation"
)

type AgentLoopRequest struct {
    Turn     TurnContext
    Mode     AgentLoopMode
    Language string
    Operator string

    LLMRequest    AgentLoopLLMRequest
    History       []AgentLoopMessage
    MemorySummary *AgentLoopMemorySummary
    InitialPlan   *AgentLoopPlan
    State         AgentLoopState

    Authorization AgentLoopAuthorization
    ToolServices  AgentLoopToolServiceBinding // daemon 仅在 adapter 边界解包 legacy service handles。
    MaxLoops      int
    Metadata      map[string]any
}

type AgentLoopResult struct {
    State                 AgentLoopState
    Messages              []AgentLoopMessage
    ToolCalls             []AgentLoopToolCall
    FinalPlan             *AgentLoopPlan
    StopReason            string // planner_error | no_actions | authorization_required | capability_blocked | max_loops | synthesis_error | completed
    AuthorizationRequired bool
    CapabilityBlocked     bool
    LoopCount             int
    ActionQueueIDs        []string
    Metadata              map[string]any
}
```

Clone 语义：`Turn.Metadata`、LLM/Auth profiles、history/messages/tool calls、memory `LastMessageAt`、plan/action args、state slices、service binding metadata、action queue IDs 与 metadata 均隔离；`Turn.Payload` 与 `ToolServices.Handle` 按 migration boundary 语义保留原引用。

执行状态机：

1. `llm_tool_loop` mode 先调用 `LLMStagePlannerChat`，返回 plan messages / planner tool calls / parsed plan；planner chat 失败时把 partial messages/toolCalls 原样返回 error，让 `SendMessage` 现有 LLM fallback branch 继续负责 warning + local route/manual authorization fallback。
2. planner chat 成功后立即经 `SessionMutationAppendMessages` append plan messages，并携带 `origin=planner_chat`、当前 task/run/turn/prompt/operation_level；如果 plan 内有 planner state，则同一次 mutation 负责 planner apply。
3. 若 plan 无 actions：
   - 仅当第 0 轮且 operator prompt 明确要求立即执行时，按当前 `agentFallbackPlannerFromPrompt` 生成 fallback actions。
   - 否则以 `no_actions` 停止，保持模型 Plan-only 行为。
4. 每轮最多取前 6 个 actions，并生成稳定 `action_queue_id`；所有 action tool call 与 message 内 tool call 都统一打上 `action_queue_id`、`action_index`、`action_count`、`action_name`。
5. action gate 顺序保持现有语义：
   - capability mismatch：先 `SessionMutationCapabilityBlocked` 写入 blocked session/task/todo/next_action，再返回 `step_replan` message/tool，停止。
   - prompt 未显式允许：`SessionMutationRequestAuthorization`，policy=`explicit_operator_intent_required`，停止。
   - operation level 不足：`SessionMutationRequestAuthorization`，policy=`operation_level_required`，停止。
   - manual authorization tool：`SessionMutationRequestAuthorization`，policy=`manual_tool_authorization_required`，停止。
   - gate 通过：调用 `planner_action_execution` turn executor（短期）或未来 engine-native tool runner（长期）。
6. action batch 返回后立即经 `SessionMutationAppendMessages` append，`llm_tool_loop` 使用 `origin=planner_loop`，`replan_action_continuation` 使用 `origin=replan_action_continuation`；authorization/capability gate 产生的 message 也走同一 append 顺序，然后停止。
7. action batch 无 message 时停止，避免空循环。
8. action batch 成功且无需授权时调用 `LLMStageToolSynthesis`；synthesis 失败不作为 loop error 外抛，而是生成当前 warning tool/message，append 后以 `synthesis_error` 停止。
9. synthesis 成功时经 `SessionMutationAppendMessages` append synthesis messages + planner apply，更新 `FinalPlan`。
10. 仅当新 plan 仍有 actions 且 `NextAction.AutoContinue=true` 时进入下一轮；达到 `MaxLoops` 后以 `max_loops` 停止。

ownership 边界：

- Engine loop runner owned：loop state、mode 分派、planner/action/synthesis 顺序、action queue metadata、stop reason、调用 stage runner / action executor / session mutator 的编排。
- LLM stage runner owned：provider fallback、streaming、planner JSON/native tool call extraction、StepReplan/final_answer 解析；不直接 append/persist。
- Planner action executor owned：单个 tool/action 的执行与失败 tool call 构造；不直接持久化 action batch message，除非 tool handler 自身有业务状态副作用。
- Session mutator owned：messages append、planner apply、PendingAuthorization / capability blocked 状态写入、session persist、memory compaction、message/memory runtime events。
- `SendMessage` lifecycle owned：`runtime.Engine.RunTurnLifecycle` 负责 BeginTurn/FinishTurn 包装与 terminal event 顺序。
- `SendMessage` daemon adapter owned（迁移期）：user message record、LLM provider failure fallback response merge、route-before-LLM/no-LLM/control/pending-auth 分支 response merge；这些分支已收敛到 bounded daemon helper，后续等 loop DTO/runtime-neutral 后再继续迁移。
- Credential ownership：API key / auth profile secret 只作为请求期 ephemeral input 传入 adapter，不进入 runtime events、persisted turn state 或长期 `AgentLoopResult`；持久化 metadata 只保留 provider/model/phase/status。

迁移顺序：

1. ✅ P0.5-N6 已把 action queue metadata 与 authorization/capability gate decision 抽成无副作用 helper，并补 parity tests。
2. ✅ P0.5-N9 已将 `runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 的 Action → append → tool synthesis → append 循环合并到 daemon-private `runAgentPlannerActionReplanCycle` helper；外层函数签名、mode precondition 与 append origin 保持不变。
3. ✅ 现有 `daemon_legacy_llm_tool_loop` runner 与 `replan_action_continuation` turn executor 已经通过各自外层函数复用同一 cycle helper，并用 focused tests 证明 planner/action/synthesis append origin、planner apply metadata、response/message/tool parity。
4. ✅ P0.5-N10 已先把 shared cycle helper 的散列参数收敛成 daemon-private `agentPlannerActionReplanCycleRequest` / `State` / `Result` DTO，明确 turn/message request/services/plan/options 与 response/tool/history state 边界。
5. ✅ P0.5-N11 已把 shared cycle request 的 turn identity/prompt/origin/operation level 收敛为 `runtime.TurnContext`；helper 只在调用 append/action/synthesis legacy adapter 时临时转换 `agentRuntimeTurnRef`。
6. ✅ P0.5-N12 已拆分 shared cycle request 的 authorization 输入与 LLM synthesis request：action execution 不再读取完整 `sendAgentMessageRequest`，完整请求暂只作为 LLM synthesis runner 兼容输入。
7. ✅ P0.5-N13 已把 shared cycle request 的 LLM synthesis 输入缩小为 cycle-private LLM DTO；operator prompt/operator/language 等 turn/body 字段不再随 LLM request 进入 cycle，legacy synthesis adapter 调用点临时转回 daemon request。
8. ✅ P0.5-N14 已把 shared cycle request 的 tool/service 输入封装为 cycle-private tool context；cycle DTO 不再直接暴露 `routerServices` 字段，legacy action executor 调用点临时解包同一 services。
9. ✅ P0.5-N15 已把 shared cycle request 的 auth profile 输入收敛为 cycle-private auth profile DTO；authorization 与 LLM request 都只在 legacy action/synthesis adapter 边界转换回 `agentHTTPAuthProfile`。
10. ✅ P0.5-N16 已把 shared cycle request/result 的 plan 输入输出封装为 cycle-private plan handle；cycle DTO 不再直接暴露 `*agentPlannerResponse` 字段，legacy action executor、synthesis 与外层 loop 边界临时解包。
11. ✅ P0.5-N17 已把 shared cycle state 的 response/history messages 与 top-level tool calls 封装为 cycle-private wrappers；cycle DTO 不再直接暴露 `[]agentMessage` / `[]agentToolCall` 字段，外层 loop、legacy synthesis 与返回边界显式转换。
12. ✅ P0.5-N18 已把 shared cycle request 的 memory summary 封装为 cycle-private memory wrapper；cycle DTO 不再直接暴露 `*agentMemorySummary` 字段，LLM synthesis legacy adapter 边界显式转换。
13. ✅ P0.5-N19 已把 shared cycle plan handle 的 legacy planner response 改为入口/出口深拷贝隔离；作为 N30 前置，避免外层 loop 或 legacy adapter 返回值共享可变指针。
14. ✅ P0.5-N20 已给 shared cycle plan handle 增加 cycle-native action/auto-continue 查询 helper；`llm_tool_loop` 与 `replan_action_continuation` 的循环判断不再为 actions/`NextAction.AutoContinue` 提前解包 legacy plan。
15. ✅ P0.5-N21 已把 shared cycle plan handle 的 action count / auto-continue 查询状态固化为构造时快照；helper 不再从内部 legacy plan 指针重新计算。
16. ✅ P0.5-N22 已把 shared cycle plan handle 的 action list 固化为构造时 `actionsSnapshot`；shared cycle action executor 直接消费 snapshot actions，不再为执行动作解包 legacy planner response。
17. ✅ P0.5-N23 已把 `runAgentLLMToolLoop` 外层 auto-continue 循环改为持有 cycle plan handle；下一轮直接使用 `cycle.Plan`，不再回填 `legacyPlannerResponse()`。
18. ✅ P0.5-N24 已把 `runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 外层循环改为持有 cycle state handle；每轮直接传递/接收 `cycleState`，不再把 response/tool/history 解包回 legacy slice 后重新包装。
19. ✅ P0.5-N25 已把 shared cycle StepReplan synthesis legacy adapter 调用集中到 `runAgentPlannerActionReplanCycleSynthesis`；主 cycle 不再直接展开 LLM request/history/memory/action message legacy 转换，返回值继续用 cycle wrappers/plan handle 承载。
20. ✅ P0.5-N26 已把 shared cycle action execution legacy adapter 调用集中到 `runAgentPlannerActionReplanCycleActionExecution`；主 cycle 不再直接展开 tool context / auth profile / plan action snapshot legacy 转换，返回值继续用 cycle wrappers 承载。
21. ✅ P0.5-N27 已把 shared cycle action/synthesis result 的 phase tagging 与 response/tool/history append 收敛到 cycle wrapper/state helper；主 cycle 仅在 append mutation 与 synthesis legacy adapter 边界临时展开 legacy messages/tool calls。
22. ✅ P0.5-N28 已把 cycle message/tool wrappers 内部存储替换为 cycle-private message/tool DTO；wrappers 不再直接持有 legacy `agentMessage` / `agentToolCall` slice，legacy 转换集中在 ingress/egress adapter。
23. ✅ P0.5-N29 已把 cycle memory summary wrapper 内部存储替换为 cycle-private memory DTO；wrapper 不再直接持有 legacy `*agentMemorySummary`，legacy 转换集中在 ingress/egress adapter。
24. ✅ P0.5-N30 已把 cycle plan handle 内部存储替换为 cycle-private plan/action DTO；handle 不再直接持有 legacy `*agentPlannerResponse`，legacy 转换集中在 ingress/egress adapter。
25. ✅ P0.5-N31 已把 cycle plan/action DTO 迁到 runtime-neutral `runtime.AgentLoopPlan` / `runtime.AgentLoopAction`；daemon 只保留 legacy planner response/action 转换 adapter。
26. ✅ P0.5-N32 已把 cycle memory summary DTO 迁到 runtime-neutral `runtime.AgentLoopMemorySummary`；daemon 只保留 legacy memory summary 转换 adapter。
27. ✅ P0.5-N33 已把 cycle authorization/auth profile DTO 迁到 runtime-neutral `runtime.AgentLoopAuthorization` / `runtime.AgentLoopAuthProfile`；daemon 只保留 legacy HTTP auth profile 转换 adapter。
28. ✅ P0.5-N34 已把 cycle LLM request DTO 迁到 runtime-neutral `runtime.AgentLoopLLMRequest`；daemon 只保留 legacy `sendAgentMessageRequest` 兼容转换 adapter。
29. ✅ P0.5-N35 已把 cycle tool call DTO 迁到 runtime-neutral `runtime.AgentLoopToolCall` / `runtime.AgentLoopToolCallError`；daemon 只保留 legacy `agentToolCall` 兼容转换 adapter。
30. ✅ P0.5-N36 已把 cycle message/link DTO 迁到 runtime-neutral `runtime.AgentLoopMessage` / `runtime.AgentLoopLink`；daemon 只保留 legacy `agentMessage` 兼容转换 adapter。
31. ✅ P0.5-N37 已把 cycle state DTO 迁到 runtime-neutral `runtime.AgentLoopState`；daemon state wrapper 仅持有 runtime state 并保留 legacy response/tool/history adapter。
32. ✅ P0.5-N38 已把 daemon LLM turn runner payload/result 迁到 runtime-neutral `runtime.AgentLoopTurnPayload` / `runtime.AgentLoopTurnResultPayload`；daemon `daemonLLMTurnPayload` 仅保留 runtime loop payload 与 service binding compatibility adapter。
33. ✅ P0.5-N39 已把 LLM stage runner planner_chat/tool_synthesis payload/result 迁到 runtime-neutral `runtime.AgentLoopLLMStagePayload` / `runtime.AgentLoopLLMStageResultPayload`；daemon stage runner 仅保留 legacy provider/plan/message 转换 adapter。
34. ✅ P0.5-N40 已把 tool service binding/adapter compatibility 迁到 runtime-neutral `runtime.AgentLoopToolServiceBinding`；LLM turn、deterministic route、replan continuation、planner action execution payload 与 shared cycle tool context 不再直接持有 `routerServices`，daemon 只在 adapter 边界解包。
35. ✅ P0.5-N41 已把 registry handler runtime 的 service 输入也迁到 runtime-neutral `runtime.AgentLoopToolServiceBinding`；builtin handlers 只通过 daemon compatibility helper 临时解包 legacy services。
36. ✅ P0.5-N42 已把 engine-native loop runner 顶层请求/结果契约下沉到 runtime-neutral `runtime.AgentLoopMode` / `runtime.AgentLoopRequest` / `runtime.AgentLoopResult`，并补 clone 隔离测试；daemon 执行行为暂不接入。
37. ✅ P0.5-N43 已给 daemon shared cycle request/result 增加 `AgentLoopRequest` / `AgentLoopResult` adapter 映射，验证 request/result roundtrip 与 clone 隔离；现有 `runAgentPlannerActionReplanCycle` 执行路径不变。
38. ✅ P0.5-N44 已新增 runtime `Engine.RunAgentLoop` / `AgentLoopRunner` seam，并挂接 daemon legacy runner adapter；`runAgentLLMToolLoop` / `replan_action_continuation` 通过 seam 调用 shared cycle，行为保持不变。
39. ✅ P0.5-N45 已将 shared cycle runner 的 action execution / tool synthesis 内部阶段进一步以 runtime `AgentLoopRequest` / `AgentLoopResult` / `AgentLoopState` 作为主状态传递，daemon-private cycle request/state wrapper 主要保留在 legacy adapter 边界。
40. ✅ P0.5-N46 已将 shared cycle action execution / tool synthesis helper 的输入输出继续收敛为 runtime `AgentLoopRequest` / `AgentLoopResult`，减少 `agentPlannerActionReplanCycleRequest` / `State` wrapper 在内部 helper 间传递。
41. ✅ P0.5-N47 已将 `runRuntimeAgentPlannerActionReplanCycle` 外层 dispatch 入口继续收敛为 runtime `AgentLoopRequest` / `AgentLoopResult`，减少外层 loop 对 cycle request/result wrapper 的依赖。
42. ✅ P0.5-N48 已将 `runAgentPlannerActionReplanCycleAgentLoop` 主流程的 turn/options/plan/state 读取拆成 runtime-native helper，主流程不再先构造完整 `agentPlannerActionReplanCycleRequest` wrapper。
43. ✅ P0.5-N49 已将 shared cycle 主流程中的 plan/action 查询替换为 runtime-native helper，`runAgentPlannerActionReplanCycleAgentLoop` 以 runtime `AgentLoopPlan` 作为 final plan，legacy plan handle 仅留在 request/result adapter 与 append/synthesis 边界。
44. ✅ P0.5-N50 已将 action execution helper 的 action list 输入收敛为 runtime `AgentLoopAction`，`executeAgentPlannerActionList` 作为 legacy executor adapter 边界统一转换到 `agentPlannerAction`。
45. ✅ P0.5-N51 已将 action gate / authorization request 路径改为 runtime `AgentLoopAction` helper，`executeAgentPlannerActionList` 内部不再维护 legacy action list，仅在 execution / authorization mutation adapter 边界转换。
46. ✅ P0.5-N52 已将 planner action execution turn payload 收敛为 runtime `AgentLoopAction`，executor payload 不再直接持有 legacy `agentPlannerAction`，legacy action 仅在 `executeAgentPlannerAction` adapter 边界构造。
47. ✅ P0.5-N53 已将 authorization request mutation payload 的 `Action` 收敛为 runtime `AgentLoopAction`，request-authorization mutation payload 不再直接持有 legacy action，legacy action 仅在 `requestAgentAuthorizationLegacy` adapter 边界构造。
48. ✅ P0.5-N54 已将 planner action execution turn payload 的 auth profile 输入收敛为 runtime `AgentLoopAuthorization`，executor payload 不再直接持有 legacy `[]agentHTTPAuthProfile`。
49. ✅ P0.5-N55 已将 `runRuntimePlannerActionExecution` / `executeAgentPlannerActionList` 的 auth profile 参数收敛为 runtime `AgentLoopAuthorization`，action execution helper 内部不再传递 legacy auth profile slice。
50. ✅ P0.5-N56 已将 `executeAgentPlannerActionList` / `runRuntimePlannerActionExecution` 的 services 参数收敛为 runtime `AgentLoopToolServiceBinding`，action execution helper 内部不再传递 legacy `routerServices`。
51. ✅ P0.5-N57 已将 planner action execution turn result payload 收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，action execution turn result 不再直接持有 legacy `agentMessage` / `agentToolCall`。
52. ✅ P0.5-N58 已将 `runRuntimePlannerActionExecution` / `executeAgentPlannerActionList` 的返回与 accumulator 收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，action execution helper 内部不再累积 legacy message/tool-call slice。
53. ✅ P0.5-N59 已将 `DecideAgentAuthorization` 批准后 action execution result 的 phase/metadata merge 收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换推迟到 append/response 边界。
54. ✅ P0.5-N60 已将 `DecideAgentAuthorization` approved-flow 的 response message/tool-call accumulator 收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换集中到 synthesis/follow-up adapter 与 final response 边界。
55. ✅ P0.5-N61 已将 `runRuntimeAgentLLMToolSynthesis` 的 tool message 输入收敛为 runtime `AgentLoopMessage`，legacy `[]agentMessage` 只在兼容 wrapper 与 LLM stage adapter 边界转换。
56. ✅ P0.5-N62 已将 `runRuntimeAgentLLMToolSynthesis` 的返回收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall` / `AgentLoopPlan`，legacy 转换推迟到 synthesis/follow-up adapter 边界。
57. ✅ P0.5-N63 已将 `runRuntimeAgentLLMPlannerChat` 的返回收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall` / `AgentLoopPlan`，legacy 转换推迟到 LLM loop adapter 边界。
58. ✅ P0.5-N64 已将 `runAgentLLMToolLoop` / `runRuntimeLLMToolLoop` 的返回收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换推迟到 `SendMessage` / LLMTurn adapter 边界。
59. ✅ P0.5-N65 已将 `SendMessage` LLM success/error response merge 内部 accumulator 收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换推迟到 `buildAgentSendResponseFromAgentLoop`、append mutation 与 warning/fallback adapter 边界。
60. ✅ P0.5-N66 已将 `SendMessage` deterministic route / no-LLM route response merge accumulator 收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，减少底部 route/synthesis 分支 legacy slice 累积。
61. ✅ P0.5-N67 已将 `SendMessage` control/error/authorization gate 简单响应分支收敛到 runtime response egress helper，消除 `runAgentSendMessageTurn` 内除 helper 外直接 `buildAgentSendResponse` 调用。
62. ✅ P0.5-N68 已将 `runAgentReplanActionContinuation` / `runRuntimeReplanActionContinuation` 返回收敛为 runtime `AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换推迟到 follow-up response merge 与 turn executor adapter 边界。
63. ✅ P0.5-N69 已将 `daemonReplanActionContinuationTurnPayload` 的 `Req` / `History` / `MemorySummary` / `Plan` 输入收敛为 runtime `AgentLoopRequest` / `AgentLoopLLMRequest` / `AgentLoopMemorySummary` / `AgentLoopPlan`，legacy 转换推迟到 continuation turn executor adapter 边界。
64. ✅ P0.5-N70 已抽出 runtime-native replan continuation request runner，`runAgentReplanActionContinuation` 成为 legacy wrapper，continuation turn executor adapter 不再把 `runtime.AgentLoopRequest` 转回散列 legacy 参数后再进入 loop。
65. ✅ P0.5-N71 已抽出 runtime-native LLM tool loop request runner，`runAgentLLMToolLoop` 成为 legacy wrapper，LLMTurn runner adapter 不再把 runtime turn payload 转回散列 legacy 参数后再进入 loop。
66. ✅ P0.5-N72 已将 `daemonLLMTurnPayload` 的 Loop/Services 进一步收敛为 `runtime.AgentLoopRequest`，复用 LLM tool loop request runner，减少 LLMTurn adapter 私有 payload wrapper。
67. ✅ P0.5-N73 已将 LLM turn runner payload 进一步改为直接携带 `runtime.AgentLoopRequest`，移除 `daemonLLMTurnPayload` 单字段 compatibility wrapper。
68. ✅ P0.5-N74 已清理已闲置的 `runtime.AgentLoopTurnPayload` DTO 与测试，LLM turn payload 契约统一到 `runtime.AgentLoopRequest` / `AgentLoopTurnResultPayload`。
69. ✅ P0.5-N75 已将 `runRuntimeLLMToolLoop` 入口收敛为 `runtime.AgentLoopRequest`，让 SendMessage LLM branch 先构造 runtime loop request 后再进入 engine LLMTurn dispatch。
70. ✅ P0.5-N76 已将 `runRuntimeReplanActionContinuation` 外层调用入口收敛为 `runtime.AgentLoopRequest`，让 follow-up continuation branch 先构造 runtime loop request 后再进入 turn executor dispatch。
71. ✅ P0.5-N77 已将 replan continuation turn executor payload 进一步改为直接携带 `runtime.AgentLoopRequest`，移除 `daemonReplanActionContinuationTurnPayload` 单字段 compatibility wrapper。
72. ✅ P0.5-N78 已将 replan continuation turn executor result payload 收敛为 `runtime.AgentLoopTurnResultPayload`，移除 `daemonReplanActionContinuationTurnResultPayload` daemon-private wrapper。
73. ✅ P0.5-N79 已将 planner action execution turn executor result payload 收敛为 `runtime.AgentLoopTurnResultPayload`，移除 `daemonPlannerActionExecutionTurnResultPayload` daemon-private wrapper。
74. ✅ P0.5-N80 已将 planner action execution turn executor payload 的 turn/language/operator/operation/services/authorization 输入收敛为 `runtime.AgentLoopRequest`，仅额外携带 `runtime.AgentLoopAction`。
75. ✅ P0.5-N81 已将 deterministic route turn executor payload 的 session/prompt/language/operator/operation/services 输入收敛为 `runtime.AgentLoopRequest`，移除 `daemonDeterministicRouteTurnPayload`。
76. ✅ P0.5-N82 已将 deterministic route turn executor result payload 收敛为 `runtime.AgentLoopTurnResultPayload`，移除 `daemonDeterministicRouteTurnResultPayload` daemon-private wrapper。
77. ✅ P0.5-N83 已为 planner action execution 的 `runtime.AgentLoopRequest + runtime.AgentLoopAction` 组合新增 runtime-neutral payload DTO，移除最后一个 turn executor daemon-private payload wrapper。
78. ✅ P0.5-N84 已将 `append_messages` session mutation payload 收敛为 `runtime.AgentLoopAppendMessagesPayload`，SessionID 改由 `SessionMutationRequest.Turn.SessionID` 承载，Messages/Plan 使用 `runtime.AgentLoopMessage` / `runtime.AgentLoopPlan`，legacy 转换留在 daemon mutator adapter 边界。
79. ✅ P0.5-N85 已将 `record_user_message` session mutation payload 收敛为 `runtime.AgentLoopRecordUserMessagePayload`，SessionID/Prompt/OperationLevel 改由 `SessionMutationRequest.Turn` 承载，payload 仅保留 Operator/Language，legacy 转换留在 daemon mutator adapter 边界。
80. ✅ P0.5-N86 已将 `request_authorization` session mutation payload 收敛为 `runtime.AgentLoopAuthorizationRequestPayload`，SessionID/OperatorPrompt/OperationLevel 改由 `SessionMutationRequest.Turn` 承载，payload 保留 Language/Action/Policy/Reason 等授权字段，legacy 转换留在 daemon mutator adapter 边界。
81. 下一步 P0.5-N87 将 `capability_blocked` session mutation payload 的 SessionID/OperatorPrompt/OperationLevel 输入收敛到 runtime turn/context，并新增 runtime-neutral payload 承载 Language/ActionName/IntentID/Reason/SuggestedPrompt，legacy 转换留在 mutator adapter 边界。
82. ✅ P0.5-N7 已收敛 `SendMessage` response merge regression matrix；✅ P0.5-N8 已把 `SendMessage` Begin/Finish 主生命周期下沉到 `RunTurnLifecycle`，response merge branch body 暂留 daemon adapter。

最低回归覆盖：

- planner chat error partial output → LLM fallback branch warning/route/manual authorization 不变。
- plan-only 无 actions 与 immediate-execution fallback actions 分支不混淆。
- capability mismatch、explicit intent、operation level、manual authorization 四个 gate 的 mutation metadata 与 message append 顺序不变。
- action queue metadata 在 tool call 与 message 内 tool call 上一致。
- synthesis success/failure、auto-continue、max loops、空 action message 停止条件不变。
- `planner_loop` 与 `replan_action_continuation` append origin、task/run/turn/prompt/operation_level metadata 对齐。

---

## 5. Slice A：质量基线与回归矩阵计划

负责人：QA Codex  
状态：已完成并提交，commit `6221048 Docs: add quality baseline regression matrix`  
类型：计划/文档/测试矩阵，不改业务逻辑

交付文件：

- `docs/CyberAgent-Quality-Baseline-and-Regression-Matrix.md`

已验证：

- `git diff --check -- docs/CyberAgent-Quality-Baseline-and-Regression-Matrix.md` 通过。
- 文档时间维度禁用词检查无命中。
- `npm test --prefix web/frontend -- --run` 通过，6 files / 24 tests。
- `go test -count=1 ./internal/agent/runtime ./internal/runtime/state/sqlite ./internal/interfaces/daemon` 通过。

### 5.1 目标

建立可靠质量基线，保证后续 UI 重设计、runtime event、approval/recovery 优化不会引入不可发现的回退。

### 5.2 最低质量门禁候选

Backend：

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build -ldflags "-X main.version=ci -X main.commit=$(git rev-parse --short HEAD)" -o bin/cyberagent ./cmd/cyberagent
./bin/cyberagent version
./bin/cyberagent --version
```

Frontend：

```bash
npm ci --prefix web/frontend
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
```

### 5.3 扩展风险验证候选

```bash
go test -cover ./internal/agent/runtime ./internal/runtime/state/sqlite ./internal/interfaces/daemon ./internal/agent/tools ./internal/agent/toolregistry
go test -race ./internal/agent/runtime ./internal/runtime/state/sqlite ./internal/interfaces/daemon
go test -count=10 ./internal/agent/runtime ./internal/interfaces/daemon ./internal/runtime/state/sqlite
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
```

### 5.4 UI 毛玻璃回归矩阵

覆盖路径：

- `/agent` 初始加载。
- 无 session。
- 已有 session。
- 运行中 session。
- 失败 session。
- session list。
- composer 输入。
- timeline 追加事件。
- tool inspector 展开。
- trace drawer 打开/关闭。
- approval pending。
- approve / deny / edit-args / cancel 操作态。
- light / dark / system theme。
- blur fallback。
- 移动、平板、桌面、大屏 viewport。

验收：

- 核心文本可读。
- 半透明卡片不遮挡信息。
- 无横向滚动。
- 关键按钮可见且可操作。
- dialog/drawer 可通过键盘关闭。
- 浏览器 console 无关键错误。

---

## 6. Slice B：Glass Tokens + Primitives

负责人：Frontend Codex  
状态：已完成并提交，commit `77349c7 Frontend: add glass design primitives`  
类型：前端设计系统基础，不改后端

### 6.1 目标

将现有分散的 `glass-panel`、`mission-panel`、`agent-console`、`subtle-surface`、`cyber-grid` 等视觉类收敛为统一设计系统。

### 6.2 Design tokens 草案

背景：

- `--app-bg-base`
- `--app-bg-gradient-a`
- `--app-bg-gradient-b`
- `--app-bg-gradient-c`
- `--app-grid-opacity`

Glass surface：

- `--glass-bg-subtle`
- `--glass-bg-panel`
- `--glass-bg-elevated`
- `--glass-border`
- `--glass-border-active`
- `--glass-highlight`
- `--glass-shadow-soft`
- `--glass-shadow-deep`
- `--glass-blur-sm`
- `--glass-blur-md`
- `--glass-blur-lg`

状态色：

- `--status-info-bg`
- `--status-info-border`
- `--status-info-text`
- `--status-success-bg`
- `--status-success-border`
- `--status-success-text`
- `--status-warning-bg`
- `--status-warning-border`
- `--status-warning-text`
- `--status-danger-bg`
- `--status-danger-border`
- `--status-danger-text`

### 6.3 Component primitives 草案

- `GlassPanel`
- `GlassCard`
- `GlassToolbar`
- `GlassAlert`
- `GlassDrawer`
- `GlassDialog`
- `GlassDivider`
- `StatusPill`

### 6.4 兼容策略

- 保留现有 class 名作为过渡。
- 新 primitives 先包装现有视觉，不一次性全量迁移。
- 优先 `/agent`，其次 settings、tasks、trace、dashboard。

### 6.5 验收口径

- 渐变背景与毛玻璃层级明显。
- 半透明卡片具备深度感。
- 正文可读性不下降。
- `backdrop-filter` 有 fallback。
- `prefers-reduced-motion` 下动效弱化。
- `npm test --prefix web/frontend -- --run` 通过。
- `npm run build --prefix web/frontend` 通过。

---

## 7. Slice C：Runtime Event Catalog + Golden Fixtures

负责人：Backend Codex  
状态：已完成并提交，commit `e1cc68d Backend: add runtime event catalog fixtures`  
类型：后端契约与测试，不改 UI

### 7.1 目标

固定 runtime event 类型、字段和语义，让前端 UI、QA 测试、后端恢复/审批逻辑有共同契约。

### 7.2 已出现 runtime event 类型

- `task_created`
- `run_started`
- `task_finished`
- `task_failed`
- `turn_started`
- `turn_finished`
- `turn_failed`
- `model_token`
- `model_reasoning`
- `tool_started`
- `tool_finished`
- `todo_updated`
- `memory_compacted`
- `approval_required`
- `approval_resolved`

### 7.3 建议补齐/规范事件

- `heartbeat`
- `tool_progress`
- `recovery_started`
- `recovery_finished`
- `recovery_failed`
- `stream_replay_started`
- `stream_replay_complete`
- `stream_gap_detected`
- `subscriber_dropped`
- `stream_live`

### 7.4 Event envelope 草案

```json
{
  "seq": 1,
  "time": "...",
  "type": "...",
  "session_id": "...",
  "task_id": "...",
  "run_id": "...",
  "turn_id": "...",
  "tool_call_id": "...",
  "data": {}
}
```

### 7.5 最小文件范围

- `internal/agent/runtime/runtime.go`
- 可新增：`internal/agent/runtime/events.go`
- `internal/agent/runtime/engine_test.go`
- `internal/runtime/state/sqlite/store_test.go`
- `internal/interfaces/daemon/agent_subscribe_test.go`
- `testdata/runtime-events/*.json`
- 可选：`docs/adr/ADR-runtime-event-ui-state.md`

### 7.6 验收口径

- runtime event 类型有唯一 catalog。
- golden fixture 覆盖：
  - normal completion。
  - tool execution。
  - approval required。
  - approval resolved。
  - recovery。
  - stream replay。
- 旧 API 字段不破坏。
- 前端 normalizer/reducer 可基于 fixture 做契约测试。

---

## 8. Slice D：Agent Cockpit 毛玻璃主界面重构

负责人：Frontend Codex  
状态：已完成并提交，核心 commit `75d3abe feat(frontend): migrate cockpit shell to glass primitives`、`566ac7c feat(frontend): migrate timeline composer to glass primitives`、`057f5df fix(frontend): tune tool inspector visual density`

### 8.1 目标

优先改造核心 `/agent` 页面，让 Agent Runtime Cockpit 成为视觉升级主体现。

### 8.2 核心区域

- TaskTree。
- TurnTimeline。
- AgentConsole。
- ToolInspector。
- Composer。
- Approval banner。
- Trace drawer。
- New task modal。

### 8.3 三层深度设计

背景层：

- 深色渐变。
- 低透明 cyber grid。
- 柔和 radial/conic 光源。

容器层：

- TaskTree / AgentConsole / ToolInspector 使用统一 `GlassPanel`。
- active / idle / critical 状态有明确描边和 glow。

内容层：

- 消息 bubble、tool card、timeline group、composer 使用更高对比 surface。
- 大段 JSON / tool result 不因透明背景变难读。

### 8.4 同步修复

- icon-only button 补 `aria-label` / `title`。
- Trace drawer 补 `role=dialog`、`aria-modal`、可访问名称。
- New task modal 补 dialog semantics、Escape close、focus return。
- 小屏 Composer 增加 safe-area padding。
- 长文本、长 JSON、长 tool result 不破版。

---

## 9. Slice E：后端 UI State 派生层

负责人：Backend Codex  
状态：已完成并提交，核心 commit `1abba3a Add derived agent UI state`、`0edd740 feat(frontend): consume optional agent ui state hints`

### 9.1 目标

后端提供稳定 `ui_state`，让毛玻璃 Cockpit 不需要从散落字段中推断状态。

### 9.2 字段草案

```json
{
  "ui_state": {
    "status": "idle | running | blocked | awaiting_approval | failed | done | recovered",
    "phase": "planning | reasoning | tool_running | waiting_user | recovering | finished",
    "severity": "info | success | warning | danger",
    "blocked_reason": "...",
    "approval_state": "...",
    "stream_state": "...",
    "error_hint": "...",
    "session_id": "...",
    "task_id": "...",
    "run_id": "...",
    "turn_id": "...",
    "tool_call_id": "..."
  }
}
```

### 9.3 最小文件范围

- `internal/interfaces/daemon/agent.go`
- `internal/interfaces/daemon/agent_session_state.go`
- `internal/interfaces/daemon/agent_session_helpers.go`
- `internal/interfaces/daemon/agent_runtime.go`
- 对应 daemon agent session 测试

### 9.4 验收口径

- session list/detail/send response 都能返回一致 `ui_state`。
- pending authorization、run approval、tool running、failed、finished、recovery 场景都有 deterministic mapping。
- 前端旧字段仍可用。
- 不破坏现有 API 消费方。

---

## 10. Slice F：Approval Canonical State + Event Bridge

负责人：Backend Codex  
状态：已完成并提交，核心 commit `e46063d Add canonical approval state view`、`7605246 fix(frontend): preserve agent gate for run hitl approvals`

### 10.1 目标

统一 run HITL approval 与 Agent tool authorization 的展示语义，避免 UI 审批卡片混淆。

### 10.2 当前问题

两类不同审批都使用：

- `approval_required`
- `approval_resolved`

但语义不同：

1. Run workspace HITL approval：来自 scan / policy gate。
2. Agent tool authorization：来自 agent 执行高风险 tool。

### 10.3 canonical 字段草案

```json
{
  "approval_kind": "run_hitl | agent_authorization",
  "canonical_state": "pending | approved | denied | cancelled | modified | stale",
  "decision": "approve | deny | cancel | edit_args",
  "resume_available": true,
  "resume_state": "ready | blocked | stale | unavailable",
  "requested_action": "...",
  "resolved_by": "...",
  "comment": "...",
  "edit_args": {}
}
```

### 10.4 最小文件范围

- `internal/core/services/approvals.go`
- `internal/runtime/approvals/store.go`
- `internal/interfaces/daemon/router.go`
- `internal/interfaces/daemon/agent_runtime.go`
- `internal/interfaces/daemon/router_approval_test.go`
- `internal/interfaces/daemon/server_test.go` 中 approval 相关用例

### 10.5 验收口径

- approve / deny / cancel / edit-args 均发布 `approval_resolved`。
- cancel 保留明确 `approval_cancelled` 语义。
- engine 未启用时 HTTP response 仍包含足够 UI 状态。
- 前端能稳定区分 run HITL 与 agent authorization。
- 重复操作、无效 approval id、agent 不存在等错误路径有测试覆盖。

---

## 11. Slice G：Settings Secret Storage 文案与最小安全修正

负责人：Frontend Codex  
状态：已完成并提交，commit `5f3f18f fix(frontend): clarify settings secret storage`

### 11.1 目标

修复设置页密钥保存文案与真实行为不一致的问题。

### 11.2 当前发现

设置页文案暗示密钥只保存 daemon，但实际 `saveSettings()` 会将以下内容写入 `localStorage`：

- bearer token。
- AI provider apiKey。
- API Auth Profile secrets。

### 11.3 风险

- 用户可能误以为敏感信息没有保存在浏览器。
- 本地持久化策略和产品说明不一致。
- 产品可信度受影响。

### 11.4 推荐 P0 范围

先做最小修正：

- 明确说明敏感配置会保存在当前浏览器本地存储。
- 提醒用户仅在可信设备使用。
- 提供清除本地配置入口，或强化已有清除入口文案。
- 不改变现有后端配置模型。

### 11.5 后续可选方向

- 默认 sessionStorage，用户主动选择“记住配置”才写入 localStorage。
- 前端只保存引用 ID，daemon 负责安全存储。
- 增加敏感配置分级与可见存储状态。

### 11.6 验收口径

- 设置页文案与真实存储行为一致。
- 用户可以理解哪些信息保存在浏览器。
- 不改变现有功能。
- 不破坏已有 settings 流程。
- 前端测试和 build 通过。

---

## 12. Slice H：Recovery 状态冲突保护

负责人：Backend Codex  
状态：已完成并提交，commit `93004e4 fix(runtime): protect recovery from stale approval state`

### 12.1 目标

避免 crash recovery 把 approval wait 等语义状态误标为 failed，避免恢复后 UI 失真。

### 12.2 当前风险

- `RecoverInterruptedRuns` 会扫描 `pending/running/awaiting_approval` run。
- 当前非 terminal run 可能被标记 `failed`。
- `awaiting_approval` 本质是等待用户决策，不一定是 crash failure。
- recovery 后用户再处理 approval，可能出现 run 已 failed 但 approval 又 resolved/resume 的冲突。

### 12.3 优化方向

- `running/pending` 可恢复为 failed 或 recovered failed。
- `awaiting_approval` 不应简单丢失审批语义。
- 对 recovery event 与 state update 建立一致记录。
- 对 SaveRun 成功但 event append 失败的情况提供 health/error 反馈或补偿策略。
- session ActiveTask / NextAction 与 runtime recovery 结果要可收敛。

### 12.4 最小文件范围

- `internal/agent/runtime/runtime.go`
- `internal/runtime/state/sqlite/store.go`
- `internal/agent/runtime/engine_test.go`
- `internal/runtime/state/sqlite/store_test.go`
- `internal/interfaces/daemon/server.go`
- `internal/interfaces/daemon/agent_run_context.go`
- `internal/interfaces/daemon/agent_session_state.go`

### 12.5 验收口径

- running/pending run recovery 有明确 recovered failed 事件。
- awaiting_approval recovery 不丢 approval 语义。
- UI 可展示“需要继续处理审批”或“审批已过期/需刷新”。
- 重复 recovery 幂等。
- 恢复后 API/SSE 可读。
- 无 panic。

---

## 13. P1 优化

### 13.1 Route-level Lazy / Bundle Splitting

当前发现：

- `App.tsx` 静态导入所有页面。
- 2026-06-06 P1-F1 基线：生产 JS `442.68KB` / gzip `127.29KB`，CSS `50.80KB` / gzip `9.98KB`。
- 当前只有 `assets/index-*.js` 单入口页面 bundle，切到任一路由都会先下载 Cockpit、ScanDetail、Settings 等所有页面代码。

优化方向：

- Dashboard / Agent / Settings / Skills / MCP / History / ScanDetail 分包。
- 加 skeleton / loading state。
- 控制毛玻璃动效和 blur 面积。

P1-F1 已确定 lazy 边界：

| 路由 | Lazy chunk 边界 | 说明 |
| --- | --- | --- |
| `/agent` | `pages/agent-cockpit` + `agent/components/*` + `agent/state/*` | Cockpit 专属交互、timeline、trace drawer、tool inspector，不应进入非 agent 首屏。 |
| `/scan/:id` | `pages/scan-detail` | trace/audit/evidence/approval 细节最多，独立分包。 |
| `/settings` | `pages/settings` | provider/auth/web search/operation level 配置表单，独立分包。 |
| `/tasks`、`/scan/new` | `pages/new-scan` + `components/schedules-panel` | 两个 route 共享同一页面 chunk。 |
| `/skills`、`/mcp` | `pages/skills`、`pages/mcp` | 先 route lazy；如果 P1-F2 build 后 chunk 过碎，再合并为 registry chunk。 |
| `/history` | `pages/history` | run list/history 单独 lazy。 |
| `/`、`/ops` | `pages/dashboard` | 继续作为 landing route；可 lazy，但当前 route 会立即加载 dashboard chunk。 |

P1-F2 建议执行方式：

- 保留 `Layout`、`ConsoleProvider`、基础 UI shell eager。
- 在 `App.tsx` 使用 `React.lazy` + `Suspense` 包住 route element，fallback 使用轻量 glass/loading skeleton，避免空白跳转。
- build 后记录主入口 JS、每个 route chunk 与 gzip 变化；确认 `/agent`、`/scan/:id`、`/settings` 不再进入初始 JS。

P1-F2 实施结果：

- `web/frontend/src/App.tsx` 已改为 `React.lazy` + `Suspense` route element；`Layout`、`ConsoleProvider` 与基础 shell 继续 eager。
- route fallback 使用 `role=status` / `aria-live=polite` / `aria-busy=true` 的轻量 glass loading skeleton，避免路由切换空白。
- build 后主入口 JS 从 `442.68KB` / gzip `127.29KB` 降到 `233.51KB` / gzip `75.24KB`（raw 约 -47.3%，gzip 约 -40.9%）。

主要 chunk：

| Chunk | Raw | Gzip |
| --- | ---: | ---: |
| `index-Bbx7BD2J.js` | `233.51KB` | `75.24KB` |
| `agent-cockpit-BtTFLFE1.js` | `80.31KB` | `23.18KB` |
| `scan-detail-Do3Ftj3a.js` | `43.15KB` | `11.22KB` |
| `settings-C_ppjGEo.js` | `32.35KB` | `9.20KB` |
| `new-scan-CVnQm8IU.js` | `23.29KB` | `7.49KB` |
| `dashboard-BXeJBaa2.js` | `6.41KB` | `2.81KB` |
| `skills-OGZH4Wev.js` | `5.98KB` | `2.11KB` |
| `mcp-BPTOjz8r.js` | `6.01KB` | `2.46KB` |
| `history-CW1qVKtj.js` | `4.48KB` | `1.88KB` |

验收：

- 首屏 bundle 降低。
- route loading 状态合理。
- 不破坏路由。
- build/test 通过。

### 13.2 移动端 Cockpit 降噪

当前问题：

- 移动端 `/agent` 把 TaskTree + 主工作区纵向堆叠。
- 底部 nav 有 7 项。
- 首屏认知负荷偏高。

优化方向：

- 移动端 TaskTree 改为 drawer/sheet。
- 底部 nav 收敛为核心入口 + More。
- Composer 增加 safe-area padding。
- 减少小屏 icon-only 控件数量。

P1-F3 实施结果：

- `/agent` 顶层布局改为 mobile flex / desktop `xl` grid；桌面 `TaskTree` 在小屏隐藏，移动端新增 `MobileTaskSwitcher` 展示当前会话、最近会话列表、刷新与新建入口。
- Header 降噪：小屏仅保留 stream + operation level，`seq`、active task status、displayHint 延后到 `sm/md` 以上展示；Trace/Refresh/Download icon-only button 提升到 44px 触摸目标。
- Timeline 降噪：Turn metadata badge 小屏隐藏，tool cards 改为 `md` 以上双列，message bubble 小屏 `94%` 宽度，减少 640px 附近横向挤压。
- Tool Inspector 移动端改为选择工具后弹出 bottom sheet，桌面仍保留右侧 aside。
- Approval pending banner 动作区在移动端换行铺满，避免 `ml-auto` 挤压；Composer 增加 safe-area padding，发送/继续/暂停/取消等移动端按钮提升触摸面积。
- CSS 对移动端 `.agent-scroll-area` 关闭 stable gutter 并缩小 scrollbar，优先通过布局收敛避免横向滚动。

验收：

- 390px、768px、1024px、1440px 下无横向滚动。
- Composer 不遮挡 timeline。
- TaskTree 可打开/关闭/恢复。
- 核心操作不被隐藏。

### 13.3 浏览器 E2E / a11y / 视觉回归

P1-F4 已落地真实浏览器 smoke：

- `scripts/agent-cockpit-browser-smoke.mjs` 随机端口启动 daemon 与 Vite dev server，并使用系统 Chrome + Playwright 打开 `/agent`。
- 脚本注入 `cyberagent_console_settings`，让前端直连本轮 daemon token，并配置一个故意失败的 OpenAI provider，用于稳定触发 LLM fallback manual authorization。
- 覆盖 daemon health prompt、session/timeline 渲染、`workspace.files.write` authorization pending、点击“允许并继续”、workspace write 执行与 daemon workspace read API 内容校验。
- 验证命令：`npm test --prefix web/frontend -- --run`、`npm run build --prefix web/frontend`、`node --check scripts/agent-cockpit-browser-smoke.mjs`、`node scripts/agent-cockpit-browser-smoke.mjs`、`git diff --check`。

P1-F5 已落地 Agent Cockpit a11y 回归：

- 新增 `web/frontend/src/agent/components/dialog-a11y.ts`，统一处理 modal 初始焦点、Tab/Shift+Tab focus trap、Escape 关闭与 unmount 后焦点返回。
- Trace drawer 与移动端 Tool Inspector sheet 补齐 `aria-labelledby`、`aria-describedby`、可编程聚焦、关闭按钮标记与焦点恢复；Composer textarea 改为真实 `<label>` + sr-only 键盘提示。
- TaskTree、mobile session list、timeline 工具卡/建议按钮补齐键盘 focus-visible ring、`aria-current` 与 `aria-pressed` 等状态语义。
- 新增 `scripts/agent-cockpit-a11y-smoke.mjs`，使用随机端口 daemon + Vite + 系统 Chrome 校验 visible button accessible name、form label、dialog name、Trace drawer focus trap、Escape close 与 focus return。
- 验证命令：`npm test --prefix web/frontend -- --run`、`npm run build --prefix web/frontend`、`node --check scripts/agent-cockpit-browser-smoke.mjs`、`node --check scripts/agent-cockpit-a11y-smoke.mjs`、`node scripts/agent-cockpit-browser-smoke.mjs`、`node scripts/agent-cockpit-a11y-smoke.mjs`、`git diff --check`。

P1-F6 已落地 Agent Cockpit 视觉回归 smoke：

- 新增 `scripts/agent-cockpit-visual-smoke.mjs`，沿用 standalone Playwright + 系统 Chrome，随机端口启动 daemon 与 Vite，不引入 `@playwright/test`、不下载浏览器。
- 脚本生成 `.tmp/e2e/.../screenshots/` 下 6 张人工可审查截图：initial desktop、session desktop、Trace drawer、approval pending desktop、destructive hover desktop、approval pending mobile。
- DOM computed style 断言覆盖 layered gradient backdrop、glass blur/fallback、半透明卡片、Trace drawer elevated overlay、approval pending warning、destructive deny hover 与移动端 approval layout。
- 验证命令：`npm test --prefix web/frontend -- --run`、`npm run build --prefix web/frontend`、`node --check scripts/agent-cockpit-visual-smoke.mjs`、`node scripts/agent-cockpit-visual-smoke.mjs`、`git diff --check`。

覆盖路径：

- `/agent` 初始加载。
- session list 可见。
- composer 可输入。
- timeline 接收事件。
- tool inspector 可打开。
- approval pending 可见。
- approve / deny / edit-args / cancel 操作态。
- Trace drawer 打开/关闭。
- New task modal 打开/关闭。
- SSE reconnect 使用 `from_seq` 不重复。
- light/dark/system theme。
- blur fallback。
- axe/a11y smoke。

验收：

- 无严重 a11y violation。
- icon-only button 有可访问名称。
- dialog 有 role、aria-modal、focus trap、Escape。
- 截图 diff 可审查。
- 核心路径无明显卡顿。

---

## 14. P2 优化

### 14.1 Run List Benchmark + 可重建索引

目标：降低 `/api/v1/runs` 随 workspace 增长后的扫描成本。

最小文件范围：

- `internal/core/services/runs.go`
- `internal/core/services/runs_test.go`
- `internal/core/services/runs_benchmark_test.go`
- 可选：`internal/core/services/run_index.go`

验收：

- benchmark 覆盖不同规模 run workspace。
- 索引可重建。
- 不替代 run workspace 事实源。
- 旧 response 字段保留。

### 14.2 Tool Registry / MCP / Scheduler Observability View

目标：为 Cockpit 能力页、MCP 页、定时任务页提供稳定状态来源。

建议字段：

```json
{
  "source": "builtin | plugin | mcp",
  "enabled": true,
  "health": "ok | degraded | failed",
  "risk": "low | medium | high",
  "approval_required": true,
  "auto_run": false,
  "last_result": "...",
  "canonical_state": "..."
}
```

文件范围：

- `internal/interfaces/daemon/agent_toolregistry.go`
- `internal/interfaces/daemon/agent_mcp.go`
- `internal/interfaces/daemon/scheduler.go`
- `internal/interfaces/daemon/router.go`
- `internal/agent/tools/registry.go`
- `internal/agent/toolregistry/registry.go`

验收：

- capability view 输出稳定。
- MCP discovery 输出 stable status/error/tool_count。
- Scheduler 输出 last_result/canonical_state/session linkage。
- 不改变 planner 执行策略。

---

## 15. 文件边界

### 15.1 Frontend Codex

允许：

- `web/frontend/**`
- 前端组件文件。
- 前端样式文件。
- 前端测试文件。
- 前端构建配置中必要小修改。

禁止：

- 修改后端 runtime / approval / recovery 逻辑。
- 修改 API contract。
- 引入重型 UI 框架。
- 只做视觉效果但忽略 a11y 和 fallback。

### 15.2 Backend Codex

允许：

- `internal/agent/runtime/**`
- `internal/runtime/state/sqlite/**`
- `internal/runtime/approvals/**`
- `internal/core/services/**`
- `internal/interfaces/daemon/**`
- 对应 Go 测试文件。
- `testdata/runtime-events/**`
- 可选 ADR 文档。

禁止：

- 修改前端 UI。
- 做无关大重构。
- 混入机械拆分。
- 同一提交混入 approval/recovery/UI state 多类风险改动。

### 15.3 QA Codex

允许：

- 测试文件。
- smoke checklist 文档。
- CI / 测试配置。
- fixtures。
- contract test。

禁止：

- 大改业务实现。
- 不经协调修改前后端核心逻辑。
- 将测试修复和业务重构混在一个提交。

---

## 16. 总体验收

### 16.1 UI 验收

- 毛玻璃视觉风格明确。
- 渐变背景自然。
- 半透明卡片具备层次和深度。
- Agent Cockpit 信息密度可控。
- Approval pending 明显。
- 破坏性操作视觉清晰。
- icon-only button 有可访问名称。
- dialog/drawer 有可访问语义和焦点管理。
- 移动端无横向滚动。
- blur fallback 可读。
- build/test 通过。

### 16.2 后端验收

- runtime event catalog 固定。
- event golden fixtures 可复用。
- `ui_state` 非破坏性输出。
- approval canonical state 清晰。
- recovery 不误伤 awaiting approval。
- SSE replay/reconnect 语义更稳定。
- 后端测试通过。
- 不破坏旧 API 字段。

### 16.3 QA 验收

- backend/frontend 测试命令固化。
- smoke checklist 可重复执行。
- UI 回归矩阵覆盖 `/agent` 核心路径。
- approval/recovery/runtime event 有重点测试。
- a11y/visual/browser smoke 有明确标准。
- 每个切片能判断是否回退。

---

## 17. 当前任务分派

### 17.1 QA Codex：Slice A 质量基线与回归矩阵计划

输出：

1. 最低质量门禁命令。
2. 扩展风险验证命令。
3. UI 毛玻璃回归矩阵。
4. runtime event / approval / recovery contract 测试建议。
5. smoke checklist 文档草案。

状态：已完成。

### 17.2 Frontend Codex：Slice B Glass Tokens + Primitives 实施计划

输出：

1. tokens 列表。
2. primitives 组件文件建议。
3. 旧 class 兼容策略。
4. `/agent` 首批迁移组件清单。
5. a11y 修复清单。
6. build/test 验收命令。

状态：已完成。

### 17.3 Backend Codex：Slice C Runtime Event Catalog 实施计划

输出：

1. runtime event 命名表。
2. event envelope 字段表。
3. golden fixture 目录结构。
4. 需要新增/修改的测试。
5. 对前端 normalizer/reducer 的契约建议。
6. 最小文件范围。

状态：已完成。

---

## 18. 变更记录

| 版本 | 更新内容 |
|---|---|
| v1 | 根据后端、前端、QA 只读调研生成后续优化方案；纳入毛玻璃 UI、runtime event catalog、UI state、approval canonical、recovery protection、质量基线、settings secret storage 文案修正。 |
| v2 | 同步 P0 Slice A–H 实际完成状态、提交证据与本地验证结果；将后续重点切换为 engine 默认路径、legacy 收敛、subagent 与 P1/P2 优化。 |
| v3 | Runtime engine 默认启用；保留 `CYBERAGENT_RUNTIME=legacy/off/disabled/false/0` 作为兼容回退开关。 |
| v4 | 更新 P0.5 registry dispatch 收敛状态：workspace mutation 与 approval approve/deny/resume-approved 自然语言本地路由已统一经 Tool Registry handler 执行。 |
| v5 | 新增 LLM runner seam 状态：`runtime.Engine.RunLLMTurn` 已可调度 daemon legacy `runAgentLLMToolLoop`，为后续 engine-native planner/synthesis/replan/session mutation 拆分做入口。 |
| v6 | 新增 LLM stage runner seam 状态：`runtime.Engine.RunLLMStage` 已可调度 planner chat stage，`runAgentLLMToolLoop` 的首个 planner chat 已经 engine stage dispatch。 |
| v7 | 扩展 LLM stage runner seam：`LLMStageToolSynthesis` 已接入 `runAgentLLMToolLoop` 内部 synthesis 调用，实际执行仍复用 daemon legacy synthesis。 |
| v8 | 扩展 StepReplan continuation seam：`runAgentReplanActionContinuation` 的 auto-continue synthesis 已复用 `LLMStageToolSynthesis`。 |
| v9 | 扩展 session-flow synthesis seam：authorization approve resume、route-before-LLM 与 standalone route synthesis 分支已统一经 `runRuntimeAgentLLMToolSynthesis` 调度。 |
| v10 | 新增 session mutation seam：`runtime.Engine.MutateSession` / `SessionMutator` 已可调度 `append_messages`，`appendMessagesWithPlanner` 经 engine dispatch 后仍复用 daemon legacy session mutation。 |
| v11 | 扩展 session mutation seam：`record_user_message` 已接入 `SendMessage` 开头 user message 记录与 history/memory snapshot，实际执行仍复用 daemon legacy helper。 |
| v12 | 扩展 session mutation seam：`resolve_authorization` 已接入 `DecideAgentAuthorization` approve/deny 状态写入，实际执行仍复用 daemon legacy helper，approve 后 action/synthesis 流程保持原路径。 |
| v13 | 扩展 session mutation seam：`control_session` 已接入 `ControlSession` continue/revise/pause/cancel/stop/new_task 状态写入；stop/cancel 的 daemon job cancel 副作用仍保留在外层 control flow。 |
| v14 | 扩展 turn executor seam：`SendMessage` no-LLM deterministic route 与 control auto-continue route 已经 `runtime.Engine.ExecuteTurn` dispatch 到 daemon legacy `routePrompt` adapter，实际 append/response 语义保持原路径。 |
| v15 | 扩展 turn executor seam：LLM provider failure fallback 的本地 route 已复用 `deterministic_route` adapter；AI failure warning、fallback messages prepend 与 tool call merge 语义保持原路径。 |
| v16 | 扩展 turn executor seam：LLM configured + route-before-LLM synthesis 的 deterministic pre-route 已复用 `deterministic_route` adapter；step phase tagging 与后续 synthesis 语义保持原路径。 |
| v17 | 扩展 turn executor seam：authorization approve resume 与 route-before-LLM 后续 auto-continue 的 `runAgentReplanActionContinuation` 已经 `replan_action_continuation` dispatch，实际 action execution / StepReplan synthesis / append 语义保持原路径。 |
| v18 | 扩展 turn executor seam：authorization approve 后的单个 planner action execution 已经 `planner_action_execution` dispatch；实际 action/tool execution、错误处理、authorization metadata 与后续 synthesis 语义保持原路径。 |
| v19 | 扩展 planner loop action execution seam：LLM planner loop 与 replan continuation 内部 `executeAgentPlannerActions` 已按单个 action 复用 `planner_action_execution` dispatch；authorization gate、错误 fallback、action queue metadata、phase tagging 与 StepReplan synthesis 语义保持原路径。 |
| v20 | 扩展 session mutation seam：planner loop / replan continuation 的 action/warning append 增加 turn-aware `append_messages` dispatch；实际 append/persist/compaction/runtime events 仍复用 daemon legacy helper。 |
| v21 | 扩展 planner-aware append seam：LLM planner chat 初始 plan append 与 tool synthesis planner apply 改为 turn-aware `append_messages` dispatch；实际 planner apply、append/persist/compaction/runtime events 仍复用 daemon legacy helper。 |
| v22 | 扩展 SendMessage append seam：deterministic route、route-before-LLM synthesis、LLM fallback route、control auto-continue 与 authorization-gate append 改为携带当前 turnRef 的 `append_messages` dispatch；`agent_session_flow.go` 剩余直接 append 仅保留在无 turnRef 的外部 control/authorization 入口。 |
| v23 | 扩展外部 control/authorization append seam：为 `ControlSession` / `DecideAgentAuthorization` 生成轻量 external turnRef，daemon job cancel、approved action 与 authorization synthesis append 改为 turn-aware `append_messages` dispatch；不接管完整 Begin/Finish turn lifecycle。 |
| v24 | 补齐 session mutation compaction metadata：append/control/resolve authorization 的 `memory_compacted` events 使用当前 mutation turnRef；legacy fallback 自动补齐 external turnRef，不新增 Begin/Finish lifecycle。 |
| v25 | 收敛 deterministic route 内部 plan-only append：capability-gap planner apply 通过 `routePromptForTurn` 沿用外层 turnRef dispatch 到 `append_messages` mutation，直接 `routePrompt` 保持 legacy 兼容。 |
| v26 | 收敛 schedule deterministic route：schedule create/list/delete 统一经 heuristic action → Tool Registry dispatch；create/delete 保留 `advanced` gate，list 保留 `read_only` gate。 |
| v27 | 收敛 advanced HTTP deterministic route：POST/PUT/PATCH/DELETE `http.request` 统一经 heuristic action → Tool Registry dispatch；保留 `advanced` gate 与原 HTTP handler/policy 语义。 |
| v28 | 收敛 generic workspace fallback deterministic route：未命中具体 workspace 子 intent 时 fallback 为 `workspace.files.list path=.` 并经 Tool Registry dispatch；不再走 legacy `workspace.prepare` direct handler。 |
| v29 | 收敛 active-run continue context fetch：no-LLM control continue 的 active run 上下文读取经 `deterministic_route` turn executor 调度，继续复用 `runs.context` registry handler 与 `active_run_continue` append 语义。 |
| v30 | 扩展 session mutation seam：planner/manual authorization request 的 `PendingAuthorization` 创建、session status/next_action 更新与 persist 已经 `request_authorization` mutation dispatch；授权提示 message append 顺序保持原路径。 |
| v31 | 扩展 session mutation seam：capability mismatch blocked state 已经 `capability_blocked` mutation dispatch；blocked session/task/todo/next_action 与 mismatch `step_replan` 返回顺序保持原路径。 |
| v32 | 补齐 LLM fallback manual authorization branch 覆盖：provider failure warning 与 authorization message append 经同一 turn-aware append mutation，pending authorization request mutation metadata 与 fallback turnRef 对齐。 |
| v33 | 输出 engine-native loop runner 设计草案：`llm_tool_loop` 与 `replan_action_continuation` 统一为单个 runner 的两个 mode，明确 payload、authorization/capability gate、action queue、synthesis、append/persist ownership 与 N6-N8 迁移顺序。 |
| v34 | 抽出 planner loop action queue metadata 与 authorization/capability gate decision helper；保持 gate 顺序、policy/reason、queue metadata 与 append/response 语义不变，并补 helper parity 覆盖。 |
| v35 | 补齐 SendMessage response merge regression matrix：覆盖 control continue/revise + LLM、pending authorization gate、route-before-LLM synthesis，并复跑 no-LLM route、LLM fallback route/authorization、active-run continue parity。 |
| v36 | 新增 `runtime.Engine.RunTurnLifecycle`，将 `SendMessage` Begin/Finish lifecycle 包装为 engine bounded runner；response merge branch body 拆入 daemon helper，补 runtime lifecycle 与 SendMessage event-order 回归。 |
| v37 | 完成 P1-F1 前端路由与 bundle 基线盘点：记录当前静态 route 单入口 JS/CSS 体积，并确定 `/agent`、`/scan/:id`、`/settings`、`/tasks` 与 `/scan/new`、`/skills` 与 `/mcp`、`/history`、`/` 与 `/ops` 的 route-level lazy 边界。 |
| v38 | 完成 P1-F2 route-level lazy / bundle splitting：`App.tsx` 使用 `React.lazy` + `Suspense` 延迟加载页面 route，主入口 JS 从 `442.68KB` / gzip `127.29KB` 降到 `233.51KB` / gzip `75.24KB`，并记录主要 route chunk。 |
| v39 | 完成 P1-F3 移动端 Agent Cockpit 降噪：新增移动端任务切换器、隐藏小屏桌面 TaskTree、收敛 header/timeline/tool card/approval/composer 密度，并补移动端 Tool Inspector bottom sheet；前端测试、build 与 diff check 通过。 |
| v40 | 完成 P1-F4 真实浏览器 Agent Cockpit smoke：新增 Playwright 脚本，用随机端口 daemon + Vite + 系统 Chrome 验证 `/agent` health prompt、manual authorization approval 与 workspace write/read 最小链路；前端测试、build、脚本语法检查、browser smoke 与 diff check 通过。 |
| v41 | 完成 P1-F5 Agent Cockpit a11y 回归：补 Trace drawer/mobile Tool Inspector dialog label/focus trap/Escape/focus return、Composer textarea label、TaskTree/timeline 键盘焦点态，并新增真实浏览器 a11y smoke；前端测试、build、browser smoke、a11y smoke 与 diff check 通过。 |
| v42 | 完成 P1-F6 Agent Cockpit 视觉回归 smoke：新增 standalone Playwright 脚本生成 6 张可审查截图，并用 computed style 校验 layered gradient、glass blur/fallback、半透明卡片、Trace drawer、approval pending、destructive hover 与移动端 approval layout；前端测试、build、脚本语法检查、visual smoke 与 diff check 通过。 |
| v43 | 完成 P0.5-N9 planner action/replan cycle helper 抽取：`llm_tool_loop` 与 `replan_action_continuation` 共享 daemon 内部 Action → append → tool synthesis → append 循环骨架，append origin、authorization/capability gate、response/tool parity 保持不变；focused daemon tests 与 daemon 包测试通过。 |
| v44 | 完成 P0.5-N10 planner action/replan cycle DTO 收敛：`runAgentPlannerActionReplanCycle` 改为 daemon-private request/state/result DTO，减少参数面并明确 future engine-native runner state 边界；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v45 | 完成 P0.5-N11 planner action/replan cycle TurnContext 边界：shared cycle request 使用 `runtime.TurnContext` 承载 session/task/run/turn/prompt/origin/operation level，legacy adapter 调用点临时转换 turnRef；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v46 | 完成 P0.5-N12 planner action/replan cycle auth/request 边界拆分：shared cycle request 将 authorization profiles/default profile 从 LLM synthesis request 中拆出，action execution 不再直接读取完整 `sendAgentMessageRequest`；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v47 | 完成 P0.5-N13 planner action/replan cycle LLM request DTO 收敛：shared cycle request 使用 cycle-private LLM DTO，只保留 origin/provider/model/API key/base URL/instructions/operation level/auth hints，并在 legacy synthesis adapter 调用点临时转回 daemon request；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v48 | 完成 P0.5-N14 planner action/replan cycle tool context 封装：shared cycle request 使用 cycle-private `agentPlannerActionReplanCycleToolContext`，不再直接暴露 `routerServices` 字段；legacy action executor 调用点临时解包同一 services，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v49 | 完成 P0.5-N15 planner action/replan cycle auth profile DTO 收敛：shared cycle request 的 authorization 与 LLM request 改为持有 cycle-private auth profile DTO，只在 legacy action/synthesis adapter 边界转换回 `agentHTTPAuthProfile`；新增 secret/enabled 深拷贝隔离测试，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v50 | 完成 P0.5-N16 planner action/replan cycle plan handle 封装：shared cycle request/result 使用 cycle-private `agentPlannerActionReplanCyclePlan`，不再直接暴露 `*agentPlannerResponse` 字段；legacy action executor、synthesis 与外层 loop 边界临时解包，新增 plan handle 边界测试，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v51 | 完成 P0.5-N17 planner action/replan cycle state message/tool wrappers：shared cycle state 使用 cycle-private `agentPlannerActionReplanCycleMessages` / `agentPlannerActionReplanCycleToolCalls`，不再直接暴露 `[]agentMessage` / `[]agentToolCall` 字段；新增 state 深拷贝隔离测试，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v52 | 完成 P0.5-N18 planner action/replan cycle memory summary wrapper：shared cycle request 使用 cycle-private `agentPlannerActionReplanCycleMemorySummary`，不再直接暴露 `*agentMemorySummary` 字段；新增 memory summary / `LastMessageAt` 深拷贝隔离测试，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v53 | 完成 P0.5-N19 planner action/replan cycle plan handle 深拷贝隔离：`agentPlannerActionReplanCyclePlan` 对 legacy planner response 做入口/出口 clone，覆盖 plan/action/todo/context/next_action 可变字段，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v54 | 完成 P0.5-N20 planner action/replan cycle plan handle 查询 helper：`agentPlannerActionReplanCyclePlan` 新增 `actionCount` / `hasActions` / `shouldAutoContinue`，外层 loop 优先通过 handle 判断 actions 与 auto-continue，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v55 | 完成 P0.5-N21 planner action/replan cycle plan handle 查询快照：`agentPlannerActionReplanCyclePlan` 将 action count / auto-continue 固化为构造时 `actionCountSnapshot` / `autoContinue`，查询 helper 不再读取内部 legacy plan 指针，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v56 | 完成 P0.5-N22 planner action/replan cycle plan handle action snapshot：`agentPlannerActionReplanCyclePlan` 将 action list 固化为构造时 `actionsSnapshot`，shared cycle action executor 改为消费 snapshot actions，不再为执行动作解包 legacy planner response，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v57 | 完成 P0.5-N23 LLM planner loop 保持 cycle plan handle：`runAgentLLMToolLoop` 外层 auto-continue 循环持有 `agentPlannerActionReplanCyclePlan`，下一轮直接使用 `cycle.Plan`，不再回填 legacy planner response；新增两轮 auto-continue 回归，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v58 | 完成 P0.5-N24 planner loop 保持 cycle state handle：`runAgentLLMToolLoop` 与 `runAgentReplanActionContinuation` 外层循环持有 `agentPlannerActionReplanCycleState`，每轮直接传递/接收 `cycleState`，不再解包 response/tool/history legacy slice 后重新包装；新增多轮 continuation state/history 回归，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v59 | 完成 P0.5-N25 planner cycle synthesis adapter helper：StepReplan synthesis legacy adapter 调用集中到 `runAgentPlannerActionReplanCycleSynthesis`，主 cycle 不再直接展开 LLM request/history/memory/action message legacy 转换；新增 helper state/payload/deep-copy/legacy-plan pointer 兼容覆盖，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v60 | 完成 P0.5-N26 planner cycle action execution adapter helper：action execution legacy adapter 调用集中到 `runAgentPlannerActionReplanCycleActionExecution`，主 cycle 不再直接展开 tool context / auth profile / plan action snapshot legacy 转换；新增 helper request/payload/deep-copy/action-queue metadata 兼容覆盖，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v61 | 完成 P0.5-N27 planner cycle wrapper phase/append helper：cycle message/tool wrappers 新增 `len` / `withPhase` / wrapper append helper，state 新增 response/history/tool append helper；主 cycle 的 action/synthesis result phase tagging 与 state append 不再直接展开 legacy slice，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v62 | 完成 P0.5-N28 planner cycle message/tool private DTO：cycle message/tool wrappers 内部存储改为 cycle-private message/tool DTO，不再直接持有 legacy `agentMessage` / `agentToolCall` slice；legacy 转换集中在 wrapper ingress/egress adapter，并补字段保真与深拷贝隔离覆盖，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v63 | 完成 P0.5-N29 planner cycle memory private DTO：cycle memory summary wrapper 内部存储改为 cycle-private memory DTO，不再直接持有 legacy `*agentMemorySummary`；legacy 转换集中在 wrapper ingress/egress adapter，并补字段保真与 LastMessageAt 深拷贝隔离覆盖，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v64 | 完成 P0.5-N30 planner cycle plan private DTO：cycle plan handle 内部存储改为 cycle-private plan/action DTO，不再直接持有 legacy `*agentPlannerResponse`；legacy 转换集中在 plan handle ingress/egress adapter，并补 plan/action 字段保真与深拷贝隔离覆盖，focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v65 | 完成 P0.5-N31 planner cycle plan runtime-neutral DTO：新增 `runtime.AgentLoopPlan` / `runtime.AgentLoopAction` 与 clone helpers，daemon cycle plan handle 改为持有 runtime DTO，legacy 转换集中在 daemon adapter；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v66 | 完成 P0.5-N32 planner cycle memory runtime-neutral DTO：新增 `runtime.AgentLoopMemorySummary` 与 clone helper，daemon cycle memory wrapper 改为持有 runtime DTO，legacy 转换集中在 daemon adapter；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v67 | 完成 P0.5-N33 planner cycle authorization runtime-neutral DTO：新增 `runtime.AgentLoopAuthorization` / `runtime.AgentLoopAuthProfile` 与 clone helpers，shared cycle request 与 LLM request auth profiles 改为持有 runtime DTO，legacy HTTP auth profile 转换集中在 daemon adapter；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v68 | 完成 P0.5-N34 planner cycle LLM request runtime-neutral DTO：新增 `runtime.AgentLoopLLMRequest` 与 clone helper，shared cycle LLM request 改为持有 runtime DTO，legacy `sendAgentMessageRequest` 兼容转换集中在 daemon adapter；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v69 | 完成 P0.5-N35 planner cycle tool call runtime-neutral DTO：新增 `runtime.AgentLoopToolCall` / `runtime.AgentLoopToolCallError` 与 clone helpers，cycle message/tool wrappers 改为持有 runtime DTO，legacy `agentToolCall` 兼容转换集中在 daemon adapter；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v70 | 完成 P0.5-N36 planner cycle message runtime-neutral DTO：新增 `runtime.AgentLoopMessage` / `runtime.AgentLoopLink` 与 clone helpers，cycle message wrapper 改为持有 runtime DTO，legacy `agentMessage` 兼容转换集中在 daemon adapter；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v71 | 完成 P0.5-N37 planner cycle state runtime-neutral DTO：新增 `runtime.AgentLoopState` 与 clone helper，daemon cycle state wrapper 改为持有 runtime state DTO，legacy response/message/tool/history 转换集中在 daemon adapter；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v72 | 完成 P0.5-N38 daemon LLM turn runner payload runtime-neutral DTO：新增 `runtime.AgentLoopTurnPayload` / `runtime.AgentLoopTurnResultPayload` 与 clone helpers，LLM turn runner payload 改为持有 runtime DTO，legacy request/history/memory/message/tool 转换集中在 daemon adapter；focused daemon/runtime tests、daemon 包测试与 diff check 通过。 |
| v73 | 完成 P0.5-N39 LLM stage payload/result runtime-neutral DTO：新增 `runtime.AgentLoopLLMStagePayload` / `runtime.AgentLoopLLMStageResultPayload` 与 plan pointer clone helper，planner_chat/tool_synthesis stage payload/result 改为 runtime DTO，legacy request/history/memory/tool messages/plan 转换集中在 daemon stage adapter；daemon/runtime 包测试与 diff check 通过。 |
| v74 | 完成 P0.5-N40 tool service binding runtime-neutral DTO：新增 `runtime.AgentLoopToolServiceBinding` 与 daemon binding adapter，LLM turn、deterministic route、replan continuation、planner action execution payload 与 shared cycle tool context 不再直接持有 `routerServices`；daemon/runtime 包测试与 diff check 通过。 |
| v75 | 完成 P0.5-N41 registry handler runtime service binding 收敛：`agentRegistryHandlerRuntime.Services` 改为 `runtime.AgentLoopToolServiceBinding`，builtin handlers 通过 compatibility helper 解包 legacy services；focused daemon tests、daemon/runtime 包测试与 diff check 通过。 |
| v76 | 完成 P0.5-N42 agent loop runner request/result runtime-neutral DTO：新增 `runtime.AgentLoopMode` / `runtime.AgentLoopRequest` / `runtime.AgentLoopResult` 与 clone helpers，固化 loop runner ownership 顶层请求/结果契约；runtime/daemon 包测试与 diff check 通过。 |
| v77 | 完成 P0.5-N43 daemon shared cycle ↔ agent loop DTO adapter：`agentPlannerActionReplanCycleRequest/Result` 新增 `runtime.AgentLoopRequest/Result` 映射与 metadata options/continue helper，覆盖 roundtrip、clone 隔离与 service binding handle 保真；runtime/daemon 包测试与 diff check 通过。 |
| v78 | 完成 P0.5-N44 runtime AgentLoopRunner seam：新增 `Engine.RunAgentLoop` / `AgentLoopRunner` 与 clone 隔离语义，daemon `AttachEngine` 挂接 legacy runner adapter，`runAgentLLMToolLoop` / `runAgentReplanActionContinuation` 的 shared cycle 先经 seam dispatch；runtime/daemon 包测试与 diff check 通过。 |
| v79 | 完成 P0.5-N45 shared cycle runtime DTO 主状态：`daemonAgentLoopRunner` 直接调用 runtime request 主流程，shared cycle 内部以 `runtime.AgentLoopState` 聚合 action execution / tool synthesis 输出并返回 `runtime.AgentLoopResult`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v80 | 完成 P0.5-N46 shared cycle helper runtime DTO 输入输出：action execution / tool synthesis helper 改为接收 `runtime.AgentLoopRequest` / `AgentLoopState` 并返回 `runtime.AgentLoopResult`，legacy helper 仅保留兼容 wrapper；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v81 | 完成 P0.5-N47 外层 cycle dispatch runtime DTO 主入口：新增 `runRuntimeAgentPlannerActionReplanCycleAgentLoop`，`runAgentLLMToolLoop` / `runAgentReplanActionContinuation` 每轮直接构造 runtime loop request 并用 runtime state/plan/result 串联；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v82 | 完成 P0.5-N48 shared cycle 主流程 runtime-native 读取：新增 turn/options/state/initial plan helper，`runAgentPlannerActionReplanCycleAgentLoop` 不再入口处构造完整 cycle request wrapper；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v83 | 完成 P0.5-N49 shared cycle plan/action runtime-native 查询：新增 runtime plan action normalize/dedupe helper，主 cycle 以 `AgentLoopPlan` 维护 final plan，action execution 通过 runtime action helper 查询后仅在 legacy executor 边界转换；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v84 | 完成 P0.5-N50 action execution action-list runtime DTO 输入：`runAgentPlannerActionReplanCycleActionExecutionAgentLoop` 直接传递 `[]runtime.AgentLoopAction`，`executeAgentPlannerActionList` 在 legacy executor adapter 边界统一转换到 `agentPlannerAction`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v85 | 完成 P0.5-N51 action gate / authorization request runtime action 收敛：新增 runtime-native gate helper，`executeAgentPlannerActionList` 内部以 `[]runtime.AgentLoopAction` 克隆/截断/遍历，authorization request 接收 runtime action，并仅在 mutation/execution adapter 边界转换 legacy action；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v86 | 完成 P0.5-N52 planner action execution payload runtime action 收敛：`runRuntimePlannerActionExecution` 与 `daemonPlannerActionExecutionTurnPayload` 改为传递 `runtime.AgentLoopAction`，daemon turn executor 仅在调用 legacy `executeAgentPlannerAction` 前转换为 `agentPlannerAction`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v87 | 完成 P0.5-N53 authorization request payload runtime action 收敛：`daemonRequestAuthorizationMutationPayload.Action` 改为 `runtime.AgentLoopAction`，session mutator 仅在调用 legacy `requestAgentAuthorizationLegacy` 前转换为 `agentPlannerAction`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v88 | 完成 P0.5-N54 planner action execution payload runtime authorization 收敛：`daemonPlannerActionExecutionTurnPayload` 改为持有 `runtime.AgentLoopAuthorization`，daemon turn executor 仅在 legacy tool execution adapter 前转换 auth profiles；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v89 | 完成 P0.5-N55 action execution helper runtime authorization 收敛：`executeAgentPlannerActionList` / `runRuntimePlannerActionExecution` 改为接收 `runtime.AgentLoopAuthorization`，legacy auth profile slice 仅在 legacy tool executor adapter 边界转换；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v90 | 完成 P0.5-N56 action execution helper runtime service binding 收敛：`executeAgentPlannerActionList` / `runRuntimePlannerActionExecution` 改为接收 `runtime.AgentLoopToolServiceBinding`，legacy `routerServices` 仅在 legacy tool executor adapter 边界转换，service binding handle 引用保真；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v91 | 完成 P0.5-N57 planner action execution result payload runtime message/tool-call 收敛：`daemonPlannerActionExecutionTurnResultPayload` 改为持有 `runtime.AgentLoopMessage` / `AgentLoopToolCall`，legacy executor 输出仅在 turn executor payload 构造处转换，`runRuntimePlannerActionExecution` egress 再转回兼容调用方；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v92 | 完成 P0.5-N58 action execution helper runtime message/tool-call 返回与累积收敛：`runRuntimePlannerActionExecution` 返回 `runtime.AgentLoopMessage` / `AgentLoopToolCall`，`executeAgentPlannerActionList` 内部 accumulator 改为 runtime DTO，legacy 转换保留在 `executeAgentPlannerActions` / approved authorization append 等兼容边界；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v93 | 完成 P0.5-N59 approved authorization action result runtime phase/metadata merge：`DecideAgentAuthorization` 批准后先以 `runtime.AgentLoopMessage` / `AgentLoopToolCall` 处理 phase 与 authorization metadata，legacy 转换推迟到 append/synthesis response 兼容边界；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v94 | 完成 P0.5-N60 approved-flow response accumulator runtime DTO 收敛：`DecideAgentAuthorization` approved-flow 内部 response message/tool-call accumulator 改为 `runtime.AgentLoopMessage` / `AgentLoopToolCall`，legacy 转换保留在 append、synthesis 与 follow-up adapter 边界；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v95 | 完成 P0.5-N61 tool synthesis 输入 runtime DTO 收敛：`runRuntimeAgentLLMToolSynthesis` 改为接收 runtime `AgentLoopMessage` tool messages，runtime-native caller 可直接透传，legacy `[]agentMessage` 仅在兼容调用点与 `synthesizeAgentToolResults` fallback adapter 边界转换；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v96 | 完成 P0.5-N62 tool synthesis 返回 runtime DTO 收敛：`runRuntimeAgentLLMToolSynthesis` 改为返回 runtime `AgentLoopMessage` / `AgentLoopToolCall` / `AgentLoopPlan`，fallback legacy synthesis 输出立即转成 runtime DTO，session append、response merge 与 follow-up continuation 边界再按需转回 legacy；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v97 | 完成 P0.5-N63 planner chat 返回 runtime DTO 收敛：`runRuntimeAgentLLMPlannerChat` 改为返回 runtime `AgentLoopMessage` / `AgentLoopToolCall` / `AgentLoopPlan`，fallback legacy planner 输出立即转成 runtime DTO，`runAgentLLMToolLoop` 仅在 planner append、错误返回与最终 SendMessage/LLMTurn 边界转回 legacy；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v98 | 完成 P0.5-N64 LLM tool loop 返回 runtime DTO 收敛：`runAgentLLMToolLoop` / `runRuntimeLLMToolLoop` 改为返回 runtime `AgentLoopMessage` / `AgentLoopToolCall`，LLMTurn result payload 与 fallback egress 直接 clone runtime DTO，legacy 转换推迟到 `SendMessage` response merge 与 LLMTurn adapter 边界；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v99 | 完成 P0.5-N65 SendMessage LLM response merge runtime accumulator 收敛：新增 `buildAgentSendResponseFromAgentLoop`，control continue/revise + LLM、route-before-LLM synthesis、main LLM success 与 LLM failure fallback route/authorization 分支内部以 runtime `AgentLoopMessage` / `AgentLoopToolCall` 聚合，legacy 转换集中到 final response、append mutation 与 warning/fallback adapter 边界；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v100 | 完成 P0.5-N66 SendMessage deterministic/no-LLM response merge runtime accumulator 收敛：active-run continue、control auto-continue deterministic route、route-before-LLM route error 与底部 no-LLM route success/error/synthesis branch 内部以 runtime `AgentLoopMessage` / `AgentLoopToolCall` 聚合，legacy 转换集中到 final response、append mutation 与 synthesis/follow-up adapter 边界；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v101 | 完成 P0.5-N67 SendMessage control/error/auth gate response egress runtime helper 收敛：control error、普通 control response 与 pending authorization gate 分支改为经 `buildAgentSendResponseFromAgentLoop` 返回，`runAgentSendMessageTurn` 内除 helper 实现外不再直接调用 legacy `buildAgentSendResponse`；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v102 | 完成 P0.5-N68 replan continuation 返回 runtime DTO 收敛：`runAgentReplanActionContinuation` / `runRuntimeReplanActionContinuation` 改为返回 runtime `AgentLoopMessage` / `AgentLoopToolCall`，turn result payload 与 fallback egress 直接 clone runtime DTO，legacy 转换推迟到 follow-up response merge 与 adapter 边界；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v103 | 完成 P0.5-N69 replan continuation turn payload 输入 runtime request 收敛：`daemonReplanActionContinuationTurnPayload` 改为只携带 `runtime.AgentLoopRequest`，legacy request/history/memory/plan/services 转换集中到 runtime request ingress 与 continuation executor compatibility adapter；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v104 | 完成 P0.5-N70 replan continuation runtime-native request runner：新增 `runAgentReplanActionContinuationAgentLoop` 直接消费 `runtime.AgentLoopRequest` 并串联 continuation cycle，legacy wrapper 只负责 ingress 转换，turn executor adapter 直接调用 runtime request runner；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v105 | 完成 P0.5-N71 LLM tool loop runtime-native request runner：新增 `runAgentLLMToolLoopAgentLoop` 直接消费 `runtime.AgentLoopRequest` 并串联 planner chat 与 shared cycle，legacy wrapper 只负责 ingress 转换，LLMTurn runner adapter 直接构造 runtime loop request 调用新 runner；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v106 | 完成 P0.5-N72 LLM turn payload runtime request 收敛：`daemonLLMTurnPayload` 改为只包装 `runtime.AgentLoopRequest`，service binding、authorization、history/state、max loops 与 metadata 随 request 传递，LLMTurn runner adapter 直接复用 LLM tool loop request runner；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v107 | 完成 P0.5-N73 LLM turn payload 直接携带 runtime request：`runtime.LLMTurnRequest.Payload` 改为直接传递 `runtime.AgentLoopRequest`，移除 `daemonLLMTurnPayload` 单字段 wrapper，daemon LLM turn runner 直接 clone/dispatch runtime loop request；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v108 | 完成 P0.5-N74 清理闲置 AgentLoopTurnPayload DTO：删除无调用方的 `runtime.AgentLoopTurnPayload` / clone helper / clone 隔离测试，LLM turn 输入契约统一为 `runtime.AgentLoopRequest`，结果侧保留 `runtime.AgentLoopTurnResultPayload`；runtime/daemon 包测试与 diff check 通过。 |
| v109 | 完成 P0.5-N75 runRuntimeLLMToolLoop runtime request 入口：新增 `runRuntimeLLMToolLoopAgentLoop` 与 legacy request 构造 helper，SendMessage control+LLM/main LLM branch 先构造 `runtime.AgentLoopRequest` 再进入 engine LLMTurn dispatch，legacy wrapper 只保留 ingress 转换；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v110 | 完成 P0.5-N76 runRuntimeReplanActionContinuation runtime request 入口：新增 `runRuntimeReplanActionContinuationAgentLoop` 与 continuation legacy request 构造/helper，authorization approve follow-up 与 route-before-LLM synthesis follow-up branch 先构造 `runtime.AgentLoopRequest` 再进入 engine `ExecuteTurn` dispatch，legacy wrapper 只保留 ingress 转换；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v111 | 完成 P0.5-N77 replan continuation turn payload 直接携带 runtime request：移除 `daemonReplanActionContinuationTurnPayload` 单字段 wrapper，`runtime.TurnContext.Payload` 直接传递 `runtime.AgentLoopRequest`，daemon continuation executor clone 后进入 runtime request runner；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v112 | 完成 P0.5-N78 replan continuation result payload runtime DTO 收敛：移除 `daemonReplanActionContinuationTurnResultPayload`，continuation turn executor result payload 统一使用 `runtime.AgentLoopTurnResultPayload` 并 clone messages/tool calls；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v113 | 完成 P0.5-N79 planner action execution result payload runtime DTO 收敛：移除 `daemonPlannerActionExecutionTurnResultPayload`，planner action execution turn executor result payload 统一使用 `runtime.AgentLoopTurnResultPayload`，保持 action execution dispatch/fallback 与 shared cycle adapter 行为；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v114 | 完成 P0.5-N80 planner action execution turn payload runtime request 收敛：`daemonPlannerActionExecutionTurnPayload` 改为携带 `runtime.AgentLoopRequest + runtime.AgentLoopAction`，turn executor 从 request clone 中读取 turn/language/operator/operation/tool services/authorization，保持 planner action dispatch/fallback 与 shared cycle adapter 行为；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v115 | 完成 P0.5-N81 deterministic route turn payload runtime request 收敛：移除 `daemonDeterministicRouteTurnPayload`，`runtime.TurnContext.Payload` 直接传递 `runtime.AgentLoopRequest`，route executor 从 request clone 读取 turn/prompt/language/operator/operation/tool services 并保持 deterministic route dispatch/fallback 语义；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v116 | 完成 P0.5-N82 deterministic route result payload runtime DTO 收敛：移除 `daemonDeterministicRouteTurnResultPayload`，route executor result payload 统一使用 `runtime.AgentLoopTurnResultPayload`，`routePromptWithRuntimeDeterministicRoute` 在 egress 边界转换回 legacy message/tool call；focused daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v117 | 完成 P0.5-N83 planner action execution runtime payload DTO：新增 `runtime.AgentLoopActionExecutionPayload` 与 action clone helper，planner action execution turn payload 不再使用 daemon-private wrapper，保持 request/action clone 隔离与 dispatch/fallback 行为；focused runtime/daemon tests、runtime/daemon 包测试与 diff check 通过。 |
| v118 | 完成 P0.5-N84 append_messages session mutation payload runtime DTO 收敛：新增 `runtime.AgentLoopAppendMessagesPayload` 与 clone helper，append payload 不再携带 SessionID/daemon-private message/plan，daemon mutator adapter 在边界转回 legacy 并沿用 turnRef compaction/events 语义；runtime/daemon 包测试与 diff check 通过。 |
| v119 | 完成 P0.5-N85 record_user_message session mutation payload runtime DTO 收敛：新增 `runtime.AgentLoopRecordUserMessagePayload` 与 clone helper，record_user_message payload 不再携带 SessionID/Prompt/OperationLevel，daemon mutator adapter 从 TurnContext 读取这些输入并保留 legacy record/persist/compaction 语义；runtime/daemon 包测试与 diff check 通过。 |
| v120 | 完成 P0.5-N86 request_authorization session mutation payload runtime DTO 收敛：新增 `runtime.AgentLoopAuthorizationRequestPayload` 与 clone helper，request_authorization payload 不再携带 SessionID/OperatorPrompt/OperationLevel，daemon mutator adapter 从 TurnContext 读取这些输入并保留 legacy pending authorization/message/session state 语义；runtime/daemon 包测试与 diff check 通过。 |

---

## 19. 待 Leader 实时更新项

P0 已完成。Leader 后续重点更新：

- 风险清单。
- 阻塞项。
- 任务状态。
- 文件边界调整。
- 验收命令调整。
- 切片依赖调整。
- 实施顺序调整。
- 方案版本记录。
