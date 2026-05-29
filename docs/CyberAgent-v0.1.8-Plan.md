# CyberAgent v0.1.8 Command Execution 计划

版本：v0.1  
日期：2026-05-25  
状态：实现已完成（含后端 API、Agent 自然语言命令、前端 Workspace 命令面板、白名单与工作区逃逸防护）

---

## 1. 目标

为 Agent 提供命令执行能力，使其能够在工作空间中执行受控低风险命令，成为真正的自主 Agent。

```text
核心价值：
  Agent 能在受控环境中执行 shell 命令
  Agent 能捕获命令输出和错误
  Agent 能处理命令执行超时
```

---

## 2. 非目标

```text
不做完整的沙箱隔离（v0.1 使用命令白名单）
不做远程命令执行
不做交互式命令执行
不做细粒度多角色命令权限管理
```

---

## 3. 设计

### 3.1 命令执行 API

**API 端点**：

```text
POST /api/v1/agent/sessions/{id}/workspace/command
```

**请求格式**：

```json
{
  "command": "ls -la",
  "timeout": 30,
  "workdir": "relative/path"
}
```

**响应格式**：

```json
{
  "command": "ls -la",
  "exit_code": 0,
  "stdout": "...",
  "stderr": "",
  "duration_ms": 123,
  "workdir": "/absolute/path"
}
```

### 3.2 安全限制

**命令白名单**：

```text
默认允许的命令：
- pwd, ls, cat, head, tail, wc, grep, find, sort, uniq, cut
- echo, printf, date, whoami, which, file, stat, uname, hostname, id, groups
- git 仅允许 status/log/show/diff/branch/rev-parse/ls-files
```

**执行限制**：

```text
- 超时限制：默认 30 秒，最大 300 秒
- 输出限制：默认 1MB，最大 10MB
- 工作目录限制：只能在工作空间内执行
- 拒绝绝对路径、../ 参数和逃逸工作区的 workdir
- 默认拒绝 rm/mv/cp/mkdir/touch/python/node/curl 等写入、脚本或网络命令
- HTTP API 将不允许命令映射为 policy_denied，将路径/工作目录逃逸映射为 bad_request
```

### 3.3 命令审计

**审计与可见性**：

```text
- Agent 自然语言命令通过 workspace.command tool_call 持久化
- 前端 Workspace 命令面板展示 command/exit_code/stdout/stderr/duration/workdir
- 命令面板本地保留最近命令历史
```

---

## 4. 实现计划

### 4.1 后端改动

```text
1. 新增 command 包（internal/command/）
2. 实现 CommandExecutor（命令白名单、超时、输出捕获）
3. 新增 API 端点处理函数
4. 新增 Agent Tool：workspace.command
5. 统一 command error → ServiceError 映射
6. 加固 Executor：只读白名单、git 子命令白名单、workdir/参数逃逸限制
```

### 4.2 前端改动

```text
1. AgentWorkspacePanel 新增命令执行区域
2. 命令输入框
3. 命令输出显示
4. 命令历史
5. 快捷命令：pwd / ls -la / git status
```

---

## 5. 验证

```text
go test ./internal/command ./internal/interfaces/daemon -run 'TestExecute|TestHTTPAgentWorkspaceCommandPolicy|TestHTTPAgentNaturalLanguageInvokesWorkspaceTools' -count=1  ✅ PASS
go test ./...                                                                                                                                      ✅ PASS
cd web/frontend && npm run build                                                                                                                    ✅ OK
```

---

## 6. 后续建议

如果继续推进，可做：

```text
- 命令收藏
- 命令模板
- 命令执行权限管理
- 命令执行审计报告
- 更强沙箱隔离（容器/namespace/资源限制）
