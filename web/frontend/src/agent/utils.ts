import { ApiError, normalizeAgentOperationLevel, type AgentApprovalState, type AgentAuthorizationRequest, type AgentMessage, type AgentNextAction, type AgentOperationLevel, type AgentSession, type AgentSessionSummary, type AgentToolCall, type AgentTodo, type AgentUIState } from '@/lib/api'
import { text, type Language } from '@/lib/console'

export type CommandSummary = {
  command: string
  exitCode?: string
  workdir?: string
  duration?: string
  stdout?: string
  stderr?: string
}

export interface AuthorizationGate {
  id: string
  action: string
  prompt?: string
  target?: string
  scope?: string
  policy?: string
  reason?: string
  arguments?: Record<string, unknown>
  requiredLevel: AgentOperationLevel
  source: 'pending' | 'legacy'
  messageIndex?: number
  stateLabel?: string
  stateDetail?: string
  uiHint?: string
}

export interface AgentSessionDisplayHint {
  label: string
  detail?: string
  tone: 'muted' | 'info' | 'success' | 'warning' | 'danger'
  source?: string
}

export function sessionTitle(item: AgentSessionSummary | AgentSession, language: Language) {
  const activeTask = 'active_task' in item ? item.active_task : undefined
  const lastMessage = 'last_message' in item ? item.last_message : ''
  const goal = activeTask?.goal || activeTask?.summary || lastMessage
  if (goal?.trim()) return truncate(goal.trim(), 42)
  const created = Date.parse(item.created_at)
  const label = Number.isFinite(created) ? new Date(created).toLocaleString() : item.session_id
  return text(language, `会话 ${label}`, `Session ${label}`)
}

export function roleLabel(role: string, language: Language) {
  if (role === 'user') return text(language, '你', 'You')
  if (role === 'tool') return text(language, '工具', 'Tool')
  if (role === 'system') return text(language, '系统', 'System')
  return 'Agent'
}

export function originLabel(origin: string | undefined, language: Language) {
  const normalized = String(origin || '').toLowerCase()
  const zh: Record<string, string> = { user: '手动', schedule: '定时', resume: '恢复', subagent: '子任务' }
  const en: Record<string, string> = { user: 'User', schedule: 'Schedule', resume: 'Resume', subagent: 'Subagent' }
  return language === 'zh' ? zh[normalized] || (origin || '手动') : en[normalized] || (origin || 'User')
}

export function statusLabel(status: string | undefined, language: Language) {
  const raw = String(status || 'idle')
  const normalized = raw.toLowerCase()
  const zh: Record<string, string> = {
    idle: '空闲',
    running: '运行中',
    pending: '等待',
    queued: '队列中',
    done: '完成',
    completed: '完成',
    succeeded: '成功',
    finished: '结束',
    failed: '失败',
    error: '错误',
    blocked: '阻塞',
    skipped: '跳过',
    wait_user: '等输入',
    approval_required: '待审批',
    authorization_required: '待授权',
    waiting_authorization: '待授权',
    paused: '暂停',
    cancelled: '取消',
    stopped: '停止',
    connecting: '连接中',
    online: '在线',
    offline: '离线',
    degraded: '降级',
  }
  const en: Record<string, string> = {
    idle: 'Idle',
    running: 'Running',
    pending: 'Pending',
    queued: 'Queued',
    done: 'Done',
    completed: 'Completed',
    succeeded: 'Succeeded',
    finished: 'Finished',
    failed: 'Failed',
    error: 'Error',
    blocked: 'Blocked',
    skipped: 'Skipped',
    wait_user: 'Waiting',
    approval_required: 'Approval',
    authorization_required: 'Authorization',
    waiting_authorization: 'Authorization',
    paused: 'Paused',
    cancelled: 'Cancelled',
    stopped: 'Stopped',
    connecting: 'Connecting',
    online: 'Online',
    offline: 'Offline',
    degraded: 'Degraded',
  }
  return language === 'zh' ? zh[normalized] || raw : en[normalized] || raw
}

export function operationLevelLabel(level: unknown, language: Language) {
  const normalized = normalizeAgentOperationLevel(level)
  const labels: Record<AgentOperationLevel, { zh: string; en: string }> = {
    read_only: { zh: '只读', en: 'Read only' },
    workspace: { zh: '工作区', en: 'Workspace' },
    workspace_write: { zh: '工作区写入', en: 'Workspace write' },
    safe_scan: { zh: '安全扫描', en: 'Safe scan' },
    approval_ops: { zh: '审批操作', en: 'Approval ops' },
    advanced: { zh: '高级', en: 'Advanced' },
    bypass: { zh: 'Bypass', en: 'Bypass' },
  }
  return labels[normalized][language]
}

export function statusPillClass(status: string | undefined) {
  const normalized = String(status || '').toLowerCase()
  if (['succeeded', 'completed', 'done', 'ok', 'finished', 'online'].includes(normalized)) return 'border-emerald-400/25 bg-emerald-400/10 text-emerald-200'
  if (['failed', 'error', 'cancelled', 'stopped', 'offline'].includes(normalized)) return 'border-red-400/25 bg-red-400/10 text-red-200'
  if (['running', 'queued', 'connecting'].includes(normalized)) return 'border-primary/25 bg-primary/10 text-primary'
  if (['approval_required', 'authorization_required', 'waiting_authorization', 'blocked', 'degraded'].includes(normalized)) return 'border-amber-400/25 bg-amber-400/10 text-amber-200'
  return 'border-white/10 bg-white/[0.04] text-muted-foreground'
}

export function toolDotClass(status: string | undefined) {
  const normalized = String(status || '').toLowerCase()
  if (['succeeded', 'completed', 'done', 'ok', 'finished'].includes(normalized)) return 'bg-emerald-300 shadow-[0_0_10px_rgba(110,231,183,.7)]'
  if (['failed', 'error', 'cancelled', 'stopped'].includes(normalized)) return 'bg-red-300 shadow-[0_0_10px_rgba(252,165,165,.7)]'
  if (['running', 'queued', 'pending'].includes(normalized)) return 'bg-primary shadow-[0_0_10px_rgba(96,165,250,.7)]'
  return 'bg-slate-500'
}

export function todoDotClass(status: string | undefined) {
  const normalized = String(status || '').toLowerCase()
  if (['done', 'finished', 'completed', 'succeeded'].includes(normalized)) return 'bg-emerald-300'
  if (['blocked', 'failed', 'error'].includes(normalized)) return 'bg-red-300'
  if (['running', 'in_progress'].includes(normalized)) return 'bg-primary'
  if (['approval_required', 'waiting'].includes(normalized)) return 'bg-amber-300'
  return 'bg-slate-500'
}

export function formatTime(value: string | undefined) {
  if (!value) return '—'
  const parsed = Date.parse(value)
  if (!Number.isFinite(parsed)) return value
  return new Date(parsed).toLocaleTimeString()
}

export function formatDateTime(value: string | undefined) {
  if (!value) return '—'
  const parsed = Date.parse(value)
  if (!Number.isFinite(parsed)) return value
  return new Date(parsed).toLocaleString()
}

export function shortId(value: string | undefined, max = 12) {
  const raw = String(value || '')
  return raw.length > max ? `${raw.slice(0, Math.max(4, max - 2))}…` : raw
}

export function truncate(value: string, max: number) {
  return value.length > max ? `${value.slice(0, max)}…` : value
}

export function safeJson(value: unknown) {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

export function formatError(err: unknown, language: Language) {
  if (err instanceof ApiError) {
    return err.status === 401
      ? text(language, 'Daemon token 无效或缺失，请在设置页配置。', 'Daemon token is invalid or missing. Configure it in Settings.')
      : `${err.message}${err.code ? ` (${err.code})` : ''}`
  }
  return err instanceof Error ? err.message : text(language, '请求失败', 'Request failed')
}

export function parseCommandSummary(content: string): CommandSummary | null {
  if (!/^(命令已执行|Command executed)\b/.test(content.trim())) return null
  const command = matchLine(content, /-\s*command:\s*`([^`]+)`/i)
  if (!command) return null
  return {
    command,
    exitCode: matchLine(content, /-\s*exit_code:\s*([^\n]+)/i),
    workdir: matchLine(content, /-\s*workdir:\s*`([^`]+)`/i),
    duration: matchLine(content, /-\s*duration:\s*([^\n]+)/i),
    stdout: matchFence(content, /stdout:\s*```(?:text)?\s*([\s\S]*?)```/i),
    stderr: matchFence(content, /stderr:\s*```(?:text)?\s*([\s\S]*?)```/i),
  }
}

export function commandFromToolCall(call: AgentToolCall): CommandSummary | null {
  if (!call.result) return null
  const command = String(call.result.command || '').trim()
  if (!command) return null
  return {
    command,
    exitCode: call.result.exit_code === undefined ? undefined : String(call.result.exit_code),
    workdir: String(call.result.workdir || ''),
    duration: call.result.duration_ms === undefined ? undefined : `${call.result.duration_ms}ms`,
    stdout: String(call.result.stdout || ''),
    stderr: String(call.result.stderr || ''),
  }
}

export function messageText(message: AgentMessage) {
  return String(message.content || '').trim()
}

export function plannedTodos(todos: AgentTodo[]) {
  return todos.filter((todo) => ['pending', 'todo', 'planned', 'running', 'in_progress', 'blocked', 'waiting', 'approval_required'].includes(String(todo.status || '').toLowerCase()))
}

export function nextActionSuggestedPrompt(nextAction: AgentNextAction | undefined) {
  if (!nextAction) return ''
  const record = nextAction as AgentNextAction & { suggest_prompt?: unknown; prompt?: unknown }
  for (const value of [record.suggested_prompt, record.suggest_prompt, record.prompt]) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

export function sessionDisplayHint(session: AgentSession | undefined | null, language: Language): AgentSessionDisplayHint | null {
  const uiState = session?.ui_state
  const approvalState = session?.approval_state
  if (!uiState && !approvalState) return null

  const uiLabel = uiState?.state ? uiStateLabel(uiState.state, language) : ''
  const approvalLabel = approvalState?.state || approvalState?.decision
    ? approvalStateLabel(approvalState.state || approvalState.decision, language)
    : ''
  const label = approvalState && String(uiState?.state || '').toLowerCase() === 'approval'
    ? approvalLabel || uiLabel
    : uiLabel || approvalLabel
  if (!label) return null

  const detail = firstDisplayText(
    uiState?.message,
    approvalState?.reason,
    approvalState?.action_name,
    uiState?.tool_name,
    uiState?.phase,
  )
  if (!approvalState && String(uiState?.state || '').toLowerCase() === 'idle' && !detail) return null
  return {
    label,
    detail: detail ? truncate(detail, 96) : undefined,
    tone: uiStateTone(uiState, approvalState),
    source: firstDisplayText(uiState?.source, approvalState?.approval_kind),
  }
}

export function buildPendingAuthorizationGate(
  auth: AgentAuthorizationRequest | undefined,
  language: Language,
  hints?: { approvalState?: AgentApprovalState; uiState?: AgentUIState },
): AuthorizationGate | null {
  if (!auth || String(auth.status || '').toLowerCase() !== 'pending') return null
  const args = recordFromUnknown(auth.arguments)
  const action = String(auth.action_name || 'agent.action')
  const target = agentAuthorizationTargetFromArgs(args)
  const scope = String(args.scope || args.scope_url || '').trim()
  const stateHint = authorizationStateHint(hints?.approvalState, hints?.uiState, language)
  return {
    id: auth.id,
    action,
    target,
    scope,
    policy: auth.policy,
    reason: auth.reason,
    arguments: args,
    requiredLevel: normalizeAgentOperationLevel(auth.required_level || requiredLevelForDeniedAction(action)),
    prompt: auth.original_prompt || buildAuthorizationRetryPrompt('', action, target, scope, language),
    source: 'pending',
    stateLabel: stateHint?.label,
    stateDetail: stateHint?.detail,
    uiHint: stateHint?.hint,
  }
}

export function buildLegacyAuthorizationGate(messages: AgentMessage[], language: Language): AuthorizationGate | null {
  let latestUserIndex = -1
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    if (messages[i].role === 'user') {
      latestUserIndex = i
      break
    }
  }

  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const calls = messages[i].tool_calls || []
    for (let j = calls.length - 1; j >= 0; j -= 1) {
      const call = calls[j]
      if (!isExplicitOperatorIntentDenied(call)) continue
      if (latestUserIndex > i) return null

      const originalPrompt = lastUserPromptBefore(messages, i)
      const action = toolCallAction(call)
      const target = inferNetworkTargetForPrompt(originalPrompt)
      const scope = inferScopeForPrompt(originalPrompt, target)
      return {
        id: ['legacy', i, action, target || '', scope || ''].join('|'),
        action,
        target,
        scope,
        requiredLevel: requiredLevelForDeniedAction(action),
        prompt: buildAuthorizationRetryPrompt(originalPrompt, action, target, scope, language),
        source: 'legacy',
        messageIndex: i,
      }
    }
  }
  return null
}

export function authorizationGateKey(gate: AuthorizationGate) {
  return gate.id || [gate.source, gate.messageIndex ?? '', gate.action, gate.target || '', gate.scope || ''].join('|')
}

function authorizationStateHint(approvalState: AgentApprovalState | undefined, uiState: AgentUIState | undefined, language: Language) {
  if (!approvalState && !uiState) return null
  const label = approvalState?.state || approvalState?.decision
    ? approvalStateLabel(approvalState.state || approvalState.decision, language)
    : uiState?.state
      ? uiStateLabel(uiState.state, language)
      : ''
  const detail = firstDisplayText(approvalState?.reason, approvalState?.action_name, uiState?.message)
  const hint = firstDisplayText(uiState?.message)
  return label || detail || hint
    ? {
        label,
        detail: detail ? truncate(detail, 96) : undefined,
        hint: hint ? truncate(hint, 96) : undefined,
      }
    : null
}

function uiStateLabel(state: unknown, language: Language) {
  const raw = String(state || '').trim()
  const normalized = raw.toLowerCase()
  const zh: Record<string, string> = {
    idle: '空闲',
    running: '运行中',
    failed: '失败',
    done: '完成',
    approval: '待审批',
    tool_running: '工具运行中',
  }
  const en: Record<string, string> = {
    idle: 'Idle',
    running: 'Running',
    failed: 'Failed',
    done: 'Done',
    approval: 'Approval',
    tool_running: 'Tool running',
  }
  return language === 'zh' ? zh[normalized] || raw : en[normalized] || raw
}

function approvalStateLabel(state: unknown, language: Language) {
  const raw = String(state || '').trim()
  const normalized = raw.toLowerCase()
  const zh: Record<string, string> = {
    pending: '审批待处理',
    approved: '已批准',
    denied: '已拒绝',
    cancelled: '已取消',
    canceled: '已取消',
    modified: '已修改',
    expired: '已过期',
    unknown: '审批状态未知',
  }
  const en: Record<string, string> = {
    pending: 'Approval pending',
    approved: 'Approved',
    denied: 'Denied',
    cancelled: 'Cancelled',
    canceled: 'Cancelled',
    modified: 'Modified',
    expired: 'Expired',
    unknown: 'Approval unknown',
  }
  return language === 'zh' ? zh[normalized] || raw : en[normalized] || raw
}

function uiStateTone(uiState: AgentUIState | undefined, approvalState: AgentApprovalState | undefined): AgentSessionDisplayHint['tone'] {
  const explicit = String(uiState?.tone || '').toLowerCase()
  if (['info', 'success', 'warning', 'danger', 'muted'].includes(explicit)) return explicit as AgentSessionDisplayHint['tone']
  if (explicit === 'neutral') return 'muted'

  const approval = String(approvalState?.state || approvalState?.decision || '').toLowerCase()
  if (approval === 'pending') return 'warning'
  if (['approved', 'modified'].includes(approval)) return 'success'
  if (['denied', 'cancelled', 'canceled', 'expired'].includes(approval)) return 'danger'

  const state = String(uiState?.state || '').toLowerCase()
  if (['approval'].includes(state)) return 'warning'
  if (['failed'].includes(state)) return 'danger'
  if (['done'].includes(state)) return 'success'
  if (['running', 'tool_running'].includes(state)) return 'info'
  return 'muted'
}

function firstDisplayText(...values: unknown[]) {
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  }
  return ''
}

function matchLine(value: string, pattern: RegExp) {
  return value.match(pattern)?.[1]?.trim()
}

function matchFence(value: string, pattern: RegExp) {
  return value.match(pattern)?.[1]?.replace(/\n$/, '').trim()
}

function buildAuthorizationRetryPrompt(originalPrompt: string, action: string, target: string, scope: string, language: Language) {
  const targetText = target || text(language, '当前目标', 'the current target')
  const scopeText = scope || target
  const actionText = action.includes('http.request')
    ? text(language, '低风险 HTTP 请求', 'low-risk HTTP request')
    : action.includes('scan')
      ? text(language, 'safe profile Web/API 扫描', 'safe-profile Web/API scan')
      : action

  if (language === 'en') {
    return [
      `I confirm that I am authorized to operate on ${targetText}${scopeText ? ` within scope=${scopeText}` : ''}.`,
      `Please execute ${actionText} now and continue the original task: ${originalPrompt || action}.`,
    ].join(' ')
  }
  return [
    `我确认已获得授权，可以在${scopeText ? ` scope=${scopeText} 内` : '当前授权范围内'}对授权目标 ${targetText} 执行${actionText}。`,
    `请立即继续执行原任务：${originalPrompt || action}。`,
  ].join('')
}

function isExplicitOperatorIntentDenied(call: AgentToolCall) {
  const result = recordFromUnknown(call.result)
  const details = recordFromUnknown(result.details)
  const policy = String(details.policy || result.policy || '').trim()
  return String(call.status || '').toLowerCase() === 'failed' && policy === 'explicit_operator_intent_required'
}

function toolCallAction(call: AgentToolCall) {
  const result = recordFromUnknown(call.result)
  const details = recordFromUnknown(result.details)
  const metadata = recordFromUnknown(call.metadata)
  return String(result.action || details.action || metadata.action_name || call.name || 'agent.action')
}

function lastUserPromptBefore(messages: AgentMessage[], index: number) {
  for (let i = Math.min(index, messages.length - 1); i >= 0; i -= 1) {
    if (messages[i].role === 'user' && messages[i].content?.trim()) return messages[i].content.trim()
  }
  return ''
}

function inferNetworkTargetForPrompt(prompt: string) {
  const explicit = prompt.match(FRONTEND_HTTP_URL_PATTERN)?.[0]
  if (explicit) return cleanPromptToken(explicit)
  FRONTEND_BARE_NETWORK_TARGET_PATTERN.lastIndex = 0
  for (const match of prompt.matchAll(FRONTEND_BARE_NETWORK_TARGET_PATTERN)) {
    const candidate = cleanPromptToken(match[1] || '')
    if (looksLikeBareNetworkTarget(candidate)) return `https://${candidate}`
  }
  return ''
}

function inferScopeForPrompt(prompt: string, target: string) {
  const scopeMatch = prompt.match(/(?:scope|范围)\s*[:=：]\s*([^\s,，;；]+)/i)
  if (scopeMatch?.[1]) return cleanPromptToken(scopeMatch[1])
  try {
    return target ? new URL(target).host : ''
  } catch {
    return target.replace(/^https?:\/\//i, '').split('/')[0] || ''
  }
}

function cleanPromptToken(value: string) {
  return String(value || '').trim().replace(/^[`"'“”]+|[`"'“”，。,)\]]+$/g, '')
}

function looksLikeBareNetworkTarget(value: string) {
  const clean = cleanPromptToken(value)
  if (!clean || clean.includes('://') || /\s/.test(clean)) return false
  const host = clean.split('/')[0].replace(/:\d{1,5}$/, '')
  if (!host.includes('.') || host.startsWith('.') || host.endsWith('.')) return false
  const tld = host.slice(host.lastIndexOf('.') + 1).toLowerCase()
  if (tld.length < 2 || looksLikeWorkspaceExtension(tld)) return false
  return /^[a-z0-9.-]+$/i.test(host)
}

function looksLikeWorkspaceExtension(ext: string) {
  return new Set([
    'md', 'txt', 'log', 'json', 'yaml', 'yml', 'toml', 'xml', 'csv',
    'go', 'py', 'js', 'jsx', 'ts', 'tsx', 'css', 'scss', 'html', 'htm',
    'sh', 'bash', 'zsh', 'sql', 'rs', 'java', 'c', 'cc', 'cpp', 'h',
    'png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'pdf', 'zip', 'gz', 'tar',
    'list', 'context', 'draft', 'safe', 'command', 'files', 'approvals', 'plugins', 'runs', 'web', 'request', 'health',
  ]).has(ext.toLowerCase())
}

function requiredLevelForDeniedAction(action: string) {
  const normalized = action.toLowerCase()
  if (normalized.includes('approval') || normalized.includes('resume_approved')) return 'approval_ops'
  if (normalized.includes('scan.web') || normalized.includes('http.request')) return 'safe_scan'
  if (normalized.includes('workspace.files.write') || normalized.includes('workspace.files.patch') ||
    normalized.includes('workspace.files.diff') || normalized.includes('workspace.files.apply') ||
    normalized.includes('workspace.files.delete') || normalized.includes('workspace.files.move') ||
    normalized.includes('workspace.files.copy') || normalized.includes('workspace.files.mkdir') ||
    normalized.includes('reports.draft')) return 'workspace_write'
  return 'read_only'
}

function agentAuthorizationTargetFromArgs(args: Record<string, unknown>) {
  const values = [args.target, args.url, args.input, args.path, args.file, args.command, args.run_id]
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function recordFromUnknown(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

const FRONTEND_HTTP_URL_PATTERN = /https?:\/\/[^\s"'<>，。)]+/i
const FRONTEND_BARE_NETWORK_TARGET_PATTERN = /(?:^|[\s（(])((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}(?::\d{1,5})?(?:\/[^\s"'<>，。)]*)?)/ig
