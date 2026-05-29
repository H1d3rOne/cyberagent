# CyberAgent v0.1.5 Agent Auto-Continue Plan

版本：v0.1  
日期：2026-05-25  
状态：实现已完成（含后端 Planner 增强 + 思考动画 + 可配置限制 + 设置页面 + 规则 Agent 多命令顺序执行 + routed tool synthesis + Timeline trace polish）

---

## 1. 目标

让 Agent 具备自动续行能力：工具执行完成后，Agent 自动观察结果、更新计划、执行下一步，形成真正的 Agent 循环。

```text
核心流程：
  用户消息 → Agent 规划 → 执行工具 → 观察结果 → 自动续行 → ... → 完成/等待人工
```

---

## 2. 非目标

```text
不做多 Agent 并行
不做长时间后台 Agent 任务
不做自动 exploit
```

---

## 3. 已实现功能

### 3.1 后端 Planner 增强

**agentNextAction 结构体新增字段**：

```text
AutoContinue bool `json:"auto_continue,omitempty"`
```

**Planner prompt 增强**：

```text
- 中文 prompt：next_action 新增 auto_continue 字段，约束说明何时设为 true/false
- 英文 prompt：next_action 新增 auto_continue 字段，约束说明何时设为 true/false
```

**auto_continue 判断逻辑**：

```text
- true：任务未完成、有明确下一步、不需要人工审批或确认
- false：任务已完成、需要人工输入、存在不确定风险
```

### 3.2 前端自动续行触发

**触发条件**（全部满足）：

```text
1. activeRunId 存在且运行状态变为 succeeded
2. agentNextAction.auto_continue === true（来自 Planner 或前端判断）
3. 用户当前未在输入（working === false）
4. 自动续行次数 < autoContinueLimit（可配置，默认 3）
```

**触发行为**：

```text
1. 前端检测到 run 状态变为 succeeded 且有更多 todos
2. 等待 2.5 秒（UI 反馈延迟）
3. 自动发送 suggested_prompt 消息
4. 显示 "自动续行 (count/limit)" 状态指示
```

**中断机制**：

```text
1. 用户发送任意消息 → 重置 autoContinueCount
2. autoContinueCount >= autoContinueLimit → 停止，等待用户确认
3. Next Action 区域显示 "停止续行" 按钮
```

### 3.3 思考动画增强

**自动续行时的思考指示**：

```text
- 消息列表区域显示 "Agent 正在自动续行 (count/limit)…" 动画
- 使用 amber 色调区分普通工作状态和自动续行状态
- Sparkles 图标带脉冲动画
```

**普通工作状态**：

```text
- 消息列表区域显示 "Agent 正在规划/调用工具…" 动画
- 使用 primary 色调
```

### 3.4 可配置自动续行限制

**ConsoleSettings 新增字段**：

```text
autoContinueLimit: number  // 自动续行次数上限，默认 3，范围 0-10
```

**设置页面配置**：

```text
- 设置页面新增 "自动续行上限" 配置项
- 支持 0-10 范围，0 表示禁用自动续行
- 默认值 3
```

**使用位置**：

```text
- 自动续行触发判断
- 思考动画显示
- Next Action 状态指示
- AgentTaskCard 组件
```

---

## 4. 涉及文件

```text
internal/interfaces/daemon/agent.go        // Planner prompt + agentNextAction 结构体
web/frontend/src/lib/api.ts                // AgentNextAction 接口 + ConsoleSettings + 默认值
web/frontend/src/pages/agent-cockpit.tsx    // 自动续行逻辑 + UI 指示 + 思考动画
web/frontend/src/pages/settings.tsx         // 设置页面配置项
```

---

## 5. 验证

```text
go test ./internal/interfaces/daemon          ✅ PASS
cd web/frontend && npm run build              ✅ OK (tsc + vite)
go build ./...                                ✅ OK
```

---

## 6. 后续建议

### 5.1 追加验证：规则 Agent 多命令顺序执行

已完成：

```text
- 规则 Agent 可从自然语言中识别多个低风险工作区命令，例如“先执行 pwd 和 ls -la，再总结”
- 多命令会按顺序执行，每个命令沉淀为独立 workspace.command tool_call
- tool_call result 增加 command_index / command_count，便于 UI 和审计链路还原执行顺序
- 响应消息会聚合每个命令的状态、exit_code 和关键输出
- 只执行 allowlist 内命令；仍沿用 workspace command 逃逸检查与策略拒绝
```

验证：

```bash
go test ./internal/interfaces/daemon -run 'TestHTTPAgentNaturalLanguageInvokesWorkspaceTools' -count=1
cd web/frontend && npm run build
go test ./...
```

---

## 7. 后续建议

### 5.2 追加验证：确定性工具结果自动 synthesis

已完成：

```text
- 配置 AI provider 时，即使命中的是规则 Agent / 确定性内置工具分支，也会在工具执行后进入 observe → synthesize
- synthesis 会读取刚刚执行的 tool observations，生成面向用户的总结、TODO 更新和 next_action
- 对多命令 workspace.command 结果同样生效：先执行多个低风险命令，再让 LLM 基于 observation 总结关键输出和下一步
- 失败时保留原工具结果，并追加 LLM synthesis failed tool message，不影响已完成的工具执行链
```

验证：

```bash
go test ./internal/interfaces/daemon -run 'TestHTTPAgentRoutedToolsAreSynthesizedWhenProviderConfigured|TestHTTPAgentNaturalLanguageInvokesWorkspaceTools|TestHTTPAgentLLMPlannerInvokesStructuredToolActions' -count=1
cd web/frontend && npm run build
go test ./...
```

---

## 8. 后续建议

### 5.3 追加验证：Agent Timeline trace polish

已完成：

```text
- Agent Cockpit 执行 Timeline 不再只显示普通 tool/event 列表
- Timeline item 会区分 planner、workspace tool、tool_synthesis、auto_continue、next_action、approval gate 等阶段
- workspace.command 多命令会展示 command_index / command_count，便于观察“先 A 后 B”的顺序
- LLM synthesis、next_action、auto_continue 以独立 visual badge 展示，能更清晰表达 plan → tool → observe → synthesize → next_action
- next_action 的 suggested_prompt 会在 Timeline 中以内联 next 提示展示
```

验证：

```bash
cd web/frontend && npm run build
go test ./internal/interfaces/daemon -run 'TestHTTPAgentRoutedToolsAreSynthesizedWhenProviderConfigured|TestHTTPAgentNaturalLanguageInvokesWorkspaceTools' -count=1
go test ./...
```

---

## 9. 后续建议

如果继续推进，可做：

```text
- 自动续行历史记录（显示每次续行的内容）
- 自动续行时显示 Agent 思考过程（reasoning）
- 多轮自动续行的 task/todo 状态跟踪优化
- 自动续行暂停/恢复功能
- Agent Workspace（文件系统访问、命令执行）
