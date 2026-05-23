# CyberAgent Release Checklist

版本：v0.1.1  
日期：2026-05-24  
状态：发布检查清单已建立

---

## 1. 适用范围

本文是 CyberAgent 的可复用发布前检查清单，适用于：

```text
v0.1.x patch release
v0.2+ minor release
本地 RC 验证
远程 GitHub Release 前最终确认
```

原则：

```text
先验证本地可复现，再创建 tag。
先确认 tag 指向，再构建发布产物。
远程 push / GitHub Release 必须显式执行。
默认不发布、不上传、不推送。
```

---

## 2. 发布前工作区检查

```bash
git status --short --ignored
git log --oneline --decorate -5
git tag --list --format='%(refname:short) %(objectname:short) %(subject)'
```

验收：

```text
没有未提交源码变更。
ignored 文件只包含 bin/、dist/、node_modules/、缓存等本地产物。
release tag 指向预期 commit。
```

---

## 3. 基础质量检查

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
```

验收：

```text
gofmt 无输出。
go test 全部通过。
go vet 无错误。
```

---

## 4. CLI 构建与版本检查

```bash
go build -ldflags "-X main.version=local -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o bin/cyberagent ./cmd/cyberagent

./bin/cyberagent version
./bin/cyberagent --version
./bin/cyberagent doctor
./bin/cyberagent plugins list
./bin/cyberagent plugins doctor
```

验收：

```text
CLI 可构建。
version 输出包含预期版本信息。
doctor 可运行。
插件列表和插件 doctor 可运行，缺失可选依赖时应降级说明而不是崩溃。
```

---

## 5. Smoke Test

推荐使用 `testdata/simple-site` 作为本地授权目标。

v0.1.1 P0 专用 smoke 流程见：`docs/CyberAgent-v0.1.1-Smoke-Test.md`。

```bash
rm -rf runs/smoke-simple-site

# 在另一个终端启动 simple-site，或使用项目已有 smoke 启动脚本/测试方式。
# 然后执行：
./bin/cyberagent scan web \
  --target http://127.0.0.1:8080 \
  --scope 127.0.0.1 \
  --profile safe \
  --out runs/smoke-simple-site

./bin/cyberagent report runs/smoke-simple-site
./bin/cyberagent replay runs/smoke-simple-site
```

验收：

```text
runs/smoke-simple-site/metadata.json 存在。
runs/smoke-simple-site/effective_config.json 存在。
runs/smoke-simple-site/plugins.snapshot.json 存在。
runs/smoke-simple-site/events.jsonl 存在。
runs/smoke-simple-site/commands.log 存在。
runs/smoke-simple-site/reports/report.md 存在。
runs/smoke-simple-site/reports/report.json 存在。
report job.status = succeeded。
events.jsonl 包含 job.finished。
```

---

## 6. 安全边界回归

必须确认：

```text
无 --scope 时拒绝主动扫描。
默认不启用 nuclei-safe。
默认不启用 ffuf-low-rate。
requires_approval step 不会自动执行。
所有 tool/plugin 调用写入 commands.log。
```

建议命令：

```bash
./bin/cyberagent scan web --target http://127.0.0.1:8080 --out runs/no-scope-check
./bin/cyberagent scan web --target http://127.0.0.1:8080 --scope 127.0.0.1 --profile safe --out runs/default-safe-check
./bin/cyberagent scan web --target http://127.0.0.1:8080 --scope 127.0.0.1 --profile safe --use-plugin ffuf-low-rate --out runs/ffuf-approval-check
```

验收：

```text
no-scope-check 应失败或拒绝执行主动扫描。
default-safe-check 不包含 nuclei-safe / ffuf-low-rate 主动执行。
ffuf-approval-check 中 ffuf-low-rate 应记录为 requires_approval / skipped，除非用户显式审批。
```

---

## 7. GoReleaser 检查

配置检查：

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
```

本地 snapshot：

```bash
rm -rf dist
go run github.com/goreleaser/goreleaser/v2@v2.15.4 release --snapshot --clean --skip=publish
```

如网络不稳定，可使用：

```bash
env GOPROXY=https://goproxy.cn,direct GOSUMDB=off   go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
```

验收：

```text
check 通过。
snapshot 生成 darwin/linux amd64/arm64 产物和 checksums.txt。
```

---

## 8. Tag 与远程发布

创建本地 tag：

```bash
git tag -a vX.Y.Z -m "CyberAgent vX.Y.Z"
git show --stat vX.Y.Z
```

推送远程仓库和 tag：

```bash
git push -u origin main
git push origin vX.Y.Z
```

真实 GitHub Release：

```bash
GITHUB_TOKEN=... goreleaser release --clean
```

注意：

```text
上述 push / release 命令需要用户明确授权后执行。
不要把本地 snapshot dist/ 作为源码提交。
```

---

## 9. 发布后验证

```bash
cyberagent version
cyberagent --version
cyberagent doctor
cyberagent plugins list
cyberagent plugins doctor
```

对 simple-site 再跑一次 safe scan，并确认：

```text
report.md 可读。
report.json 可解析。
replay 输出可解释每一步状态。
TUI 可只读打开 run-dir。
```

---

## 10. 当前 v0.1.1 进度

```text
[x] CI: test/vet/build/goreleaser check workflow
[x] README 安装方式补齐
[x] release checklist 固化
[x] HITL Approval MVP
[x] Plugin Hardening
[x] Run Reproducibility
[x] v0.1.1 smoke test 文档
```
