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

1. Route-level Lazy / Bundle Splitting。
2. 移动端 Cockpit 降噪。
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
   - runtime engine 当前负责 SQLite 事件桥、SSE replay 与 recovery；尚未接管 turn 调度和 planner loop。
2. `internal/interfaces/daemon/agent_planner_loop.go`
   - LLM JSON planner / native tool calls 仍由 daemon 解析、回写 session state，并通过 `executeAgentPlannerActions` 执行。
   - operation level、explicit intent gate、manual authorization gate、action queue metadata 仍在 daemon 层。
3. `internal/interfaces/daemon/agent_prompt_dispatch.go`
   - `routePrompt` 仍是 no-LLM、本地 fallback、确定性 pre-route、auto-continue 的规则路由入口。
   - 本切片已删除 `executeAgentPlannerAction` 内部的 legacy per-tool switch fallback；builtin action 只能通过 Tool Registry handler 执行，handler 缺失会以 `internal_error` 失败。
   - 本切片已将 read-only/system、safe HTTP、safe scan、workspace staged change、reports draft、workspace mutation 与 approval approve/deny/resume-approved 路由分支收敛为 `agentHeuristicActionsFromPrompt` 产出 action，再通过 Tool Registry handler 执行；schedule create/delete、non-safe HTTP 与 generic workspace fallback 仍保留原路由。
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
   - 保留 POST / PUT / PATCH / DELETE 等 non-safe HTTP 方法的 direct handler 路径与 `advanced` operation-level gate，暂不扩大写动作面。
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
11. 当前收敛策略：不一次性搬迁 legacy turn，继续按 registry dispatch → lifecycle seam → LLM runner seam → engine-native runner 分层推进。
   - 原因：`SendMessage`、planner loop、授权 gate、session mutation 仍由 daemon 保证语义；一次性把 `runAgentLLMToolLoop` 完整下沉会同时牵动 session persist、authorization resume、tool synthesis、auto-continue 与 SSE replay，切片过大。
   - 已完成确定性工具 registry 化、`BeginTurn`/`FinishTurn` lifecycle API 与 `RunLLMTurn` dispatch seam。
   - 后续更稳妥路径是继续拆分 planner chat、tool synthesis、step replan、session mutation，再收敛 authorization resume / auto-continue。

高风险 / 写动作 registry 化分批建议：

| 批次 | 动作 | 风险与状态 | 迁移建议 |
| --- | --- | --- | --- |
| A | read-only/system：daemon.health、plugins.list、runs.list、runs.context、runs.approvals.list、workspace list/tree/read/search、allowlisted workspace.command、schedule.list | 已完成 | 保持 registry dispatch，并用 source metadata 观察回归 |
| B | safe HTTP：GET / HEAD / OPTIONS `http.request` | 已完成本切片；仍会受 safe_scan 权限与 HTTP SSRF policy 约束 | 继续补 trace / auth profile 场景，不把 non-safe method 混入 |
| C | `ports.scan.safe` | 已完成本切片；网络探测但不落盘，参数边界较清晰 | 继续观察 port allowlist / source metadata parity |
| D | `scan.web.safe` | 已完成本切片；会启动 daemon job、创建 run/evidence/report；不是文件写动作但状态面大 | 继续观察 job/run/context refs/timeline 与 source metadata parity |
| E | workspace diff/rebase | 已完成自然语言 staged-change 本地路由小切片；diff/rebase 不直接改文件但会持久化 pending change state | 继续观察 staged state、conflict/rebase 与 source metadata parity；diff_many 暂无自然语言 extractor |
| E2 | `reports.draft` | 已完成本切片；会写当前 Agent workspace 的报告草稿，需要人工复核 | 继续观察 workspace draft path、review_required 与 source metadata parity |
| E3 | workspace diff_many | 结构化 state-only changeset，当前无自然语言 extractor | 后续独立切片，先补自然语言/结构化参数测试，再考虑 registry 化 local route |
| F | workspace write/patch/apply/discard/delete/move/copy/mkdir | 已完成自然语言本地路由 registry dispatch；真实 workspace mutation 或 staged change resolution，风险高 | 继续观察 `workspace_write` gate、explicit intent gate、source metadata 与既有 conflict/atomicity 测试 parity |
| G | runs.approvals.approve / deny / resume_approved | 已完成自然语言本地路由 registry dispatch；审批状态写动作，会改变 run 执行链 | 继续观察 `approval_ops` gate、最新用户显式指令匹配、comment/audit/event parity 与 source metadata |

engine-native turn lifecycle 下沉入口建议：

1. ✅ 先定义 engine 侧 `TurnExecutor`/`TurnContext` seam，只复用现有 daemon legacy executor，不改变行为。
2. ✅ 将 `beginRuntimeTurn` / `finishRuntimeTurn` 的状态迁移到 engine API，daemon 仍负责 session adapter。
3. 进行中：新增 `RunLLMTurn`/`LLMTurnRunner` seam、`RunLLMStage`/`LLMStageRunner` seam 与 `MutateSession`/`SessionMutator` seam；`SendMessage` 的 LLM 主入口已经 engine dispatch 到 daemon legacy `runAgentLLMToolLoop`，loop 内部 planner chat / tool synthesis、auto-continue StepReplan synthesis、authorization resume synthesis、route-before-LLM/standalone route synthesis 已经 engine stage dispatch 到 `handleAgentLLMChat` / `synthesizeAgentToolResults`；`appendMessagesWithPlanner`、`SendMessage` 开头 user message record、`DecideAgentAuthorization` approve/deny 状态写入与 `ControlSession` 状态写入已经 engine session mutation dispatch；no-LLM deterministic route、control auto-continue route、LLM provider failure fallback route、route-before-LLM deterministic pre-route 与后续 replan action continuation 已经 `ExecuteTurn` dispatch 到 daemon legacy turn adapters。下一步准备继续收敛 authorization approved action execution 与 remaining SendMessage branches。
4. 最后收敛 authorization approved action execution 与 remaining SendMessage branches 为 engine turn lifecycle。

已完成的 seam 边界：

- `runtime.Engine` 新增可选 `TurnExecutor`，`ExecuteTurn` 只转发到 attached executor，不主动发 runtime lifecycle events。
- `TurnContext` 承载 session/task/run/turn ID、message ID、prompt/goal、origin、operation level、turn index、timestamp、metadata，以及迁移期 daemon-private `Payload`；daemon session mutation、planner loop、authorization gate 仍留在现有路径。
- daemon `AttachEngine` 会挂接 `daemon_legacy_adapter`；默认无 kind 时仍返回 `running` 占位结果，明确表示 legacy `SendMessage` 仍拥有 turn execution；`deterministic_route` kind 会执行现有 daemon `routePrompt` 并返回 daemon-private messages/toolCalls payload，`replan_action_continuation` kind 会执行现有 daemon `runAgentReplanActionContinuation` 并返回 daemon-private messages/toolCalls payload，因此 no-LLM route、control auto-continue route、LLM provider failure fallback route、route-before-LLM deterministic pre-route 与后续 auto-continue continuation 可先经 `ExecuteTurn` seam 调度且不改变 append/response/runtime event parity。

已完成的 lifecycle API 边界：

- `runtime.Engine.BeginTurn` 接管 task/run/turn state 写入与 `task_created`、`run_started`、`turn_started` 事件发布。
- `runtime.Engine.FinishTurn` 接管 turn state 写入、`turn_finished`/`turn_failed` 发布，以及 approval required、task finished、task failed run 终态分派。
- daemon `beginRuntimeTurn` / `finishRuntimeTurn` 只保留 session snapshot、message ID、response completion 到 `TurnContext` / `TurnResult` 的 adapter 逻辑；`SendMessage` 主流程、planner loop、authorization resume、auto-continue 后续分支仍未完整下沉。

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
- 已接入 mutation 为 `append_messages`、`record_user_message`、`control_session` 与 `resolve_authorization`，覆盖 legacy `appendMessagesWithPlanner` 的 messages append、tool/link/planner state apply、session persist、memory compaction 与 runtime events 发布，`SendMessage` 开头的 user message 记录、ActiveTask start/touch、history/memory snapshot 与 persist，`ControlSession` 的 continue/revise/pause/cancel/stop/new_task 状态更新、persist、compaction 与 runtime events 发布，以及 `DecideAgentAuthorization` approve/deny 的 pending 校验、授权状态更新、session/task/todo/next_action 更新、persist、compaction 与 runtime events 发布。
- daemon `AttachEngine` 会挂接 `daemon_legacy_session`；实际执行仍复用原 `appendMessagesWithPlannerLegacy`，保持 session lock 顺序、planner apply、compaction、persist、`memory_compacted` 与 message runtime events 语义不变。
- `record_user_message` 实际执行仍复用 daemon legacy helper，保持 user message timestamp、task start/touch、history/memory snapshot、persist best-effort、`BeginTurn` 先于 memory compaction event 的原有顺序。
- `control_session` 实际执行仍复用 daemon legacy helper，保持 continue/revise/pause/cancel/stop/new_task 对 session/task/todo/next_action/message、persist、`memory_compacted` 与 message runtime events 的原有语义；stop/cancel 的 daemon job cancel 副作用仍留在 `ControlSession` 外层 control flow。
- `resolve_authorization` 实际执行仍复用 daemon legacy helper，保持 pending authorization ID 校验、operation level uplift、approve/deny message 文案与 phase、ActiveTask/Todos/NextAction、`approval_resolved`、`memory_compacted` 与 message runtime events 语义不变；approve 后挂起 action 执行、tool message append、synthesis 与 auto-continue 仍留在原 `DecideAgentAuthorization` 后续路径。

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
- 生产 JS 约 `432.84KB`。
- gzip 约 `125.34KB`。

优化方向：

- Dashboard / Agent / Settings / Skills / MCP / History / ScanDetail 分包。
- 加 skeleton / loading state。
- 控制毛玻璃动效和 blur 面积。

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

验收：

- 390px、768px、1024px、1440px 下无横向滚动。
- Composer 不遮挡 timeline。
- TaskTree 可打开/关闭/恢复。
- 核心操作不被隐藏。

### 13.3 浏览器 E2E / a11y / 视觉回归

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
