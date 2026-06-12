# CyberAgent Desktop / Wails 技术 Spike

日期：2026-06-09
状态：L1 技术 spike / 不引入 Wails 依赖

## 1. 目标

本 spike 只验证 Desktop/Wails 可以复用当前 Local Daemon / Agent Cockpit，而不是立即交付完整桌面应用。

成功标准：

- Desktop shell 能以内嵌方式启动 daemon。
- daemon 只绑定 loopback ephemeral port。
- shell 能拿到可注入 WebView 的 API URL 与 bearer token。
- safe-to-log runtime contract 不泄露 bearer token。
- 权限、日志、更新边界在引入 Wails 前被固定下来。

## 2. 当前落点

代码落点：

- `internal/interfaces/daemon/server.go`
  - `Server.Serve(ctx, net.Listener)` 支持 caller-owned listener。
  - `ListenAndServe` 复用 `Serve`，CLI daemon 行为保持不变。
- `internal/interfaces/desktop/bootstrap.go`
  - `StartEmbeddedDaemon(ctx, BootstrapConfig)` 在 `127.0.0.1:0` 上启动 daemon。
  - `RuntimeContract` 是 safe-to-log 合约，不包含 bearer token。
  - `WebViewBootstrap` 是敏感 WebView 注入 payload，包含 token，不应写日志。
  - `PermissionModel` 固定 Desktop shell 的权限边界。

## 3. Bootstrap 流程

```text
Wails main.go / Desktop shell
        |
        | StartEmbeddedDaemon(ctx, BootstrapConfig)
        v
net.Listen("tcp", "127.0.0.1:0")
        |
        | daemon.NewServer + Server.Serve(ctx, listener)
        v
Local Daemon API / Agent Cockpit assets
        |
        | RuntimeContract + WebViewBootstrap
        v
WebView loads existing frontend and calls /api/v1/* with Bearer token
```

建议 Wails shell 行为：

1. 进程启动时调用 `desktop.StartEmbeddedDaemon`。
2. 将 `RuntimeContract` 写入 debug/status UI 或诊断信息。
3. 只通过内存通道把 `WebViewBootstrap` 注入 WebView，不写日志、不落盘。
4. WebView 侧设置 API base URL 与 `Authorization: Bearer <token>`。
5. 应用退出时调用 `EmbeddedDaemon.Stop(ctx)`，等待 daemon shutdown。

## 4. Runtime Contract

`RuntimeContract` 可安全记录：

```json
{
  "api_base_url": "http://127.0.0.1:<ephemeral>",
  "auth_mode": "bearer_token_file",
  "token_file": ".../cyberagent/token",
  "runs_dir": "runs",
  "log_dir": ".../cyberagent/logs",
  "permissions": {
    "network_binding": "loopback_ephemeral_only",
    "token_exposure": "inject_to_local_webview_only_never_log",
    "filesystem_scope": "configured_runs_token_log_dirs_only",
    "shell_execution": "reuse_daemon_policy_gates_no_desktop_bypass",
    "updates": "desktop_shell_owned_explicit_user_action",
    "external_navigation": "open_external_browser_with_confirmed_url"
  }
}
```

`WebViewBootstrap` 是敏感对象：

```json
{
  "api_base_url": "http://127.0.0.1:<ephemeral>",
  "authorization_type": "Bearer",
  "authorization_token": "<secret>"
}
```

## 5. 权限模型

- 网络：只允许 loopback；`0.0.0.0`、公网 IP 或 LAN IP 都应拒绝。
- Token：daemon token 仍由 Local Daemon token file 管理；desktop shell 只负责把 token 注入本地 WebView。
- 文件系统：默认只暴露 configured `runs_dir`、`token_file`、`log_dir`。
- Shell/工具执行：不新增 Desktop bypass；继续复用 daemon / agent runtime 的 policy gates、approval 与 operation level。
- 外链：从 WebView 打开外部 URL 时应交给系统浏览器，并在 shell 层保留可审计 URL。

## 6. 日志与更新模型

日志：

- `RuntimeContract.LogDir` 先固定 Desktop shell 的日志目录。
- access log / structured log 属于后续 Local API / Daemon 稳定版范围。
- 禁止记录 `WebViewBootstrap.AuthorizationToken`。

更新：

- 更新动作由 Desktop shell 触发，不通过 daemon HTTP API 静默自更新。
- 初期只支持显式用户动作，例如打开 release page、下载说明或后续 signed updater。
- daemon runtime / token / runs 数据迁移仍归 Local API / Daemon 稳定版处理。

## 7. 非目标

本 spike 不做：

- 完整 Wails app scaffold。
- system tray / auto-start。
- signed updater。
- 多用户 Web 平台。
- Remote Runner。
- 长期 Memory。

## 8. 验证

已覆盖：

```bash
gofmt -w internal/interfaces/daemon/server.go internal/interfaces/daemon/server_test.go internal/interfaces/desktop/bootstrap.go internal/interfaces/desktop/bootstrap_test.go
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon -run 'TestServerServeSupportsEmbeddedEphemeralLoopbackListener|TestNewServerCreatesTokenFileAndNormalizesDefaults'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/desktop
git diff --check
```

验证点：

- daemon 嵌入 listener 可工作。
- Desktop bootstrap 可启动 daemon 并请求 `/api/v1/healthz`。
- `RuntimeContract` JSON 不泄露 bearer token。
- 非 loopback listen host 被拒绝。
