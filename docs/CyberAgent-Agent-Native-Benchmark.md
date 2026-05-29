# CyberAgent Agent-Native Benchmark

版本：v0.1  
日期：2026-05-25  
状态：新增基线；作为后续实现和验收的最高优先级约束

---

## 1. 背景

CyberAgent 不能只是“安全系统 + 聊天框”。

本项目的核心定位应对标 Claude Code / Codex 这类可执行 Agent：

```text
用户用自然语言描述目标。
Agent 维护会话、目标、计划、上下文和工具调用。
Agent 能持续执行、观察结果、更新计划、处理错误、请求确认、继续完成任务。
```

CyberAgent 的特殊领域是：

```text
授权 Web/API 安全分析、测试、运维与证据报告。
```

因此，CyberAgent 应该是：

```text
Cybersecurity Agent Workbench
= 可执行 Agent Runtime
+ 安全工具/扫描/证据/报告能力
+ Human-in-the-Loop 安全边界
```

---

## 2. 对标能力矩阵

| 能力 | Claude Code / Codex 类 Agent | CyberAgent 必须具备 |
|---|---|---|
| 会话保持 | 保留对话、任务状态、上下文 | 保存 messages、active task、todos、tool calls、context refs |
| 任务目标 | 识别用户当前目标 | 每个 session 有 current goal / active task |
| 计划管理 | 生成并更新 todo | 生成安全分析 todo，展示 pending/running/done/blocked |
| 工具调用 | 读文件、改代码、运行命令、看输出 | 调用 scan、doctor、plugins、runs、artifacts、reports、approvals、HTTP/browser/shell 工具 |
| 工作区上下文 | 理解 repo 文件、测试、错误 | 理解 run workspace、events、artifacts、findings、reports、plugin manifests、config |
| 执行循环 | plan → act → observe → update | plan → tool call → observation → evidence → todo update → next action |
| 可打断 | stop / continue / revise | stop / continue / revise / approve / deny / new task |
| 错误恢复 | 修复失败、重试、降级 | provider 失败、tool 失败、session 丢失、scope 缺失时重试/降级/询问 |
| 可解释性 | 说明下一步和改动 | 展示 Agent 正在做什么、为什么调用工具、下一步计划 |
| 安全边界 | 遵循项目/用户约束 | Scope Guard、Risk Policy、HITL、Evidence Redaction |

---

## 3. Agent-Native 硬性验收标准

### 3.1 Web/TUI 不是普通 Dashboard

Agent Cockpit 的主界面必须包含：

```text
Chat：用户自然语言入口。
Current Task：当前任务目标、状态、active run。
Todo：Agent 计划步骤和状态。
Tool Calls：工具调用输入摘要、输出摘要、错误、关联资源。
Workspace Context：run / artifacts / findings / reports / approvals 引用。
HITL：审批、拒绝、修改参数、继续。
Controls：stop / continue / new task。
```

不合格形态：

```text
只有扫描表单。
只有运行列表。
只有普通聊天回复。
只有扫描状态卡片。
```

### 3.2 Runtime 不能只做意图路由

不合格：

```text
if prompt contains "scan" → call scan API
if prompt contains "doctor" → call doctor API
```

合格：

```text
User Message
  → Load Session
  → Understand Goal
  → Create/Update Task
  → Create/Update Todos
  → Select Tool
  → Policy Gate
  → Execute Tool
  → Observe Result
  → Update Todo/Context
  → Decide Next Action
  → Persist State
```

### 3.3 State Store 必须保存 Agent 状态

最小持久化对象：

```text
AgentSession
AgentMessage
AgentTask
AgentTodo
AgentToolCall
AgentContextRef
AgentDecision
```

浏览器 localStorage 只能作为 UI 缓存，不能代替 daemon State Store。

### 3.4 Agent 必须能读工作区

Agent 可以通过受控工具读取：

```text
runs 列表
run detail
events.jsonl
artifacts/index.json
artifact 摘要
findings.json
reports/report.md / report.json
approvals
plugins manifest / doctor
daemon health/version
```

### 3.5 安全能力通过 Agent 简化

用户不应该被迫理解所有底层页面。

目标形态：

```text
用户：扫描这个授权目标，scope 是 example.local，只跑 safe profile。
Agent：生成计划 → 检查 daemon → 启动扫描 → 跟踪运行 → 读取 findings → 总结报告 → 给下一步建议。
```

---

## 4. 第一阶段 Agent 化实现目标

### A1：Agent Session 持久化

```text
后端保存 session/messages/task/todos/tool calls/context refs。
daemon 重启后可恢复。
前端刷新只从后端恢复，localStorage 只是兜底缓存。
```

### A2：Task / Todo 模型

```text
每次用户发起任务，Agent 创建 active task。
Agent 为任务生成 todo list。
工具执行后更新 todo 状态。
UI 实时展示 todo。
```

### A3：Tool Loop

```text
把当前规则路由升级成 Agent loop。
每次响应不仅返回 messages，还返回 task/todos/tool_calls/context_refs/next_action。
```

### A4：Workspace Context

```text
Agent 能读取 run、events、artifacts、findings、reports。
支持“分析这个 finding / artifact / run”的上下文追问。
```

### A5：Control

```text
支持 stop / continue / revise / new task。
支持 approval approve/deny。
```

---

## 5. API 方向

建议新增或扩展：

```text
POST   /api/v1/agent/sessions
GET    /api/v1/agent/sessions/{session_id}
POST   /api/v1/agent/sessions/{session_id}/messages
GET    /api/v1/agent/sessions/{session_id}/state
POST   /api/v1/agent/sessions/{session_id}/continue
POST   /api/v1/agent/sessions/{session_id}/stop
POST   /api/v1/agent/sessions/{session_id}/new-task
GET    /api/v1/agent/sessions/{session_id}/context
```

消息响应至少包含：

```json
{
  "session_id": "agent_xxx",
  "messages": [],
  "task": {},
  "todos": [],
  "tool_calls": [],
  "context_refs": [],
  "next_action": {
    "type": "continue|wait_user|approval_required|finished|blocked",
    "reason": ""
  }
}
```

---

## 6. UI 方向

Agent Cockpit 推荐布局：

```text
左侧 65%：
  Chat + Execution Stream

右侧 35%：
  Current Task
  Todo
  Tool Calls
  Workspace Context
  Approvals
```

交互原则：

```text
用户始终从 Agent 输入框开始。
扫描页只是手动参数构造和“交给 Agent”的辅助入口。
总览页只读监控。
设置页统一配置 provider/token/language/operator。
```

---

## 7. 非目标

第一阶段不做：

```text
完全自主攻击。
绕过 scope 的工具调用。
未审批高风险动作。
多用户协作。
远程 Runner。
复杂长期 RAG。
企业资产平台化。
```

---

## 8. 纠偏结论

后续任何功能如果不能增强以下能力，就不应作为优先级最高项：

```text
Agent 会话
Agent 任务
Agent Todo
Agent 工具循环
Agent 工作区上下文
Agent 可恢复状态
Agent 可解释执行
```

CyberAgent 的核心不是“安全后台”，而是：

```text
通过 Agent 简化安全分析、测试、运维和报告流程。
```
