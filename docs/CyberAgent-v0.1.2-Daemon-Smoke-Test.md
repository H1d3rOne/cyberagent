# CyberAgent v0.1.2 Daemon Smoke Test

版本：v0.1.2
日期：2026-05-24
状态：smoke test 草案完成；daemon skeleton 可执行基础 health/version/auth 子集；完整 P0 待实现后验证

---

## 1. 目标

本 smoke test 用于 v0.1.2 发布前快速验证 Local Daemon / Web API Foundation 的 P0 主链路。

覆盖范围：

```text
build / version / doctor 基础回归
daemon 本地启动
token auth 默认启用
health / version / doctor / plugins API
run list / run detail / report / artifacts / events API
scan web async job API
job status / cancel MVP
approval list / approve / deny API
resume-approved API
scope required 回归
path traversal 防护回归
默认 safe profile 与 no external tools 边界
```

注意：

```text
不要在 smoke test 中 push tag。
不要在 smoke test 中创建 GitHub Release。
daemon 默认只监听 127.0.0.1。
daemon 默认必须启用 token auth。
不要默认启用外部扫描插件。
ffuf-low-rate 仅在 approval 专项用例中显式启用。
```

---

## 2. 前置条件

```text
Go toolchain 可用。
Python 3 可用，用于启动 testdata/simple-site 静态 fixture。
当前工作目录为仓库根目录。
v0.1.2 daemon/API P0 已实现。
```

fixture：

```text
testdata/simple-site/index.html
testdata/simple-site/app.js
testdata/simple-site/robots.txt
testdata/simple-site/sitemap.xml
```

推荐使用独立临时目录：

```text
.tmp/smoke-v012/
runs/smoke-v012-*/
```

---

## 3. 环境变量

```bash
export CYBERAGENT_SMOKE_ROOT=".tmp/smoke-v012"
export CYBERAGENT_TOKEN_FILE="$CYBERAGENT_SMOKE_ROOT/token"
export CYBERAGENT_DAEMON_LOG="$CYBERAGENT_SMOKE_ROOT/daemon.log"
export CYBERAGENT_DAEMON_PID="$CYBERAGENT_SMOKE_ROOT/daemon.pid"
export CYBERAGENT_BASE_URL="http://127.0.0.1:18787"
export CYBERAGENT_TARGET_URL="http://127.0.0.1:18080"
export CYBERAGENT_SCOPE="127.0.0.1"
```

准备目录：

```bash
rm -rf "$CYBERAGENT_SMOKE_ROOT"
mkdir -p "$CYBERAGENT_SMOKE_ROOT"
```

---

## 4. 构建与基础回归

```bash
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build \
  -ldflags "-X main.version=v0.1.2-smoke -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
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
version / --version 可输出。
doctor 可运行。
plugins doctor --json 可输出机器可读 JSON。
缺失可选外部工具依赖时应报告为不可用，而不是崩溃。
```

---

## 5. 启动本地授权目标

在单独终端启动 fixture：

```bash
python3 -m http.server 18080 --directory testdata/simple-site
```

快速检查：

```bash
curl -sS "$CYBERAGENT_TARGET_URL/" | head
```

验收：

```text
本地目标可访问。
后续 scan 使用 --scope / scope = 127.0.0.1。
```

---

## 6. 启动 daemon

P0 推荐启动命令：

```bash
./bin/cyberagent daemon \
  --listen 127.0.0.1:18787 \
  --runs-dir runs \
  --token-file "$CYBERAGENT_TOKEN_FILE" \
  >"$CYBERAGENT_DAEMON_LOG" 2>&1 &

echo $! > "$CYBERAGENT_DAEMON_PID"
```

等待 daemon ready：

```bash
for i in $(seq 1 50); do
  if test -s "$CYBERAGENT_TOKEN_FILE"; then
    break
  fi
  sleep 0.1
done

export CYBERAGENT_TOKEN="$(cat "$CYBERAGENT_TOKEN_FILE")"
export CYBERAGENT_AUTH_HEADER="Authorization: Bearer $CYBERAGENT_TOKEN"
```

验收：

```text
daemon 进程保持运行。
token 文件存在且非空。
daemon 默认/显式监听 127.0.0.1。
```

可选权限检查：

```bash
ls -l "$CYBERAGENT_TOKEN_FILE"
```

期望：

```text
token 文件权限建议为 0600。
如果平台不支持严格权限，应至少给出明确 warning，不应泄露 token。
```

---

## 7. Auth 负向测试

无 token：

```bash
curl -sS -i "$CYBERAGENT_BASE_URL/api/v1/version"
```

错误 token：

```bash
curl -sS -i \
  -H "Authorization: Bearer wrong-token" \
  "$CYBERAGENT_BASE_URL/api/v1/version"
```

断言方向：

```text
HTTP status 为 401。
response.ok = false。
response.error.code = unauthorized。
daemon access log 不应打印 Authorization token。
```

可用 Python 断言：

```bash
python3 - <<'PY'
import os
from pathlib import Path
log = Path(os.environ["CYBERAGENT_DAEMON_LOG"])
if log.exists():
    text = log.read_text(errors="ignore")
    assert "wrong-token" not in text
print("auth log assertion passed")
PY
```

---

## 8. Health / Version / Doctor / Plugins API

Health：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/healthz" \
  | python3 -m json.tool
```

Version：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/version" \
  | python3 -m json.tool
```

Doctor：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/doctor" \
  | python3 -m json.tool
```

Plugins：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/plugins" \
  | python3 -m json.tool

curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/plugins/doctor" \
  | python3 -m json.tool
```

断言：

```bash
python3 - <<'PY'
import json
import os
import subprocess

base = os.environ["CYBERAGENT_BASE_URL"]
auth = os.environ["CYBERAGENT_AUTH_HEADER"]

def get(path):
    out = subprocess.check_output(["curl", "-sS", "-H", auth, base + path])
    data = json.loads(out)
    assert data["ok"] is True
    assert data["error"] is None
    return data["data"]

health = get("/api/v1/healthz")
version = get("/api/v1/version")
doctor = get("/api/v1/doctor")
plugins = get("/api/v1/plugins")
plugin_doctor = get("/api/v1/plugins/doctor")

assert health["status"] == "ok"
assert version["version"]
assert "plugins" in plugins
assert "ok" in doctor
assert "ok" in plugin_doctor
print("basic daemon API assertions passed")
PY
```

---

## 9. Scan Web Async Job

启动 safe scan：

```bash
export CYBERAGENT_SCAN_RESPONSE_FILE="$CYBERAGENT_SMOKE_ROOT/scan-response.json"

curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d "{\"target\":\"$CYBERAGENT_TARGET_URL\",\"scope\":\"$CYBERAGENT_SCOPE\",\"profile\":\"safe\",\"enabled_plugins\":[],\"no_external_tools\":true}" \
  "$CYBERAGENT_BASE_URL/api/v1/scans/web" \
  > "$CYBERAGENT_SCAN_RESPONSE_FILE"

python3 -m json.tool "$CYBERAGENT_SCAN_RESPONSE_FILE"
```

提取 job/run：

```bash
export CYBERAGENT_JOB_ID="$(
  python3 - <<'PY'
import json
import os
from pathlib import Path
data = json.loads(Path(os.environ["CYBERAGENT_SCAN_RESPONSE_FILE"]).read_text())
assert data["ok"] is True
print(data["data"]["job_id"])
PY
)"

export CYBERAGENT_RUN_ID="$(
  python3 - <<'PY'
import json
import os
from pathlib import Path
data = json.loads(Path(os.environ["CYBERAGENT_SCAN_RESPONSE_FILE"]).read_text())
assert data["ok"] is True
print(data["data"]["run_id"])
PY
)"
```

验收：

```text
response.ok = true。
data.job_id 非空。
data.run_id 非空。
scan API 立即返回，不等待完整扫描阻塞 HTTP handler。
```

---

## 10. Poll Job Status

轮询 job：

```bash
for i in $(seq 1 100); do
  JOB_RESPONSE="$(
    curl -sS \
      -H "$CYBERAGENT_AUTH_HEADER" \
      "$CYBERAGENT_BASE_URL/api/v1/jobs/$CYBERAGENT_JOB_ID"
  )"
  echo "$JOB_RESPONSE" > "$CYBERAGENT_SMOKE_ROOT/job-response.json"
  STATUS="$(python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path(".tmp/smoke-v012/job-response.json").read_text())
assert data["ok"] is True
print(data["data"]["status"])
PY
)"
  echo "job status: $STATUS"
  if test "$STATUS" = "succeeded" || test "$STATUS" = "failed" || test "$STATUS" = "cancelled"; then
    break
  fi
  sleep 0.2
done
```

断言：

```bash
python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path(".tmp/smoke-v012/job-response.json").read_text())
assert data["ok"] is True
job = data["data"]
assert job["status"] == "succeeded", job
assert job["run_id"]
assert job["run_dir"]
print("job succeeded assertion passed")
PY
```

---

## 11. Run / Report / Artifacts / Events API

Run list：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs?limit=50" \
  | python3 -m json.tool
```

Run detail：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs/$CYBERAGENT_RUN_ID" \
  | python3 -m json.tool
```

Report JSON：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs/$CYBERAGENT_RUN_ID/report?format=json" \
  > "$CYBERAGENT_SMOKE_ROOT/report-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/report-response.json"
```

Report Markdown：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs/$CYBERAGENT_RUN_ID/report?format=md" \
  > "$CYBERAGENT_SMOKE_ROOT/report-md-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/report-md-response.json"
```

Artifacts：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs/$CYBERAGENT_RUN_ID/artifacts" \
  > "$CYBERAGENT_SMOKE_ROOT/artifacts-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/artifacts-response.json"
```

Events：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs/$CYBERAGENT_RUN_ID/events?limit=100" \
  > "$CYBERAGENT_SMOKE_ROOT/events-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/events-response.json"
```

断言：

```bash
python3 - <<'PY'
import json
from pathlib import Path

root = Path(".tmp/smoke-v012")
report = json.loads((root / "report-response.json").read_text())
report_md = json.loads((root / "report-md-response.json").read_text())
artifacts = json.loads((root / "artifacts-response.json").read_text())
events = json.loads((root / "events-response.json").read_text())

assert report["ok"] is True
assert report["data"]["format"] == "json"
assert "report" in report["data"]

assert report_md["ok"] is True
assert report_md["data"]["format"] == "md"
assert "content" in report_md["data"]

assert artifacts["ok"] is True
assert artifacts["data"]["count"] == len(artifacts["data"]["artifacts"])

assert events["ok"] is True
assert isinstance(events["data"]["items"], list)
assert "limit" in events["data"]
assert "eof" in events["data"]
print("run/report/artifacts/events assertions passed")
PY
```

---

## 12. Scope Required 负向测试

缺少 scope 的主动扫描必须失败：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d "{\"target\":\"$CYBERAGENT_TARGET_URL\",\"profile\":\"safe\"}" \
  "$CYBERAGENT_BASE_URL/api/v1/scans/web" \
  > "$CYBERAGENT_SMOKE_ROOT/no-scope-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/no-scope-response.json"
```

断言：

```bash
python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path(".tmp/smoke-v012/no-scope-response.json").read_text())
assert data["ok"] is False
assert data["error"]["code"] == "scope_required"
print("scope required assertion passed")
PY
```

---

## 13. Path Traversal 负向测试

非法 run id 必须拒绝：

```bash
curl -sS --path-as-is \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs/../report" \
  > "$CYBERAGENT_SMOKE_ROOT/path-traversal-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/path-traversal-response.json" || true
```

更稳定的 URL encoded 测试：

```bash
curl -sS --path-as-is \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs/%2e%2e%2fsecret" \
  > "$CYBERAGENT_SMOKE_ROOT/path-traversal-encoded-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/path-traversal-encoded-response.json" || true
```

断言方向：

```text
HTTP status 不应为 200。
response.ok = false。
error.code 应为 bad_request / policy_denied / not_found 之一。
daemon 不应读取 runs-dir 外文件。
```

---

## 14. Approval / Resume Smoke

该用例用于验证 API approval 与 CLI approval 状态互通。

创建需要审批的 run：

```bash
APPROVAL_SCAN_RESPONSE="$(
  curl -sS \
    -H "$CYBERAGENT_AUTH_HEADER" \
    -H "Content-Type: application/json" \
    -d "{\"target\":\"$CYBERAGENT_TARGET_URL\",\"scope\":\"$CYBERAGENT_SCOPE\",\"profile\":\"safe\",\"enabled_plugins\":[\"ffuf-low-rate\"],\"no_external_tools\":false}" \
    "$CYBERAGENT_BASE_URL/api/v1/scans/web"
)"

echo "$APPROVAL_SCAN_RESPONSE" > "$CYBERAGENT_SMOKE_ROOT/approval-scan-response.json"
python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/approval-scan-response.json"
```

轮询 approval scan job：

```bash
export CYBERAGENT_APPROVAL_JOB_ID="$(
  python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path(".tmp/smoke-v012/approval-scan-response.json").read_text())
assert data["ok"] is True
print(data["data"]["job_id"])
PY
)"

export CYBERAGENT_APPROVAL_RUN_ID="$(
  python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path(".tmp/smoke-v012/approval-scan-response.json").read_text())
assert data["ok"] is True
print(data["data"]["run_id"])
PY
)"

for i in $(seq 1 100); do
  curl -sS \
    -H "$CYBERAGENT_AUTH_HEADER" \
    "$CYBERAGENT_BASE_URL/api/v1/jobs/$CYBERAGENT_APPROVAL_JOB_ID" \
    > "$CYBERAGENT_SMOKE_ROOT/approval-job-response.json"
  STATUS="$(python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path(".tmp/smoke-v012/approval-job-response.json").read_text())
assert data["ok"] is True
print(data["data"]["status"])
PY
)"
  echo "approval job status: $STATUS"
  if test "$STATUS" = "succeeded" || test "$STATUS" = "failed" || test "$STATUS" = "cancelled"; then
    break
  fi
  sleep 0.2
done
```

读取 approvals：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  "$CYBERAGENT_BASE_URL/api/v1/runs/$CYBERAGENT_APPROVAL_RUN_ID/approvals" \
  > "$CYBERAGENT_SMOKE_ROOT/approvals-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/approvals-response.json"
```

提取 pending approval：

```bash
export CYBERAGENT_APPROVAL_ID="$(
  python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path(".tmp/smoke-v012/approvals-response.json").read_text())
assert data["ok"] is True
approvals = data["data"]["approvals"]
pending = [a for a in approvals if a.get("status") == "pending"]
assert pending, approvals
print(pending[0]["id"])
PY
)"
```

Approve：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d '{"operator":"smoke-test","comment":"approved for v0.1.2 daemon smoke"}' \
  "$CYBERAGENT_BASE_URL/api/v1/runs/$CYBERAGENT_APPROVAL_RUN_ID/approvals/$CYBERAGENT_APPROVAL_ID/approve" \
  > "$CYBERAGENT_SMOKE_ROOT/approve-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/approve-response.json"
```

CLI 互通检查：

```bash
APPROVAL_RUN_DIR="$(
  python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path(".tmp/smoke-v012/approval-scan-response.json").read_text())
assert data["ok"] is True
print(data["data"]["run_dir"])
PY
)"

./bin/cyberagent approvals list "$APPROVAL_RUN_DIR"
```

Resume approved：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d '{"operator":"smoke-test"}' \
  "$CYBERAGENT_BASE_URL/api/v1/runs/$CYBERAGENT_APPROVAL_RUN_ID/resume-approved" \
  > "$CYBERAGENT_SMOKE_ROOT/resume-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/resume-response.json"
```

验收：

```text
API approvals list 能看到 pending approval。
API approve 后 CLI approvals list 能看到 approved 状态。
resume-approved 返回 job_id 或明确结构化结果。
若 ffuf 依赖缺失，应记录 dependency_missing / skipped，不应破坏整个 run。
Scope Guard 仍在 resume 前执行。
```

---

## 15. Job Cancel Smoke

P0 cancel 只保证对 daemon 当前进程启动且仍 running 的 job 生效。

如果 simple-site scan 太快，cancel 用例可以使用测试 fixture 或后续专用慢插件。

建议流程：

```text
1. POST /api/v1/scans/web 创建一个可持续 running 的 job。
2. 立即 POST /api/v1/jobs/{job_id}/cancel。
3. GET /api/v1/jobs/{job_id}。
4. 断言 status = cancelled，或若 job 已完成则返回 conflict / final status。
```

最低验收：

```text
cancel endpoint 存在。
对不存在 job 返回 not_found。
对已完成 job 返回 conflict 或当前最终状态，不 panic。
```

不存在 job：

```bash
curl -sS \
  -H "$CYBERAGENT_AUTH_HEADER" \
  -X POST \
  "$CYBERAGENT_BASE_URL/api/v1/jobs/job_not_found/cancel" \
  > "$CYBERAGENT_SMOKE_ROOT/cancel-not-found-response.json"

python3 -m json.tool "$CYBERAGENT_SMOKE_ROOT/cancel-not-found-response.json"
```

---

## 16. 清理 daemon

```bash
if test -f "$CYBERAGENT_DAEMON_PID"; then
  kill "$(cat "$CYBERAGENT_DAEMON_PID")" || true
  rm -f "$CYBERAGENT_DAEMON_PID"
fi
```

确认 daemon 已退出：

```bash
curl -sS "$CYBERAGENT_BASE_URL/api/v1/healthz" || true
```

---

## 17. 完成定义

v0.1.2 daemon smoke test 通过需要满足：

```text
[ ] gofmt/go test/go vet/go build 通过
[ ] 本地 fixture 可访问
[ ] daemon 可启动并监听 127.0.0.1
[ ] token 文件生成且非空
[ ] 无 token / 错误 token 被拒绝
[ ] Authorization token 不写入 daemon log
[ ] health/version/doctor/plugins API 返回 ok envelope
[ ] scan web API 可异步启动 safe scan
[ ] job status 可从 running 进入 succeeded
[ ] run detail 可读取
[ ] report json/md 可读取
[ ] artifacts index 可读取
[ ] events cursor polling 可读取
[ ] 缺少 scope 的 scan 被 scope_required 拒绝
[ ] path traversal 请求被拒绝
[ ] approvals list/approve 与 CLI 状态互通
[ ] resume-approved 复用 CLI replay 语义
[ ] cancel endpoint 对不存在/已完成/running job 给出结构化响应
```

---

## 18. 与 v0.1.2 发布的关系

v0.1.2 RC 前应执行：

```text
docs/CyberAgent-v0.1.2-Daemon-Smoke-Test.md 中的基础 daemon smoke。
docs/api/v0.1.2-api-contract.md 与实际响应字段对齐。
docs/CyberAgent-Implementation-Status.md 记录最终验证命令和结果。
```

如果 smoke test 中某个 P0 endpoint 暂不实现：

```text
必须在 v0.1.2 release notes 中明确标注为 deferred。
不得在 API contract 中伪装为已完成。
```
