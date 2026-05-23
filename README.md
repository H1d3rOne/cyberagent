# CyberAgent

CyberAgent 是一个 **CLI/TUI-first** 的授权 Web/API 安全分析 Agent。MVP 优先提供可复现、可审计、低风险的本地 safe profile 扫描闭环；Desktop、Web、Remote Runner 和复杂 LLM 增强后置。

## 当前 MVP 能力

- Deterministic Web safe scan：`cyberagent scan web`
- 只读 TUI run viewer：`cyberagent tui <run-dir>`
- Run workspace：`target.json`、`scope.json`、`job.json`、`plan.json`、`events.jsonl`、`commands.log`
- State Store：bbolt
- Event Store：JSONL
- Scope Guard / Risk Policy
- Evidence / Finding / Markdown + JSON Report
- 插件系统：manifest discovery、doctor、external command、subprocess JSONL
- 可选插件：`nuclei-safe`、`python-js-route-analyzer`、`ffuf-low-rate`

## 安装

### 方式一：从源码构建（当前推荐）

```bash
git clone https://github.com/H1d3rOne/cyberagent.git
cd cyberagent

go test ./...
go build -o bin/cyberagent ./cmd/cyberagent
./bin/cyberagent version
```

### 方式二：使用 Go 安装

适合已经安装 Go 工具链的用户：

```bash
go install github.com/H1d3rOne/cyberagent/cmd/cyberagent@latest
cyberagent version
```

说明：在远程 GitHub Release 正式发布前，`@latest` 取决于远程仓库是否已经推送对应 tag。

### 方式三：下载 Release 二进制

v0.1.0 本地 release 产物已验证；远程 GitHub Release 发布后可下载对应平台压缩包：

```text
cyberagent_v0.1.0_darwin_arm64.tar.gz
cyberagent_v0.1.0_darwin_amd64.tar.gz
cyberagent_v0.1.0_linux_amd64.tar.gz
cyberagent_v0.1.0_linux_arm64.tar.gz
```

解压后执行：

```bash
./cyberagent version
./cyberagent doctor
```

## 快速开始

```bash
# 本地开发验证
go test ./...
go vet ./...

# 构建 CLI
go build -o bin/cyberagent ./cmd/cyberagent

# 查看版本
./bin/cyberagent version

# 环境检查
./bin/cyberagent doctor

# 插件检查
./bin/cyberagent plugins list
./bin/cyberagent plugins doctor
```

执行一次 safe Web/API 扫描：

```bash
./bin/cyberagent scan web \
  --target http://localhost:8080 \
  --scope localhost \
  --profile safe \
  --out runs/demo
```

查看输出：

```bash
./bin/cyberagent report runs/demo
./bin/cyberagent replay runs/demo
./bin/cyberagent tui runs/demo
```

## 可选插件

默认不会运行外部安全工具。必须显式启用：

```bash
./bin/cyberagent scan web \
  --target http://localhost:8080 \
  --scope localhost \
  --use-plugin nuclei-safe \
  --out runs/with-nuclei

./bin/cyberagent scan web \
  --target http://localhost:8080 \
  --scope localhost \
  --use-plugin python-js-route-analyzer \
  --out runs/with-js-plugin
```

插件行为：

| Plugin | 默认启用 | 风险 | MVP 行为 |
|---|---:|---|---|
| `nuclei-safe` | 否 | low | 显式启用后运行；缺少 `nuclei` 时跳过并写入 `commands.log` |
| `python-js-route-analyzer` | 否 | low | 只分析已保存 JS artifact，不主动联网；输出必须是合法 JSON/JSONL |
| `ffuf-low-rate` | 否 | medium + approval | 显式启用后进入 plan；safe profile 下生成 pending approval 并默认跳过；approved 后可用 `replay --resume-approved` 恢复执行 |

插件执行边界：

```text
默认 stdout 捕获上限 1MiB，stderr 捕获上限 256KiB。
timeout / output_limit / invalid_output / dependency_missing / policy_skipped / execution_failed 会写入 commands.log 的 error_class。
插件输出会先校验再记录最终 ToolCall，避免非法输出被误记为 succeeded。
```

## 审批命令

`requires_approval` 的步骤默认不会自动执行。当前 v0.1.1 支持审批状态管理：

```bash
./bin/cyberagent approvals list runs/with-ffuf
./bin/cyberagent approvals approve runs/with-ffuf step_optional_ffuf_low_rate_001
./bin/cyberagent approvals deny runs/with-ffuf step_optional_ffuf_low_rate_001
./bin/cyberagent replay runs/with-ffuf --resume-approved
```

说明：

```text
approvals list      查看 approvals.json 中的 pending/approved/rejected 请求
approvals approve   将 pending 请求标记为 approved，并写入 events.jsonl / commands.log / bbolt
approvals deny      将 pending 请求标记为 rejected，并写入 events.jsonl / commands.log / bbolt
replay --resume-approved  恢复执行已 approved 的可恢复 step，并继续 replay events.jsonl
```

当前 v0.1.1 P0-05 只支持恢复执行 approved 的 `ffuf-low-rate` step；其他 approved step 会明确标记为 skipped/unsupported，不会隐式执行。

## 内置 Web 分析能力

- HTTP probe / fetch
- Security header checker
- Cookie attribute checker
- robots.txt / sitemap.xml fetch + parse
- HTML link/form/script extractor
- JavaScript asset collector
- Light JS route extractor
- API hint detector
- Small low-rate content discovery
- TLS inspector
- TCP common web port check

## 安全边界

- 没有 `--scope` 时拒绝主动扫描
- 所有主动 URL 请求经过 Scope Guard
- 中高风险或 `requires_approval` 步骤由 Risk Policy 拦截
- 外部工具默认不启用
- 所有 tool/plugin 调用写入 `commands.log`
- Evidence / Finding / Artifact 互相引用，方便复现和审计

## 项目结构

```text
cmd/cyberagent/                 CLI entrypoint
internal/core/                  contracts / planner / policy / runtime / reporting
internal/runtime/               runs / state / artifacts / command recorder
internal/interfaces/cli/        Cobra CLI
internal/interfaces/tui/        Bubble Tea TUI viewer
internal/plugins/               plugin manifest / doctor / runner
internal/tools/builtin/         built-in Web/API tools
plugins/                        optional plugin manifests and adapters
schemas/                        plugin and contract schemas
docs/                           planning, ADR, roadmap, test/release docs
```

## 开发命令

```bash
make fmt
make test
make build-quick

# 或直接
gofmt -w cmd internal
go test ./...
go vet ./...
go build -o bin/cyberagent ./cmd/cyberagent
```

## 发布前检查

详细清单见：`docs/CyberAgent-Release-Checklist.md`。

最小本地检查：

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build -ldflags "-X main.version=local -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o bin/cyberagent ./cmd/cyberagent
./bin/cyberagent version
./bin/cyberagent --version
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
```

真实远程发布需要显式推送 tag 并运行 GoReleaser；本项目默认不会自动 push 或创建 GitHub Release。

## 当前限制

- MVP 只聚焦授权 Web URL / Web API safe profile
- Desktop、Web 多用户平台、Remote Runner 后置
- LLM 能力存在实验性代码路径，但不是 MVP Core 依赖
- `ffuf-low-rate` 不会在 safe profile 中自动执行；必须显式启用、显式审批，并通过 `replay --resume-approved` 恢复
- 不做自动 exploit、爆破、横向移动或破坏性 payload

## License

MIT
