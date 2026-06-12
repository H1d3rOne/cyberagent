# CyberAgent Web 多用户平台架构切片

日期：2026-06-09
状态：L3-A1 architecture/design slice、L3-A2 no-op local skeleton、L3-A3 router middleware、L3-A4 service entrypoint context read、L3-A5 storage resolver gate、L3-A6 frontend session/workspace UX skeleton、L3-A7a audit/admin/rate-limit/hardening config skeleton、L3-A7b audit JSONL sink skeleton、L3-A7c admin bootstrap readiness skeleton、L3-A7d provider/web-search/API auth secret status-only/server-side storage skeleton、L3-A7e rate-limit/deployment hardening runtime gates、L3-A8 identity/tenant/workspace API skeleton、L3-A9 frontend server-side scope hydrate skeleton、L3-A10 auth/session/RBAC model skeleton、L3-A11 server-side session store skeleton、L3-A12 session cookie resolver middleware skeleton、L3-A13 Web login/logout/refresh endpoint skeleton、L3-A14 RBAC middleware/enforcer skeleton、L3-A15 membership store skeleton、L3-A16 RBAC policy loading/evaluation skeleton、L3-A17 RBAC deny/audit enforcement gate skeleton、L3-A18 Web session principal/tenant resolver skeleton、L3-A19 membership admin API skeleton 已完成；不改变当前 Local Daemon / Desktop 默认行为

## 1. 目标

L3 的目标是把当前“本地单用户 Agent Cockpit + Local Daemon”演进为可部署的 Web 多用户平台，同时继续保留本地 daemon、dashboard 与 Desktop/Wails bootstrap 的简单路径。

本切片只定义边界与迁移顺序，不直接改认证、路由或数据布局。

成功标准：

- 明确 AuthN/AuthZ、HTTP session、tenant、workspace、API、storage 与 audit 边界。
- 给出最小迁移路径，确保每一步可小范围测试、可回滚。
- 明确本地模式兼容要求：现有 bearer token、`/api/v1/*`、runs dir、runtime DB、Desktop bootstrap 不发生行为变化。
- 明确非目标：不把本地 daemon token 直接提升为互联网多用户认证方案，不用 UI-only tenant switch 伪隔离。

## 2. 当前单用户现状

当前代码面以 Local Daemon 为中心：

- `internal/interfaces/daemon/router.go`
  - `newRouter(svc, token)` 使用同一个 bearer token 保护 `/api/`。
  - 前端路由不经 auth；API 路由通过 `bearerAuth(token, mux)`。
  - `routerServices` 是单租户 service bundle，包含 version、plugins、runs、approvals、jobs、agent、scanManager、schedules、providers。
- `internal/interfaces/daemon/auth.go`
  - `validBearerToken` 对 `Authorization: Bearer <token>` 做常量时间比较。
  - token 只是本地 daemon secret，不表达 user、role、tenant 或 workspace。
- `internal/interfaces/daemon/web_session_middleware.go`
  - L3-A12 后提供 Web session cookie resolver skeleton；默认 disabled/no-op，显式启用也只把 active server-side session 写入 request context，不替换 bearer auth。
- `internal/interfaces/daemon/web_auth_handlers.go`
  - L3-A13 后提供 login/logout/session refresh endpoint skeleton；默认 closed，显式启用也只返回 unsupported 边界，不创建 session/cookie。
- `internal/interfaces/daemon/rbac_middleware.go`
  - L3-A14 后提供 route/action/scope/role requirement table 与 request-scoped RBAC decision；默认 disabled/no-op，显式启用也不执行 deny。
  - L3-A16 后接入 policy evaluator skeleton：可读取内置/显式 policy requirement，并用 membership store 计算 allowed/missing-role。
  - L3-A17 后新增显式 deny/audit gate：默认关闭；仅 `rbac.enforcement_enabled=true` 时对非 local bearer principal 执行 deny，local bearer 继续豁免。
  - L3-A18 后新增显式 Web session principal resolver gate：默认关闭；仅 `web_sessions.principal_resolver_enabled=true` 且已有 active server-side session context 时，才把 Web session 映射为 `TenantContext`。
- `internal/interfaces/daemon/rbac_policy.go`
  - L3-A16 后提供 YAML policy loading skeleton；显式 `policy_path` 仅在 `rbac.enabled=true` 时加载，角色会做 known-role 与 tenant/workspace scope validation。
- `internal/core/tenant/membership_store.go`
  - L3-A15 后提供 tenant/workspace membership persistence boundary；默认 disabled/no-op，in-memory store 已在 L3-A16 RBAC evaluator skeleton 中作为显式启用时的 lookup boundary。
- `internal/interfaces/daemon/server.go`
  - `NewServer` 用单个 `cfg.RunsDir` 初始化 RunService、ApprovalService、JobManager、ScheduleManager 与 AgentManager。
  - runtime SQLite 默认落在 `runsDir/.agent/runtime/runtime.db`。
  - provider store 默认是 per-user JSON：`defaultProviderStorePath()`。
- `internal/interfaces/cli/dashboard.go`
  - bundled dashboard 暴露 loopback-only `/api/local/daemon-config`，把本地 daemon URL/token 注入浏览器。
- `web/frontend/src/lib/api.ts` 与 `web/frontend/src/lib/console.tsx`
  - Console settings 使用 `localStorage` 保存 base URL、daemon token、provider/API auth metadata 与 configured 状态；AI provider、web-search、API auth raw secret 只作为内存 draft，保存到 daemon/provider store。
  - `ApiClient` 对普通请求与 SSE 都设置 `Authorization: Bearer <token>`。

结论：当前是“可信本机用户 + loopback daemon token”的单用户模型，适合 CLI/dashboard/Desktop，不适合作为多租户 Web 平台的安全边界。

## 3. L3 目标模型

### 3.1 术语

| 对象 | 含义 |
|---|---|
| User | 可登录主体；可来自本地 bootstrap、密码登录、OIDC、service account 或后续 SSO。 |
| Tenant | 组织 / 团队隔离边界；计费、配额、provider secrets、audit log 的上层作用域。 |
| Workspace | tenant 下的项目/工作区；runs、agent sessions、jobs、schedules、memory、artifacts 默认按 workspace 隔离。 |
| Principal | 一次请求认证后的主体快照，包含 subject、auth method、roles、tenant/workspace 选择。 |
| TenantContext | 服务层消费的作用域上下文，包含 tenant_id、workspace_id、principal 与 mode。 |
| Local Principal | 本地 daemon/desktop/dashboard 的兼容主体：`local-user` / `local` / `default`，不改变当前行为。 |

### 3.2 最小类型边界

L3-A2 已新增 dependency-light 类型骨架：

```go
type Principal struct {
    SubjectID   string
    DisplayName string
    AuthMethod  string
    Roles       []string
}

type TenantContext struct {
    TenantID    string
    WorkspaceID string
    Principal   Principal
    Mode        string // "local" | "multi_user"
}

type TenantResolver interface {
    ResolveTenant(ctx context.Context, r *http.Request, principal Principal) (TenantContext, error)
}
```

代码落点：

- `internal/core/tenant/context.go`
  - `Principal` / `TenantContext` / `TenantResolver`
  - `LocalPrincipal()` / `LocalContext()`
  - `LocalResolver.ResolveTenant(...)`：始终返回 `local/default`，不会读取 header、cookie、URL query 或前端传入的 tenant/workspace。
  - `WithContext` / `FromContext`：在 `context.Context` 中保存 cloned `TenantContext`，避免 roles slice alias。
  - `FromContextOrLocal`：服务入口可读取作用域；缺失时回退 `local/default`，不从请求参数推导作用域。
- `internal/core/tenant/context_test.go`
  - 覆盖 local/default scope、provided principal clone、request query 不影响 local scope、context roundtrip clone 与 fallback clone。
- `internal/interfaces/daemon/tenant_middleware.go`
  - `withLocalTenantContext` 在 bearer auth 通过后向 request context 注入 local/default scope。
  - middleware 不读取 header、cookie、URL query 或前端传入的 tenant/workspace。
- `internal/interfaces/daemon/router.go`
  - `/api/` chain 当前为 `corsMiddleware(withDaemonHardening(withDaemonRateLimit(bearerAuth(token, withLocalTenantContext(mux)))))`，其中 hardening/rate-limit 默认 no-op。
  - CORS preflight 仍由 `corsMiddleware` 在 auth/gates 前处理；普通 API 请求在显式启用 gates 后先过 hardening/rate-limit，再过 bearer auth 并进入 tenant context middleware。
- `internal/interfaces/daemon/tenant_middleware_test.go`
  - 覆盖 valid bearer 后 request context 存在 local/default scope。
  - 覆盖 unauthorized request 不会运行 tenant context middleware。
- `internal/core/services/tenant_context.go`
  - `tenantContextForServiceEntry` 让 runs / approvals service entrypoint 可读取 `TenantContext`；local mode 仍使用现有 `RunsDir`。
- `internal/core/services/tenant_context_test.go`
  - 覆盖 `RunService`、run index 与 `ApprovalService` 在带 future multi-user scope 的 context 下仍读取配置的 local `runsDir`。
- `internal/interfaces/daemon/tenant_scope.go`
  - `daemonTenantContext` 让 daemon agent / jobs / schedules / providers entrypoint 可读取 `TenantContext`；当前只做 no-op scope read。
- `internal/interfaces/daemon/tenant_context_entrypoints_test.go`
  - 覆盖 job runner context propagation、schedule store、provider store 与 agent session store 在带 tenant context 下仍使用配置的 local 路径。

本地模式默认值：

```text
Principal.SubjectID = "local-user"
TenantID            = "local"
WorkspaceID         = "default"
Mode                = "local"
```

## 4. AuthN / AuthZ 边界

### 4.1 Local / Desktop 模式

- 继续使用本地 token file + bearer token。
- 继续允许 dashboard loopback bootstrap 注入 token。
- 继续允许 Desktop shell 只把 token 注入本地 WebView。
- `CYBERAGENT_RUNTIME` 仍只作为 whole-engine rollback switch，与 L3 认证无关。

### 4.2 Web 多用户模式

多用户模式必须通过显式 config 开启，不能由当前 daemon token 隐式升级。

建议认证方式：

- Browser：server-side session + HttpOnly/Secure/SameSite cookie。
- API / Automation：scoped personal access token 或 service account token。
- SSO：后续可选 OIDC/SAML adapter，但不阻塞最小本地账号实现。

建议授权层：

- Tenant role：owner / admin / operator / viewer。
- Workspace role：admin / operator / analyst / viewer。
- Operation gate：继续复用现有 operation level 与 approval gate，但输入必须包含 `TenantContext`。
- Secret access：provider API key、API auth profile、web-search secret 只能在服务端按 tenant/workspace 解密使用，不能再进入浏览器 localStorage。

### 4.3 L3-A10 Auth / Session / RBAC skeleton

L3-A10 已新增 server-side 模型和配置 gate，但仍不启用真实 Web login：

- `internal/core/tenant/auth_model.go`
  - future `User`、`Session`、`Role`、`TenantMembership`、`WorkspaceMembership` 类型。
  - 角色常量覆盖 tenant owner/admin/operator/viewer 与 workspace admin/operator/analyst/viewer。
  - `NormalizeRole` 只接受显式已知角色；当前 `LocalPrincipal().Roles == ["local"]` 不会被归一化成 Web RBAC 权限。
  - `Session` / membership clone helper 避免 roles slice / revoked timestamp alias。
- `daemon.auth.web_sessions` config gate：
  - 默认 disabled，不设置 cookie、不创建 HTTP login session。
  - 显式配置时完成 normalize / config-file mapping / audit readiness 字段；L3-A12 后只接入 cookie resolver context，不把 cookie 当成 `/api/` auth credential。
- `daemon.auth.rbac` config gate：
  - 默认 disabled，不做 RBAC enforcement。
  - 显式配置时仅保存未来 policy path/readiness 状态，不替代 operation approval gate。
- local bearer 行为验证：
  - 即使配置 `Auth.WebSessions.Enabled=true` 与 `Auth.RBAC.Enabled=true`，`GET /api/v1/me` 仍返回 `local/default`、`auth_method=local_bearer`，且不会设置 `Set-Cookie`。

后续仍需补齐：真实 login/logout/session refresh 语义、CSRF/secure cookie hardening、membership write/admin mutation gate 与 scoped data enforcement。

### 4.4 L3-A11 SessionStore skeleton

L3-A11 已新增 server-side session store 接口；该切片本身不接入路由或 cookie：

- `internal/core/tenant/session_store.go`
  - `SessionStore` interface：`CreateSession` / `GetSession` / `RevokeSession`。
  - `DisabledSessionStore`：默认 local mode placeholder，调用会返回 `ErrSessionStoreDisabled`，确保不会静默启用 Web session。
  - `MemorySessionStore`：单进程 server-side store skeleton，保存/读取均 clone `Session`，避免 roles / revoked timestamp alias。
  - `Session.ActiveAt(now)`：后续 auth middleware 可复用的 expiry/revoked/required fields 判断。
- L3-A12 后仅由 cookie resolver 读取 active session 到 request context；仍不设置 `Set-Cookie`，不改变 `/api/v1/me` 的 `local_bearer` session payload。
- 后续需要把 login/logout/session refresh endpoint、secure cookie、CSRF 与 persistent session backend 小切片接上。

### 4.5 L3-A12 Session cookie resolver middleware skeleton

L3-A12 已新增 cookie resolver，但仍不把 cookie 当成 `/api/` 的认证凭证：

- `internal/interfaces/daemon/web_session_middleware.go`
  - `withWebSessionCookieResolver(...)` 在 `web_sessions.enabled=false` 时直接 no-op：不读取 cookie、不访问 session store。
  - 显式启用时按 normalized cookie name 读取 session id，从 `SessionStore` 读取 active `web_session`，并以 clone 后的 request context 值交给后续 middleware/handler。
  - 仅接受 `AuthMethodWebSession` 且 `Session.ActiveAt(now)` 为 true 的 server-side session；expired/revoked/unknown session 不会进入 context。
- `/api/` chain 保持 bearer-first：
  - cookie-only 请求仍返回 unauthorized。
  - 默认 `principal_resolver_enabled=false` 时，local bearer 请求即使携带 session cookie，`GET /api/v1/me` 仍返回 `local/default` + `local_bearer`。
  - L3-A18 后可显式开启 `principal_resolver_enabled=true`：在 bearer 已通过且 cookie resolver 找到 active server-side session 时，用 session 的 subject/tenant/workspace hydrate request `TenantContext`；无 cookie/无 active session 仍 fallback 到 local bearer。
  - middleware 不设置 `Set-Cookie`，不创建/刷新/撤销 session。
- `NewServer` 在显式 `Auth.WebSessions.Enabled=true` 时提供空的 in-memory session store skeleton，供后续 login/logout/refresh 切片复用；默认 local mode 仍是 disabled store。

后续需要把真实 login/logout/session refresh、CSRF/secure cookie hardening、persistent session backend、membership write/admin mutation gate 与 tenant/workspace data isolation 小切片接上。

### 4.6 L3-A13 Web auth endpoint skeleton

L3-A13 已新增 Web auth endpoint skeleton，但仍不交付真实账号体系：

- 新增 endpoint：
  - `POST /api/v1/auth/login`
  - `POST /api/v1/auth/logout`
  - `POST /api/v1/auth/session/refresh`
- 当前 endpoint 仍在既有 `/api/` bearer auth chain 后面：
  - 不开放 public login。
  - 无 bearer 的 cookie-only 请求仍返回 unauthorized。
  - bearer 通过后，默认 `web_sessions.enabled=false` 返回 `not_found`。
  - 显式 `web_sessions.enabled=true` 返回 `unsupported`，表示 skeleton 已挂接但真实账号 adapter/session mutation 尚未实现。
- response details 仅暴露边界信息：
  - CSRF header：`X-CyberAgent-CSRF`。
  - cookie policy：HttpOnly、SameSite=strict、non-loopback secure-required。
  - `account_adapter_configured=false`。
- 当前不读取 password/OIDC token、不创建/revoke/refresh session、不设置 `Set-Cookie`。

后续需要补真实 account adapter、CSRF token issuance/validation、secure cookie issuance、session rotation/revocation 与 RBAC enforcement。

### 4.7 L3-A14 RBAC middleware/enforcer skeleton

L3-A14 已新增 RBAC middleware/enforcer skeleton，但仍不执行真实授权阻断：

- `internal/interfaces/daemon/rbac_middleware.go`
  - 定义 `rbacRequirement`：action、scope、required roles。
  - 定义 `rbacDecision` request context：enabled/matched/enforced/action/scope/required_roles/principal_roles/reason。
  - 固化一组最小 route requirement：
    - identity read：`GET /api/v1/me`
    - tenant/workspace list：`GET /api/v1/tenants`、`GET /api/v1/workspaces`
    - workspace select：`POST /api/v1/workspaces/{workspace_id}/select`
    - admin bootstrap read：`GET /api/v1/admin/bootstrap`
    - auth session skeleton：login/logout/refresh
- `rbac.enabled=false`：完全 no-op，不写 context decision。
- `rbac.enabled=true`：L3-A14 只写 `skeleton_not_enforced` decision；L3-A16 后会读取内置/显式 policy 并 lookup membership store，但仍不 deny、不把 local bearer `["local"]` 映射为 Web RBAC role。
- `/api/` 行为保持兼容：
  - bearer auth 仍是当前唯一 API auth gate。
  - `GET /api/v1/me` 仍返回 `local/default` + `local_bearer`。

后续需要补 tenant/workspace data scoping、membership write/admin mutation gate 与 admin policy API。

### 4.8 L3-A15 Membership store skeleton

L3-A15 已新增 tenant/workspace membership store skeleton，但仍不接入真实 RBAC deny：

- `internal/core/tenant/membership_store.go`
  - `MembershipStore` interface：
    - tenant membership upsert/get。
    - workspace membership upsert/get。
  - `DisabledMembershipStore`：默认 local mode placeholder，调用返回 `ErrMembershipStoreDisabled`。
  - `MemoryMembershipStore`：dependency-light in-memory skeleton，所有读写都 clone membership，避免 roles alias。
- validation 边界：
  - tenant/workspace/subject id 必填并 trim。
  - tenant membership 只接受 tenant-scoped roles。
  - workspace membership 只接受 workspace-scoped roles。
  - unknown role 被拒绝。
- 当前不接入：
  - login/session creation。
  - RBAC deny。
  - policy admin API。
  - workspace storage resolver。
  - frontend membership admin。

后续需要补 membership write/admin mutation gate 与 admin policy API。

### 4.9 L3-A16 RBAC policy loading/evaluation skeleton

L3-A16 已接上 policy loading 与 membership-backed evaluation，但仍不执行真实授权阻断：

- `internal/interfaces/daemon/rbac_policy.go`
  - 内置 policy 继续来自 route/action/scope/role requirement table。
  - 显式 `daemon.auth.rbac.policy_path` 仅在 `rbac.enabled=true` 时加载；当前 YAML skeleton 支持 `routes[]`：`method`、`path`、`action`、`scope`、`roles`。
  - 加载时校验 known role、tenant/workspace role scope、absolute API path 与非空 action/roles。
  - 显式 policy 作为 replacement policy 使用；未声明的 route 不生成 RBAC requirement。
- `internal/interfaces/daemon/rbac_middleware.go`
  - `withRBACPolicyEvaluator` 在 `rbac.enabled=false` 时仍完全 no-op。
  - `rbac.enabled=true` 时读取 policy requirement，并按 tenant/workspace scope 查询 `MembershipStore`。
  - request-scoped decision 新增 policy source、evaluation reason、membership subject/roles、allowed 等调试字段；读取时继续 clone。
  - `Enforced=false` 保持不变：membership missing、missing role、policy error 都不会 deny 当前 local bearer 请求。
- `internal/interfaces/daemon/server.go`
  - 默认 RBAC disabled 使用 `DisabledMembershipStore` 且不加载 policy path。
  - 显式 RBAC enabled 使用 in-memory membership store skeleton；显式 `policy_path` 缺失/非法会让 `NewServer` fail fast。

示例 policy skeleton：

```yaml
routes:
  - method: GET
    path: /api/v1/me
    action: identity.read
    scope: tenant
    roles: [tenant_viewer]
```

L3-A17 已把 allowed/missing-role decision 接入显式 deny/audit gate；L3-A18 已补真实 Web session principal resolver skeleton；L3-A19 已补只读 membership admin API skeleton。

### 4.10 L3-A17 RBAC deny/audit enforcement gate skeleton

L3-A17 已新增显式 deny/audit gate，但默认仍保持 Local Daemon 兼容：

- `daemon.auth.rbac.audit_enabled=false` 默认关闭；开启后写入 `rbac.decision` audit event。
  - audit details 只包含 action、scope、required roles、evaluation reason、policy source type、membership found/count 等非敏感 metadata。
  - 不写 bearer token、request path、policy file path、API key 或 secret。
- `daemon.auth.rbac.enforcement_enabled=false` 默认关闭；开启后只对非 local bearer principal 执行 RBAC deny。
  - membership missing、missing role、lookup error 等未 allowed 的 matched requirement 会返回 `policy_denied`。
  - local bearer 继续豁免；`LocalPrincipal().Roles == ["local"]` 仍不会被解释为 Web RBAC role。
- `NewServer` / `newRouter` 的默认路径仍是 no-op：不开启 `rbac.enabled` 时不加载 policy、不查 membership、不写 RBAC audit、不 deny。
- 当前仍没有 public login 或 membership write API；L3-A17 只是把 L3-A16 decision 接到可回滚 gate，L3-A18 再把 active Web session 到 tenant principal 的映射放到独立可回滚 gate，L3-A19 仅提供显式 gate 下的只读 membership lookup。

### 4.11 L3-A18 Web session principal/tenant resolver skeleton

L3-A18 已让 active server-side Web session 可以在显式 gate 下参与 request tenant context，但仍不开放 public login：

- 新增 `daemon.auth.web_sessions.principal_resolver_enabled=false`，默认关闭。
- API chain 仍是 bearer-first：cookie-only 请求仍在 `bearerAuth` 返回 unauthorized，不会因为 session cookie 获得 API 访问权。
- `principal_resolver_enabled=true` 且 `web_sessions.enabled=true` 时：
  - `withWebSessionCookieResolver` 先解析 active `web_session` 到 request context。
  - `withRequestTenantContext` 再把 session 的 `SubjectID`、`TenantID`、`WorkspaceID` 与 session roles 映射到 `TenantContext` / `Principal`。
  - 无 cookie、unknown/expired/revoked session 或 resolver 未启用时继续 fallback 到 `local/default` + `local_bearer`。
- 该 skeleton 已能喂给 L3-A17 RBAC enforcement gate：Web session principal 缺 membership 会被 `policy_denied`，补齐 membership 后允许；local bearer 仍不受 Web RBAC deny 影响。
- 当前仍不创建 session、不设置 cookie、不做 CSRF token issuance，也没有 account adapter 或 membership 写入 API。

### 4.12 L3-A19 membership admin API skeleton

L3-A19 已新增只读 membership admin API skeleton，用于先固化后端 contract 与安全边界：

- 新增 `daemon.admin.membership_api_enabled=false`，默认关闭；关闭时 `GET /api/v1/admin/memberships` 在 bearer 通过后返回 `not_found`。
- 显式开启后仅支持 `GET /api/v1/admin/memberships?scope=tenant&tenant_id=...&subject_id=...` 与 `scope=workspace&tenant_id=...&workspace_id=...&subject_id=...`。
- 响应只返回 membership found、scope、tenant/workspace/subject、roles 与 created/updated timestamp；不返回 bearer token、bootstrap token、policy path 或 store path。
- POST/PUT/DELETE 仍返回 method error，不写 membership store；`write_endpoints_enabled=false` 明确保留后续独立切片。
- RBAC route table 新增 `admin.membership.read`，最低 future role 为 `tenant_admin` / `tenant_owner`；local bearer 仍受 L3-A17 exemption 保护，不因 Web RBAC 缺失被 deny。

## 5. Session 边界

需要区分三类 session：

| Session | 当前位置 | L3 边界 |
|---|---|---|
| HTTP login session | 目前不存在 | 多用户浏览器登录态；只表达 Web 身份，不等价于 Agent session。 |
| Agent session | `agentManager` / `/api/v1/agent/sessions` | 必须归属 tenant/workspace/user；session_id 不能跨作用域读取。 |
| Runtime task/run/turn session | SQLite runtime store 与 runs dir | 必须增加 tenant/workspace scoping 或迁移到按作用域隔离的 store。 |

本地兼容要求：在 local mode 中，所有现有 agent session 与 runtime session 自动挂到 `local/default`，API 响应可先不暴露新字段，避免破坏前端和脚本。

## 6. Storage / Workspace 隔离策略

推荐分阶段处理，不一次性搬迁现有 runs：

### 阶段 A：上下文先行

- 在 request context 中注入 `TenantContext`。
- 服务层先能读取 context，但 local mode path 仍为现有 `cfg.RunsDir`。
- 所有新增审计与测试先断言 context 传播，不改变持久化路径。

### 阶段 B：新建数据按作用域落盘

L3-A5 已落地第一层显式 gate 与 resolver skeleton：

- 配置入口：`daemon.multi_user.enabled` + `daemon.multi_user.storage_root`，也可通过 CLI `--multi-user` / `--multi-user-storage-root` 显式开启；默认关闭。
- Resolver：`internal/core/tenant/storage.go` 中的 `StorageResolver` / `PathStorageResolver` 只在 config gate 开启且 `TenantContext.Mode == "multi_user"` 时解析 scoped path。
- Local fallback：gate 关闭时，即使 context 带 future `multi_user` scope，仍返回现有 local `runsDir`、provider store path 与 runtime DB path，不迁移历史数据。
- 当前已接入的最小写路径：daemon provider store 的 `LoadContext` / `SaveContext` 和 web scan job 的新 run dir 可通过 resolver 在 gate 开启后走 per-workspace path；其余 service adoption 后续继续小切片推进。

多用户模式新建数据建议路径：

```text
<data_dir>/tenants/<tenant_id>/workspaces/<workspace_id>/runs/
<data_dir>/tenants/<tenant_id>/workspaces/<workspace_id>/.agent/runtime/runtime.db
<data_dir>/tenants/<tenant_id>/provider-secrets/
<data_dir>/tenants/<tenant_id>/audit/
```

保守替代方案是单 runtime DB 增加 `tenant_id` / `workspace_id` 列；但当前 runs dir 仍是 run list source of truth，因此最小可行迁移应优先按目录隔离，降低 schema 大迁移风险。

### 阶段 C：显式迁移

- 本地历史 runs 默认保留在 local/default。
- 如果用户要把本地 runs 导入某个 Web workspace，应通过显式 import/migrate 命令完成。
- provider secrets、API auth profiles、memory 不应静默从 localStorage 或 per-user JSON 导入到共享 tenant。

## 7. API 边界

### 保留

- 保留当前 `/api/v1/*` local daemon contract。
- 保留现有 bearer auth 行为作为 local mode。
- 保留 Desktop/Wails loopback bootstrap seam。

### 新增

L3-A8 已在同一 API version 下新增显式身份/作用域 skeleton 端点：

```text
GET  /api/v1/me
GET  /api/v1/tenants
GET  /api/v1/workspaces
POST /api/v1/workspaces/{workspace_id}/select
```

当前端点仍受 bearer auth 保护，只返回/选择 `local/default`，用于把后端真实作用域状态暴露给 Console；不会把前端 workspace switch 当作服务端隔离边界。后续多用户模式再扩展：

```text
GET  /api/v1/audit/events
```

既有业务端点在多用户模式下由 middleware 注入 `TenantContext` 后再进入 handler，例如：

```text
Auth middleware
  -> Principal
  -> TenantResolver
  -> TenantContext in request.Context()
  -> existing /api/v1/agent/sessions, /runs, /jobs, /schedules, /providers
```

禁止事项：

- 禁止只靠前端 workspace switch 做隔离。
- 禁止让前端传入任意 `tenant_id` 后服务端无校验使用。
- 禁止把 daemon bearer token 作为共享 Web 管理员 token 暴露。

## 8. Frontend 边界

Local mode：

- 继续支持 Settings 页面配置 base URL / daemon token。
- 继续允许 `/api/local/daemon-config` 自动填充本地 daemon token。
- L3-A6 已新增前端 `ConsoleSessionSettings` / normalize helper 与顶部 identity/workspace switcher；默认显示 `本地 Daemon` + `local/default`，并继续复用现有 bearer token/localStorage 设置。
- L3-A9 起 Console 在 token 可用后调用 `GET /api/v1/me` hydrate 当前显示的 local/default scope；失败或未配置 token 时回退 `defaultConsoleSession`，不把前端 workspace state 当作服务端隔离边界。

Multi-user mode：

- 需要登录页、当前用户菜单、tenant/workspace switch。
- daemon token 输入应隐藏或改为只读诊断。
- AI provider key、API auth profile secret、web search API key 不再保存到 `localStorage`；前端只显示 secret 状态与更新入口。
- 所有敏感操作应显示当前 workspace 与审批/权限状态，避免用户在错误 workspace 执行写操作。
- L3-A6 只交付 disabled login placeholder 与 Settings 内 Session/Workspace panel；真实 Web login、server-side session、workspace selection API 与 secret server-side storage 留给 L3-A7+ 后续切片。

## 9. Audit / Admin / Deployment

多用户平台最小审计事件：

- 登录、登出、session refresh、token 创建/撤销。
- tenant/workspace 创建、成员与角色变更。
- provider secret 创建/更新/删除、secret 使用摘要。
- agent session 创建、prompt 输入摘要、operation level、approval decision。
- workspace command、file write、patch/apply、scan/job/schedule 创建与取消。
- run/artifact/report 读取与导出。

部署硬化要求：

- HTTPS 或可信反向代理头校验。
- Secure cookie、CSRF protection、CORS allowlist。
- request ID / audit event ID 贯穿 API、runtime event 与日志。
- rate limit / quota：按 principal、tenant、workspace、provider 维度。
- DB migration、backup/restore、secret encryption key rotation。

L3-A7a 已落地第一层 daemon 配置骨架，默认保持 local no-op：

```yaml
daemon:
  auth:
    web_sessions:
      enabled: false
      cookie_name: cyberagent_session
      ttl_seconds: 28800
      storage_path: data/auth/sessions.jsonl
      principal_resolver_enabled: false
    rbac:
      enabled: false
      policy_path: data/auth/rbac.yaml
      audit_enabled: false
      enforcement_enabled: false
  audit:
    enabled: true
    path: data/audit/events.jsonl
  admin:
    bootstrap_token_file: data/admin/bootstrap-token
    membership_api_enabled: false
  rate_limit:
    enabled: true
    requests_per_minute: 120
    burst: 30
  hardening:
    require_loopback: true
```

- `auth.web_sessions` 默认关闭；关闭时不填充 cookie 默认值、不创建 session store、不设置 cookie。显式开启时只填充 `cyberagent_session` / `28800s` 等 skeleton 默认值，当前 router 仍走 bearer auth。
- `auth.rbac` 默认关闭；只有显式 `rbac.enabled` + `rbac.enforcement_enabled` 才对非 local bearer principal 执行 Web RBAC deny。
- `audit` 默认关闭；启用但未提供路径时，daemon normalize 会使用 runs dir 下的 `.daemon/audit/events.jsonl`。
- `admin.bootstrap_token_file` 未配置时不启用 bootstrap 行为；`admin.membership_api_enabled` 默认 false，L3-A19 起仅显式开启只读 membership lookup。
- `rate_limit` 默认关闭；启用但未配置数值时填充 `120 rpm / burst 30`。
- `hardening.require_loopback` 默认 false；L3-A7e 起启用后会拒绝非 loopback API 请求。
- L3-A7a 不改变现有 bearer token auth、frontend localStorage 设置、`/api/v1/*` 响应或 local daemon 默认启动行为。

L3-A7b 已落地 audit JSONL sink skeleton：

- audit disabled 时使用 no-op sink，不创建文件。
- audit enabled 时 append JSONL，每条事件包含 `event_id`、UTC `time`、`type`、`outcome` 与安全 details。
- details 会递归 redaction `token`、`api_key`、`secret`、`password`、`authorization`、`credential` 等敏感 key。
- `NewServer` 在 audit enabled 时写入一条 `daemon.startup` readiness event；不写入 bearer token、token file path 或 API key。

L3-A7c 已落地 admin bootstrap readiness skeleton：

- `daemon.admin.bootstrap_token_file` 配置后，daemon 在服务端读取 bootstrap token，只保存内部 token 值。
- 新增 `GET /api/v1/admin/bootstrap` 诊断端点，仍受现有 bearer auth 保护。
- 诊断响应只返回 `configured`、`ready`、`message`，不返回 token、token 文件路径或 bootstrap secret。
- `daemon.startup` audit event 只记录 `admin_bootstrap_configured` / `admin_bootstrap_ready` 布尔状态。

L3-A7d-1 已落地 provider/web-search secret redaction response skeleton：

- `GET /api/provider-profiles` 不再返回 raw `api_key`，只返回 `api_key_configured` 状态。
- `PUT /api/provider-profiles` 仍可接收并保存 secret；当前端发送空 `api_key` 时，同 provider/profile scope 会保留服务端既有 secret，避免 redacted GET 后再次保存误清空。
- Agent runtime 内部 `ProviderStore.Load/LoadContext` 仍读取 raw secret，用于 LLM fallback 与 `web.search`。

L3-A7d-2 已落地 Settings provider/web-search status-only secret UX：

- Provider 与 web-search API key 在 Settings 中只作为本轮内存 draft；保存后写入 daemon/provider store，并清空前端明文。
- `localStorage` 不保存 provider/web-search raw API key；legacy 本地明文 key 会在 `loadSettings()` 归一化时清空，且不会被误标成 daemon configured。
- redacted `GET /api/provider-profiles` hydrate 后只显示 `api_key_configured` 状态；输入框留空表示保留 daemon 现有 secret。
- Provider type 或 web-search provider 变更会清空对应 configured 状态/内存 draft，避免跨 scope 误用 secret。

L3-A7d-3 已落地 API auth profile server-side storage/status-only skeleton：

- `ProviderConfig` 新增 `auth_profiles` / `default_auth_profile_id`；`GET /api/provider-profiles` 返回 API auth profile metadata 与 `secret_configured`、`token_configured`、`password_configured`、`header_value_configured`、`cookie_configured`，不返回 raw `token` / `password` / `header_value` / `cookie`。
- `PUT /api/provider-profiles` 支持 auth profile partial update；请求只包含 `auth_profiles` 时会保留既有 providers/web-search。空 secret 且同 `id+type` scope 会保留 daemon 既有 secret；type/ID 变化不会跨 scope 搬运 secret。
- Settings API Auth Profiles 的 secret 输入只保留在内存 draft；点击“保存到 daemon”写入 provider store，localStorage 只保留 metadata/configured 状态并清理 legacy raw secret。
- Agent cockpit 只发送启用 profile 的 metadata 与空 secret 字段；daemon 在 `SendMessage` / `Authorize` 入口通过 server-side provider store 合并可用 secret，LLM 仍只看到 profile ID。

L3-A7e 已落地 rate-limit middleware 与 deployment hardening gates：

- `rate_limit.enabled=false` 默认 no-op，不改变本地 daemon 默认行为。
- 启用 `rate_limit.enabled=true` 后，API chain 按 `RemoteAddr` host 作为 client key 执行最小 token-bucket rate limit。
- 超限响应为 HTTP 429，error code 为 `rate_limited`，并返回 `Retry-After` header。
- 启用 `hardening.require_loopback=true` 后，daemon 会拒绝非 loopback API request，响应 `policy_denied`。
- CORS preflight 仍在 auth / hardening / rate-limit gate 前处理，保留当前浏览器预检行为。

L3-A8 已落地 server-side identity/tenant/workspace API skeleton：

- `GET /api/v1/me` 返回当前 bearer-authenticated local principal、tenant、workspace 与 session mode，不返回 token 或任何 secret。
- `GET /api/v1/tenants` 返回 selected `local` tenant；`GET /api/v1/workspaces` 返回 selected `local/default` workspace。
- `POST /api/v1/workspaces/default/select` 是 local/default no-op select；未知 workspace 返回 `not_found`，避免制造 UI-only 伪隔离。
- 所有端点仍走 `/api/` auth/gates/tenant middleware；当前不新增 cookie session、真实 Web login、tenant role model 或数据迁移。

L3-A9 已落地 frontend server-side identity/workspace scope hydrate skeleton：

- `web/frontend/src/lib/api.ts` 增加 identity/scope endpoint 类型与 API client 方法。
- `web/frontend/src/lib/session.ts` 增加 `/api/v1/me` payload → `ConsoleSessionSettings` 的安全映射，local mode 强制保持 `local/default`，future multi-user scope 仅接受安全 segment。
- `ConsoleProvider` 在 local daemon token 可用后同步 `/api/v1/me`；失败回退 `defaultConsoleSession`。
- Layout 与 Settings 显示 server scope sync/fallback 状态；仍保留 bearer token/localStorage 设置，不启用真实 Web login 或前端任意 workspace select。

L3-A10 已落地 server-side auth/session/RBAC model skeleton：

- `internal/core/tenant/auth_model.go` 定义 future Web auth/RBAC 模型，且通过 tests 固化 clone 隔离和 local bearer 不授予 Web RBAC role。
- `internal/core/config.Config` 与 daemon `Config.Normalize()` 支持 `daemon.auth.web_sessions` / `daemon.auth.rbac`，默认关闭。
- `applyDaemonConfigFile` 可从 config file 映射 auth skeleton 字段；当前不新增 CLI 开关，避免误把 local daemon 一键升级成 Web auth。
- `NewServer` audit readiness details 只记录 `auth_web_sessions_enabled` / `auth_rbac_enabled` 布尔值，不写 cookie、policy path 或 secret。
- 当前不新增 login/logout endpoint，不改变 `/api/` chain，不替换 bearer token/localStorage 请求链路。

L3-A11 已落地 server-side session store skeleton：

- `SessionStore` interface 与 `DisabledSessionStore` 默认保持 Web session store 关闭。
- `MemorySessionStore` 提供后续登录切片可复用的 server-side session CRUD/revoke 基础，读写均返回 clone。
- `Session.ActiveAt()` 固化 expiry/revocation 判断，当前仅作为模型 helper，不参与 bearer auth。
- 当前不改变 local daemon token、Desktop bootstrap 或 frontend localStorage 请求链路。

L3-A12 已落地 session cookie resolver middleware skeleton：

- 默认关闭时是 no-op，带 cookie 请求不会读取 session store。
- 显式启用时只解析 active `web_session` 到 request context，方便后续 auth/login/RBAC 切片接管。
- `/api/` 仍必须通过 bearer token；cookie-only 不被接受，且不设置 `Set-Cookie`。

L3-A13 已落地 Web auth endpoint skeleton：

- `/api/v1/auth/login`、`/api/v1/auth/logout`、`/api/v1/auth/session/refresh` 已挂接在 bearer-protected `/api/` chain 后。
- 默认 disabled 返回 `not_found`；显式 enabled 返回 `unsupported` skeleton boundary。
- 所有路径均不创建 session、不设置 cookie、不接入真实账号体系。

L3-A14 已落地 RBAC middleware/enforcer skeleton：

- 默认 `rbac.enabled=false` 完全 no-op。
- 显式启用时只生成 request-scoped RBAC decision，不执行 deny。
- route requirement table 只使用已知 Web RBAC role；local bearer `["local"]` 不会被提升为 Web role。

L3-A15 已落地 membership store skeleton：

- 默认 disabled store 拒绝使用。
- in-memory store 支持 tenant/workspace membership upsert/get，读写 clone 隔离。
- 固化 unknown role 与 role scope validation，仍不接入 RBAC deny。

L3-A16 已落地 RBAC policy loading/evaluation skeleton：

- 默认 `rbac.enabled=false` 完全 no-op，不加载 policy path、不读取 membership store。
- 显式启用时可加载内置/显式 YAML policy，并查询 membership store 计算 allowed/missing-role decision。

L3-A17 已落地 RBAC deny/audit enforcement gate skeleton：

- 新增 `rbac.audit_enabled` / `rbac.enforcement_enabled`，均默认关闭。
- `audit_enabled=true` 时写入 `rbac.decision` audit event，只记录 action/scope/required roles/evaluation reason/policy source type 等非敏感 metadata，不写 bearer token、request path 或 policy file path。
- `enforcement_enabled=true` 时只对非 local bearer principal 执行 RBAC deny；local bearer 继续不受 Web RBAC deny 影响。

L3-A18 已落地 Web session principal/tenant resolver skeleton：

- 新增 `web_sessions.principal_resolver_enabled`，默认关闭。
- cookie-only 请求仍 unauthorized；显式开启后也必须先通过 bearer auth。
- active server-side `web_session` 可 hydrate `TenantContext` 并接入 RBAC enforcement；无 active session fallback 到 local bearer。

## 10. 迁移计划

- [x] L3-A1：architecture/design slice：定义 auth/session/tenant/workspace/API/storage/audit 边界。
- [x] L3-A2：新增 `Principal` / `TenantContext` / `TenantResolver` no-op local skeleton，默认返回 `local/default`，不改变路由行为。
- [x] L3-A3：给 daemon router 增加 local principal/tenant context middleware；focused tests 断言 context 存在且 bearer auth 行为不变。
- [x] L3-A4：让 agent/runs/jobs/schedules/providers 关键 service entrypoints 可读取 tenant context；local mode 仍使用现有 `runsDir` / provider store / runtime DB，API 响应不新增 scope 字段。
- [x] L3-A5：设计并实现 multi-user mode 显式 config gate 与 per-workspace storage resolver skeleton；默认 local mode 不启用、不迁移历史 runs。
- [x] L3-A6：前端新增 login/session/workspace UX skeleton，同时保留 local daemon settings 与 bearer auth 请求链路。
- [x] L3-A7：补 audit log、admin bootstrap、secret server-side storage、rate limit 与 deployment hardening。
  - [x] L3-A7a：daemon audit/admin/rate-limit/hardening 配置骨架与 config-file merge/CLI mapping/normalize 覆盖；默认 local no-op。
  - [x] L3-A7b：audit event JSONL sink skeleton，保证 token/API key/secret 不进入 audit JSON；enabled 时写入 `daemon.startup` readiness event。
  - [x] L3-A7c：admin bootstrap token loading 与 readiness/diagnostic endpoint，仍不改变 bearer auth。
  - [x] L3-A7d：server-side provider/API auth secret storage skeleton，frontend 只显示 secret 状态。
    - [x] L3-A7d-1：`GET /api/provider-profiles` 返回 provider/web-search secret 状态并脱敏 raw `api_key`；`PUT` 空 `api_key` 不覆盖同 scope 既有 secret，Agent runtime 内部 raw store 仍可读取。
    - [x] L3-A7d-2：Settings 前端 provider/web-search secret status-only UX，不把 provider/web-search API key 写入 localStorage；保存后只保留 configured 状态。
    - [x] L3-A7d-3：API auth profile server-side storage skeleton；Settings/Agent 只保留 profile metadata/configured 状态，不把 token/password/header_value/cookie 写入 localStorage。
  - [x] L3-A7e：rate-limit middleware 与 deployment hardening gates 的最小可验证接入；默认 no-op，启用后 API chain 执行 loopback gate 与 per-client token-bucket rate limit。
  - [x] L3-A8：server-side identity/tenant/workspace API skeleton；新增 `/api/v1/me`、`/api/v1/tenants`、`/api/v1/workspaces` 与 local/default workspace select no-op。
  - [x] L3-A9：frontend server-side identity/workspace scope hydrate skeleton；Console 从 `/api/v1/me` 同步 local/default display，失败回退 `defaultConsoleSession`，不改变 bearer token/localStorage 请求链路。
  - [x] L3-A10：server-side auth/session/RBAC model skeleton；新增 future auth/RBAC 类型与 config gate，默认关闭，不改变 local bearer 行为。
  - [x] L3-A11：server-side session store skeleton；新增 disabled/memory `SessionStore` 与 expiry/revoke helper，作为后续 cookie resolver/login 切片基础。
  - [x] L3-A12：session cookie resolver middleware skeleton；默认 no-op，显式启用时仅解析 active server-side session 到 request context，仍不替换 bearer auth。
  - [x] L3-A13：Web login/logout/session refresh endpoint skeleton；默认关闭，先固化 contract、CSRF/secure-cookie 边界与兼容测试，不接入真实账号体系。
  - [x] L3-A14：RBAC middleware/enforcer skeleton；默认关闭，先定义 route/action role requirements 与 no-op compatibility tests。
  - [x] L3-A15：tenant/workspace membership store skeleton；默认 local-only/no-op，先固化 membership persistence 边界与 clone/role validation。
  - [x] L3-A16：RBAC policy loading/evaluation skeleton；默认关闭，连接 membership store 与 route requirements 但仍不 deny local bearer。
  - [x] L3-A17：RBAC deny/audit enforcement gate skeleton；默认关闭，local bearer 继续不受 Web RBAC deny 影响。
  - [x] L3-A18：Web session principal/tenant resolver skeleton；默认关闭，cookie-only 仍不替代 bearer，local bearer 默认行为不变。
  - [x] L3-A19：membership admin API skeleton；默认关闭/只读先行，避免破坏 local bearer 兼容。
  - [ ] L3-A20：membership write/admin mutation gate skeleton；继续默认关闭，需分离 bootstrap/owner/admin 权限与 audit。

## 11. 非目标与风险

非目标：

- 本切片不交付公网 Web 平台。
- 本切片不修改 bearer token auth。
- 本切片不迁移现有 runs、provider store 或 runtime DB。
- 本切片不引入新的 deprecated runtime API compatibility flag。

主要风险：

- tenant 隔离如果只在 UI 层实现，会造成跨租户数据泄露。
- provider/API auth secret 继续放在浏览器 localStorage，不适合多用户共享浏览器和受管环境。
- 直接迁移 runs/runtime DB 可能破坏本地历史数据；需要先有 read-only audit 与显式 import。
- Web session 与 Agent session 名称容易混淆；代码中应使用 `Principal` / `TenantContext` 明确区分。

## 12. 验证

当前 L3-A6/L3-A7a-e/L3-A8/L3-A9/L3-A10/L3-A11/L3-A12/L3-A13/L3-A14/L3-A15/L3-A16/L3-A17/L3-A18/L3-A19 切片验证命令：

```bash
test -f docs/CyberAgent-Web-Multi-User-Platform.md
rg -n "ConsoleSession|session-workspace|L3-A6|L3-A9|L3-A10|L3-A11|L3-A12|L3-A13|L3-A14|L3-A15|L3-A16|L3-A17|L3-A18|L3-A19|local/default|getMe|auth_model|session_store|membership_store|membership_admin|web_session_middleware|web_auth_handlers|rbac_middleware|rbac_policy|web_sessions|rbac" docs/CyberAgent-Web-Multi-User-Platform.md docs/CyberAgent-TODO.md web/frontend/src internal/core/tenant internal/interfaces/daemon internal/core/config
npm test --prefix web/frontend -- --run session
npm test --prefix web/frontend -- --run
npm run build --prefix web/frontend
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/core/config ./internal/interfaces/cli ./internal/interfaces/daemon -run 'Test.*MembershipStore|TestRBAC|TestLoadDaemonRBACPolicy|TestNewServerLoadsExplicitRBACPolicyPath|TestWebAuthSkeleton|Test.*Session|Test.*Auth|TestIdentity|TestConfigNormalize|TestLoadFrom|TestMergeOverlayWins'
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/interfaces/daemon
GOCACHE=$PWD/.tmp/go-build GOTMPDIR=/private/tmp/cyberagent-go-tmp go test -count=1 ./internal/core/tenant ./internal/core/config ./internal/interfaces/cli ./internal/interfaces/daemon
git diff --check
```
