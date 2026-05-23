# CyberAgent Core Contracts

版本：v0.2-confirmed  
日期：2026-05-23  
状态：核心契约已确认；MVP Core Contracts 已实现

---

## 1. 文档目标

本文定义 CyberAgent Core 的核心数据契约和接口边界。

目标是让后续实现满足：

```text
CLI / TUI / Desktop / Web 共用同一套 Core
内置工具 / 外部插件 / LLM 增强共用同一套任务模型
所有步骤可追踪、可恢复、可审计、可复现
所有安全结论都有 evidence 引用
所有高风险动作可被 Policy 和 HITL 拦截
```

本文只定义契约，不实现代码。

---

## 2. 契约设计原则

```text
1. Core Contract 优先于 UI 设计。
2. 契约稳定，具体实现可替换。
3. 所有主动行为必须绑定 Scope。
4. 所有工具调用必须绑定 Step。
5. 所有产物必须成为 Artifact。
6. 所有证据必须可追溯到 Artifact / ToolCall / Step。
7. 所有 Finding 必须引用 Evidence。
8. 所有高风险动作必须经过 PolicyDecision。
9. 所有人工介入必须记录为 Event。
10. LLM 只读取 ContextPack，不直接读取全部原始数据。
```

---

## 3. 顶层对象关系

```text
Target
  ↓
Scope
  ↓
Job
  ↓
Plan
  ↓
Step
  ↓
ToolCall
  ↓
Artifact
  ↓
Evidence
  ↓
Finding
  ↓
Report
```

横向对象：

```text
Event              记录所有状态变化和运行事件
PolicyDecision     记录策略判断结果
HITLRequest         记录人工确认请求
ReflectionResult    记录反思/质量门结果
PluginManifest      描述插件能力
Runner              描述执行环境
ContextPack         描述 LLM / Analyzer 可读取上下文
```

---

## 4. ID 命名规范

建议 ID 采用可读前缀：

```text
target_...
scope_...
job_...
plan_...
step_...
toolcall_...
artifact_...
evidence_...
finding_...
report_...
event_...
plugin_...
runner_...
hitl_...
reflection_...
```

示例：

```text
job_20260523_031500_a1b2c3
step_http_probe_001
evidence_header_security_001
finding_missing_hsts_001
```

---

## 5. Target Contract

Target 表示用户希望分析的对象。

### 5.1 MVP 支持类型

```text
web_url
```

### 5.2 后续类型

```text
api_spec
postman_collection
source_dir
har_file
burp_export
docker_compose
host
cidr
```

### 5.3 Schema 草案

```json
{
  "id": "target_001",
  "type": "web_url",
  "value": "http://localhost:8080",
  "normalized": {
    "scheme": "http",
    "host": "localhost",
    "port": 8080,
    "base_url": "http://localhost:8080",
    "path": "/"
  },
  "labels": ["local", "lab"],
  "created_at": "2026-05-23T03:20:00+08:00",
  "metadata": {}
}
```

### 5.4 设计要求

```text
Target 只描述目标，不描述授权边界。
授权边界必须由 Scope 独立声明。
Target 规范化失败时不能创建主动扫描 Job。
```

---

## 6. Scope Contract

Scope 描述授权范围，是 Policy Engine 的核心输入。

### 6.1 Schema 草案

```json
{
  "id": "scope_001",
  "allowed": {
    "hosts": ["localhost", "127.0.0.1"],
    "domains": [],
    "cidrs": ["127.0.0.0/8"],
    "ports": [8080, 3000, 80, 443],
    "schemes": ["http", "https"],
    "paths": ["/*"]
  },
  "denied": {
    "hosts": [],
    "domains": [],
    "cidrs": [],
    "ports": [],
    "paths": ["/logout", "/delete*", "/admin/destructive*"]
  },
  "limits": {
    "max_requests_per_second": 2,
    "max_concurrency_per_host": 2,
    "max_total_requests": 1000,
    "max_runtime_seconds": 1800
  },
  "profiles": ["safe"],
  "created_at": "2026-05-23T03:20:00+08:00"
}
```

### 6.2 设计要求

```text
没有 Scope，不执行主动网络动作。
Scope Check 必须发生在 Step 执行前。
重定向后的 URL 必须再次经过 Scope Check。
插件不能绕过 Scope。
```

---

## 7. Job Contract

Job 表示一次完整分析任务。

### 7.1 状态枚举

```text
created
planning
ready
running
waiting_approval
paused
succeeded
failed
cancelled
partially_succeeded
```

### 7.2 Schema 草案

```json
{
  "id": "job_20260523_032000_a1b2c3",
  "kind": "web_scan",
  "status": "created",
  "target_id": "target_001",
  "scope_id": "scope_001",
  "profile": "safe",
  "workspace": "runs/job_20260523_032000_a1b2c3",
  "plan_id": "plan_001",
  "created_at": "2026-05-23T03:20:00+08:00",
  "started_at": null,
  "finished_at": null,
  "options": {
    "interactive": true,
    "use_llm": false,
    "allow_external_tools": true,
    "report_formats": ["markdown", "json"]
  },
  "summary": {
    "steps_total": 0,
    "steps_succeeded": 0,
    "steps_failed": 0,
    "findings_total": 0
  }
}
```

---

## 8. Plan Contract

Plan 是一组可执行 Step。

### 8.1 Schema 草案

```json
{
  "id": "plan_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "version": 1,
  "planner": "builtin.web.safe.v1",
  "status": "ready",
  "steps": ["step_scope_check_001", "step_http_probe_001"],
  "created_at": "2026-05-23T03:20:05+08:00",
  "updated_at": "2026-05-23T03:20:05+08:00",
  "rationale": "safe web profile baseline plan"
}
```

### 8.2 设计要求

```text
Plan 可以版本化。
Replan 不覆盖旧 Plan，生成新 version。
Step 增删改必须记录 Event。
```

---

## 9. Step Contract

Step 是最小可调度执行单元。

### 9.1 状态枚举

```text
pending
blocked
ready
running
waiting_approval
succeeded
failed
skipped
cancelled
```

### 9.2 风险等级

```text
info
low
medium
high
critical
forbidden
```

### 9.3 Step 类型

```text
builtin_tool
external_tool
parser
analyzer
reporter
policy_check
hitl
reflection
```

### 9.4 Schema 草案

```json
{
  "id": "step_http_probe_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "plan_id": "plan_001",
  "name": "HTTP Probe",
  "type": "builtin_tool",
  "status": "pending",
  "risk": "low",
  "requires_approval": false,
  "depends_on": ["step_scope_check_001"],
  "tool": {
    "name": "builtin.http_probe",
    "version": "v1"
  },
  "input": {
    "url": "http://localhost:8080",
    "methods": ["HEAD", "GET"],
    "timeout_seconds": 10
  },
  "expected_outputs": ["artifact_http_probe", "evidence_http_status"],
  "timeout_seconds": 30,
  "retry": {
    "max_attempts": 1,
    "backoff_seconds": 2
  },
  "created_at": "2026-05-23T03:20:05+08:00"
}
```

### 9.5 设计要求

```text
所有工具调用必须通过 Step 发起。
Step input 必须经过 schema 校验。
Step 执行前必须产生 policy decision。
Step 执行结果必须产生 Event。
```

---

## 10. ToolCall Contract

ToolCall 记录一次具体工具执行。

### 10.1 Schema 草案

```json
{
  "id": "toolcall_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "step_id": "step_http_probe_001",
  "tool_name": "builtin.http_probe",
  "tool_version": "v1",
  "runner_id": "runner_local_001",
  "kind": "builtin",
  "command": null,
  "args": null,
  "cwd": "runs/job_20260523_032000_a1b2c3",
  "env_policy": "sanitized",
  "started_at": "2026-05-23T03:20:08+08:00",
  "finished_at": "2026-05-23T03:20:09+08:00",
  "exit_code": 0,
  "stdout_artifact_id": null,
  "stderr_artifact_id": null,
  "artifacts": ["artifact_http_probe_001"],
  "status": "succeeded"
}
```

### 10.2 外部命令要求

外部工具 ToolCall 必须记录：

```text
executable
args
cwd
timeout
env allowlist
exit_code
stdout artifact
stderr artifact
```

所有外部命令必须写入：

```text
commands.log
```

### 10.3 v0.1.1 插件失败分类约定

插件 ToolCall 必须在 `metadata.error_class` 中记录可审计失败类别：

```text
dependency_missing  依赖二进制不存在或不可用
timeout             Command Executor 超时
output_limit        stdout/stderr 捕获超过上限
invalid_output      插件 stdout 不符合预期 JSON/JSONL 输出
execution_failed    进程非零退出或其他执行错误
policy_skipped      Risk Policy / Scope Guard 阻止执行
```

插件输出限制：

```text
stdout 默认最多捕获 1MiB
stderr 默认最多捕获 256KiB
stdout_truncated / stderr_truncated 写入 ToolCall.metadata
```

插件 runner 必须先校验输出，再写入最终 ToolCall，避免非法输出被误记为 `succeeded`。

---

## 11. Event Contract

Event 是运行时状态的事实记录。

### 11.1 Event 类型

```text
job.created
job.started
job.finished
plan.created
plan.updated
step.started
step.finished
step.failed
step.skipped
policy.checked
approval.requested
approval.resolved
tool.started
tool.output
tool.finished
artifact.created
evidence.created
finding.created
reflection.created
report.created
error
warning
```

### 11.2 Schema 草案

```json
{
  "id": "event_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "step_id": "step_http_probe_001",
  "type": "step.started",
  "level": "info",
  "timestamp": "2026-05-23T03:20:08+08:00",
  "message": "HTTP probe started",
  "data": {
    "url": "http://localhost:8080"
  }
}
```

### 11.3 设计要求

```text
Event 使用 JSONL 落盘。
TUI / API 订阅 Event，而不是直接读取内部状态。
Event 不存大块原始数据，大块内容作为 Artifact。
```

---

## 12. Artifact Contract

Artifact 是原始或半结构化产物。

### 12.1 Artifact 类型

```text
html
http_headers
json
jsonl
text
xml
screenshot
stdout
stderr
binary
report
```

### 12.2 Schema 草案

```json
{
  "id": "artifact_homepage_html_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "step_id": "step_fetch_homepage_001",
  "toolcall_id": "toolcall_002",
  "type": "html",
  "path": "evidence/http/homepage.html",
  "sha256": "...",
  "size_bytes": 15432,
  "mime_type": "text/html",
  "created_at": "2026-05-23T03:20:12+08:00",
  "redacted": false,
  "metadata": {
    "url": "http://localhost:8080/",
    "status_code": 200
  }
}
```

---

## 13. Evidence Contract

Evidence 是从 Artifact 中提取出的可引用证据。

### 13.1 Evidence 类型

```text
http_response
http_header
cookie_attribute
html_link
html_form
js_route
api_hint
port_status
tls_certificate
tool_finding
screenshot_region
```

### 13.2 Schema 草案

```json
{
  "id": "evidence_missing_hsts_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "step_id": "step_header_check_001",
  "artifact_id": "artifact_headers_001",
  "type": "http_header",
  "title": "HSTS header is missing",
  "summary": "Strict-Transport-Security header was not present in HTTPS response.",
  "target_ref": "http://localhost:8080/",
  "data": {
    "header": "Strict-Transport-Security",
    "present": false
  },
  "confidence": "high",
  "created_at": "2026-05-23T03:20:15+08:00"
}
```

### 13.3 设计要求

```text
Evidence 不应包含大量原文。
大块内容通过 artifact_id 引用。
敏感字段必须经过 Redactor。
```

---

## 14. Finding Contract

Finding 是安全分析结论。

### 14.1 严重性枚举

```text
info
low
medium
high
critical
```

### 14.2 置信度枚举

```text
low
medium
high
confirmed
```

### 14.3 Schema 草案

```json
{
  "id": "finding_missing_hsts_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "title": "Missing HSTS Header",
  "severity": "low",
  "confidence": "high",
  "category": "security_headers",
  "affected_targets": ["http://localhost:8080/"],
  "evidence_ids": ["evidence_missing_hsts_001"],
  "description": "The response does not include the Strict-Transport-Security header.",
  "impact": "Browsers may not enforce HTTPS-only access for this host.",
  "reproduction": [
    "curl -I http://localhost:8080/",
    "Check that Strict-Transport-Security is absent."
  ],
  "remediation": "Enable HSTS on HTTPS responses with an appropriate max-age value.",
  "references": ["OWASP Secure Headers Project"],
  "source": {
    "tool": "builtin.header_checker",
    "step_id": "step_header_check_001"
  },
  "created_at": "2026-05-23T03:20:20+08:00"
}
```

### 14.4 设计要求

```text
Finding 必须引用至少一个 Evidence。
Finding 必须包含复现方式或解释为什么不可复现。
Finding 可以标记 false_positive / accepted_risk / needs_review，后续版本支持。
```

---

## 15. Report Contract

Report 是面向用户的输出集合。

### 15.1 Schema 草案

```json
{
  "id": "report_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "formats": ["markdown", "json"],
  "paths": {
    "markdown": "report.md",
    "json": "report.json"
  },
  "finding_ids": ["finding_missing_hsts_001"],
  "generated_at": "2026-05-23T03:21:00+08:00",
  "generator": "builtin.reporter.markdown.v1"
}
```

---

## 16. PluginManifest Contract

PluginManifest 描述插件能力。

### 16.1 插件类型

```text
tool
parser
analyzer
reporter
llm_provider
context_builder
```

### 16.2 Schema 草案

```json
{
  "id": "plugin_nuclei_safe",
  "name": "nuclei-safe",
  "version": "0.1.0",
  "type": "tool",
  "runtime": "external_command",
  "description": "Run selected low-risk nuclei templates",
  "capabilities": ["vulnerability_template_scan"],
  "risk": "medium",
  "requires_approval": false,
  "dependencies": [
    {
      "name": "nuclei",
      "kind": "binary",
      "version_constraint": ">=3.0.0",
      "required": true
    }
  ],
  "input_schema": "schemas/plugins/nuclei-safe.input.schema.json",
  "output_schema": "schemas/plugins/nuclei-safe.output.schema.json",
  "artifact_policy": {
    "store_stdout": true,
    "store_stderr": true,
    "store_files": ["nuclei.jsonl"],
    "redact": true
  },
  "supported_profiles": ["safe", "standard"],
  "entrypoint": {
    "kind": "command_builder",
    "path": "plugins/nuclei-safe/plugin.yaml"
  }
}
```

### 16.3 设计要求

```text
插件必须声明风险等级。
插件必须声明依赖。
插件必须声明输入输出 schema。
插件不能直接绕过 Runner。
插件默认无权访问全局文件系统，只能访问 Job workspace 和明确允许路径。
```

---

## 17. Runner Contract

Runner 表示执行环境。

### 17.1 Runner 类型

```text
local
docker
remote
k8s
```

### 17.2 Schema 草案

```json
{
  "id": "runner_local_001",
  "type": "local",
  "status": "available",
  "capabilities": ["builtin_tools", "external_commands", "python_plugins"],
  "limits": {
    "max_parallel_steps": 4,
    "max_memory_mb": null,
    "max_runtime_seconds": 3600
  },
  "metadata": {
    "os": "darwin",
    "arch": "arm64"
  }
}
```

### 17.3 Runner 接口草案

```text
submit(step) -> ToolCall
cancel(step_id) -> bool
stream_events(job_id) -> EventStream
collect_artifacts(step_id) -> Artifact[]
healthcheck() -> RunnerStatus
```

---

## 18. PolicyDecision Contract

PolicyDecision 记录策略判断。

### 18.1 决策枚举

```text
allow
allow_with_limits
require_approval
deny
```

### 18.2 Schema 草案

```json
{
  "id": "policy_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "step_id": "step_ffuf_001",
  "decision": "require_approval",
  "reasons": ["medium_risk", "directory_enumeration"],
  "applied_limits": {
    "rate_limit": "2 rps",
    "max_requests": 500
  },
  "created_at": "2026-05-23T03:20:30+08:00"
}
```

---

## 18.3 v0.1.1 持久化约定

```text
bbolt bucket: policy_decisions
key: PolicyDecision.id
scope: 至少保存每个 Step 执行前的 Risk Policy 决策
require_approval: 必须保存，并与 HITLRequest.policy_decision_id 关联
事件: events.jsonl 写入 policy.checked，data 包含 policy_decision_id / decision / reasons
```

---

## 19. HITLRequest Contract

HITLRequest 记录人工确认请求。

### 19.1 状态枚举

```text
pending
approved
rejected
modified
expired
cancelled
```

### 19.2 Schema 草案

```json
{
  "id": "hitl_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "step_id": "step_sqlmap_001",
  "policy_decision_id": "policy_step_sqlmap_001",
  "status": "pending",
  "reason": "High-risk SQL injection verification may change target state.",
  "requested_action": "execute_step",
  "options": ["approve", "deny"],
  "risk_summary": {
    "risk": "high",
    "possible_impact": ["state_change", "high_request_volume"]
  },
  "created_at": "2026-05-23T03:20:30+08:00",
  "resolved_at": null,
  "resolution": null
}
```

---

## 19.3 v0.1.1 持久化约定

```text
run workspace: approvals.json
bbolt bucket: hitl_requests
key: HITLRequest.id
状态: require_approval 默认生成 pending request，但 step 在 safe MVP 中仍为 skipped，不自动执行
事件: events.jsonl 写入 approval.requested，data 包含 approval_request_id / policy_decision_id / status / requested_action
命令审计: commands.log 的 ToolCall.metadata 包含 policy_decision_id 和 approval_request_id
恢复执行: approved 的可恢复 step 可由 replay --resume-approved 显式恢复，恢复前必须重新检查 Scope Guard
```

---

## 19.4 v0.1.1 CLI 操作约定

```bash
cyberagent approvals list <run-dir>
cyberagent approvals approve <run-dir> <approval-id-or-step-id>
cyberagent approvals deny <run-dir> <approval-id-or-step-id>
cyberagent replay <run-dir> --resume-approved
```

```text
list: 读取 run-dir/approvals.json
approve: pending -> approved，同时同步 bbolt hitl_requests，追加 approval.resolved 事件和 commands.log 审计记录
deny: pending -> rejected，同时同步 bbolt hitl_requests，追加 approval.resolved 事件和 commands.log 审计记录
resume-approved: 读取 approved HITL request，校验 bbolt step / policy decision 后恢复可恢复 step；当前 MVP 只支持 ffuf-low-rate
unsupported: 其他 approved step 明确返回 skipped/unsupported，不做隐式执行
```

---

## 20. ReflectionResult Contract

ReflectionResult 是质量门输出。

### 20.1 触发点

```text
step_failed
evidence_conflict
finding_low_confidence
before_high_risk_step
before_report
```

### 20.2 动作枚举

```text
continue
retry
skip
replan
request_approval
request_correction
mark_uncertain
```

### 20.3 Schema 草案

```json
{
  "id": "reflection_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "trigger": "before_report",
  "mode": "rule_based",
  "input_refs": {
    "finding_ids": ["finding_missing_hsts_001"],
    "evidence_ids": ["evidence_missing_hsts_001"]
  },
  "decision": "continue",
  "reasons": ["all_findings_have_evidence", "all_findings_have_reproduction"],
  "created_at": "2026-05-23T03:20:50+08:00"
}
```

---

## 21. ContextPack Contract

ContextPack 是给 LLM / Analyzer 的裁剪上下文。

### 21.1 Schema 草案

```json
{
  "id": "context_001",
  "job_id": "job_20260523_032000_a1b2c3",
  "purpose": "report_enrichment",
  "scope_summary": "Authorized target: http://localhost:8080 only.",
  "target_summary": {
    "base_url": "http://localhost:8080",
    "status": 200,
    "technologies": []
  },
  "plan_summary": {
    "completed_steps": 8,
    "failed_steps": 0
  },
  "evidence_summaries": [
    {
      "evidence_id": "evidence_missing_hsts_001",
      "summary": "HSTS header missing."
    }
  ],
  "finding_candidates": ["finding_missing_hsts_001"],
  "policy": {
    "do_not_execute_tools": true,
    "do_not_expand_scope": true,
    "output_schema_required": true
  },
  "redaction": {
    "applied": true,
    "rules": ["authorization_header", "cookie", "token"]
  }
}
```

### 21.2 设计要求

```text
LLM 不直接读取 evidence/ 全目录。
LLM 不直接读取 commands.log 全文。
ContextPack 必须经过 Redactor。
LLM 输出必须经过 schema 校验。
```

---

## 22. Job Workspace 规范

MVP 目录结构：

```text
runs/{job_id}/
  target.json
  scope.json
  job.json
  plan.json
  state/bolt.db
  events.jsonl
  commands.log
  artifacts/
    stdout/
    stderr/
    http/
    tool-results/
    screenshots/
  evidence/
    evidence.jsonl
    index.json
  findings.json
  reports/
    report.md
    report.json
```

说明：

```text
artifacts/ 存原始产物。
evidence/ 存可引用证据索引。
findings.json 存安全结论。
reports/ 存最终报告。
events.jsonl 是运行事实流。
commands.log 是外部命令审计日志。
```

---

## 23. MVP 已确认默认决策

为便于继续推进，已确认采用：

```text
TUI：纳入 MVP，但允许先实现只读观察视图，HITL 可先走 CLI prompt。
nuclei-safe：作为可选插件进入 MVP，但默认不启用；用户显式 --use-plugin nuclei-safe 或配置启用。
State Store：MVP 使用 bbolt 嵌入式 KV 保存 Job / Plan / Step 当前状态快照。
SQLite：MVP 阶段跳过；后续如需要复杂关系查询、报表过滤、Web 多任务检索再评估引入。
LLM Gateway：MVP 只定义接口，不默认接 Provider。
Reflection：MVP 使用规则实现，LLM reflection 后置。
Target Memory：后置到 v1.1 或 Desktop/Web 阶段。
```

---

## 24. 契约完成状态

本文档作为实现前的 Core Contract 基线，后续 Go struct / JSON schema / plugin protocol 都应从这里派生。

变更规则：

```text
新增核心对象字段 → 同步更新 MVP Spec / Implementation Blueprint / Test Plan。
改变状态机字段 → 同步更新 Event / State Store / TUI 设计。
改变 Evidence/Finding 字段 → 同步更新 Reporter / Test Plan。
```
