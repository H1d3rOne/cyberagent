# CyberAgent v0.1.2 Plan

版本：v0.1.2
日期：2026-05-24
状态：计划完成；Daemon Read-only API、Approval write/resume API 与 scan/job API 已完成；curl smoke 待执行

---

## 1. 阶段定位

v0.1.2 的核心方向是：

```text
Web API / Local Daemon Foundation
```

它不是直接做 Desktop，也不是做 Web 多用户平台，而是先把现有 CLI/TUI-first 的 Agent Core 抽象成可被多个入口复用的本地服务层。

阶段目标：

```text
CLI/TUI 继续可用
Core Runtime 不被 UI 绑定
Local Daemon 提供本地 HTTP API
API Contract 可被未来 Web Console / Desktop / 外部脚本复用
任务、事件、报告、审批、插件健康状态都可通过统一服务访问
```

阶段关键词：

```text
local_daemon
core_service_api
api_contract
async_jobs
event_stream
desktop_ready
web_console_ready
safe_by_default
```

---

## 2. 关键决策

### 2.1 产品入口顺序

推荐继续采用：

```text
CLI/TUI -> Local Daemon / Web API -> Desktop/Wails -> Web/Remote Runner
```

原因：

```text
CLI/TUI 最适合早期 Agent 调试、自动化和可复现。
Local Daemon 是 Desktop/Web 的共同后端，提前抽象可以避免后续重复实现。
Desktop 满足“简化操作”，但应建立在稳定本地 API 之上。
Web/Remote Runner 涉及多用户、权限、远程执行和更复杂审计，应后置。
```

### 2.2 技术栈延续

```text
Core / Daemon：Go
CLI：Cobra
TUI：Bubble Tea / Lip Gloss
API：本地 HTTP JSON + SSE
Plugin：subprocess + JSON/JSONL
Python：作为可选插件/工具生态，不作为 Core 默认运行时
```

结论：

```text
Go 负责性能、并发、分发、daemon 和核心安全边界。
Python 负责生态型工具插件、离线分析脚本、快速适配外部工具。
二者通过插件协议隔离，避免 Python 成为 Core 的性能瓶颈或部署前置依赖。
```

### 2.3 MVP State Store 决策

v0.1.2 不替换当前 State Store。

推荐继续使用：

```text
bbolt：Job / Step / Approval 等结构化状态
JSONL：events.jsonl / commands.log 等 append-only 审计流
filesystem：artifacts / reports / evidence / snapshots
```

暂不引入 SQLite：

```text
SQLite 对复杂查询、Web UI 搜索和多表关系有价值；
但 v0.1.2 重点是 API 边界和 daemon 生命周期，不应同时替换存储层。
若 run listing 性能不足，优先增加轻量 run index / cache，而不是立即迁移 SQLite。
SQLite 评估放入 P2，等 API 查询模式稳定后再决定。
```

---

## 3. 目标与非目标

### 3.1 目标

```text
1. 抽象 Core Service API，让 CLI/TUI/Daemon/未来 Desktop 共享同一业务入口。
2. 增加本地 daemon 命令，提供 HTTP JSON API。
3. 支持 run list / run detail / report / artifacts / events 查询。
4. 支持通过 API 启动 safe web scan，并查询 job status。
5. 支持 approvals list / approve / deny / resume-approved 的 API 映射。
6. 支持 plugins doctor / version / health / readiness 查询。
7. 提供初版 API contract：OpenAPI 或等价 JSON schema 文档。
8. 保持本地安全默认：127.0.0.1 监听、token auth、默认不开放 CORS。
9. 为后续 Desktop/Wails 和 Web Console 预留清晰集成边界。
```

### 3.2 非目标

```text
1. 不做多用户 SaaS。
2. 不默认监听 0.0.0.0。
3. 不做 Remote Runner。
4. 不做 Desktop/Wails 实现。
5. 不做完整 Web UI。
6. 不引入 OAuth / RBAC / 团队权限系统。
7. 不把 LLM 变成默认执行依赖。
8. 不默认启用 nuclei-safe / ffuf-low-rate 等可选外部扫描插件。
9. 不增加高风险自动利用链。
10. 不在本版本迁移到 SQLite。
```

---

## 4. 目标架构

### 4.1 当前形态

```text
CLI Command
  -> Core Runtime
    -> State Store / Event Store / Artifacts / Reports
```

### 4.2 v0.1.2 目标形态

```text
CLI Command
TUI
Local HTTP Daemon
Future Desktop/Wails
Future Web Console
        |
        v
Core Service API
        |
        v
Core Runtime / Planner / Policy / Plugins / Reporting
        |
        v
bbolt + JSONL + filesystem artifacts
```

设计原则：

```text
Daemon 是 adapter，不承载核心业务规则。
CLI 不应该复制 daemon 的业务逻辑。
Scope Guard / Risk Policy / Command Recorder 仍在 Core Runtime 内执行。
所有入口产生的 run-dir 都应可通过同一套 report / replay / tui 打开。
```

---

## 5. v0.1.2 能力范围

### 5.1 Core Service API

新增或抽象内部 service 层，建议能力：

```text
VersionService
  GetVersion()
  Doctor()

RunService
  ListRuns(filter, pagination)
  GetRun(runIDOrPath)
  GetRunSummary(runIDOrPath)
  GetEvents(runIDOrPath, cursor)
  GetArtifactIndex(runIDOrPath)
  GetReport(runIDOrPath, format)

ScanService
  StartWebScan(request)
  GetJobStatus(jobID)
  CancelJob(jobID)

ApprovalService
  ListApprovals(runIDOrPath)
  Approve(runIDOrPath, approvalID)
  Deny(runIDOrPath, approvalID)
  ResumeApproved(runIDOrPath)

PluginService
  ListPlugins()
  Doctor(jsonMode)
```

MVP 要求：

```text
service 层可以先是 internal 包，不要求公开 SDK。
service 返回结构化结果和结构化错误，不直接 fmt.Println。
CLI 可逐步迁移到 service 层，但不要求一次性重构全部命令。
```

### 5.2 Local Daemon

建议命令：

```bash
cyberagent daemon
cyberagent daemon --listen 127.0.0.1:8787
cyberagent daemon --runs-dir ./runs
cyberagent daemon --token-file ~/.cyberagent/token
```

默认行为：

```text
只监听 127.0.0.1。
默认需要 bearer token。
token 文件权限建议 0600。
默认不允许跨域 CORS。
默认并发执行任务数为 1。
退出时尽量优雅取消 daemon-launched running job。
```

### 5.3 HTTP API MVP

建议路径统一使用：

```text
/api/v1/...
```

P0 endpoint 候选：

| Method | Path | 用途 |
|---|---|---|
| `GET` | `/api/v1/healthz` | daemon 存活检查 |
| `GET` | `/api/v1/version` | binary version / commit / build date |
| `GET` | `/api/v1/doctor` | 本地环境 readiness |
| `GET` | `/api/v1/plugins` | 插件列表 |
| `GET` | `/api/v1/plugins/doctor` | 插件健康检查 |
| `GET` | `/api/v1/runs` | run 列表 |
| `GET` | `/api/v1/runs/{run_id}` | run summary/detail |
| `GET` | `/api/v1/runs/{run_id}/events` | 事件查询；P1 可扩展 SSE |
| `GET` | `/api/v1/runs/{run_id}/report?format=md|json` | 报告读取 |
| `GET` | `/api/v1/runs/{run_id}/artifacts` | artifacts/index.json |
| `GET` | `/api/v1/runs/{run_id}/approvals` | approvals list |
| `POST` | `/api/v1/runs/{run_id}/approvals/{approval_id}/approve` | approve |
| `POST` | `/api/v1/runs/{run_id}/approvals/{approval_id}/deny` | deny |
| `POST` | `/api/v1/runs/{run_id}/resume-approved` | replay approved steps |
| `POST` | `/api/v1/scans/web` | 启动 safe web scan |
| `GET` | `/api/v1/jobs/{job_id}` | job status |
| `POST` | `/api/v1/jobs/{job_id}/cancel` | cancel daemon-launched job |

### 5.4 API 数据约定

统一响应 envelope：

```json
{
  "ok": true,
  "data": {},
  "error": null
}
```

统一错误：

```json
{
  "ok": false,
  "data": null,
  "error": {
    "code": "scope_required",
    "message": "scope is required",
    "details": {}
  }
}
```

建议错误分类：

```text
bad_request
unauthorized
not_found
scope_required
policy_denied
approval_required
conflict
job_running
job_cancelled
tool_dependency_missing
internal_error
```

### 5.5 Job 执行模型

v0.1.2 只需要本地单进程 job manager：

```text
POST /scans/web 创建 job 并立即返回 job_id/run_id。
实际 scan 在 daemon 内 goroutine 执行。
job status 可查询 queued/running/succeeded/failed/cancelled。
events 仍写入 run-dir/events.jsonl。
cancel 只保证取消 daemon 当前进程启动且仍在运行的 job。
对历史 CLI run 不提供强制取消。
```

建议约束：

```text
默认 max_concurrent_jobs = 1。
第二个任务可返回 conflict，或进入 queued；MVP 推荐先返回 conflict，减少状态复杂度。
每个 job 必须记录 command/effective_config/metadata，与 CLI run 保持一致。
```

### 5.6 Event Stream

MVP 可以先做轮询读取：

```text
GET /api/v1/runs/{run_id}/events?cursor=<offset_or_sequence>
```

P1 再增加 SSE：

```text
GET /api/v1/runs/{run_id}/events/stream
```

原则：

```text
不要一次性加载超大 events.jsonl。
支持 limit。
返回 next_cursor。
客户端自行轮询或订阅。
```

### 5.7 认证与本地安全

默认策略：

```text
Bearer token required
bind 127.0.0.1 only
CORS disabled by default
no public mode in P0
```

安全边界：

```text
API 只能调用 Core Service API，不能绕过 Scope Guard。
artifact/report 读取必须限制在 run workspace 内，禁止 path traversal。
approve/deny 必须进入 events.jsonl / commands.log 或对应审计记录。
所有主动扫描仍要求 target + scope。
```

---

## 6. 优先级拆分

### P0：必须完成

```text
P0-01 Core Service API 最小抽象：run/report/plugin/approval/scan/version
P0-02 daemon 命令与本地监听：127.0.0.1 默认、可配置 listen/runs-dir/token-file
P0-03 token auth middleware：默认启用，healthz 可按需免认证或保持认证
P0-04 version / healthz / doctor / plugins doctor API
P0-05 run list / run detail / report / artifacts API
P0-06 approvals list / approve / deny API
P0-07 resume-approved API，复用现有 replay --resume-approved 行为
P0-08 POST /scans/web 异步启动 safe scan
P0-09 job status 查询，至少支持 running/succeeded/failed/cancelled
P0-10 daemon-launched job cancel MVP
P0-11 API error envelope 与基础 contract 文档
P0-12 daemon smoke test 文档与本地验证
```

当前实现进度：

```text
[x] P0-01 Slice 1：ServiceError / VersionService / PluginService / RunService 只读能力
[x] P0-01 Slice 2：ApprovalService / ResumeApproved / ScanService validation skeleton
[x] P0-02：cyberagent daemon / local HTTP server skeleton
[x] P0-03：Bearer token auth middleware
[x] P0-04：healthz / version / doctor / plugins API
[x] P0-05：run list / detail / report / artifacts API
[x] P0-06：events cursor polling API
[x] P0-07：approvals list / approve / deny API
[x] P0-08：resume-approved API
[x] P0-09+：scans / jobs HTTP API
```

### P1：应该完成

```text
P1-01 events cursor polling：limit + next_cursor
P1-02 SSE event stream：/events/stream
P1-03 run list pagination / sort / simple filter
P1-04 daemon readiness：区分 process health 与 tool readiness
P1-05 API client package 或内部 client，方便 TUI/Desktop 复用
P1-06 TUI read path 适配 service 层，但保持离线 run-dir 可打开
P1-07 OpenAPI 文件或 JSON schema 文件进入 docs/api/
P1-08 structured access log，避免泄露 token
P1-09 config file 支持 daemon defaults
P1-10 report/artifact 大文件读取限制与下载策略
```

### P2：可后置

```text
P2-01 只读 Web Console preview
P2-02 Desktop/Wails spike
P2-03 SQLite read-model 评估
P2-04 queued job model
P2-05 WebSocket 替代或补充 SSE
P2-06 API generated client
P2-07 多 run 搜索和 finding 聚合视图
P2-08 system tray / auto-start 相关 desktop 需求调研
```

### P3：明确不做

```text
P3-01 公网 daemon 模式
P3-02 多用户 / RBAC / Team workspace
P3-03 Remote Runner
P3-04 OAuth / SSO
P3-05 云端同步
P3-06 高风险自动利用链
P3-07 默认启用外部扫描插件
P3-08 存储层强制迁移 SQLite
```

---

## 7. 性能与扩展性要求

### 7.1 性能原则

```text
API handler 不做长时间阻塞扫描；扫描必须异步 job 化。
events/report/artifacts 读取避免无界加载。
run list 先基于 metadata/index，避免递归深扫所有 artifact。
daemon 保持小内存常驻；大文件按需读取。
外部插件仍通过 timeout/output limit 约束。
```

### 7.2 扩展性原则

```text
Core Service API 是稳定边界，UI 只依赖 service/API contract。
HTTP daemon 只是 adapter，后续可增加 desktop adapter/web adapter。
存储层通过 repository/service 隔离，未来 SQLite read-model 不影响 API contract。
job manager 先单机，未来 Remote Runner 再引入 runner registration/task dispatch。
```

### 7.3 Go / Python 分工

```text
Go：daemon、job manager、API、state、policy、scope、report、插件执行边界。
Python：可选离线分析、外部安全工具包装、快速实验型插件。
```

判断标准：

```text
需要常驻、并发、分发、安全边界、跨平台二进制：用 Go。
需要复用安全生态、快速脚本、数据处理原型：用 Python 插件。
```

---

## 8. 安全边界

v0.1.2 不改变现有安全原则：

```text
Scope Guard 永远在执行前检查。
Risk Policy 决策必须记录。
人工审批只能解除当前 run/step 的执行阻塞，不能扩大 scope。
插件输出仍是不可信输入，需要 schema validate、限流和归档。
run-dir 仍是复现与审计的最小可信单元。
LLM 不参与 P0 默认执行链。
```

新增 daemon 边界：

```text
默认只监听 127.0.0.1。
默认需要 bearer token。
禁止 API 任意读取本机路径。
run_id/path 解析必须限定在配置的 runs-dir 或显式允许的 run-dir 内。
artifact download 必须防 path traversal。
access log 不记录 Authorization token。
CORS 默认关闭；未来 Web Console 需要显式 allow origin。
```

---

## 9. 推荐实现顺序

```text
1. 梳理 CLI 现有业务调用，识别可抽象 service 边界。
2. 新增 Core Service API interfaces 与最小实现。
3. 将 run/report/plugin/approval 查询路径接到 service。
4. 新增 daemon command 和 HTTP router。
5. 实现 health/version/doctor/plugins API。
6. 实现 run list/detail/report/artifacts API。
7. 实现 approval approve/deny/resume-approved API。
8. 实现 async scan web job manager。
9. 实现 job status/cancel。
10. 增加 API contract 文档与 smoke test。
11. 补齐单元测试、集成测试、gofmt/go test/go vet/build 验证。
```

原则：

```text
先读路径，后写路径。
先查询已有 run，再创建新 run。
先 contract，再 UI。
先本地安全默认，再考虑可配置。
```

---

## 10. 验收标准

v0.1.2 完成时应满足：

```text
[x] cyberagent daemon 可启动并监听 127.0.0.1
[x] API 默认需要 token
[x] GET /healthz /version /doctor 可用
[x] GET /plugins /plugins/doctor 可用
[x] GET /runs 可列出已有 run
[x] GET /runs/{run_id} 可返回 run summary
[x] GET /runs/{run_id}/report?format=json|md 可读取报告
[x] GET /runs/{run_id}/artifacts 可读取 artifact index
[x] GET /runs/{run_id}/approvals 可读取 approval 状态
[x] POST approve/deny 可改变 approval 状态并记录审计
[x] POST resume-approved 复用现有恢复执行逻辑
[x] POST /scans/web 可启动 safe profile scan
[x] GET /jobs/{job_id} 可查询状态
[x] POST /jobs/{job_id}/cancel 对 daemon-launched running job 生效或给出明确 unsupported/conflict
[x] API contract 文档存在
[ ] daemon smoke test 文档存在并通过
[x] go test ./... / go vet ./... / go build 通过
```

---

## 11. 测试计划

建议新增覆盖：

```text
service: run list/detail/report/artifact/approval unit tests
service: scan request validation and scope required tests
daemon: auth required / bad token / valid token tests
daemon: health/version/doctor handlers
daemon: run read handlers with fixture run-dir
daemon: approvals approve/deny handler tests
daemon: start scan job status transition tests
daemon: cancel running job tests
daemon: path traversal rejection tests
contract: JSON envelope / error code snapshot tests
smoke: start daemon -> create scan -> poll job -> fetch report -> stop daemon
```

---

## 12. 风险与控制

| 风险 | 控制 |
|---|---|
| 过早做完整 Web UI 导致范围膨胀 | v0.1.2 只做 daemon/API，Web UI 放 P2 |
| API 绕过 CLI 已有安全边界 | API 只调用 Core Service API，执行仍走 Runtime |
| daemon 被误暴露到公网 | P0 默认 127.0.0.1 + token；不提供 public mode |
| run list 性能差 | 先基于 metadata/index，必要时加轻量 run index |
| 事件流一次性加载过多 | cursor + limit，P1 SSE 增量推送 |
| 存储迁移影响稳定性 | v0.1.2 不迁移 SQLite |
| cancel 语义过度承诺 | MVP 只支持 daemon 当前进程启动的 running job |

---

## 13. 与后续版本关系

```text
v0.1.2：Local Daemon / Web API foundation
v0.1.3：TUI Control 或只读 Web Console preview，可基于 v0.1.2 API
v0.2.0：Desktop/Wails preview，复用 daemon/API
v0.3.0：LLM Gateway 可选增强稳定化
v0.4.0+：Remote Runner / Web 多用户平台评估
```

v0.1.2 的成功标准不是“界面好看”，而是：

```text
同一个 Core 能被 CLI、TUI、daemon、未来 Desktop/Web 稳定复用。
```

---

## 14. 实施拆解

实施级任务拆解见：

```text
docs/CyberAgent-v0.1.2-Implementation-Breakdown.md
```

该文档包含：

```text
工作流分组
P0/P1/P2 tickets
依赖顺序
拟新增模块边界
验收测试
推荐提交节奏
风险控制
```

---

## 15. API Contract

v0.1.2 P0 API contract 草案见：

```text
docs/api/v0.1.2-api-contract.md
```

当前决策：

```text
P0 先使用 Markdown contract 固定 envelope、auth、错误码、endpoint 和请求/响应示例。
OpenAPI/YAML 放 P1，等 P0 实现与字段稳定后再补。
```

---

## 16. Daemon Smoke Test

v0.1.2 daemon smoke test 草案见：

```text
docs/CyberAgent-v0.1.2-Daemon-Smoke-Test.md
```

该 smoke test 覆盖：

```text
daemon 启动与 token auth
health/version/doctor/plugins API
scan web async job
job status/cancel
run/report/artifacts/events API
scope required 与 path traversal 负向测试
approval approve/deny/resume-approved API
```
