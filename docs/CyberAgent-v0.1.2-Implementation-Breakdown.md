# CyberAgent v0.1.2 Implementation Breakdown

版本：v0.1.2
日期：2026-05-24
状态：实施拆解完成；Daemon Read-only API 与 Approval write/resume API 已完成；job manager 待实现

---

## 1. 本文档定位

本文档是 `docs/CyberAgent-v0.1.2-Plan.md` 的实施级拆解。

目标不是实现代码，而是把 v0.1.2 的 Local Daemon / Web API Foundation 拆成可执行、可验收、可按顺序提交的任务。

范围：

```text
Core Service API 抽象
Local daemon 命令
HTTP API adapter
本地认证与安全默认
run/report/artifact/event 查询 API
approval 操作 API
scan web async job API
job status/cancel MVP
API contract 与 smoke test
```

非范围：

```text
不写代码
不做 Desktop/Wails
不做完整 Web UI
不做 Remote Runner
不做多用户 SaaS
不迁移 SQLite
不默认启用外部扫描插件
```

---

## 2. 总体实施策略

v0.1.2 应按“先抽象、后暴露、先读、后写、先本地安全、后扩展”的顺序推进。

推荐策略：

```text
1. 先抽象 Core Service API，让 CLI/Daemon/TUI 未来可共用。
2. 先完成只读 API：version/doctor/plugins/runs/report/artifacts/events。
3. 再完成有状态写 API：approve/deny/resume-approved。
4. 最后完成异步任务 API：scan web start/status/cancel。
5. 全程保持 CLI 原有路径可用，不做大规模破坏性重构。
```

关键原则：

```text
Daemon 只是 adapter，不复制 Core Runtime 规则。
HTTP handler 只做协议转换、认证、参数校验和响应封装。
所有安全决策仍由 Scope Guard / Risk Policy / Approval / Command Recorder 处理。
所有 run 仍落在 run-dir，继续兼容 report/replay/tui。
```

---

## 3. 推荐新增/调整模块边界

说明：以下是拟新增或调整的目录/包，不代表本阶段已实现。

```text
internal/core/services/
  version_service.go
  run_service.go
  scan_service.go
  approval_service.go
  plugin_service.go
  errors.go
  types.go

internal/interfaces/daemon/
  command.go
  config.go
  server.go
  router.go
  response.go
  auth.go
  handlers_health.go
  handlers_plugins.go
  handlers_runs.go
  handlers_approvals.go
  handlers_scans.go
  handlers_jobs.go

internal/runtime/jobs/
  manager.go
  job.go
  cancel.go

docs/api/
  v0.1.2-api-contract.md
  v0.1.2-openapi.yaml（P1 可选）

docs/
  CyberAgent-v0.1.2-Daemon-Smoke-Test.md
```

边界要求：

```text
internal/core/services 不依赖 HTTP。
internal/interfaces/daemon 可依赖 services，但 services 不反向依赖 daemon。
internal/runtime/jobs 只负责 daemon-launched job 的内存生命周期，不替代 run workspace。
CLI 可以逐步迁移到 services，但 v0.1.2 不要求一次性全部迁移。
```

---

## 4. 工作流分组

### WF-0：准备与边界冻结

目标：

```text
冻结 API scope、模块边界、错误模型和安全默认。
```

任务：

```text
WF0-01 确认 v0.1.2 P0 endpoint 列表
WF0-02 确认统一响应 envelope
WF0-03 确认错误 code 枚举
WF0-04 确认 token auth 默认策略
WF0-05 确认 run_id/path 解析规则
WF0-06 确认 daemon 默认 listen/runs-dir/token-file
```

验收：

```text
docs/CyberAgent-v0.1.2-Plan.md 与本拆解文档没有目标冲突。
P0 endpoint 和安全边界清晰。
```

---

### WF-1：Core Service API

目标：

```text
把现有 CLI/TUI 使用的核心业务能力整理成内部 service 层。
```

任务：

```text
[x] SVC-01 定义 service 层通用错误类型与错误 code
[x] SVC-02 定义 RunService request/response DTO
[x] SVC-03 定义 VersionService / PluginService / ApprovalService / ScanService DTO
[x] SVC-04 实现 VersionService：version / doctor 数据适配
[x] SVC-05 实现 PluginService：plugins list / doctor 结构化返回
[x] SVC-06 实现 RunService.ListRuns：基于 runs-dir 和 metadata 快照
[x] SVC-07 实现 RunService.GetRunSummary：读取 metadata/effective_config/report summary
[x] SVC-08 实现 RunService.GetReport：读取 report.md/report.json
[x] SVC-09 实现 RunService.GetArtifactIndex：读取 artifacts/index.json
[x] SVC-10 实现 RunService.GetEvents：cursor + limit 读取 events.jsonl
[x] SVC-11 实现 ApprovalService：list/approve/deny 复用现有 approvals runtime
[x] SVC-12 实现 ApprovalService.ResumeApproved：复用 replay --resume-approved 业务路径
[x] SVC-13 实现 ScanService.StartWebScan：接口与 target/scope 校验骨架；异步执行接入放 WF-6
```

当前实现记录：

```text
Core Service Slice 1 已完成：errors.go / version.go / plugins.go / runs.go。
对应测试：version_test.go / plugins_test.go / runs_test.go。
Slice 1 不包含 daemon、HTTP handler、ApprovalService、ScanService 或 JobManager。
Core Service Slice 2 已完成：approvals.go / scan.go。
对应测试：approvals_test.go / scan_test.go。
ApprovalService 已覆盖 list / approve / deny / resume-approved；ScanService 当前只做请求 DTO 与 target/scope 校验，不执行异步扫描。
daemon、HTTP handler 与 JobManager 仍待后续 slice 实现。
```

依赖：

```text
WF-0
```

验收：

```text
service 方法返回结构化结果，不直接输出终端文本。
service 层没有 HTTP request/response 依赖。
service 层测试可直接用 fixture run-dir 验证。
```

---

### WF-2：Daemon 基础设施

目标：

```text
新增 cyberagent daemon 命令和本地 HTTP server skeleton。
```

任务：

```text
[x] DMN-01 新增 daemon config：listen / runs-dir / token-file / max-concurrent-jobs
[x] DMN-02 新增 cyberagent daemon 命令帮助信息
[x] DMN-03 daemon 默认 listen = 127.0.0.1:8787
[x] DMN-04 daemon 默认 runs-dir = ./runs 或现有默认 run root
[x] DMN-05 daemon 启动时加载或生成 token 文件
[x] DMN-06 token 文件权限建议 0600，权限异常给出明确 warning/error
[x] DMN-07 新增 HTTP router，统一 /api/v1 前缀
[x] DMN-08 新增统一 response envelope
[x] DMN-09 新增 error -> HTTP status 映射
[~] DMN-10 access log 后置；当前未记录 request header，因此不会记录 Authorization token
[x] DMN-11 支持 graceful shutdown
```

依赖：

```text
WF-1 的基础错误模型
```

验收：

```text
cyberagent daemon --help 可查看参数。
daemon 可启动和退出。
默认只监听 127.0.0.1。
响应统一 JSON envelope。
```

当前实现记录：

```text
Daemon Skeleton 已完成：config.go / token.go / auth.go / response.go / router.go / server.go。
CLI 已新增 daemon.go 并挂载 cyberagent daemon。
当前 HTTP handler 只包含 /api/v1/healthz 与 /api/v1/version。
doctor/plugins/runs/approvals/scans/jobs handler 后续按 WF-4/WF-5/WF-6 接入。
```

---

### WF-3：认证与本地安全默认

目标：

```text
确保 daemon P0 默认不会被误暴露或绕过本地边界。
```

任务：

```text
[x] SEC-01 Bearer token auth middleware
[x] SEC-02 未携带 token 返回 unauthorized
[x] SEC-03 token 错误返回 unauthorized
[x] SEC-04 token 正确允许访问 API
[x] SEC-05 Authorization header 不进入日志：当前不记录 headers
[x] SEC-06 默认 CORS disabled
[x] SEC-07 明确不提供 public mode
[ ] SEC-08 禁止 run_id/path path traversal
[ ] SEC-09 artifact/report 读取限制在 run workspace 内
[ ] SEC-10 主动扫描仍强制 target + scope
```

依赖：

```text
WF-2
```

验收：

```text
无 token 不能访问受保护 API。
错误 token 不能访问 API。
token 不在日志输出中泄露。
../ 等 path traversal 请求被拒绝。
```

---

### WF-4：只读 HTTP API

目标：

```text
先通过 HTTP 暴露已有 run 与环境信息，降低状态复杂度。
```

任务：

```text
[x] API-R-01 GET /api/v1/healthz
[x] API-R-02 GET /api/v1/version
[x] API-R-03 GET /api/v1/doctor
[x] API-R-04 GET /api/v1/plugins
[x] API-R-05 GET /api/v1/plugins/doctor
[x] API-R-06 GET /api/v1/runs
[x] API-R-07 GET /api/v1/runs/{run_id}
[x] API-R-08 GET /api/v1/runs/{run_id}/report?format=md|json
[x] API-R-09 GET /api/v1/runs/{run_id}/artifacts
[x] API-R-10 GET /api/v1/runs/{run_id}/events?cursor=&limit=
[x] API-R-11 GET /api/v1/runs/{run_id}/approvals
```

依赖：

```text
WF-1
WF-2
WF-3
```

验收：

```text
可通过 API 查看已有 run。
report/artifacts/events 读取与 CLI 生成结果一致。
events 支持 limit 和 next_cursor。
缺失 run/report/artifacts 时返回结构化 not_found，而不是 panic。
```

当前实现记录：

```text
Read-only handler 已接入 internal/interfaces/daemon/router.go。
handler 复用 VersionService / PluginService / RunService / ApprovalService。
report JSON 会解码为 report 对象，markdown 返回 content 字符串。
events 使用 RunService cursor + limit 读取，不一次性加载超大 events.jsonl。
approvals list / approve / deny / resume-approved 已接入 daemon router。
```

---

### WF-5：Approval 写 API 与 Resume

目标：

```text
通过 API 管理 HITL approvals，并复用现有 CLI 审计与恢复执行语义。
```

任务：

```text
API-A-01 POST /api/v1/runs/{run_id}/approvals/{approval_id}/approve
API-A-02 POST /api/v1/runs/{run_id}/approvals/{approval_id}/deny
API-A-03 POST /api/v1/runs/{run_id}/resume-approved
API-A-04 approve/deny 写入现有 approvals 状态
API-A-05 approve/deny 写入事件/审计记录
API-A-06 resume-approved 复用现有 replay --resume-approved 逻辑
API-A-07 resume-approved 后刷新报告和 artifact index
API-A-08 unsupported approved step 返回明确 skipped/unsupported
```

依赖：

```text
WF-4
```

验收：

```text
API approve 后 CLI approvals list 能看到状态变化。
CLI approve 后 API approvals list 能看到状态变化。
resume-approved 行为与 CLI replay --resume-approved 一致。
Scope Guard 仍在恢复执行前检查。
```

当前实现记录：

```text
Approval write handler 已接入 internal/interfaces/daemon/router.go。
POST approve / deny 复用 ApprovalService.Approve / Deny。
POST resume-approved 复用 ApprovalService.ResumeApproved，并同步返回 resumed/skipped/run_id/run_dir。
approve/deny 已覆盖成功、重复审批 conflict、missing approval not_found、bad JSON、path traversal、wrong method 测试。
```

---

### WF-6：Async Job Manager 与 Scan API

目标：

```text
通过 daemon API 启动 safe web scan，并查询 daemon-launched job 状态。
```

任务：

```text
JOB-01 定义 job 状态：queued/running/succeeded/failed/cancelled
JOB-02 新增 in-memory job manager
JOB-03 默认 max_concurrent_jobs = 1
JOB-04 P0 第二个并发任务返回 conflict，不做复杂 queue
JOB-05 POST /api/v1/scans/web 参数校验
JOB-06 scan web request 必须包含 target + scope
JOB-07 scan web 默认 profile=safe
JOB-08 scan web 默认不启用外部插件
JOB-09 启动 goroutine 执行现有 scan runtime
JOB-10 GET /api/v1/jobs/{job_id}
JOB-11 POST /api/v1/jobs/{job_id}/cancel
JOB-12 cancel 只承诺 daemon 当前进程启动且仍 running 的 job
JOB-13 job status 关联 run_id / run_dir / error / timestamps
```

依赖：

```text
WF-1 ScanService
WF-2
WF-3
WF-4
```

验收：

```text
POST /scans/web 立即返回 job_id 和 run_id/run_dir。
GET /jobs/{job_id} 可观察 running -> succeeded/failed。
缺少 scope 的 scan request 被拒绝。
并发启动第二个任务返回明确 conflict。
cancel 对 running job 生效或返回明确 unsupported/conflict。
```

---

### WF-7：API Contract 与文档

目标：

```text
让 Desktop/Web/脚本在实现前就能基于稳定 contract 对接。
```

任务：

```text
DOC-01 新增 docs/api/v0.1.2-api-contract.md 或 openapi.yaml
DOC-02 记录统一 response envelope
DOC-03 记录 error code 与 HTTP status 映射
DOC-04 记录 auth header
DOC-05 记录 endpoint 参数、请求、响应示例
DOC-06 记录 event cursor 语义
DOC-07 记录 job status 语义
DOC-08 记录 cancel 边界
DOC-09 记录安全默认与非目标
DOC-10 新增 daemon smoke test 文档
```

依赖：

```text
WF-0 到 WF-6 设计稳定
```

验收：

```text
文档可指导 curl 级别 smoke test。
API contract 和实现 endpoint 一致。
```

当前决策：

```text
P0 先使用 docs/api/v0.1.2-api-contract.md 作为 contract 草案。
OpenAPI/YAML 放 P1，等实现与 contract 稳定后再补。
Daemon smoke test 使用 docs/CyberAgent-v0.1.2-Daemon-Smoke-Test.md 固化 curl 级别验收流程。
```

---

### WF-8：测试与发布前验证

目标：

```text
保证 daemon/API 不破坏已有 CLI/TUI/run 复现路径。
```

任务：

```text
TST-01 service unit tests：version/plugin/run/report/artifact/events
TST-02 service unit tests：approval approve/deny/resume
[x] TST-03 daemon handler tests：auth success/failure
[x] TST-04 daemon handler tests：health/version/doctor/plugins
[x] TST-05 daemon handler tests：runs/report/artifacts/events
[x] TST-06 daemon handler tests：path traversal rejection
TST-07 job manager tests：start/status/conflict/cancel
TST-08 scan API tests：scope required
TST-09 smoke test：daemon -> scan -> poll -> report -> approvals
TST-10 regression：go test ./...
TST-11 regression：go vet ./...
TST-12 regression：go build ./cmd/cyberagent
```

依赖：

```text
WF-1 到 WF-7
```

验收：

```text
新增测试覆盖 P0 API。
原有 CLI/TUI 测试不回退。
release checklist 可增加 v0.1.2 daemon smoke test。
```

---

## 5. P0 任务总表

| ID | 任务 | 依赖 | 验收重点 |
|---|---|---|---|
| P0-01 | Core Service API 最小抽象 | WF-0 | service 不依赖 HTTP |
| P0-02 | daemon 命令与本地监听 | P0-01 | 默认 127.0.0.1 |
| P0-03 | token auth middleware | P0-02 | 默认需要 bearer token |
| P0-04 | health/version/doctor/plugins API | P0-01~03 | 环境与插件状态可查询 |
| P0-05 | run/report/artifact API | P0-01~03 | 可读取已有 run |
| P0-06 | events cursor polling API | P0-05 | limit + next_cursor |
| P0-07 | approvals list/approve/deny API | P0-05 | CLI/API 状态互通 |
| P0-08 | resume-approved API | P0-07 | 复用 CLI replay 语义 |
| P0-09 | async scan web API | P0-01~03 | 立即返回 job_id |
| P0-10 | job status/cancel MVP | P0-09 | 状态可查询，可取消 running job |
| P0-11 | API contract 文档 | P0-04~10 | endpoint 与实现一致 |
| P0-12 | daemon smoke test 文档 | P0-04~11 | 可用 curl 复现 |

---

## 6. P1 任务总表

| ID | 任务 | 说明 |
|---|---|---|
| P1-01 | SSE event stream | 在 cursor polling 稳定后增加 |
| P1-02 | run list pagination/sort/filter | 先做简单分页，不做全文搜索 |
| P1-03 | readiness 分层 | process health 与 tool readiness 分开 |
| P1-04 | internal API client | 为未来 TUI/Desktop 复用 |
| P1-05 | TUI 读取路径 service 化 | 保持离线 run-dir 可打开 |
| P1-06 | OpenAPI 文件 | 若 P0 先用 markdown contract，P1 补 OpenAPI |
| P1-07 | access log 完善 | 结构化日志、耗时、状态码 |
| P1-08 | daemon config file | 持久化 daemon 默认配置 |
| P1-09 | 大文件读取限制 | report/artifact download limit/range 策略 |
| P1-10 | API compatibility snapshot tests | 防止 contract 意外变化 |

---

## 7. P2 后置评估

```text
P2-01 只读 Web Console preview
P2-02 Desktop/Wails spike
P2-03 SQLite read-model 评估
P2-04 queued job model
P2-05 WebSocket 替代或补充 SSE
P2-06 generated API client
P2-07 多 run 搜索和 finding 聚合
P2-08 tray / auto-start desktop 调研
```

进入 P2 的条件：

```text
P0 API 稳定
P0 daemon smoke test 通过
P1 中至少 events/pagination/readiness 方向已明确
```

---

## 8. 依赖顺序图

```text
WF-0 Boundary Freeze
        |
        v
WF-1 Core Service API
        |
        v
WF-2 Daemon Skeleton
        |
        v
WF-3 Auth/Safety
        |
        v
WF-4 Read-only API
        |
        +---------> WF-5 Approval/Resume API
        |
        +---------> WF-6 Async Scan/Job API
                         |
                         v
WF-7 Contract/Smoke Docs
        |
        v
WF-8 Tests/Release Verification
```

关键路径：

```text
Core Service API -> Daemon Skeleton -> Auth -> Read-only API -> Scan Job API -> Contract/Smoke -> RC
```

---

## 9. 推荐提交节奏

每个提交应尽量小而可验证。

```text
commit 1: service error/types + version/plugin service tests
commit 2: run service list/detail/report/artifact/events tests
commit 3: approval service list/approve/deny/resume tests
commit 4: daemon command/server/router/response skeleton
commit 5: auth middleware + security tests
commit 6: health/version/doctor/plugins handlers
commit 7: runs/report/artifacts/events handlers
commit 8: approvals/resume handlers
commit 9: job manager
commit 10: scan web async API + job status/cancel handlers
commit 11: API contract + daemon smoke test docs
commit 12: final gofmt/go test/go vet/go build + status docs
```

不建议：

```text
一次性大重构 CLI、TUI、Runtime、Daemon。
先做 Web UI 再补 API。
先做 queue/multi-user/remote runner。
先迁移 SQLite。
```

---

## 10. 验收命令规划

实现完成后建议至少执行：

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build -ldflags "-X main.version=v0.1.2-rc -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

daemon smoke test 预期流程：

```text
详见 docs/CyberAgent-v0.1.2-Daemon-Smoke-Test.md。
```

approval smoke test 预期流程：

```text
1. create run with optional ffuf-low-rate enabled, safe profile requires approval
2. GET approvals
3. POST approve
4. POST resume-approved
5. GET report
6. confirm plugin failure/skipped/succeeded notes are consistent with CLI behavior
```

---

## 11. 风险清单

| 风险 | 触发点 | 控制方式 |
|---|---|---|
| service 抽象过大 | 试图一次性重写 CLI | 只抽 P0 API 所需能力 |
| HTTP handler 复制业务逻辑 | 为快速返回直接读写底层文件 | handler 只调用 service |
| path traversal | run_id/report/artifact 参数未净化 | run resolver 限制 runs-dir/workspace |
| token 泄露 | access log 打印 header | 日志过滤 Authorization |
| daemon 误公网暴露 | 默认或文档建议 0.0.0.0 | P0 固定默认 127.0.0.1，不提供 public mode |
| events 大文件内存膨胀 | 一次性读取 events.jsonl | cursor + limit |
| job cancel 语义过度承诺 | 尝试取消历史 CLI run | 只支持 daemon-launched running job |
| P0 范围膨胀 | 提前做 UI/queue/SSE | P0 只做 foundation |

---

## 12. v0.1.2 完成定义

```text
[x] Core Service API P0 抽象完成
[x] daemon command 可启动
[x] 默认 127.0.0.1 + token auth 生效
[x] 只读 API 可读取已有 run/report/artifact/events
[x] approval API 与 CLI 状态互通
[ ] scan web API 可异步启动 safe scan
[ ] job status/cancel MVP 可用
[ ] API contract 文档完成
[ ] daemon smoke test 文档完成并通过
[ ] gofmt/go test/go vet/go build 通过
[ ] v0.1.2 release notes 草案准备
```

---

## 13. 下一步建议

如果继续实现，建议从：

```text
SVC-01 service 层通用错误类型与错误 code
SVC-02 RunService request/response DTO
SVC-04 VersionService：version / doctor 数据适配
```

开始。

理由：

```text
service 错误模型会影响 daemon response envelope。
Version/Doctor/Plugin 是最容易验证的只读路径。
先完成这些可以快速跑通 daemon skeleton，而不触碰 scan runtime。
```
