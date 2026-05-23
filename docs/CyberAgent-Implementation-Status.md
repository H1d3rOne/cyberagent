# CyberAgent Implementation Status

版本：v0.1.2 Implementation
日期：2026-05-24
状态：v0.1.1 本地 RC 准备完成；v0.1.2 Local Daemon / Web API 已开始实现；Daemon Read-only API 完成

---

## 1. 当前结论

```text
规划阶段：已完成
MVP 主链路：已跑通
插件系统 MVP：已接入三类声明插件，其中 ffuf-low-rate 在 safe profile 下默认审批跳过
MVP Stabilization：本地验证通过
MVP Release：v0.1.0 本地 release 准备完成；远程发布未执行
v0.1.1 RC：本地实现、文档、测试、release notes 已准备；tag/push/GitHub Release 未执行
v0.1.2 Planning：计划、实施拆解、API Contract 与 daemon smoke test 草案已完成
v0.1.2 Implementation：Daemon Read-only API 已完成；Approval 写 API / scan job manager 尚未实现
Next：接入 Approval write API：approve / deny / resume-approved
```

---

## 2. 已完成能力

### CLI / TUI

```text
[x] cyberagent version
[x] cyberagent doctor
[x] cyberagent plugins list
[x] cyberagent plugins doctor
[x] cyberagent plugins doctor --json
[x] cyberagent scan web
[x] cyberagent report <run-dir> --format md|json
[x] cyberagent replay <run-dir>
[x] scan web / replay skipped steps 摘要与 next actions
[x] cyberagent tui <run-dir>（Plan / Events / Approvals / Evidence / Findings / Report）
[x] cyberagent daemon（Local HTTP API skeleton）
```

### Core Runtime

```text
[x] Core Contracts
[x] Run Workspace
[x] bbolt State Store
[x] JSONL Event Store
[x] Command Recorder: commands.log
[x] Artifact Index: artifacts/index.json
[x] Scope Guard
[x] Risk Policy
[x] Safe Profile Planner
[x] Job / Step lifecycle
[x] Evidence / Finding / Report 链路
```

### Core Services / v0.1.2

```text
[x] ServiceError / ErrorCode 边界模型
[x] VersionService: GetVersion / Doctor
[x] PluginService: ListPlugins / Doctor
[x] RunService: ListRuns / GetRun / GetRunSummary
[x] RunService: GetReport / GetArtifactIndex / GetEvents cursor polling
[x] RunService path traversal / missing run 错误映射测试
[x] ApprovalService: list / approve / deny
[x] ApprovalService: resume-approved
[x] ScanService: StartWebScan validation skeleton
[ ] ScanService: async job manager execution
```

### Local Daemon / v0.1.2

```text
[x] Daemon config: listen / runs-dir / token-file / max-concurrent-jobs
[x] cyberagent daemon --help
[x] Default listen: 127.0.0.1:8787
[x] Token file load/create with 0600 permission
[x] Bearer token auth middleware
[x] Unified /api/v1 response envelope
[x] ServiceError -> HTTP status mapping
[x] GET /api/v1/healthz
[x] GET /api/v1/version
[x] GET /api/v1/doctor
[x] GET /api/v1/plugins / plugins/doctor
[x] GET /api/v1/runs / report / artifacts / events / approvals
[ ] Approval write HTTP API
[ ] Scan async job manager / job status / cancel
```

### Builtin Web/API Tools

```text
[x] HTTP Probe
[x] HTTP Fetcher
[x] Header Checker
[x] Cookie Checker
[x] robots.txt Fetch + Parse
[x] sitemap.xml Fetch + Parse
[x] HTML Extractor
[x] JavaScript Asset Collector
[x] Light JS Route Extractor
[x] API Hint Detector
[x] Small Content Discovery
[x] TLS Inspector
[x] TCP common web port check
```

### Plugin MVP

```text
[x] Plugin Manifest discovery
[x] Plugin Doctor dependency check
[x] Plugin Doctor schema check
[x] external_command runner boundary
[x] subprocess_jsonl runner boundary
[x] nuclei-safe: explicit opt-in, missing dependency skipped
[x] python-js-route-analyzer: explicit opt-in, offline JS artifact analysis
[x] ffuf-low-rate: explicit opt-in, safe profile 下 requires_approval 并跳过审计
```

---

## 3. 当前验证

已通过：

```bash
go test ./...
```

关键测试覆盖：

```text
scope required
safe planner optional plugins disabled by default
plugins doctor --json machine-readable health output
nuclei-safe explicit opt-in + missing dependency skip
python-js-route-analyzer explicit opt-in + route evidence
ffuf-low-rate explicit opt-in + approval skip
ffuf-low-rate approved 后可通过 replay --resume-approved 恢复执行
plugin timeout / output_limit / invalid_output 可审计并不会破坏 safe scan
run metadata / effective config / plugin snapshot 可复现快照
commands.log audit trail
state store step lifecycle
report/evidence/finding output
scan web / replay skipped steps summary
report plugin failure reason / plugin_failures output
TUI run viewer
```

---

## 3.1 最新本地验证记录

已通过：

```bash
go test ./...
go vet ./...
go build -ldflags "-X main.version=v0.1.0-test -X main.commit=local -X main.date=2026-05-24" -o bin/cyberagent ./cmd/cyberagent
```

Smoke test 已通过：

```text
testdata/simple-site
runs/smoke-simple-site
version / doctor / plugins list / plugins doctor / scan web / report / replay
report.json job.status = succeeded
report.md status = succeeded
events.jsonl contains job.finished
```

v0.1.1 P0 smoke test 已通过：

```text
docs/CyberAgent-v0.1.1-Smoke-Test.md
runs/smoke-v011-simple-site
runs/smoke-v011-ffuf-approval
baseline safe scan
metadata/effective_config/plugins.snapshot JSON assertions
no-scope rejection
ffuf-low-rate approval pending
approvals approve audit trail
fake ffuf resume-approved
CLI skipped step summary 由 go test ./internal/interfaces/cli 覆盖
report plugin failure reason 由 go test ./internal/core/reporting ./internal/core/runtime ./internal/interfaces/cli 覆盖
```

GoReleaser 已通过：

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean --skip=publish
```

v0.1.1 RC 本地发布前验证已通过：

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build -ldflags "-X main.version=v0.1.1-rc -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
go run "$(go env GOMODCACHE)/github.com/goreleaser/goreleaser/v2@v2.15.4" check -f .goreleaser.yml
```

备注：

```text
一次在线 go run github.com/goreleaser/goreleaser/v2@v2.15.4 check 因 Go proxy/sumdb 网络超时失败；
随后使用本地 module cache 中的 GoReleaser v2.15.4 对 .goreleaser.yml 执行 check，通过。
未创建 v0.1.1 tag，未 push，未发布 GitHub Release。
```

v0.1.2 Core Service API Slice 1 验证已通过：

```bash
gofmt -w internal/core/services
go test ./internal/core/services
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-services-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

Slice 1 边界记录：

```text
已新增 internal/core/services，仅提供内部 service 层能力。
本批未新增 daemon command，未新增 HTTP handler，未改变 CLI 行为。
RunService 支持读取已有 run 的 summary/detail/report/artifacts/events。
PluginService 复用现有 plugins.Manager / plugins.Doctor。
VersionService 复用 runtime doctor。
ApprovalService / ScanService / JobManager 在 Slice 1 时仍待实现。
```

v0.1.2 Core Service API Slice 2 验证已通过：

```bash
gofmt -w internal/core/services internal/runtime/approvals
go test ./internal/core/services ./internal/runtime/approvals
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-services-slice2-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

当前边界：

```text
ApprovalService 已支持 list / approve / deny / resume-approved。
approve/deny 复用现有 approvals runtime，并支持 service/API 传入 operator/comment。
ScanService 当前只提供 StartWebScan request/response DTO 与 target/scope 校验骨架。
异步执行、job status/cancel、daemon command 和 HTTP handler 仍待实现。
```

v0.1.2 Daemon Skeleton 验证已通过：

```bash
gofmt -w internal/interfaces/daemon internal/interfaces/cli
go test ./internal/interfaces/daemon ./internal/interfaces/cli
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-daemon-skeleton-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

当前边界：

```text
已新增 internal/interfaces/daemon local HTTP adapter。
cyberagent daemon 已接入 CLI。
P0 默认监听 127.0.0.1:8787，默认使用 bearer token。
token 文件不存在时自动生成，权限为 0600。
HTTP API 已统一 envelope，并完成 /api/v1/healthz 与 /api/v1/version。
doctor/plugins/runs/report/artifacts/events/approvals/scans/jobs HTTP handler 在 skeleton 阶段仍待后续 slice 接入。
```

v0.1.2 Daemon Read-only API 验证已通过：

```bash
gofmt -w internal/interfaces/daemon internal/runtime/doctor
go test ./internal/interfaces/daemon ./internal/runtime/doctor
go test ./...
test -z "$(gofmt -l cmd internal)"
go vet ./...
go build -ldflags "-X main.version=v0.1.2-daemon-readonly-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
```

当前边界：

```text
已接入 /api/v1/doctor。
已接入 /api/v1/plugins 与 /api/v1/plugins/doctor。
已接入 /api/v1/runs、/api/v1/runs/{run_id}。
已接入 /api/v1/runs/{run_id}/report?format=json|md。
已接入 /api/v1/runs/{run_id}/artifacts。
已接入 /api/v1/runs/{run_id}/events?cursor=&limit=。
已接入 /api/v1/runs/{run_id}/approvals 只读列表。
doctor report JSON 字段已补充稳定 json tag。
Approval write API、resume-approved API、scan web async job API、job status/cancel 仍待实现。
```

---

## 4. 里程碑状态

| Milestone | 状态 | 说明 |
|---|---|---|
| M0 Planning Complete | 完成 | 规划文档已冻结 |
| M1 Project Skeleton Ready | 完成 | CLI 可编译运行 |
| M2 Core Runtime Ready | 完成 | State/Event/Policy/Scope/Workspace 已实现 |
| M3 Safe Profile Plan Ready | 完成 | safe web plan 已实现 |
| M4 Builtin Web Analysis Ready | 完成 | 内置 Web 分析闭环已实现 |
| M5 CLI Safe Scan Ready | 完成 | `scan web` 可生成 run/report |
| M6 Plugin MVP Ready | 基本完成 | 三个插件均进入 MVP 链路；ffuf 当前默认审批跳过 |
| M7 TUI Run Viewer Ready | 完成 | 只读打开已有 run |
| M8 MVP Release Candidate | 完成 | README/限制/示例/test/vet/build/smoke/GoReleaser snapshot 已完成 |
| M9 MVP v0.1 Release | 本地准备完成 | 本地 git/origin/release notes/GoReleaser snapshot 完成；本地 tag 指向本提交，尚未 push/release 到 GitHub |
| M10 v0.1.1 RC | 本地准备完成 | v0.1.1 P0/P1、release notes、test/vet/build 本地验证完成；尚未创建 tag、push 或 GitHub Release |
| M11 v0.1.2 Planning | 完成 | 选择 Local Daemon / Web API foundation；计划文档已新增，尚未实现 |
| M12 v0.1.2 Implementation Breakdown | 完成 | 工作流、tickets、依赖、验收、测试与提交节奏已拆解；尚未实现 |
| M13 v0.1.2 API Contract Draft | 完成 | HTTP API envelope、auth、错误码、endpoint 与请求/响应示例已形成草案；尚未实现 |
| M14 v0.1.2 Daemon Smoke Test Draft | 完成 | daemon 启动、认证、scan/job/report/artifact/event/approval/cancel smoke 流程已形成草案；尚未实现 |
| M15 v0.1.2 Core Service Slice 1 | 完成 | ServiceError、VersionService、PluginService、RunService 只读能力已实现并测试；daemon 尚未实现 |
| M16 v0.1.2 Core Service Slice 2 | 完成 | ApprovalService、ResumeApproved 与 ScanService 校验骨架已实现并测试；daemon/job manager 尚未实现 |
| M17 v0.1.2 Daemon Skeleton | 完成 | cyberagent daemon、token auth、response envelope、healthz/version API 已实现并测试；业务 read/write API 待接入 |
| M18 v0.1.2 Daemon Read-only API | 完成 | doctor/plugins/runs/report/artifacts/events/approvals list 已接入并测试；write/job API 待接入 |

---

## 5. 待完成项

### MVP Stabilization

```text
[x] 生成或整理一个可提交的示例 run / testdata
[x] 最终检查 README 与 docs 是否完全一致
[x] 明确 LLM agent 代码路径是 experimental，不作为 MVP Core 默认路径
[x] 清理重复 release 配置：.goreleaser.yaml / .goreleaser.yml 二选一
[x] 最终 smoke test：build + scan + report + replay + plugin doctor；TUI 由测试覆盖 LoadRun/viewer
```

### Release

```text
[x] 初始化或确认 git 仓库
[x] 配置远程 GitHub 仓库：github.com/H1d3rOne/cyberagent
[x] 准备 v0.1.0 release notes
[x] 验证 goreleaser build
[x] 创建本地 tag v0.1.0
[ ] 推送 tag v0.1.0
[ ] 发布 GitHub Release 二进制产物
```

### v0.1.1 下一阶段

```text
[x] 生成 v0.1.1 计划文档：docs/CyberAgent-v0.1.1-Plan.md
[x] CI: test/vet/build
[x] release checklist 与 README 安装说明更新
[x] PolicyDecision(require_approval) / approval request 持久化结构
[x] approvals list/approve/deny CLI MVP
[x] resume approved step
[x] plugin timeout/output/schema hardening
[x] run metadata/effective config/plugin snapshot
[x] v0.1.1 smoke test 文档
[x] plugins doctor --json
[x] CLI/TUI/report UX polish
[x] v0.1.1 release notes：docs/release-notes/v0.1.1.md
[x] v0.1.1 本地 tag 可准备（未创建；需用户显式授权）
```

### v0.1.2 Local Daemon / Web API Foundation

```text
[x] 选择 v0.1.2 方向：Web API / Local Daemon foundation
[x] 生成 v0.1.2 计划文档：docs/CyberAgent-v0.1.2-Plan.md
[x] 生成 v0.1.2 实施拆解文档：docs/CyberAgent-v0.1.2-Implementation-Breakdown.md
[x] 生成 v0.1.2 API Contract 草案：docs/api/v0.1.2-api-contract.md
[x] 生成 v0.1.2 daemon smoke test 草案：docs/CyberAgent-v0.1.2-Daemon-Smoke-Test.md
[x] Core Service API Slice 1：ServiceError / VersionService / PluginService / RunService 只读能力
[x] Core Service API Slice 2：ApprovalService / ResumeApproved / ScanService validation skeleton
[x] cyberagent daemon 命令
[x] 本地 token auth / 127.0.0.1 默认监听
[x] /api/v1/healthz 与 /api/v1/version
[x] /api/v1/doctor 与 plugins API
[x] run/report/artifact/event/approval list 查询 API
[ ] scan web async job API
[ ] approval approve/deny/resume-approved API
[ ] job status/cancel MVP
[ ] daemon smoke test 本地验证通过
```

### 后置能力

```text
[ ] Desktop/Wails
[ ] Local API / Daemon 稳定版
[ ] Web 多用户平台
[ ] Remote Runner
[ ] 长期 Target/User/Team Memory
[ ] LLM Gateway 作为可选增强稳定化
```
