# CyberAgent Implementation Status

版本：v0.1.0  
日期：2026-05-24  
状态：MVP 实现进度快照；v0.1.1 下一阶段计划已补充

---

## 1. 当前结论

```text
规划阶段：已完成
MVP 主链路：已跑通
插件系统 MVP：已接入三类声明插件，其中 ffuf-low-rate 在 safe profile 下默认审批跳过
MVP Stabilization：本地验证通过
MVP Release：本地 release 准备完成；远程发布未执行
Next：v0.1.1 stabilization / HITL / plugin hardening / reproducibility
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
[x] cyberagent tui <run-dir>
```

### Core Runtime

```text
[x] Core Contracts
[x] Run Workspace
[x] bbolt State Store
[x] JSONL Event Store
[x] Command Recorder: commands.log
[x] Scope Guard
[x] Risk Policy
[x] Safe Profile Planner
[x] Job / Step lifecycle
[x] Evidence / Finding / Report 链路
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
```

GoReleaser 已通过：

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean --skip=publish
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
[ ] CLI/TUI/report UX polish（remaining: artifact index / TUI approvals / skipped summary / plugin failure reason）
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
