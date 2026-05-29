# CyberAgent v0.1.3 Agent-Native Correction Plan

版本：v0.1.3  
日期：2026-05-27  
状态：Agent-native 主链路增量已完成第二轮可验证实现；后续进入 v0.1.4 体验打磨

---

## 1. 目标

v0.1.3 的目标是把 CyberAgent 从“安全控制台 + 聊天入口”纠偏为真正的 Cybersecurity Agent。

对标对象：

```text
Claude Code
Codex
```

核心要求：

```text
Agent 能保持会话。
Agent 有当前任务。
Agent 有 Todo / Plan。
Agent 有工具调用循环。
Agent 能读写工作区上下文。
Agent 能被用户 stop / continue / revise / new task。
Agent 状态由 daemon 持久化，前端 localStorage 只做兜底缓存。
```

---

## 2. 非目标

v0.1.3 不做：

```text
多用户 Web 平台
Remote Runner
Desktop/Wails 完整版
复杂长期 RAG
长期 Target Memory
自动 exploit
高风险动作自动批准
企业资产平台
```

---

## 3. 当前问题

当前 Web Agent Cockpit 已具备聊天入口和部分工具路由，但仍存在结构性不足：

```text
后端 Agent session 以内存为主，daemon 重启后丢失。
前端 localStorage 只保留 UI 聊天，不是完整 Agent state。
Agent 没有明确 Active Task。
Agent 没有结构化 Todo。
Agent send message 主要还是规则意图路由，不是完整 Tool Loop。
Tool Call、Context Ref 没有作为 Agent 状态统一返回。
UI 右侧更多是运行状态卡，不是 Agent 工作状态。
```

---

## 4. Agent State Contract

新增或稳定以下数据模型：

```text
AgentSession
AgentMessage
AgentTask
AgentTodo
AgentToolCall
AgentContextRef
AgentNextAction
```

建议字段：

```text
AgentSession
  session_id
  operator
  language
  status
  created_at
  updated_at
  messages[]
  active_task
  todos[]
  tool_calls[]
  context_refs[]
  next_action

AgentTask
  id
  goal
  kind
  status
  active_run_id
  summary
  created_at
  updated_at

AgentTodo
  id
  title
  detail
  status: pending|running|done|blocked|skipped
  priority
  tool_call_id
  context_ref_ids[]
  blocked_reason
  updated_at

AgentToolCall
  id
  name
  status
  input_summary
  output_summary
  error
  started_at
  finished_at
  result_refs[]

AgentContextRef
  id
  kind: run|event|artifact|finding|report|approval|plugin|config
  ref_id
  label
  to
  summary

AgentNextAction
  type: continue|wait_user|approval_required|finished|blocked
  reason
  suggested_prompt
```

---

## 5. 后端 API 计划

### 5.1 Session API

```text
POST /api/v1/agent/sessions
GET  /api/v1/agent/sessions/{session_id}
```

返回完整 session state：

```json
{
  "session": {
    "session_id": "agent_xxx",
    "messages": [],
    "active_task": {},
    "todos": [],
    "tool_calls": [],
    "context_refs": [],
    "next_action": {}
  }
}
```

### 5.2 Message API

```text
POST /api/v1/agent/sessions/{session_id}/messages
```

请求：

```json
{
  "message": "扫描 http://127.0.0.1:18080 scope=127.0.0.1",
  "operator": "local-user",
  "language": "zh",
  "provider": "openai",
  "model": "xxx"
}
```

响应：

```json
{
  "session_id": "agent_xxx",
  "messages": [],
  "active_task": {},
  "todos": [],
  "tool_calls": [],
  "context_refs": [],
  "next_action": {
    "type": "continue",
    "reason": "scan started"
  }
}
```

### 5.3 Control API

```text
POST /api/v1/agent/sessions/{session_id}/continue
POST /api/v1/agent/sessions/{session_id}/stop
POST /api/v1/agent/sessions/{session_id}/new-task
```

已实现补充：

```text
GET  /api/v1/agent/sessions/{session_id}/state
GET  /api/v1/agent/sessions/{session_id}/context
POST /api/v1/agent/sessions/{session_id}/continue
POST /api/v1/agent/sessions/{session_id}/stop
POST /api/v1/agent/sessions/{session_id}/new-task

/api/v1/agent/capabilities 已暴露 endpoint discovery 清单，方便 Web/TUI/CLI 自动发现 state、context、control、stream 等 Agent-native 接口。
```

---

## 6. 后端实现任务

### A：Agent State Store

内容：

```text
定义 Agent state bucket / filesystem layout。
保存 session/messages/task/todos/tool_calls/context_refs/next_action。
启动 daemon 后加载已有 session。
```

建议存储：

```text
runs/.agent/sessions/{session_id}.json
```

或：

```text
runtime/state bbolt bucket: agent_sessions
```

MVP 推荐：

```text
先用 runs/.agent/sessions/*.json，便于调试和迁移；
后续如需要索引查询，再迁移到 bbolt/SQLite。
```

### B：Task / Todo Manager

内容：

```text
根据用户意图创建 active task。
scan 意图生成固定 Todo。
doctor/plugin/runs/approval/report 分析意图生成对应 Todo。
工具执行后更新 Todo。
```

scan Todo 示例：

```text
1. 确认目标和 scope
2. 检查 daemon / 插件健康
3. 启动 safe profile Web scan
4. 跟踪运行事件和审批
5. 读取 findings / artifacts / report
6. 总结结果和下一步建议
```

### C：Tool Loop

内容：

```text
把 send message 从单次 intent route 升级为：
load session → update task/todo → call tool → observe → update context → persist → respond。
```

### D：Workspace Context Reader

内容：

```text
run list / run detail
events
artifacts
findings/report
approvals
plugins/doctor
health/version
```

输出统一转成 `AgentContextRef`。

---

## 7. 前端实现任务

### A：API Types

更新：

```text
AgentSession
AgentTask
AgentTodo
AgentContextRef
SendAgentMessageResponse
```

### B：Agent Cockpit 结构

首页布局：

```text
左侧：Chat + Execution Stream
右侧：Current Task / Todo / Tool Calls / Workspace Context / Approvals
```

### C：状态恢复

```text
页面加载优先 GET /api/v1/agent/sessions/{id}
后端不可用时才使用 localStorage 兜底
后端 session 404 时创建新 session，但保留本地聊天并提示已重建 runtime session
```

### D：控制按钮

```text
New Task
Stop
Continue
```

### E：职责边界

```text
总览页只读监控，不能启动扫描。
扫描页只编辑本次 target/scope/mode，然后交给 Agent。
设置页统一配置 daemon token / AI provider / language / operator。
```

---

## 8. 验收标准

### 8.1 后端验收

```text
go test ./internal/interfaces/daemon ./internal/agent ./internal/runtime/state 通过。
POST create session 后可 GET 完整 state。
send message 后返回 task/todos/tool_calls/context_refs/next_action。
daemon 重启后 session state 可恢复。
session 404 时有明确错误，前端可重建。
```

### 8.2 前端验收

```text
cd web/frontend && npm run build 通过。
首页展示 Chat / Current Task / Todo / Tool Calls / Workspace Context。
发送 scan prompt 后 Todo 状态更新。
刷新页面后聊天和 Todo 仍在。
切换菜单再回来状态仍在。
daemon 重启后前端不清空历史，可重建 runtime session。
```

### 8.3 产品验收

```text
从页面第一眼能看出这是 Agent Cockpit，不是普通安全 Dashboard。
用户可以通过聊天指挥 Agent 完成扫描、分析、报告查看。
Agent 能解释当前任务、下一步、工具调用结果。
```

---

## 9. 推荐提交顺序

```text
1. docs: Agent-Native plan and contracts
2. backend: Agent state structs and persistence
3. backend: session get/create/send response shape
4. backend: task/todo manager
5. backend: tool loop context refs
6. frontend: API types and session restore
7. frontend: cockpit task/todo/context panels
8. frontend: stop/continue/new task controls
9. tests: daemon + frontend build + smoke
```

---

## 10. 完成定义

v0.1.3 完成后，CyberAgent 应满足：

```text
它不是一个带聊天框的安全系统。
它是一个能保持任务、计划步骤、工具调用和工作区上下文的 Cybersecurity Agent。
```

## 11. 当前实现状态（增量更新）

截至 2026-05-27，v0.1.3 已完成第二轮可验证实现：

```text
已完成：
- 后端 session/task/todo/tool_calls/context_refs/next_action 状态持久化
- 后端 Planner JSON 解析与 session 状态回写
- 后端 continue/stop/new_task/revise 控制流
- 后端会话连续性修复：普通 follow-up 不再每次重置 ActiveTask/Todos
- 后端 tool-call 结果自动回写 active_run_id 与 run context summary
- 后端 Planner answer 自动补全 ActiveTask summary
- 前端 Agent Cockpit（Chat/Current Task/Todo/Tool Calls/Approvals/Next Action）
- 前端 checkApprovals 优先 activeRunId
- 前端 Continue/Revise 体验增强
- 本地 Dashboard 自动接入 daemon URL/token：`/api/local/daemon-config`
- 后端 active_run 终态同步：成功收敛 `idle/wait_user`，失败或取消收敛 `blocked`
- Agent capabilities 暴露 `workspace.files.write` / `workspace.files.patch` / `workspace.files.diff` / `workspace.files.apply` / `workspace.files.discard`
- 自然语言工作区工具链：`workspace.command` / `workspace.files.list` / `workspace.files.read` / `workspace.files.write` / `workspace.files.patch` / `workspace.files.diff` / `workspace.files.apply` / `workspace.files.discard`
- 自然语言“写入文件 notes.md 内容: ...”可触发 `workspace.files.write`
- 自然语言“修改文件 notes.md old: ... new: ...”可触发 `workspace.files.patch`，并返回 diff preview
- 自然语言“预览修改文件 notes.md old: ... new: ...”可触发 `workspace.files.diff`，生成 `pending_changes` 和 `change_id`，不修改文件
- 自然语言“应用 change_id: ...”可触发 `workspace.files.apply`，重新校验后落盘并标记 applied；冲突时返回 409 并标记 conflict
- 自然语言“丢弃 change_id: ...”可触发 `workspace.files.discard`，不修改文件并把 pending/conflict change 标记 discarded
- 自然语言“应用所有待应用修改 / 丢弃所有待应用修改”可批量处理所有 pending staged changes
- `workspace.files.diff_many` 可一次生成多文件 staged change set，并通过共享 `group_id` apply/discard
- `workspace.files.rebase` 可在 conflict 后基于当前文件 old 文本重新生成 staged change，并 supersede 旧 change
- Agent Cockpit 右侧 Staged Changes 面板展示 `pending_changes`、group_id、diff preview，并支持 Open / Apply / Discard / 内联 Rebase / 选区填充 old 文本 / Rebase 片段 diff 与完整文件级 diff 预览 / Apply all / Discard all / Apply group / Discard group / conflict error 展示
- `workspace.files.read/write/patch/diff/apply/discard` 自动沉淀 `workspace_file` context ref
- active_run 恢复/读取/list 自动补齐 run/report_json/artifact_index/finding/approval context refs，并从 report.json / artifacts index 提炼 lead finding / evidence / artifact 摘要写入 refs summary 和 ActiveTask summary
- 真实浏览器 smoke 发现并修复 run/history 污染：RunService 只列出真实 run workspace，`runs/.agent` 与普通非 run 目录不再显示为“最近扫描”
- 后端新增 TestHTTPAgentSessionContinuityAcrossMessages
- 后端新增/更新 TestHTTPAgentCapabilitiesAPI 与 TestHTTPAgentNaturalLanguageInvokesWorkspaceTools
- 后端新增 TestRunServiceIgnoresInternalAgentStateDirectory，覆盖 `.agent` 与普通非 run 目录过滤
- go test ./internal/interfaces/daemon 通过
- cd web/frontend && npm run build 通过
```

仍建议继续推进：

```text
- Agent 复杂多步自然语言任务的自动 continue 策略
- frontend task -> todo -> tool_calls -> context_refs 的更强视觉串联
- HITL approval 与 run state 的基础联动闭环已完成；后续继续细化 step timeline 定位
- workspace 单文件精确 patch 与 staged diff/apply/discard 已完成；后续继续规划 multi-file patch、review diff UI 与 staged apply 体验增强
```

### 11.1 计划完成状态检查清单

```text
[done] 后端 Agent state contract
[done] 后端 session/task/todo/tool_calls/context_refs/next_action 持久化
[done] 后端 Planner 解析与回写
[done] 后端 continue/stop/new_task/revise
[done] 后端 Agent API aliases：/state、/context、/continue、/stop、/new-task
[done] 后端 capabilities endpoint discovery：/api/v1/agent/capabilities 暴露 sessions/messages/state/context/control/stream endpoint 清单
[done] 后端会话连续性修复
[done] 后端 run context 自动串联
[done] 后端 active_run 恢复/读取/list 自动补齐 run context snapshot（run/report_json/artifact_index/finding/approval refs + task summary）
[done] 后端 active_run report/evidence/artifact 摘要提炼：report_json/finding/artifact_index refs 包含关键 finding、evidence、artifact 线索
[done] 前端 Agent Cockpit
[done] 前端 task -> todo -> tool_calls -> run 可视化串联
[done] 前端 approval 闭环增强：审批详情、risk_summary、approve/deny/resume、active run pending approvals 指标
[done] 后端 active_run pending approvals 自动同步 next_action=approval_required
[done] 前端 context refs 面板：session context_refs + active run + finding + approval gate 合并展示
[done] 前端 Planner fallback / reason 可见性
[done] 本地 Dashboard daemon bootstrap
[done] active_run 终态同步
[done] 自然语言 workspace command/list/read/write/patch/diff/apply/discard
[done] workspace_file context ref 沉淀
[done] RunService 真实 run workspace 过滤，避免 Agent state 污染 run/history UI
[done] 回归测试与前端构建验证
```

建议后续进入 v0.1.4 时再处理：

```text
- Planner 异常路径更细粒度 UI 反馈
- approval step timeline 精确定位
- agent session resume 后自动补齐更完整的报告摘要 / 最新 evidence 摘要已在 v0.1.4 完成基础实现；后续继续做 step timeline 精确定位与多 session 摘要压缩
```
