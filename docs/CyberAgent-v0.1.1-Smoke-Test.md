# CyberAgent v0.1.1 Smoke Test

版本：v0.1.1  
日期：2026-05-24  
状态：P0 smoke test 文档；本地验证通过

---

## 1. 目标

本 smoke test 用于发布前快速验证 v0.1.1 P0 主链路：

```text
CLI build/version/doctor
safe web scan baseline
run metadata / effective config / plugin snapshot
approval request 持久化
approvals list/approve/deny CLI
replay --resume-approved
plugin failure taxonomy 基础回归
report / replay 可读
默认 safe profile 保守边界
```

注意：

```text
不要在 smoke test 中 push tag 或创建 GitHub Release。
不要默认启用外部扫描插件。
ffuf-low-rate 只在显式 --use-plugin 后进入 approval 流程。
```

---

## 2. 前置条件

```text
Go toolchain 可用。
Python 3 可用，用于启动 testdata/simple-site 静态 fixture。
当前工作目录为仓库根目录。
```

fixture：

```text
testdata/simple-site/index.html
testdata/simple-site/app.js
testdata/simple-site/robots.txt
testdata/simple-site/sitemap.xml
```

---

## 3. 构建与基础检查

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build \
  -ldflags "-X main.version=v0.1.1-smoke -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/cyberagent ./cmd/cyberagent

./bin/cyberagent version
./bin/cyberagent --version
./bin/cyberagent doctor
./bin/cyberagent plugins list
./bin/cyberagent plugins doctor
./bin/cyberagent plugins doctor --json
```

验收：

```text
gofmt / test / vet / build 全部通过。
version 与 --version 均可输出。
doctor 可运行。
plugins list / doctor / doctor --json 可运行；缺失可选外部依赖时应报告为不可用，而不是崩溃。
```

---

## 4. 启动本地授权目标

在一个终端启动 fixture：

```bash
python3 -m http.server 18080 --directory testdata/simple-site
```

如果端口被占用，可换端口，但后续命令中的 `--target` 和 `--scope` 需要同步调整。

---

## 5. Baseline safe scan

```bash
rm -rf runs/smoke-v011-simple-site

./bin/cyberagent scan web \
  --target http://127.0.0.1:18080 \
  --scope 127.0.0.1 \
  --profile safe \
  --out runs/smoke-v011-simple-site \
  --no-external-tools

./bin/cyberagent report runs/smoke-v011-simple-site
./bin/cyberagent replay runs/smoke-v011-simple-site
```

必须存在：

```text
runs/smoke-v011-simple-site/metadata.json
runs/smoke-v011-simple-site/effective_config.json
runs/smoke-v011-simple-site/plugins.snapshot.json
runs/smoke-v011-simple-site/target.json
runs/smoke-v011-simple-site/scope.json
runs/smoke-v011-simple-site/job.json
runs/smoke-v011-simple-site/plan.json
runs/smoke-v011-simple-site/events.jsonl
runs/smoke-v011-simple-site/commands.log
runs/smoke-v011-simple-site/evidence/index.json
runs/smoke-v011-simple-site/findings.json
runs/smoke-v011-simple-site/reports/report.md
runs/smoke-v011-simple-site/reports/report.json
```

JSON 快速断言：

```bash
python3 - <<'PY'
import json
from pathlib import Path

run = Path("runs/smoke-v011-simple-site")
metadata = json.loads((run / "metadata.json").read_text())
effective = json.loads((run / "effective_config.json").read_text())
plugins = json.loads((run / "plugins.snapshot.json").read_text())
report = json.loads((run / "reports/report.json").read_text())
events = (run / "events.jsonl").read_text()

assert metadata["schema_version"] == "0.1.1"
assert metadata["binary"]["version"]
assert metadata["command"]["name"] == "scan web"
assert metadata["runtime"]["goos"]
assert effective["profile"] == "safe"
assert effective["target"] == "http://127.0.0.1:18080"
assert effective["scope"] == "127.0.0.1"
assert effective["no_external_tools"] is True
assert "ffuf-low-rate" in plugins.get("disabled", [])
assert report["job"]["status"] == "succeeded"
assert "job.finished" in events
print("baseline smoke assertions passed")
PY
```

---

## 6. 安全边界回归

无 scope 应拒绝主动扫描：

```bash
rm -rf runs/smoke-v011-no-scope

if ./bin/cyberagent scan web \
  --target http://127.0.0.1:18080 \
  --out runs/smoke-v011-no-scope; then
  echo "unexpected success: scan without --scope"
  exit 1
fi
```

验收：

```text
命令失败。
错误包含 scope is required。
```

---

## 7. Approval / HITL smoke

显式启用 `ffuf-low-rate`，但不审批：

```bash
rm -rf runs/smoke-v011-ffuf-approval

./bin/cyberagent scan web \
  --target http://127.0.0.1:18080 \
  --scope 127.0.0.1 \
  --profile safe \
  --use-plugin ffuf-low-rate \
  --out runs/smoke-v011-ffuf-approval

./bin/cyberagent approvals list runs/smoke-v011-ffuf-approval
./bin/cyberagent replay runs/smoke-v011-ffuf-approval
```

断言：

```bash
python3 - <<'PY'
import json
from pathlib import Path

run = Path("runs/smoke-v011-ffuf-approval")
approvals = json.loads((run / "approvals.json").read_text())
plugins = json.loads((run / "plugins.snapshot.json").read_text())
commands = [json.loads(line) for line in (run / "commands.log").read_text().splitlines() if line.strip()]
ffuf_calls = [c for c in commands if c.get("tool_name") == "plugin.ffuf-low-rate"]

assert len(approvals) == 1
assert approvals[0]["status"] == "pending"
assert approvals[0]["step_id"] == "step_optional_ffuf_low_rate_001"
assert "ffuf-low-rate" in plugins.get("enabled", [])
assert ffuf_calls
assert ffuf_calls[-1]["status"] == "skipped"
assert ffuf_calls[-1]["metadata"]["error_class"] == "policy_skipped"
assert ffuf_calls[-1]["metadata"]["policy_decision"] == "require_approval"
print("approval smoke assertions passed")
PY
```

审批：

```bash
./bin/cyberagent approvals approve \
  runs/smoke-v011-ffuf-approval \
  step_optional_ffuf_low_rate_001

./bin/cyberagent approvals list runs/smoke-v011-ffuf-approval
```

断言：

```bash
python3 - <<'PY'
import json
from pathlib import Path

run = Path("runs/smoke-v011-ffuf-approval")
approvals = json.loads((run / "approvals.json").read_text())
commands = [json.loads(line) for line in (run / "commands.log").read_text().splitlines() if line.strip()]

assert approvals[0]["status"] == "approved"
assert any(c.get("tool_name") == "cyberagent.approvals" and c.get("status") == "succeeded" for c in commands)
print("approval approve assertions passed")
PY
```

---

## 8. Resume approved smoke（可选）

如果本机没有 `ffuf`，可用临时 fake binary 验证恢复路径；该 fake binary 只输出固定 JSON，不做网络探测。

```bash
FAKE_BIN="$(mktemp -d)"
cat > "$FAKE_BIN/ffuf" <<'SH'
#!/bin/sh
printf '%s\n' '{"results":[{"url":"http://127.0.0.1:18080/admin","status":200,"length":5}]}'
SH
chmod +x "$FAKE_BIN/ffuf"

PATH="$FAKE_BIN:$PATH" ./bin/cyberagent replay \
  runs/smoke-v011-ffuf-approval \
  --resume-approved
```

断言：

```bash
python3 - <<'PY'
import json
from pathlib import Path

run = Path("runs/smoke-v011-ffuf-approval")
commands = [json.loads(line) for line in (run / "commands.log").read_text().splitlines() if line.strip()]
findings = json.loads((run / "findings.json").read_text())

assert any(c.get("tool_name") == "plugin.ffuf-low-rate" and c.get("status") == "succeeded" for c in commands)
assert any(f.get("category") == "plugin/ffuf-low-rate" for f in findings)
assert (run / "artifacts/tool-results/ffuf-low-rate.jsonl").exists()
print("resume-approved smoke assertions passed")
PY
```

---

## 9. Plugin hardening smoke

P0 smoke 不需要真实执行所有外部工具；插件加固主路径由单元测试固定：

```bash
go test -count=1 ./internal/runtime/command ./internal/plugins ./internal/core/runtime ./internal/interfaces/cli
```

验收：

```text
timeout / output_limit / invalid_output / dependency_missing / policy_skipped / execution_failed 均有测试覆盖。
坏插件输出不会把 ToolCall 误记为 succeeded。
插件失败不会破坏整个 safe scan。
```

---

## 10. 清理

```bash
rm -rf runs/smoke-v011-simple-site
rm -rf runs/smoke-v011-no-scope
rm -rf runs/smoke-v011-ffuf-approval
```

如果使用了临时 fake binary：

```bash
rm -rf "$FAKE_BIN"
```

---

## 11. 完成判定

```text
[x] gofmt / go test / go vet / go build 通过
[x] baseline safe scan 成功
[x] metadata.json / effective_config.json / plugins.snapshot.json 断言通过
[x] no-scope 安全边界断言通过
[x] ffuf-low-rate approval pending 断言通过
[x] approvals approve 审计断言通过
[x] 可选 resume-approved 断言通过
[x] report / replay 可读
```

---

## 12. 本次验证记录

验证日期：2026-05-24  
验证状态：通过

已执行：

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./internal/runtime/command ./internal/plugins ./internal/core/runtime ./internal/interfaces/cli
go vet ./...
go build -ldflags "-X main.version=v0.1.1-smoke -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o bin/cyberagent ./cmd/cyberagent
./bin/cyberagent version
./bin/cyberagent --version
./bin/cyberagent doctor
./bin/cyberagent plugins list
./bin/cyberagent plugins doctor
./bin/cyberagent plugins doctor --json
```

已通过 smoke 场景：

```text
baseline safe scan: runs/smoke-v011-simple-site
metadata/effective_config/plugins.snapshot JSON assertions
no-scope rejection
ffuf-low-rate approval pending
approvals approve audit trail
fake ffuf resume-approved
report / replay readability
plugin hardening unit coverage
plugins doctor --json machine-readable health output
```
