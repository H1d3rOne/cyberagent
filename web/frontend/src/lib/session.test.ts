import { describe, expect, it } from 'vitest'
import {
  consoleSessionAuthLabel,
  consoleSessionIdentityLabel,
  consoleSessionModeLabel,
  consoleSessionScopeLabel,
  consoleSessionWorkspaceLabel,
  consoleSessionFromServerIdentity,
  isConsoleMultiUserLoginEnabled,
  normalizeConsoleSession,
} from './session'

describe('console session helpers', () => {
  it('normalizes missing settings to local/default local bearer mode', () => {
    const session = normalizeConsoleSession(undefined)

    expect(session).toMatchObject({
      mode: 'local',
      principalId: 'local-user',
      tenantId: 'local',
      workspaceId: 'default',
      authMethod: 'local_bearer',
    })
    expect(consoleSessionScopeLabel(session)).toBe('local/default')
    expect(consoleSessionModeLabel(session, 'zh')).toBe('本地 Daemon')
    expect(consoleSessionAuthLabel(session, 'en')).toBe('Local bearer token')
  })

  it('derives the local principal id from the current operator without changing scope', () => {
    const session = normalizeConsoleSession({ mode: 'local', tenantId: 'evil', workspaceId: 'other' }, 'analyst_1')

    expect(session.principalId).toBe('analyst_1')
    expect(session.tenantId).toBe('local')
    expect(session.workspaceId).toBe('default')
    expect(consoleSessionIdentityLabel(session, 'en')).toBe('Local User')
  })

  it('preserves explicit future multi-user scope with safe path segments', () => {
    const session = normalizeConsoleSession({
      mode: 'multi_user',
      principal_id: 'alice',
      principal_label: 'Alice Admin',
      tenant_id: 'acme',
      tenant_label: 'Acme Security',
      workspace_id: 'prod_api',
      workspace_label: 'Production API',
      auth_method: 'web_session',
    })

    expect(session).toMatchObject({
      mode: 'multi_user',
      principalId: 'alice',
      principalLabel: 'Alice Admin',
      tenantId: 'acme',
      workspaceId: 'prod_api',
      authMethod: 'web_session',
    })
    expect(consoleSessionWorkspaceLabel(session)).toBe('Production API (prod_api)')
    expect(consoleSessionScopeLabel(session)).toBe('acme/prod_api')
    expect(isConsoleMultiUserLoginEnabled(session)).toBe(true)
  })

  it('rejects unsafe future multi-user path segments', () => {
    const session = normalizeConsoleSession({
      mode: 'multi_user',
      principalId: '../alice',
      tenantId: '.hidden',
      workspaceId: 'prod/api',
      authMethod: 'cookie',
    })

    expect(session.principalId).toBe('local-user')
    expect(session.tenantId).toBe('tenant')
    expect(session.workspaceId).toBe('default')
    expect(session.authMethod).toBe('unknown')
    expect(isConsoleMultiUserLoginEnabled(session)).toBe(false)
  })

  it('hydrates local/default scope from the server identity endpoint without trusting workspace input', () => {
    const session = consoleSessionFromServerIdentity({
      principal: {
        subject_id: 'local-user',
        display_name: 'Local Operator',
        auth_method: 'local_bearer',
      },
      tenant: { id: 'local', label: 'Local', mode: 'local' },
      workspace: { id: 'default', tenant_id: 'local', label: 'default', mode: 'local' },
      session: { mode: 'local', auth_method: 'local_bearer' },
    }, 'browser-operator')

    expect(session).toMatchObject({
      mode: 'local',
      principalId: 'local-user',
      principalLabel: 'Local Operator',
      tenantId: 'local',
      workspaceId: 'default',
      authMethod: 'local_bearer',
    })
    expect(consoleSessionScopeLabel(session)).toBe('local/default')
    expect(normalizeConsoleSession(session, 'browser-operator').principalId).toBe('local-user')
  })

  it('falls back to safe local/default scope for malformed server identity payloads', () => {
    const session = consoleSessionFromServerIdentity({
      principal: { subject_id: '../operator', display_name: '' },
      tenant: { id: 'evil', label: '' },
      workspace: { id: 'prod/api', label: '' },
      session: { mode: 'local', auth_method: 'cookie' },
    }, 'analyst_2')

    expect(session).toMatchObject({
      mode: 'local',
      principalId: 'local-user',
      principalLabel: 'Local User',
      tenantId: 'local',
      workspaceId: 'default',
      authMethod: 'local_bearer',
    })
  })

  it('maps future multi-user server identity scope through safe segment normalization', () => {
    const session = consoleSessionFromServerIdentity({
      principal: { subject_id: 'alice', display_name: 'Alice' },
      tenant: { id: 'acme', label: 'Acme', mode: 'multi_user' },
      workspace: { id: 'blue_team', tenant_id: 'acme', label: 'Blue Team' },
      session: { mode: 'multi_user', auth_method: 'web_session' },
    })

    expect(session).toMatchObject({
      mode: 'multi_user',
      principalId: 'alice',
      tenantId: 'acme',
      workspaceId: 'blue_team',
      authMethod: 'web_session',
    })
    expect(isConsoleMultiUserLoginEnabled(session)).toBe(true)
  })
})
