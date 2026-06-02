# CyberAgent Quality Baseline and Regression Matrix

状态：Slice A 最小可提交成果
负责人：QA Codex
边界：质量文档、测试矩阵、smoke checklist；不修改业务逻辑

---

## 1. Scope and Non-goals

### Scope

本文件固化 CyberAgent 后续优化的质量基线与回归矩阵，覆盖：

- 最低质量门禁命令。
- 扩展风险验证命令。
- UI 毛玻璃重设计回归矩阵。
- Runtime / approval / recovery contract 测试清单。
- Smoke checklist 草案。
- 失败定位与验收口径。

### Non-goals

本文件不定义业务功能实现，不替代总优化方案，不修改 runtime、daemon、frontend 或 CLI 行为。

---

## 2. Minimum Quality Gates

最低质量门禁用于保护每个可提交切片，目标是让 Go、frontend、CLI build 的基础回归在进入扩展矩阵前先保持绿色。

### 2.1 Backend / Go gates

```bash
git status --short
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
go build \
  -ldflags "-X main.version=ci -X main.commit=$(git rev-parse --short HEAD)" \
  -o bin/cyberagent ./cmd/cyberagent
./bin/cyberagent version
./bin/cyberagent --version
```

Acceptance:

- `git status --short` contains no unexpected source changes.
- `gofmt` check prints no files.
- All Go packages pass tests.
- `go vet` reports no errors.
- CLI binary builds and version commands execute successfully.

### 2.2 Frontend gates

```bash
npm ci --prefix web/frontend
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
```

Acceptance:

- Frontend dependencies install from lockfile.
- Vitest suite passes.
- TypeScript and Vite build pass.
- Frontend build output remains compatible with dashboard packaging.

### 2.3 Release config gate

```bash
go run github.com/goreleaser/goreleaser/v2@v2.15.4 check
```

Acceptance:

- GoReleaser configuration is valid.
- Artifact configuration errors are caught before release smoke execution.

### 2.4 Failure triage

| Failure area | First triage target |
|---|---|
| gofmt | Formatting or generated-file drift |
| Go test | Backend logic, contract, persistence, concurrency |
| Go vet | Static correctness issue |
| Frontend test | Reducer, stream parser, normalizer, fixtures |
| Frontend build | TypeScript, Vite, asset, CSS import |
| CLI build | Command entrypoint, version injection, dependency compile |
| GoReleaser check | Release metadata, platform matrix, artifact config |

---

## 3. Extended Risk Verification

扩展风险验证用于覆盖高风险基础设施：runtime event bus、SQLite state、daemon router、approval bridge、frontend state reducer、browser smoke、visual regression。

### 3.1 Coverage and focused packages

```bash
go test -cover \
  ./internal/agent/runtime \
  ./internal/runtime/state/sqlite \
  ./internal/interfaces/daemon \
  ./internal/agent/tools \
  ./internal/agent/toolregistry
```

Acceptance:

- Runtime, SQLite state, and toolregistry coverage do not regress.
- Daemon coverage is treated as no-regression until narrower package boundaries exist.
- `internal/agent/tools` remains a focused low-coverage improvement target.

### 3.2 Race-sensitive packages

```bash
go test -race ./internal/agent/runtime ./internal/runtime/state/sqlite ./internal/interfaces/daemon
```

Risk coverage:

- Runtime publish / subscribe.
- SSE fan-out.
- SQLite reopen / recovery.
- Approval resolution event bridge.
- Agent session state access.

Acceptance:

- No data race is reported.
- Failures identify a package and test case.
- Slow subscriber handling does not block unrelated subscribers.

### 3.3 Flake candidates

```bash
go test -count=10 ./internal/agent/runtime ./internal/interfaces/daemon ./internal/runtime/state/sqlite
npm test --prefix web/frontend -- --run
```

Risk coverage:

- Runtime replay and recent event windows.
- Daemon stream forwarding.
- Approval decision route tests.
- Job polling and stream disconnect handling.
- Frontend reducer idempotency.

Acceptance:

- Repeated execution remains stable.
- Intermittent failure is isolated into a focused case before enforcing it as a gate.

### 3.4 Performance and backpressure candidates

Suggested benchmark shape:

```bash
go test -run '^$' -bench 'Runtime|Event|SQLite|Subscribe|Replay' ./internal/agent/runtime ./internal/runtime/state/sqlite
```

Risk coverage:

- High-volume runtime event write and replay.
- SQLite `from_seq`, recent window, run filter, and session filter.
- Multiple SSE subscribers.
- Slow subscriber behavior.
- Frontend reducer with long timelines and large tool results.

Acceptance:

- A comparable baseline is produced.
- Large event volume does not produce obvious memory growth or extreme latency.
- Slow consumers do not block global publish flow.

---

## 4. Glass UI Regression Matrix

毛玻璃 UI 重设计需要验证渐变背景、半透明卡片、blur fallback、层级深度、可读性、键盘可达性、响应式布局和浏览器兼容。

### 4.1 Visual state matrix

| Area | State | Acceptance |
|---|---|---|
| `/agent` Cockpit | Empty state | Gradient background, glass empty card, and primary action are readable |
| `/agent` Cockpit | Active session | Session list, composer, timeline, and inspector remain visually separated |
| Timeline | Model / tool / todo events | Event text remains readable over translucent surfaces |
| Tool Inspector | Long JSON / error result | Content does not overflow outside intended scroll area |
| Approval UI | Required / resolved / cancelled | Action, danger, and focus states are clear |
| Trace Drawer | Loading / data / error | Drawer z-index and backdrop keep content readable |
| Dashboard / Settings / History | Regular state | Glass tokens remain consistent outside Cockpit |

### 4.2 Theme matrix

| Theme | Acceptance |
|---|---|
| Light | Body text, muted text, borders, and badges remain readable over glass surfaces |
| Dark | Shadows, borders, and status colors remain visible |
| System | Theme switching does not leave stale styles or unreadable token combinations |
| Reduced motion | State feedback remains understandable with motion reduced |

### 4.3 Responsive matrix

| Viewport class | Acceptance |
|---|---|
| Mobile narrow | No horizontal overflow; composer remains usable; drawers can close |
| Tablet | Session list, main workspace, and inspector collapse or stack cleanly |
| Desktop | Multi-column layout keeps visual hierarchy and reading comfort |
| Large screen | Content width remains controlled; gradient background does not look sparse |

### 4.4 Browser matrix

| Browser | Acceptance |
|---|---|
| Chromium | Primary interaction and screenshot baseline |
| WebKit | `backdrop-filter`, blur, fixed layers, and sticky areas render acceptably |
| Firefox | Blur fallback, translucent layers, focus rings, and scrolling remain usable |

### 4.5 Accessibility and CSS risks

Checks:

- Text contrast on translucent surfaces.
- Non-text contrast for borders, badges, and focus rings.
- Keyboard focus visibility for composer, inspector tabs, approval actions, drawer controls.
- `prefers-reduced-motion` behavior.
- Fallback background when `backdrop-filter` is unsupported.
- Scroll performance for long timeline and large tool output.

Acceptance:

- No severe accessibility violation in core Cockpit flows.
- Key text meets WCAG AA contrast expectations.
- Focus ring is visible on all core controls.
- Unsupported blur degrades to readable solid or semi-solid surfaces.
- Screenshot differences are reviewable and attributable to token, component, or layout changes.

---

## 5. Runtime / Approval / Recovery Contract Test Checklist

### 5.1 Runtime event catalog fixtures

Recommended fixture layout:

```text
testdata/runtime-events/
  v1-basic-turn.jsonl
  v1-tool-call.jsonl
  v1-approval-flow.jsonl
  v1-recovery-flow.jsonl
  v1-memory-compaction.jsonl
  v1-sse-replay.jsonl
```

Required envelope fields:

- `seq`
- `time`
- `type`
- `session_id`
- `task_id`
- `run_id`
- `turn_id`
- `tool_call_id`
- `data`

Required event types:

- `task_created`
- `run_started`
- `turn_started`
- `model_token`
- `model_reasoning`
- `tool_started`
- `tool_progress`
- `tool_finished`
- `todo_updated`
- `approval_required`
- `approval_resolved`
- `task_finished`
- `task_failed`
- `memory_compacted`
- `heartbeat`

Backend acceptance:

- Runtime marshaling matches fixtures.
- SQLite replay preserves sequence ordering.
- `FromSeq` returns only incremental events.
- Run, task, and session filters do not leak unrelated events.

Frontend acceptance:

- Normalizer consumes the same fixtures.
- Reducer derives expected task, turn, tool, approval, and memory state.
- Duplicate replay does not duplicate rendered state.
- Missing optional fields degrade safely.

### 5.2 Approval contract

Test cases:

- approve publishes `approval_resolved`.
- deny publishes `approval_resolved`.
- edit-args publishes `approval_resolved` and preserves `edit_args`.
- cancel publishes `approval_resolved` and `task_failed`.
- Runtime engine absent keeps HTTP behavior successful and event bridge as no-op.
- Missing session mapping keeps at least `run_id` traceability and avoids panic.

Required data:

- Envelope: `session_id`, `task_id`, `run_id`.
- Data: `approval_id`, `step_id`, `run_id`, `decision`, `status`, `requested_action`, `resolved_by`, `comment`, `edit_args` when present.
- Cancel data: `reason=approval_cancelled`.

Acceptance:

- Approval HTTP response and runtime event data agree.
- Cancel terminal semantics are visible to both backend replay and frontend reducer.
- Event bridge absence does not break approval APIs.

### 5.3 Recovery contract

Test cases:

- Engine mode startup scans interrupted runs.
- Running run does not remain indefinitely active after recovery.
- Recovery result is persisted as runtime event.
- SQLite reopen seeds later event sequence correctly.
- Recovery failure closes runtime store and returns error.
- Repeated recovery remains idempotent.

Acceptance:

- No orphan running run remains after recovery logic executes.
- SSE replay can observe recovery outcome.
- API run state and runtime store state agree.
- Repeated recovery does not create misleading duplicate terminal events.

### 5.4 Fault injection contract

| Fault | Acceptance |
|---|---|
| SQLite closed | Error returned without panic |
| SQLite write failure | No silent half-state success |
| Slow subscriber | Other subscribers still receive events |
| Stream disconnect | Resources release and reconnect can resume from `from_seq` |
| Duplicate replay | Frontend deduplicates by sequence or stable fallback id |
| Out-of-order event | Frontend remains stable and preserves inspectable timeline |
| Missing session mapping | Event remains traceable by run or task identifiers |

---

## 6. Smoke Checklist

### 6.1 Baseline smoke

```bash
git status --short
test -z "$(gofmt -l cmd internal)"
go test -count=1 ./...
go vet ./...
npm ci --prefix web/frontend
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
go build -o bin/cyberagent ./cmd/cyberagent
```

Acceptance:

- All commands pass.
- No unexpected source changes remain.
- Frontend and Go builds are reproducible.

### 6.2 CLI smoke

```bash
./bin/cyberagent version
./bin/cyberagent --version
./bin/cyberagent doctor
./bin/cyberagent plugins list
./bin/cyberagent plugins doctor
./bin/cyberagent plugins doctor --json
```

Acceptance:

- Commands execute successfully.
- Missing optional external tools degrade with clear messages.
- JSON output parses successfully where JSON is requested.

### 6.3 Safe scan / report / replay smoke

Command shape:

```bash
python3 -m http.server 18080 --directory testdata/simple-site

./bin/cyberagent scan web \
  --target http://127.0.0.1:18080 \
  --scope 127.0.0.1 \
  --profile safe \
  --out runs/smoke-simple-site \
  --no-external-tools

./bin/cyberagent report runs/smoke-simple-site
./bin/cyberagent replay runs/smoke-simple-site
```

Acceptance:

- Run workspace contains metadata, effective config, plugin snapshot, events, commands, artifacts, and reports.
- Events include terminal job event.
- Report and replay output are readable.
- Safe profile does not execute unauthorized high-risk external tooling.

### 6.4 Daemon / dashboard / Cockpit smoke

Command shape:

```bash
npm run build --prefix web/frontend
go build -o .tmp/cyberagent-dev ./cmd/cyberagent

.tmp/cyberagent-dev daemon --listen 127.0.0.1:8787
.tmp/cyberagent-dev dashboard --addr 127.0.0.1:8080 --daemon-url http://127.0.0.1:8787
```

Browser path:

```text
http://127.0.0.1:8080/agent
```

Acceptance:

- Page loads latest built assets.
- Daemon online status is visible.
- Session list, composer, timeline, and inspector are visible.
- Stream connects successfully.
- Reconnect uses `from_seq` and avoids duplicate key events.
- Approval UI can display required, resolved, and cancelled states.

### 6.5 Glass UI smoke

Screenshot paths:

- `/agent` empty state.
- `/agent` active session.
- Timeline with tool inspector.
- Approval required state.
- Error or failed state.
- Settings and dashboard regular pages.

Acceptance:

- Screenshot artifacts are reviewable.
- Light, dark, and system themes remain readable.
- Mobile, tablet, and desktop layouts avoid horizontal overflow.
- No severe accessibility violation is present in core paths.
- Blur fallback remains readable.
- Core controls have visible focus state.

---

## 7. Dependencies and Ownership

| Matrix | Depends on | Suggested owner |
|---|---|---|
| Minimum gates | Existing Go and frontend toolchains | QA / CI owner |
| Runtime contract fixtures | Slice C runtime event catalog | Backend + QA |
| Frontend fixture consumption | Shared runtime fixtures | Frontend + QA |
| Glass visual regression | Slice B glass tokens and primitives | Frontend + QA |
| Browser smoke | Stable daemon/dashboard harness | Frontend + QA |
| Approval/recovery contracts | Event bridge and runtime store behavior | Backend + QA |

---

## 8. Acceptance Summary

Slice A is acceptable when:

- Minimum quality gate commands are documented and copyable.
- Extended risk commands are documented with clear scope and acceptance.
- Glass UI regression matrix covers state, theme, viewport, browser, accessibility, and fallback behavior.
- Runtime / approval / recovery contract checklist identifies fixtures, event fields, event types, and failure cases.
- Smoke checklist covers baseline, CLI, safe scan, daemon/dashboard/Cockpit, and glass UI paths.
- The document contains no implementation changes and does not modify business behavior.
