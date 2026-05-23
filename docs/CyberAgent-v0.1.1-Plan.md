# CyberAgent v0.1.1 Plan

版本：v0.1.1  
日期：2026-05-24  
状态：计划完成，待实现

---

## 1. 阶段定位

v0.1.0 已完成 CLI/TUI-first 的 MVP 闭环：授权目标输入、scope/policy 守卫、safe web scan、证据链、报告、插件声明与本地发布准备。

v0.1.1 不追求“大功能扩张”，核心目标是把 MVP 从“能跑通”推进到“更稳定、更可复现、更适合继续扩展”。

阶段关键词：

```text
stabilization
policy/HITL
plugin hardening
run reproducibility
developer experience
release hygiene
```

---

## 2. 目标与非目标

### 2.1 目标

```text
1. 补齐 v0.1.0 发布后的工程稳定性：CI、测试、文档、发布检查。
2. 完成 medium-risk step 的最小人工审批闭环，为 ffuf-low-rate 恢复执行做准备。
3. 加固插件系统：schema、超时、输出大小、错误处理、审计记录。
4. 优化 run 复现能力：命令、配置、环境、版本、artifact index 更清晰。
5. 改善 CLI/TUI 使用体验，但仍保持 CLI/TUI-first。
6. 保持默认 safe profile 保守，不扩大主动探测强度。
```

### 2.2 非目标

```text
1. 不在 v0.1.1 做 Desktop/Wails。
2. 不在 v0.1.1 做 Web 多用户平台。
3. 不把 LLM 变成默认执行依赖。
4. 不默认启用 nuclei-safe、ffuf-low-rate 等可选插件。
5. 不做高风险自动利用能力。
6. 不引入复杂分布式 Runner。
```

---

## 3. v0.1.1 能力范围

### 3.1 Release / CI Hygiene

内容：

```text
GitHub Actions: test / vet / build
GoReleaser dry-run 或 check workflow
README 安装方式补齐
release checklist 固化
版本信息注入路径确认
```

验收：

```text
本地和 CI 都能执行 go test ./... / go vet ./... / go build。
PR/commit 级别能快速暴露基础破坏。
发布前 checklist 可复用。
```

### 3.2 HITL Approval MVP

内容：

```text
PolicyDecision(require_approval) 持久化
Approval Request 记录到 run workspace
CLI 支持查看 pending approvals
CLI 支持显式 approve/deny 单个 step
scan web 支持 resume approved steps
commands.log 记录审批动作
```

建议命令形态：

```bash
cyberagent approvals list <run-dir>
cyberagent approvals approve <run-dir> <step-id>
cyberagent approvals deny <run-dir> <step-id>
cyberagent replay <run-dir> --resume-approved
```

安全约束：

```text
默认仍跳过 require_approval step。
必须显式 approve 才能恢复执行。
审批只对当前 run + 当前 step 生效。
审批动作进入 events.jsonl 和 commands.log。
审批不能绕过 Scope Guard。
```

### 3.3 Plugin Contract Hardening

内容：

```text
插件 manifest schema 更严格
插件执行 timeout
插件 stdout/stderr 大小限制
插件输出 JSON/JSONL 校验
插件失败分类：dependency_missing / timeout / invalid_output / policy_skipped / execution_failed
插件产物归档路径统一
```

验收：

```text
坏插件不会破坏整个 run。
插件失败能在 report 中解释。
插件执行边界可审计。
```

### 3.4 Run Reproducibility

内容：

```text
run metadata 增加 binary version / commit / build date
保存 scan 参数快照
保存 effective config 快照
保存 enabled/disabled plugins 快照
artifact index 增加 hash / size / mime / created_at
replay 输出更明确的步骤状态摘要
```

验收：

```text
拿到一个 run-dir 后，可以清楚知道它由哪个版本、哪些参数、哪些插件、哪些策略生成。
replay 能帮助定位失败步骤与跳过原因。
```

### 3.5 CLI/TUI UX Polish

内容：

```text
scan web 输出 run-dir、report 路径、skipped steps 摘要
plugins doctor 输出 machine-readable JSON 可选
report 命令支持 --format json|md
TUI 增加 approval/readiness 只读展示
错误消息补充 next action
```

验收：

```text
新用户完成一次 scan 后知道下一步看哪里。
调试插件/策略问题时不需要手动翻多个文件。
```

### 3.6 Testing / Fixture Expansion

内容：

```text
增加插件坏输出 fixture
增加 approval/resume 单元测试
增加 run metadata 测试
增加 report snapshot 测试
增加 CLI smoke script
```

验收：

```text
go test ./... 覆盖 v0.1.1 新增核心路径。
本地 smoke script 可作为发布前固定动作。
```

---

## 4. 优先级拆分

### P0：必须完成

```text
P0-01 GitHub Actions: test/vet/build
P0-02 release checklist 与 README 安装说明更新
P0-03 PolicyDecision(require_approval) 持久化结构确认
P0-04 approval list/approve/deny CLI MVP
P0-05 resume approved step MVP
P0-06 plugin timeout/output limit/invalid output 处理
P0-07 run metadata/effective config 快照
P0-08 v0.1.1 smoke test 文档
```

### P1：应该完成

```text
[x] P1-01 plugins doctor --json
[x] P1-02 report --format json|md
[x] P1-03 artifact index hash/size/mime
[x] P1-04 TUI approvals 只读展示
[x] P1-05 CLI skipped step 摘要优化
[x] P1-06 report 中展示 plugin failure reason
```

### P2：可后置

```text
P2-01 TUI 中直接审批
P2-02 长任务暂停/恢复
P2-03 更完整的 plugin SDK 文档
P2-04 Web API daemon 初版
P2-05 LLM Gateway 稳定化
```

### P3：明确不做

```text
P3-01 Desktop/Wails
P3-02 Web 多用户平台
P3-03 Remote Runner
P3-04 高风险自动利用链
P3-05 默认启用外部扫描插件
```

---

## 4.1 当前进度

```text
[x] P0-01 GitHub Actions: test/vet/build
[x] P0-02 release checklist 与 README 安装说明更新
[x] P0-03 PolicyDecision(require_approval) 持久化结构确认
[x] P0-04 approval list/approve/deny CLI MVP
[x] P0-05 resume approved step MVP
[x] P0-06 plugin timeout/output limit/invalid output 处理
[x] P0-07 run metadata/effective config 快照
[x] P0-08 v0.1.1 smoke test 文档
```

本地验证记录：

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build -ldflags "-X main.version=ci-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check

# P0-03 additional verification
go test ./internal/runtime/state ./internal/runtime/runs ./internal/core/runtime
go test ./...
go vet ./...
go build -ldflags "-X main.version=p0-03-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent

# P0-04 additional verification
go test ./internal/runtime/approvals ./internal/interfaces/cli
go test ./...
go vet ./...
go build -ldflags "-X main.version=p0-04-local -X main.commit=$(git rev-parse --short HEAD) -X main.date=2026-05-24T00:00:00Z" -o bin/cyberagent ./cmd/cyberagent
./bin/cyberagent approvals --help

# P0-05 additional verification
go test ./internal/plugins ./internal/core/runtime ./internal/interfaces/cli
go test ./internal/interfaces/cli -run TestReplayResumeApprovedFFUFLowRate -count=1

# P0-06 additional verification
go test ./internal/runtime/command ./internal/plugins ./internal/core/runtime ./internal/interfaces/cli

# P0-07 additional verification
go test ./internal/core/runtime ./internal/interfaces/cli

# P0-08 additional verification
docs/CyberAgent-v0.1.1-Smoke-Test.md 中的 baseline / no-scope / approval / resume-approved smoke 流程通过

# P1-01 additional verification
go test ./internal/interfaces/cli
./bin/cyberagent plugins doctor --json

# P1-02 additional verification
go test ./internal/interfaces/cli
./bin/cyberagent report <run-dir> --format json

# P1-03 additional verification
go test ./internal/runtime/artifacts ./internal/runtime/runs ./internal/core/runtime ./internal/interfaces/cli
./bin/cyberagent scan web --target <target> --scope <scope> --out <run-dir> --profile safe --no-external-tools
python3 -m json.tool <run-dir>/artifacts/index.json >/dev/null

# P1-04 additional verification
go test ./internal/interfaces/tui

# P1-05 additional verification
go test ./internal/interfaces/cli

# P1-06 additional verification
go test ./internal/core/reporting ./internal/core/runtime ./internal/interfaces/cli
```

P0-05 当前边界：

```text
cyberagent replay <run-dir> --resume-approved 已支持恢复执行 approved 的 ffuf-low-rate step。
恢复执行前会读取 run workspace、bbolt step / HITL / policy decision，并重新执行 Scope Guard。
其他 approved step 暂返回 skipped/unsupported，不会隐式执行。
```

P0-06 当前边界：

```text
插件执行使用 Command Executor timeout，默认捕获 stdout<=1MiB、stderr<=256KiB。
插件 runner 校验 stdout 后再写入最终 commands.log，避免 invalid output 被误记为 succeeded。
commands.log 的 ToolCall.metadata.error_class 记录 dependency_missing / timeout / output_limit / invalid_output / policy_skipped / execution_failed。
插件执行失败会作为 skipped plugin result 返回给 scan runtime，不中断整个 safe scan。
```

P0-07 当前边界：

```text
每个 scan web run 在 workspace 创建后写入 metadata.json / effective_config.json / plugins.snapshot.json。
metadata.json 记录 binary version / commit / build date / module、runtime、command name/args、target/scope/profile。
effective_config.json 记录 target / scope / profile / out / enabled_plugins / no_external_tools / interactive 的最终生效值。
plugins.snapshot.json 记录 run 创建时的 discovered plugins，以及 enabled / disabled 状态；依赖可用性仍以 plugins doctor 或 runner 审计为准。
这些快照描述 run 创建时事实，不在 replay/resume 中覆盖。
```

P1-01 当前边界：

```text
plugins doctor 默认文本输出保持不变。
plugins doctor --json 输出 schema_version / ok / count / plugins[]。
plugins[] 使用现有 plugin Health 结构，包含 manifest、dependencies、schemas 和 ok。
--json 用于脚本/CI/未来 TUI/Web 集成读取；缺失可选外部依赖时仍由 Health.ok 表达，不改变命令退出码。
```

P1-02 当前边界：

```text
report 默认输出 Markdown。
report --format md/markdown 输出 reports/report.md。
report --format json 输出 reports/report.json。
report 命令不重新生成报告，只读取 run-dir 已有 report 文件。
```

P1-03 当前边界：

```text
scan web 结束时写入 artifacts/index.json。
replay --resume-approved 新增或替换插件 artifact 后会重新生成 artifacts/index.json。
artifacts/index.json 包含 schema_version / generated_at / count / artifacts[]。
artifacts[] 使用 Artifact contract，并确保 sha256 / size_bytes / mime_type / created_at 可用于复现和审计。
evidence/index.json 与 reports/report.json 使用补全后的 artifact metadata。
artifact index 描述当前 run-dir 内已登记 artifacts，不扫描未登记的额外文件。
```

P1-04 当前边界：

```text
TUI 新增 Approvals 页签。
LoadRun 读取 approvals.json 中的 HITLRequest 列表；缺失 approvals.json 时按空列表处理。
Approvals 页签只读展示 status / id / step / action / policy / reason / options / resolution。
TUI 不直接 approve/deny；审批仍通过 cyberagent approvals approve|deny CLI 完成。
```

P1-05 当前边界：

```text
scan web 完成后若存在 skipped steps，会输出 skipped steps 摘要。
replay 输出 events 前会输出 skipped steps 摘要。
摘要读取 bbolt step 状态和 approvals.json。
摘要展示 step id / tool / reason / approval status。
若存在 pending/approved approval，会给出 approvals list / approve / replay --resume-approved next actions。
该摘要只读，不改变 run 状态。
```

P1-06 当前边界：

```text
report.md 在存在未解决的 plugin skipped/failed 状态时增加 Plugin Execution Notes。
report.json 增加 plugin_failures 机器可读字段。
plugin_failures 来源于 commands.log 中每个 plugin step 的最新 ToolCall 状态。
展示 tool_call_id / step_id / tool_name / status / error_class / reason / policy_decision / approval_request_id / runtime / risk。
replay --resume-approved 成功后会重新生成报告；若该 plugin step 最新状态为 succeeded，旧的 policy_skipped failure 不再展示。
该能力只解释已记录的 plugin 执行/跳过原因，不重新执行插件、不改变审批状态。
```

---

## 5. 建议里程碑

| Milestone | 目标 | 验收 |
|---|---|---|
| M10 v0.1.1 Planning | 本文档冻结 | 范围和非目标明确 |
| M11 CI/Release Hygiene | CI 与发布检查可复用 | test/vet/build workflow 通过 |
| M12 HITL Approval MVP | medium step 可显式审批后恢复 | ffuf-low-rate 可在审批后执行或进入明确等待态 |
| M13 Plugin Hardening | 插件失败不破坏 run | timeout/invalid output/dependency missing 均可审计 |
| M14 Reproducibility/UX | run 更易复现和排错 | metadata/config/plugin snapshot 完成 |
| M15 v0.1.1 RC | 发布候选 | test/vet/build/smoke/goreleaser check 通过 |

---

## 6. 推荐实现顺序

```text
1. CI 与 release checklist
2. run metadata/effective config snapshot
3. approval 数据模型与事件
4. approvals CLI: list/approve/deny
5. resume approved step 执行路径
6. plugin hardening: timeout/output/schema/error taxonomy
7. CLI/TUI/report UX polish
8. tests + smoke 文档
9. v0.1.1 release prep
```

原则：先让状态可记录，再让命令可操作，最后恢复执行。

---

## 7. 关键设计约束

```text
Scope Guard 永远在执行前检查。
Risk Policy 决策必须记录。
人工审批只能降低“是否执行”的阻塞，不能扩大 scope。
插件输出永远是不可信输入，需要校验、限流、归档。
run-dir 是复现与审计的最小可信单元。
LLM 仍是增强层，不参与 P0 默认路径。
```

---

## 8. v0.1.1 完成定义

```text
[x] CI: test/vet/build 通过
[x] release checklist 更新
[x] approval request 可持久化
[x] approval CLI 可用
[x] approved step 可恢复执行或明确进入可恢复状态
[x] plugin timeout/output/schema 错误可审计
[x] run metadata/effective config/plugin snapshot 可查看
[x] smoke test 通过并写入文档
[x] report/replay 能解释 skipped/failed/approved 状态
[ ] 本地 tag v0.1.1 可准备
```

---

## 9. v0.1.2 之后候选方向

v0.1.1 完成后，再择一进入：

```text
A. Web API / Daemon：给 Desktop/Web 复用本地 Agent Core。
B. TUI Control：从只读 viewer 进化到任务控制台。
C. LLM Gateway：把 agentic planning/reflection 做成可选增强闭环。
D. Desktop/Wails：包装本地 daemon，降低使用门槛。
```
