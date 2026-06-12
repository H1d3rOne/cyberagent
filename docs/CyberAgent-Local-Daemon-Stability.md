# CyberAgent Local API / Daemon 稳定化记录

日期：2026-06-09
状态：L2 已完成

## 1. L2 成功标准

Local API / Daemon 稳定版需要逐步覆盖：

- 配置：daemon defaults 可由 `.cyberagent.yaml` 或显式 `--config` 管理，CLI flags 仍具最高优先级。
- Token：bearer token file 行为稳定、权限明确、不会被日志误泄露。
- Runtime DB migration：SQLite runtime store schema / migration / recovery 行为可审计。
- 兼容策略：`CYBERAGENT_RUNTIME` 继续只作为 whole-engine rollback switch；不新增额外 deprecated runtime API opt-in flag。

## 2. L2-D1：daemon config file defaults

当前已支持在通用 CyberAgent config 中加入 daemon section：

```yaml
daemon:
  listen: 127.0.0.1:8787
  runs_dir: runs
  token_file: .cyberagent/daemon-token
  max_concurrent_jobs: 1
```

行为：

- `cyberagent daemon` 默认读取 user config 与项目 `.cyberagent.yaml`。
- `cyberagent daemon --config path/to/config.yaml` 读取指定配置。
- CLI flags 覆盖 config file：例如显式 `--listen` 不会被 config 中的 `daemon.listen` 覆盖。
- 未设置 daemon section 时保留现有 daemon defaults。

代码落点：

- `internal/core/config/config.go`
  - 新增 `DaemonConfig` 与 `Config.Daemon`。
  - merge 支持 `listen`、`runs_dir`、`token_file`、`max_concurrent_jobs`。
- `internal/interfaces/cli/daemon.go`
  - `daemon` command 新增 `--config`。
  - `applyDaemonConfigFile` 按“config defaults + changed flags override”生成 effective daemon config。

验证：

```bash
gofmt -w internal/core/config/config.go internal/core/config/config_test.go internal/interfaces/cli/daemon.go internal/interfaces/cli/root_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/config ./internal/interfaces/cli -run 'TestMergeOverlayWins|TestLoadFromFile|TestDaemonCommandHelp|TestApplyDaemonConfigFileUsesDefaultsAndPreservesChangedFlags'
git diff --check
```

## 3. L2-D2：token stability audit

当前 token 稳定化新增 daemon package helper：

- `InspectTokenFile(path)`：返回 safe-to-log 诊断视图，不包含 token value。覆盖 missing、empty、readable、length、mode、secure permissions 与 error。
- `RedactBearerTokenForLog(token)`：只保留 trimmed token length 或 empty marker，禁止日志输出 token 原文。

当前 token 行为边界：

- `NewServer` / `loadOrCreateToken` 仍负责创建缺失 token file，生成的 token file 权限为 `0600`（非 Windows）。
- 既有 token file 不在本切片强制 chmod 或拒绝 loose mode；`InspectTokenFile` 先提供诊断证据，避免破坏已有部署。
- Dashboard / Desktop bootstrap 仍可在 loopback / WebView 注入路径返回 token；日志和 safe runtime contract 必须使用 redacted/diagnostic view。

验证：

```bash
gofmt -w internal/interfaces/daemon/token_inspection.go internal/interfaces/daemon/token_inspection_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestInspectTokenFile|TestRedactBearerToken'
git diff --check
```

## 4. L2-D3：runtime DB migration audit

当前 SQLite runtime store 新增最小 schema version / audit seam：

- `CurrentSchemaVersion = 1`，对应 SQLite `PRAGMA user_version`。
- `Open(path)` 初始化 schema 后会把既有 `user_version=0` 的 runtime DB 非破坏性标记为版本 1；不会重写已经带版本的 DB。
- `Store.AuditSchema(ctx)` 返回 safe-to-log `SchemaAudit`：
  - DB path、当前 `user_version`、期望 schema version；
  - 当前 table / index 列表；
  - required table / index 的 missing 列表。

当前必需 schema 对象：

- tables：`sessions`、`tasks`、`runs`、`turns`、`events`、`approvals`、`schedules`、`memories`
- indexes：`idx_runtime_tasks_session_created`、`idx_runtime_runs_task_created`、`idx_runtime_turns_run_idx`、`idx_runtime_events_task_seq`、`idx_runtime_events_run_seq`、`idx_runtime_events_session_seq`

行为边界：

- 本切片只做 schema version metadata 与审计入口，不做破坏性 migration。
- interrupted runs / corrupt DB recovery 仍沿用现有 runtime replay/recovery 行为；后续如要自动 repair，应先通过 `AuditSchema` 输出证据再设计显式修复命令。
- `runtime.db` 默认路径仍由 daemon 使用 `runsDir/.agent/runtime/runtime.db` 打开。

验证：

```bash
gofmt -w internal/runtime/state/sqlite/store.go internal/runtime/state/sqlite/store_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/runtime/state/sqlite -run 'TestStoreAuditSchema|TestStoreOpenSetsUserVersion'
git diff --check
```

## 5. L2-D4：compatibility strategy doc/tests

本切片固化 Local Daemon runtime 兼容策略：

- `CYBERAGENT_RUNTIME` 是唯一 runtime 环境开关，并且只作为 whole-engine rollback switch 使用。
- 默认 `CYBERAGENT_RUNTIME` 为空或未知值时启用 runtime engine。
- `legacy`、`off`、`disabled`、`disable`、`false`、`0`、`none`（大小写与空白归一化后）关闭整个 runtime engine，用于回退到 no-engine legacy path。
- 默认 `AttachEngine` 仍挂接 `AttachLLMTurnRunner` 与 `AttachTurnExecutor`，继续为 deprecated external runtime API 提供兼容，不新增 dedicated opt-in flag。
- 不新增 `CYBERAGENT_RUNTIME_COMPAT` 或 `CYBERAGENT_DEPRECATED_RUNTIME_API`。

代码落点：

- `internal/interfaces/daemon/server.go`
  - `agentRuntimeEngineEnabledFromEnv(os.Getenv("CYBERAGENT_RUNTIME"))` 作为唯一 runtime gate。
  - 默认路径打开 `runsDir/.agent/runtime/runtime.db`、创建 `runtime.Engine`、`AttachEngine` 并执行 interrupted-run recovery。
- `internal/interfaces/daemon/agent_runtime.go`
  - `AttachEngine` 默认挂接 `AttachTurnExecutor`、`AttachLLMTurnRunner`、`AttachLLMStageRunner`、`AttachAgentLoopRunner`、`AttachAgentLoopActionExecutor`、`AttachSessionMutator`。
- `internal/interfaces/daemon/agent_runtime_compatibility_test.go`
  - `TestDaemonLocalRuntimeCompatibilityStrategyKeepsWholeEngineSwitchAndDefaultAttachments` 覆盖 whole-engine switch、opt-out aliases、默认 attachments 与禁止新增 dedicated runtime compatibility flag。

验证：

```bash
gofmt -w internal/interfaces/daemon/agent_runtime_compatibility_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestDaemonLocalRuntimeCompatibilityStrategy|TestAgentRuntimeEngineEnabledFromEnv|TestNewServerDefaultRuntimeEngineCanOptOutToLegacy'
git diff --check
```

## 6. L2 完成结论

Local API / Daemon 稳定版的四个子项已完成：

- [x] L2-D1 daemon config file defaults
- [x] L2-D2 token diagnostics / redaction
- [x] L2-D3 runtime DB migration audit
- [x] L2-D4 compatibility strategy doc/tests

后续建议进入 L3 Web 多用户平台，先做最小 architecture/design slice，再逐步实现 auth、workspace tenancy、API boundary 与 deployment hardening。
