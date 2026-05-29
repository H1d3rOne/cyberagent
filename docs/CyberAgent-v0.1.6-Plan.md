# CyberAgent v0.1.6 Agent Reasoning 可见性计划

版本：v0.1  
日期：2026-05-25  
状态：实现已完成

---

## 1. 目标

让 Agent 的思考过程对用户透明，提升信任度和可控性。

```text
核心价值：
  用户能看到 Agent 为什么做出某个决策
  用户能理解 Agent 的推理过程
  用户能基于 Agent 的 reasoning 做出更好的判断
```

---

## 2. 非目标

```text
不做完整的 chain-of-thought 可视化
不做 reasoning 模型集成（如 o1/o3）
不做完整 reasoning 搜索/对比/评分
```

---

## 3. 已实现功能

### 3.1 后端 Planner 增强

**Planner response 新增字段**：

```json
{
  "answer": "...",
  "task": {...},
  "todos": [...],
  "context_refs": [...],
  "next_action": {...},
  "reasoning": "Agent 的思考过程：为什么选择这个任务、为什么这样规划、为什么这个下一步"
}
```

**reasoning 内容**：

```text
- 任务选择理由：为什么选择这个任务
- 规划理由：为什么这样规划 todos
- 下一步理由：为什么选择这个下一步
- 风险考虑：可能的风险和缓解措施
```

### 3.2 前端展示

**Chat 区域**：

```text
- 每次 Agent 响应时，如果 reasoning 存在，显示“推理摘要 / reasoning”块
- 使用低噪音紫色调，与 planner/synthesis/tool 阶段区分
- reasoning 内容随 ChatMessage 本地缓存与后端 session 同步展示
```

**Runbook / Timeline 区域**：

```text
- Runbook 新增“推理 / 计划修订”面板
- 展示最近 reasoning、TODO 完成进度、next_action 状态、auto/gate 标记与 suggested_prompt
- Timeline 新增 reasoning 阶段 item，与 planner/tool/observe/synthesis/next_action 串成可追溯执行链
- Planner 多个结构化 actions 执行时，Timeline 和工具徽章展示队列步骤，例如 step 1/3、step 2/3
```

**数据持久化**：

```text
- ChatMessage 接口新增 reasoning 字段
- AgentMessage API 类型新增 reasoning 字段
- 后端将 planner/synthesis reasoning 写入 agent message，并随 send response 与 persisted session 返回
- 后端为 Planner 多动作执行链沉淀 action_queue_id / action_index / action_count / action_name，并随 tool_calls 返回和持久化
```

### 3.3 Planner 变更 workspace 显式意图 gate

**执行策略硬化**：

```text
- LLM Planner 不能仅凭自身规划自动修改工作空间。
- 对 workspace.files.write / patch / diff / diff_many / rebase / apply / discard 等变更类 action，后端会检查最新用户消息是否明确表达写入、修改、预览、应用或丢弃意图。
- 如果用户最新消息没有明确授权，后端生成 policy_denied tool_call，不执行落盘或 staged change 变更。
- 被拒绝的 action 仍保留 action_queue_id / action_index / action_count / action_name 元数据，方便 Timeline 和审计链追踪。
- policy_denied observation 会进入 LLM synthesis，由 Agent 解释拒绝原因并引导用户明确路径、内容或变更意图。
```

**安全价值**：

```text
- 降低模型幻觉或过度主动导致的 workspace 意外写入风险。
- 保持 Claude Code / Codex 型 Agent 的可执行能力，同时保留用户最后一跳确认边界。
- 让“plan → tool → observe → synthesize”链路在拒绝执行时仍可追溯、可解释、可恢复。
```

### 3.4 自动续行时的 reasoning

**自动续行时显示 reasoning**：

```text
- 自动续行消息中包含 reasoning
- 用户能看到 Agent 为什么决定自动续行
- 提升透明度和信任度
```

---

## 4. 涉及文件

```text
internal/interfaces/daemon/agent.go        // Planner prompt + agentPlannerResponse + agentMessage + applyAgentPlannerLocked
internal/interfaces/daemon/server_test.go  // reasoning 返回/持久化测试 + action queue trace 测试 + mutating workspace intent gate 测试
web/frontend/src/lib/api.ts                // AgentMessage 接口
web/frontend/src/pages/agent-cockpit.tsx    // ChatMessage + ChatBubble + Runbook reasoning + Timeline reasoning/action queue
```

---

## 5. 验证

```text
cd web/frontend && npm run build
go test ./internal/interfaces/daemon -run 'TestHTTPAgentMessageAppliesLLMPlannerState|TestHTTPAgentLLMPlannerInvokesStructuredToolActions|TestHTTPAgentRoutedToolsAreSynthesizedWhenProviderConfigured|TestHTTPAgentNaturalLanguageInvokesWorkspaceTools' -count=1
go test ./internal/interfaces/daemon -run 'TestHTTPAgentLLMPlannerInvokesStructuredToolActions|TestHTTPAgentLLMNativeToolCallsAreExecutedAndSynthesized' -count=1
go test ./internal/interfaces/daemon -run 'TestHTTPAgentLLMPlannerMutatingWorkspaceActionRequiresExplicitIntent|TestHTTPAgentLLMPlannerInvokesStructuredToolActions|TestHTTPAgentLLMNativeToolCallsAreExecutedAndSynthesized' -count=1
go test ./...
```

---

## 6. 后续建议

如果继续推进，可做：

```text
- reasoning 评分（用户可以对 reasoning 质量打分）
- reasoning 搜索（搜索历史 reasoning）
- reasoning 对比（对比不同 Agent 的 reasoning）
- Agent Workspace（文件系统访问、命令执行）
- reasoning 摘要（自动生成 reasoning 摘要）
```
