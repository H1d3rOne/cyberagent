# CyberAgent Release Plan

版本：v0.1  
日期：2026-05-24  
状态：本地发布准备完成，远程发布待执行；发布检查清单已固化

---

## 1. 发布目标

MVP v0.1 发布目标：

```text
提供一个可本地运行、可复现、默认安全保守的 CLI/TUI-first Web/API 安全分析 Agent。
```

---

## 2. 版本策略

建议版本：

```text
v0.1.0：MVP 首个可试用版本
v0.1.x：Bugfix / 文档修复 / 小增强
v0.2.0：插件系统增强或 TUI 增强
v0.3.0：LLM 增强
v0.4.0：Desktop / Local API 预览
v1.0.0：稳定契约和可长期使用版本
```

---

## 3. MVP v0.1 发布范围

必须包含：

```text
CLI safe scan
内置低风险 Web 分析
Run workspace
Event log
State store
Evidence/Finding/Report
doctor
TUI run viewer
基础文档
```

可选包含：

```text
nuclei-safe 插件，但默认不启用
ffuf-low-rate 插件，但默认不启用
python-js-route-analyzer 插件
```

不包含：

```text
Desktop
Web
Remote Runner
自动 exploit
多用户
PDF 报告
LLM 默认启用
```

---

## 4. 发布前检查清单

可复用清单见：`docs/CyberAgent-Release-Checklist.md`。

最小检查项：

```text
gofmt 已执行
go test ./... 通过
go vet ./... 通过
CLI build/version/doctor 通过
README 快速开始可用
docs 索引完整
示例 target 测试通过
默认不运行 nuclei-safe / ffuf-low-rate
无 scope 拒绝扫描
requires_approval step 不自动执行
report.md 可读
release notes 完成
GoReleaser check/snapshot 通过
```

---

## 4.1 当前本地检查结果

截至 2026-05-24，本地已通过：

```bash
go test ./...
go vet ./...
go build -ldflags "-X main.version=v0.1.0-test -X main.commit=local -X main.date=2026-05-24" -o bin/cyberagent ./cmd/cyberagent
```

GoReleaser 本地验证已通过：

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean --skip=publish
```

本地 tag 计划：

```text
v0.1.0 指向 release-prep 提交；远程 tag 待推送。
```

Smoke test 已通过：

```text
testdata/simple-site
version / doctor / plugins list / plugins doctor / scan web / report / replay
```

---

## 5. 构建产物

MVP 建议产物：

```text
macOS arm64
macOS amd64
Linux amd64
Linux arm64
```

Windows 后置评估。

命名建议：

```text
cyberagent_v0.1.0_darwin_arm64.tar.gz
cyberagent_v0.1.0_darwin_amd64.tar.gz
cyberagent_v0.1.0_linux_amd64.tar.gz
cyberagent_v0.1.0_linux_arm64.tar.gz
```

---

## 6. Release Notes

当前 release notes 文件：

```text
docs/release-notes/v0.1.0.md
docs/release-notes/v0.1.1.md
docs/release-notes/v0.1.2.md
```

## 6.0 Runtime API Compatibility Migration Window

Production Agent Runtime 的旧 engine seam 在当前 release gate 只作为外部兼容入口保留：

```text
runtime.Engine.RunLLMTurn / runtime.LLMTurnRunner: Deprecated；新 bounded loop 调用方迁移到 runtime.Engine.RunAgentLoop。
runtime.Engine.ExecuteTurn / runtime.TurnExecutor: Deprecated；AgentLoop-shaped 调用方迁移到 runtime.Engine.RunAgentLoop，单 action 执行迁移到 runtime.Engine.ExecuteAgentLoopAction。
迁移窗口：current_release_gate。
默认 attachment 策略：AttachLLMTurnRunner 与 AttachTurnExecutor retained_until_release_gate。
opt-in compatibility flag：当前 release gate 不新增；是否把兼容 attachment 转为 opt-in，推迟到 current_release_gate 之后做独立决策。
```

发布边界：

```text
release notes、API contract 与 release checklist 必须同步上述 Deprecated / current_release_gate / replacement API / retained_until_release_gate / opt-in 策略。
在上述文档发布前，不将 AttachLLMTurnRunner 或 AttachTurnExecutor 移入删除候选。
在 release gate 后，如要新增 opt-in compatibility flag，必须作为单独小切片实现并保留默认行为迁移说明。
```

## 6.1 Release Notes 模板

```text
# CyberAgent v0.1.0

## Highlights
- CLI/TUI-first authorized Web/API security analysis agent.
- Safe profile with built-in low-risk checks.
- Evidence-first report generation.

## Features
- ...

## Security Boundaries
- No active scan without scope.
- High-risk actions require approval.
- Optional tools disabled by default.

## Known Limitations
- TUI is read-only run viewer.
- No Desktop/Web yet.
- LLM not enabled by default.

## Upgrade Notes
- First release.
```

---

## 7. 发布后验证

```text
下载二进制
运行 cyberagent version
运行 cyberagent doctor
对 simple-site 执行 safe scan
打开 TUI run viewer
检查 report.md
```
