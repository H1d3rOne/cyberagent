export type ConsoleSessionMode = 'local' | 'multi_user'
export type ConsoleSessionAuthMethod = 'local_bearer' | 'web_session' | 'unknown'
export type ConsoleLanguage = 'zh' | 'en'

export interface ConsoleSessionSettings {
  mode: ConsoleSessionMode
  principalId: string
  principalLabel: string
  tenantId: string
  tenantLabel: string
  workspaceId: string
  workspaceLabel: string
  authMethod: ConsoleSessionAuthMethod
}

export interface ConsoleServerIdentityScopePayload {
  principal?: {
    subject_id?: unknown
    subjectId?: unknown
    display_name?: unknown
    displayName?: unknown
    auth_method?: unknown
    authMethod?: unknown
  }
  tenant?: {
    id?: unknown
    label?: unknown
    mode?: unknown
  }
  workspace?: {
    id?: unknown
    tenant_id?: unknown
    tenantId?: unknown
    label?: unknown
    mode?: unknown
  }
  session?: {
    mode?: unknown
    auth_method?: unknown
    authMethod?: unknown
  }
}

const fallbackLocalOperator = 'local-user'
const safeSegmentPattern = /^[A-Za-z0-9_-]+$/

export const defaultConsoleSession: ConsoleSessionSettings = {
  mode: 'local',
  principalId: fallbackLocalOperator,
  principalLabel: 'Local User',
  tenantId: 'local',
  tenantLabel: 'Local',
  workspaceId: 'default',
  workspaceLabel: 'default',
  authMethod: 'local_bearer',
}

export function normalizeConsoleSession(value: unknown, operator = fallbackLocalOperator): ConsoleSessionSettings {
  const item = (value && typeof value === 'object' ? value : {}) as Partial<ConsoleSessionSettings> & Record<string, unknown>
  const mode = item.mode === 'multi_user' ? 'multi_user' : 'local'
  if (mode === 'local') {
    return {
      ...defaultConsoleSession,
      principalId: safeSegment(item.principalId ?? item.principal_id ?? operator, fallbackLocalOperator),
      principalLabel: nonEmptyString(item.principalLabel, defaultConsoleSession.principalLabel),
    }
  }

  return {
    mode,
    principalId: safeSegment(item.principalId ?? item.principal_id, fallbackLocalOperator),
    principalLabel: nonEmptyString(item.principalLabel ?? item.principal_label, 'Web User'),
    tenantId: safeSegment(item.tenantId ?? item.tenant_id, 'tenant'),
    tenantLabel: nonEmptyString(item.tenantLabel ?? item.tenant_label, 'Tenant'),
    workspaceId: safeSegment(item.workspaceId ?? item.workspace_id, 'default'),
    workspaceLabel: nonEmptyString(item.workspaceLabel ?? item.workspace_label, 'default'),
    authMethod: normalizeAuthMethod(item.authMethod ?? item.auth_method),
  }
}

export function consoleSessionFromServerIdentity(value: unknown, operator = fallbackLocalOperator): ConsoleSessionSettings {
  const item = (value && typeof value === 'object' ? value : {}) as ConsoleServerIdentityScopePayload
  const principal = objectValue(item.principal)
  const tenant = objectValue(item.tenant)
  const workspace = objectValue(item.workspace)
  const session = objectValue(item.session)
  const mode = normalizeSessionMode(session.mode ?? tenant.mode ?? workspace.mode)
  const authMethod = normalizeAuthMethod(session.auth_method ?? session.authMethod ?? principal.auth_method ?? principal.authMethod)

  if (mode === 'local') {
    return {
      ...defaultConsoleSession,
      principalId: safeSegment(principal.subject_id ?? principal.subjectId ?? operator, fallbackLocalOperator),
      principalLabel: nonEmptyString(principal.display_name ?? principal.displayName, defaultConsoleSession.principalLabel),
      tenantLabel: nonEmptyString(tenant.label, defaultConsoleSession.tenantLabel),
      workspaceLabel: nonEmptyString(workspace.label, defaultConsoleSession.workspaceLabel),
      authMethod: authMethod === 'unknown' ? defaultConsoleSession.authMethod : authMethod,
    }
  }

  return normalizeConsoleSession({
    mode,
    principalId: principal.subject_id ?? principal.subjectId,
    principalLabel: principal.display_name ?? principal.displayName,
    tenantId: tenant.id ?? workspace.tenant_id ?? workspace.tenantId,
    tenantLabel: tenant.label,
    workspaceId: workspace.id,
    workspaceLabel: workspace.label,
    authMethod,
  }, operator)
}

export function consoleSessionModeLabel(session: ConsoleSessionSettings, language: ConsoleLanguage) {
  if (session.mode === 'multi_user') return language === 'zh' ? '多用户 Web' : 'Multi-user Web'
  return language === 'zh' ? '本地 Daemon' : 'Local Daemon'
}

export function consoleSessionAuthLabel(session: ConsoleSessionSettings, language: ConsoleLanguage) {
  switch (session.authMethod) {
    case 'web_session':
      return language === 'zh' ? '服务端 Web Session' : 'Server-side web session'
    case 'local_bearer':
      return language === 'zh' ? '本地 Bearer Token' : 'Local bearer token'
    default:
      return language === 'zh' ? '未知认证方式' : 'Unknown auth method'
  }
}

export function consoleSessionIdentityLabel(session: ConsoleSessionSettings, language: ConsoleLanguage) {
  if (session.mode === 'local') return language === 'zh' ? '本地用户' : session.principalLabel
  return session.principalLabel || (language === 'zh' ? 'Web 用户' : 'Web User')
}

export function consoleSessionScopeLabel(session: ConsoleSessionSettings) {
  return `${session.tenantId}/${session.workspaceId}`
}

export function consoleSessionWorkspaceLabel(session: ConsoleSessionSettings) {
  if (session.workspaceLabel && session.workspaceLabel !== session.workspaceId) {
    return `${session.workspaceLabel} (${session.workspaceId})`
  }
  return session.workspaceId
}

export function isConsoleMultiUserLoginEnabled(session: ConsoleSessionSettings) {
  return session.mode === 'multi_user' && session.authMethod === 'web_session'
}

function normalizeSessionMode(value: unknown): ConsoleSessionMode {
  return String(value || '').trim() === 'multi_user' ? 'multi_user' : 'local'
}

function normalizeAuthMethod(value: unknown): ConsoleSessionAuthMethod {
  switch (String(value || '').trim()) {
    case 'web_session':
      return 'web_session'
    case 'local_bearer':
      return 'local_bearer'
    default:
      return 'unknown'
  }
}

function nonEmptyString(value: unknown, fallback: string) {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback
}

function safeSegment(value: unknown, fallback: string) {
  const candidate = nonEmptyString(value, fallback)
  if (candidate === '.' || candidate === '..' || candidate.startsWith('.')) return fallback
  if (!safeSegmentPattern.test(candidate)) return fallback
  return candidate
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {}
}
