const SETTINGS_KEY = 'cyberagent_console_settings'
const RECENT_JOBS_KEY = 'cyberagent_recent_jobs'

export type ApiStatus = 'idle' | 'loading' | 'ok' | 'error'
export type DaemonConnectionStatus = 'online' | 'unauthorized' | 'offline'
export type ScanStatus = 'running' | 'completed' | 'failed' | 'stopped' | string
export type RunStatus = 'created' | 'planning' | 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'skipped' | string
export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | string
export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'modified' | 'expired' | 'cancelled' | string
export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info' | string
export type AgentOperationLevel = 'read_only' | 'workspace' | 'workspace_write' | 'safe_scan' | 'approval_ops' | 'advanced' | 'bypass'
export type ApiAuthProfileType = 'bearer' | 'basic' | 'api_key' | 'cookie' | 'custom'
export type AgentProviderType = 'openai' | 'anthropic'

export interface AgentProviderProfile {
  id: string
  name: string
  type: AgentProviderType
  model: string
  apiKey: string
  baseUrl: string
  instructions: string
  maxTurns: number
}

export interface AgentProviderSettings {
  provider: string
  model: string
  apiKey: string
  providerBaseUrl: string
  instructions: string
  maxTurns: number
}

export interface AgentAuthorizationRequest {
  id: string
  action_name: string
  arguments?: Record<string, unknown>
  required_level?: AgentOperationLevel | string
  policy?: string
  reason?: string
  original_prompt?: string
  status: 'pending' | 'approved' | 'denied' | string
  created_at: string
  updated_at: string
}

export interface ScanDraft {
  target: string
  scope: string
  multiAgent: boolean
  interactive: boolean
}

export interface ApiAuthProfile {
  id: string
  name: string
  type: ApiAuthProfileType
  baseUrl: string
  token: string
  username: string
  password: string
  headerName: string
  headerValue: string
  cookie: string
  enabled: boolean
}

export interface ConsoleSettings {
  baseUrl: string
  token: string
  operator: string
  language: 'zh' | 'en'
  operationLevel: AgentOperationLevel
  agentProvider: AgentProviderSettings
  aiProviders: AgentProviderProfile[]
  activeAiProviderId: string
  apiAuthProfiles: ApiAuthProfile[]
  defaultApiAuthProfileId: string
  scanDraft: ScanDraft
  autoContinueLimit: number
}

export interface LocalDaemonBootstrap {
  available: boolean
  base_url: string
  token?: string
  token_file?: string
  message?: string
}

export const defaultAgentProvider: AgentProviderSettings = {
  provider: 'openai',
  model: 'gpt-4o',
  apiKey: '',
  providerBaseUrl: '',
  instructions: '',
  maxTurns: 50,
}

export const defaultAgentProviderProfile: AgentProviderProfile = {
  id: 'openai-default',
  name: 'OpenAI Compatible',
  type: 'openai',
  model: 'gpt-4o',
  apiKey: '',
  baseUrl: '',
  instructions: '',
  maxTurns: 50,
}

export const defaultScanDraft: ScanDraft = {
  target: '',
  scope: '',
  multiAgent: false,
  interactive: false,
}

export const defaultApiAuthProfile: ApiAuthProfile = {
  id: '',
  name: '',
  type: 'bearer',
  baseUrl: '',
  token: '',
  username: '',
  password: '',
  headerName: '',
  headerValue: '',
  cookie: '',
  enabled: true,
}

export const defaultSettings: ConsoleSettings = {
  baseUrl: 'http://127.0.0.1:8787',
  token: '',
  operator: 'local-user',
  language: 'zh',
  operationLevel: 'read_only',
  agentProvider: defaultAgentProvider,
  aiProviders: [defaultAgentProviderProfile],
  activeAiProviderId: defaultAgentProviderProfile.id,
  apiAuthProfiles: [],
  defaultApiAuthProfileId: '',
  scanDraft: defaultScanDraft,
  autoContinueLimit: 3,
}

function stringOrDefault(value: unknown, fallback: string) {
  return typeof value === 'string' ? value : fallback
}

function nonEmptyStringOrDefault(value: unknown, fallback: string) {
  return typeof value === 'string' && value.trim() ? value : fallback
}

function normalizeMaxTurns(value: unknown) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return defaultAgentProvider.maxTurns
  return Math.min(200, Math.max(1, Math.round(parsed)))
}

function normalizeAutoContinueLimit(value: unknown) {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return defaultSettings.autoContinueLimit
  return Math.min(10, Math.max(0, Math.round(parsed)))
}

function normalizeAgentProviderType(value: unknown): AgentProviderType {
  const normalized = String(value || '').trim().toLowerCase()
  return normalized.includes('anthropic') || normalized.includes('claude') ? 'anthropic' : 'openai'
}

function normalizeAgentProviderProfiles(value: unknown, fallback: AgentProviderSettings): AgentProviderProfile[] {
  if (Array.isArray(value)) {
    const profiles = value.slice(0, 20).map((raw, index) => {
      const item = (raw && typeof raw === 'object' ? raw : {}) as Partial<AgentProviderProfile> & Record<string, unknown>
      const type = normalizeAgentProviderType(item.type ?? item.provider)
      const id = stringOrDefault(item.id, `ai_provider_${index + 1}`).trim() || `ai_provider_${index + 1}`
      const name = stringOrDefault(item.name, type === 'anthropic' ? 'Anthropic Compatible' : 'OpenAI Compatible')
      return {
        id,
        name,
        type,
        model: stringOrDefault(item.model, type === 'anthropic' ? 'claude-sonnet-4-20250514' : 'gpt-4o'),
        apiKey: stringOrDefault(item.apiKey ?? item.api_key, ''),
        baseUrl: stringOrDefault(item.baseUrl ?? item.base_url ?? item.providerBaseUrl, ''),
        instructions: stringOrDefault(item.instructions, ''),
        maxTurns: normalizeMaxTurns(item.maxTurns ?? item.max_turns),
      }
    }).filter((profile) => profile.id.trim())
    if (profiles.length) return profiles
  }
  const legacyType = normalizeAgentProviderType(fallback.provider)
  return [{
    id: `legacy-${legacyType}`,
    name: legacyType === 'anthropic' ? 'Anthropic Compatible' : 'OpenAI Compatible',
    type: legacyType,
    model: fallback.model || (legacyType === 'anthropic' ? 'claude-sonnet-4-20250514' : 'gpt-4o'),
    apiKey: fallback.apiKey,
    baseUrl: fallback.providerBaseUrl,
    instructions: fallback.instructions,
    maxTurns: fallback.maxTurns,
  }]
}

function agentProviderSettingsFromProfile(profile: AgentProviderProfile): AgentProviderSettings {
  return {
    provider: profile.type,
    model: profile.model,
    apiKey: profile.apiKey,
    providerBaseUrl: profile.baseUrl,
    instructions: profile.instructions,
    maxTurns: profile.maxTurns,
  }
}

export function normalizeAgentOperationLevel(value: unknown): AgentOperationLevel {
  switch (String(value || '').trim()) {
    case 'workspace':
    case 'workspace_write':
    case 'safe_scan':
    case 'approval_ops':
    case 'advanced':
    case 'bypass':
      return String(value).trim() as AgentOperationLevel
    default:
      return 'read_only'
  }
}

function normalizeApiAuthProfileType(value: unknown): ApiAuthProfileType {
  switch (String(value || '').trim()) {
    case 'basic':
      return 'basic'
    case 'api_key':
    case 'apikey':
    case 'api-key':
      return 'api_key'
    case 'cookie':
      return 'cookie'
    case 'custom':
    case 'header':
      return 'custom'
    default:
      return 'bearer'
  }
}

function normalizeApiAuthProfiles(value: unknown): ApiAuthProfile[] {
  if (!Array.isArray(value)) return []
  return value.slice(0, 12).map((raw, index) => {
    const item = (raw && typeof raw === 'object' ? raw : {}) as Partial<ApiAuthProfile> & Record<string, unknown>
    const id = stringOrDefault(item.id, `api_auth_${index + 1}`)
    return {
      id,
      name: stringOrDefault(item.name, id),
      type: normalizeApiAuthProfileType(item.type),
      baseUrl: stringOrDefault(item.baseUrl ?? item.base_url, ''),
      token: stringOrDefault(item.token, ''),
      username: stringOrDefault(item.username, ''),
      password: stringOrDefault(item.password, ''),
      headerName: stringOrDefault(item.headerName ?? item.header_name, ''),
      headerValue: stringOrDefault(item.headerValue ?? item.header_value, ''),
      cookie: stringOrDefault(item.cookie, ''),
      enabled: item.enabled !== false,
    }
  }).filter((profile) => profile.id.trim())
}

function normalizeSettings(raw: unknown): ConsoleSettings {
  const value = (raw && typeof raw === 'object' ? raw : {}) as Partial<ConsoleSettings> & {
    scanDefaults?: Partial<AgentProviderSettings & ScanDraft>
  }
  const legacyScanDefaults: Partial<AgentProviderSettings & ScanDraft> =
    value.scanDefaults && typeof value.scanDefaults === 'object' ? value.scanDefaults : {}
  const agentProvider: Partial<AgentProviderSettings> =
    value.agentProvider && typeof value.agentProvider === 'object' ? value.agentProvider : {}
  const scanDraft: Partial<ScanDraft> =
    value.scanDraft && typeof value.scanDraft === 'object' ? value.scanDraft : {}
  const legacyAgentProvider: AgentProviderSettings = {
    provider: stringOrDefault(agentProvider.provider, stringOrDefault(legacyScanDefaults.provider, defaultAgentProvider.provider)),
    model: stringOrDefault(agentProvider.model, stringOrDefault(legacyScanDefaults.model, defaultAgentProvider.model)),
    apiKey: stringOrDefault(agentProvider.apiKey, stringOrDefault(legacyScanDefaults.apiKey, defaultAgentProvider.apiKey)),
    providerBaseUrl: stringOrDefault(
      agentProvider.providerBaseUrl,
      stringOrDefault(legacyScanDefaults.providerBaseUrl, defaultAgentProvider.providerBaseUrl),
    ),
    instructions: stringOrDefault(
      agentProvider.instructions,
      stringOrDefault(legacyScanDefaults.instructions, defaultAgentProvider.instructions),
    ),
    maxTurns: normalizeMaxTurns(agentProvider.maxTurns ?? legacyScanDefaults.maxTurns),
  }
  const aiProviders = normalizeAgentProviderProfiles(value.aiProviders, legacyAgentProvider)
  const activeAiProviderId = stringOrDefault(value.activeAiProviderId, aiProviders[0]?.id || defaultAgentProviderProfile.id)
  const activeAiProvider = aiProviders.find((profile) => profile.id === activeAiProviderId) || aiProviders[0] || defaultAgentProviderProfile
  return {
    baseUrl: nonEmptyStringOrDefault(value.baseUrl, defaultSettings.baseUrl),
    token: stringOrDefault(value.token, defaultSettings.token),
    operator: stringOrDefault(value.operator, defaultSettings.operator),
    language: value.language === 'en' ? 'en' : 'zh',
    operationLevel: normalizeAgentOperationLevel(value.operationLevel),
    agentProvider: agentProviderSettingsFromProfile(activeAiProvider),
    aiProviders,
    activeAiProviderId: activeAiProvider.id,
    apiAuthProfiles: normalizeApiAuthProfiles(value.apiAuthProfiles),
    defaultApiAuthProfileId: stringOrDefault(value.defaultApiAuthProfileId, ''),
    scanDraft: {
      target: stringOrDefault(scanDraft.target, stringOrDefault(legacyScanDefaults.target, defaultScanDraft.target)),
      scope: stringOrDefault(scanDraft.scope, stringOrDefault(legacyScanDefaults.scope, defaultScanDraft.scope)),
      multiAgent: scanDraft.multiAgent === true || legacyScanDefaults.multiAgent === true,
      interactive: scanDraft.interactive === true || legacyScanDefaults.interactive === true,
    },
    autoContinueLimit: normalizeAutoContinueLimit(value.autoContinueLimit),
  }
}

export function loadSettings(): ConsoleSettings {
  const stored = localStorage.getItem(SETTINGS_KEY)
  if (!stored) return defaultSettings
  try {
    return normalizeSettings(JSON.parse(stored))
  } catch {
    return defaultSettings
  }
}

export function saveSettings(settings: ConsoleSettings) {
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(normalizeSettings(settings)))
}

function shouldSkipLocalDaemonBootstrap() {
  if (typeof window === 'undefined') return false
  const port = window.location.port
  // `/api/local/daemon-config` only exists on the bundled Go dashboard. Vite
  // dev/preview runs the frontend standalone, so probing this endpoint creates
  // noisy 500/404 console errors before the user has configured a daemon.
  return port === '5173' || port === '5174' || port === '5175' || port === '4173'
}

export async function loadLocalDaemonBootstrap(): Promise<LocalDaemonBootstrap | null> {
  if (shouldSkipLocalDaemonBootstrap()) return null
  try {
    const response = await fetch('/api/local/daemon-config', {
      headers: { Accept: 'application/json' },
      cache: 'no-store',
    })
    if (!response.ok) return null
    const body = (await response.json().catch(() => null)) as LocalDaemonBootstrap | null
    if (!body || typeof body !== 'object') return null
    return body
  } catch {
    return null
  }
}

export interface RecentJobRef {
  job_id: string
  run_id: string
  created_at: string
}

export function loadRecentJobs(): Record<string, RecentJobRef> {
  const stored = localStorage.getItem(RECENT_JOBS_KEY)
  if (!stored) return {}
  try {
    return JSON.parse(stored) as Record<string, RecentJobRef>
  } catch {
    return {}
  }
}

export function rememberJob(ref: RecentJobRef) {
  const jobs = loadRecentJobs()
  jobs[ref.run_id] = ref
  localStorage.setItem(RECENT_JOBS_KEY, JSON.stringify(jobs))
}

export function recentJobForRun(runId: string): RecentJobRef | undefined {
  return loadRecentJobs()[runId]
}

export interface ApiErrorPayload {
  code: string
  message: string
  details?: Record<string, unknown>
}

export class ApiError extends Error {
  code: string
  status: number
  details?: Record<string, unknown>

  constructor(status: number, payload: ApiErrorPayload) {
    super(payload.message || payload.code)
    this.name = 'ApiError'
    this.status = status
    this.code = payload.code
    this.details = payload.details
  }
}

interface Envelope<T> {
  ok: boolean
  data: T | null
  error: ApiErrorPayload | null
}

export interface Healthz {
  status: string
  service: string
  time: string
}

export interface VersionInfo {
  version: string
  commit: string
  date: string
  module: string
}

export interface DoctorCheck {
  name: string
  ok?: boolean
  message?: string
  status?: string
}

export interface DoctorReport {
  ok?: boolean
  healthy?: boolean
  checks?: DoctorCheck[]
  [key: string]: unknown
}

export interface PluginManifest {
  id: string
  name: string
  version?: string
  type?: string
  runtime?: string
  description?: string
  capabilities?: string[]
  risk?: string
  requires_approval?: boolean
  supported_profiles?: string[]
  dependencies?: Array<{ name: string; kind?: string; required?: boolean; version_constraint?: string }>
  entrypoint?: { kind?: string; path?: string }
}

export interface PluginDependencyHealth {
  name: string
  available: boolean
  reason?: string
}

export interface PluginHealth {
  id: string
  ok: boolean
  dependencies?: PluginDependencyHealth[]
}

export interface PluginDoctorReport {
  schema_version: string
  ok: boolean
  count: number
  plugins: PluginHealth[]
}

export interface RunCounts {
  events: number
  artifacts: number
  findings: number
  approvals: number
  pending_approvals?: number
}

export interface RunSummary {
  run_id: string
  run_dir: string
  target?: string
  scope?: string
  profile?: string
  status?: RunStatus
  created_at?: string
  updated_at?: string
  report?: {
    json: boolean
    markdown: boolean
  }
  counts?: RunCounts
}

export interface RunListResponse {
  items: RunSummary[]
  next_cursor?: string
  eof: boolean
  limit: number
}

export interface RunDetail {
  run_id: string
  run_dir: string
  metadata?: Record<string, unknown>
  effective_config?: Record<string, unknown>
  job?: {
    id: string
    kind: string
    status: RunStatus
    profile?: string
    created_at?: string
    started_at?: string
    finished_at?: string
    summary?: {
      steps_total?: number
      steps_succeeded?: number
      steps_failed?: number
      findings_total?: number
    }
    options?: {
      interactive?: boolean
      use_llm?: boolean
      allow_external_tools?: boolean
      report_formats?: string[]
    }
  }
  summary: RunSummary
}

export interface RuntimeEvent {
  id: string
  job_id: string
  step_id?: string
  type: string
  level: 'debug' | 'info' | 'warn' | 'error' | string
  timestamp: string
  message: string
  data?: Record<string, unknown>
}

export interface EventItem {
  sequence: number
  cursor: string
  event: RuntimeEvent
}

export interface EventPage {
  items: EventItem[]
  next_cursor?: string
  eof: boolean
  limit: number
}

export interface Finding {
  id?: string
  job_id?: string
  title?: string
  severity?: Severity
  confidence?: string
  category?: string
  url?: string
  affected_targets?: string[]
  evidence_ids?: string[]
  description?: string
  evidence?: string
  impact?: string
  remediation?: string
  source?: {
    tool?: string
    step_id?: string
  }
  metadata?: Record<string, unknown>
}

export interface JsonReport {
  job?: { status?: RunStatus }
  findings?: Finding[]
  [key: string]: unknown
}

export interface ReportPayload {
  format: 'json' | 'md' | string
  content_type: string
  report?: JsonReport
  content?: string
}

export interface Artifact {
  id: string
  job_id?: string
  step_id?: string
  type?: string
  path: string
  sha256?: string
  size_bytes?: number
  mime_type?: string
  created_at?: string
  redacted?: boolean
}

export interface ArtifactIndex {
  schema_version: string
  generated_at: string
  count: number
  artifacts: Artifact[]
}

export interface ApprovalRequest {
  id: string
  job_id?: string
  step_id?: string
  policy_decision_id?: string
  status: ApprovalStatus
  reason?: string
  requested_action?: string
  options?: string[]
  risk_summary?: Record<string, unknown>
  created_at?: string
  resolved_at?: string | null
  resolution?: Record<string, unknown> | null
}

export interface StartWebScanRequest {
  target: string
  scope: string
  profile?: 'safe'
  enabled_plugins?: string[]
  no_external_tools?: boolean
}

export interface StartWebScanResponse {
  job_id: string
  kind: string
  status: JobStatus
  run_id: string
  run_dir: string
  status_url: string
  run_url: string
}

export interface DaemonJob {
  job_id: string
  kind: string
  status: JobStatus
  run_id?: string
  run_dir?: string
  created_at: string
  started_at?: string
  finished_at?: string
  error?: { code: string; message: string } | null
}

export interface ResumeApprovedResult {
  run_id: string
  run_dir: string
  resumed: string[]
  skipped: string[]
}

export interface AgentLink {
  to: string
  label: string
  kind?: string
}

export interface AgentToolCall {
  id: string
  name: string
  status: JobStatus | RunStatus
  detail?: string
  started_at: string
  finished_at?: string
  result?: Record<string, unknown>
  error?: { code: string; message: string } | null
  metadata?: Record<string, unknown>
}

export interface AgentMessage {
  id: string
  role: 'agent' | 'user' | 'tool' | string
  content: string
  timestamp: string
  phase?: 'plan' | 'step' | 'step_replan' | 'final_answer' | string
  tool_calls?: AgentToolCall[]
  links?: AgentLink[]
  reasoning?: string
}

export interface AgentTask {
  id: string
  goal: string
  kind: string
  status: string
  active_run_id?: string
  summary?: string
  created_at: string
  updated_at: string
}

export interface AgentTodo {
  id: string
  title: string
  detail?: string
  status: 'pending' | 'running' | 'done' | 'blocked' | 'skipped' | string
  priority: number
  tool_call_id?: string
  context_ref_ids?: string[]
  blocked_reason?: string
  updated_at: string
}

export interface AgentContextRef {
  id: string
  kind: string
  ref_id: string
  label: string
  to?: string
  summary?: string
}

export interface AgentNextAction {
  type: 'continue' | 'wait_user' | 'approval_required' | 'finished' | 'blocked' | string
  reason?: string
  suggested_prompt?: string
  suggest_prompt?: string
  auto_continue?: boolean
}

export interface AgentWorkspaceChange {
  id: string
  group_id?: string
  path: string
  old_text?: string
  new_text?: string
  size_before: number
  size_after: number
  replacements: number
  diff_preview: string
  content_preview?: string
  status: string
  error?: string
  created_at: string
  updated_at: string
  applied_at?: string
  tool_call_id?: string
}

export interface AgentMemorySummary {
  summary: string
  message_count: number
  last_message_id?: string
  last_message_at?: string
  updated_at: string
}

export interface AgentSession {
  session_id: string
  created_at: string
  updated_at: string
  operator: string
  language: 'zh' | 'en' | string
  operation_level?: AgentOperationLevel | string
  status?: string
  messages: AgentMessage[]
  active_task?: AgentTask
  todos?: AgentTodo[]
  tool_calls?: AgentToolCall[]
  context_refs?: AgentContextRef[]
  pending_changes?: AgentWorkspaceChange[]
  pending_authorization?: AgentAuthorizationRequest
  memory_summary?: AgentMemorySummary
  next_action?: AgentNextAction
  run_summary?: AgentSessionRunSummary
}

export interface AgentSessionSummary {
  session_id: string
  created_at: string
  updated_at: string
  operator: string
  language: 'zh' | 'en' | string
  operation_level?: AgentOperationLevel | string
  status: string
  active_task?: AgentTask
  next_action?: AgentNextAction
  pending_authorization?: AgentAuthorizationRequest
  memory_summary?: AgentMemorySummary
  run_summary?: AgentSessionRunSummary
  last_message?: string
  message_count: number
  tool_call_count: number
  context_count: number
}

export interface AgentSessionRunSummary {
  run_id: string
  status?: string
  target?: string
  findings: number
  artifacts: number
  events: number
  approvals: number
  pending_approvals: number
  report_json?: boolean
  summary?: string
  lead_finding?: string
  lead_evidence?: string
  lead_artifact?: string
}

export interface CreateAgentSessionResponse {
  session: AgentSession
}

export interface ListAgentSessionsResponse {
  items: AgentSessionSummary[]
  count: number
  limit: number
}

export interface DeleteAgentSessionResponse {
  session_id: string
  status: string
}

export interface SendAgentMessageResponse {
  session_id: string
  messages: AgentMessage[]
  tool_calls?: AgentToolCall[]
  active_task?: AgentTask
  todos?: AgentTodo[]
  context_refs?: AgentContextRef[]
  pending_changes?: AgentWorkspaceChange[]
  pending_authorization?: AgentAuthorizationRequest
  memory_summary?: AgentMemorySummary
  next_action?: AgentNextAction
}

export interface AgentSessionStateResponse {
  session: AgentSession
  session_id: string
  status?: string
  active_task?: AgentTask
  todos?: AgentTodo[]
  tool_calls?: AgentToolCall[]
  context_refs?: AgentContextRef[]
  pending_changes?: AgentWorkspaceChange[]
  pending_authorization?: AgentAuthorizationRequest
  memory_summary?: AgentMemorySummary
  next_action?: AgentNextAction
  run_summary?: AgentSessionRunSummary
}

export interface AgentSessionContextResponse {
  session_id: string
  active_task?: AgentTask
  context_refs?: AgentContextRef[]
  pending_changes?: AgentWorkspaceChange[]
  pending_authorization?: AgentAuthorizationRequest
  memory_summary?: AgentMemorySummary
  run_summary?: AgentSessionRunSummary
}

export interface AgentTraceResponse {
  trace_version: string
  generated_at: string
  session_id: string
  operator: string
  language: string
  operation_level?: AgentOperationLevel | string
  status?: string
  created_at: string
  updated_at: string
  active_task?: AgentTask
  todos?: AgentTodo[]
  context_refs?: AgentContextRef[]
  pending_changes?: AgentWorkspaceChange[]
  pending_authorization?: AgentAuthorizationRequest
  memory_summary?: AgentMemorySummary
  next_action?: AgentNextAction
  run_summary?: AgentSessionRunSummary
  messages: AgentMessage[]
  tool_calls: AgentToolCall[]
  timeline: Array<Record<string, unknown>>
  summary?: Record<string, unknown>
  replay_hints?: string[]
}

export interface AgentCapability {
  id: string
  label: string
  description: string
  examples?: string[]
}

export interface AgentCapabilityEndpoint {
  method: string
  path: string
  description: string
}

export interface AgentCapabilityStage {
  id: string
  label: string
  description: string
}

export interface AgentCapabilityTool {
  name: string
  risk: string
  auto_run: boolean
  description: string
  input_schema?: Record<string, unknown>
}

export interface AgentCapabilitySafety {
  scope_policy: string
  auto_execute_policy: string
  approval_required: string[]
  command_allowlist: string[]
}

export interface AgentCapabilityItem {
  id: string
  label: string
  description: string
}

export interface AgentCapabilitiesResponse {
  api_version: string
  name: string
  description: string
  entry_point: string
  endpoints: AgentCapabilityEndpoint[]
  capabilities: AgentCapability[]
  stages: AgentCapabilityStage[]
  builtin_tools: AgentCapabilityTool[]
  skills?: AgentSkillSpec[]
  tool_capabilities?: AgentToolCapabilitySpec[]
  security_tools?: AgentSecurityToolSpec[]
  mcp?: AgentMcpIntegrationSpec
  safety: AgentCapabilitySafety
  integrations: AgentCapabilityItem[]
}

export interface AgentSkillSpec {
  id: string
  source?: string
  path?: string
  label: string
  description: string
  intent_tags: string[]
  required_tools?: string[]
  optional_tools?: string[]
  fallback_prompts?: string[]
  risk: string
  examples?: string[]
}

export interface AgentToolCapabilitySpec {
  name: string
  source: string
  catalog_path?: string
  intent_tags: string[]
  can_do: string[]
  cannot_do?: string[]
  risk: string
  required_level: string
  auto_run: boolean
  description: string
  input_schema?: Record<string, unknown>
  examples?: string[]
}

export interface AgentSecurityToolSpec {
  name: string
  source: string
  path?: string
  plugin_path?: string
  intent_tags?: string[]
  can_do?: string[]
  cannot_do?: string[]
  risk?: string
  required_level?: string
  auto_run?: boolean
  input_schema?: Record<string, unknown>
  examples?: string[]
}

export interface AgentMcpIntegrationSpec {
  status: string
  description: string
  next_steps: string[]
}

export interface AgentSkillRegistryResponse {
  skills: AgentSkillSpec[]
  tool_capabilities: AgentToolCapabilitySpec[]
  security_tools?: AgentSecurityToolSpec[]
  mcp: AgentMcpIntegrationSpec
}

export interface AgentMcpToolConfig {
  name: string
  description?: string
  intent_tags?: string[]
  can_do?: string[]
  cannot_do?: string[]
  risk?: string
  required_level?: string
  enabled: boolean
}

export interface AgentMcpServerConfig {
  id: string
  name: string
  transport: string
  command?: string
  args?: string[]
  url?: string
  enabled: boolean
  preset?: boolean
  status?: string
  last_error?: string
  last_sync_at?: string
  created_at?: string
  updated_at?: string
  description?: string
  tools?: AgentMcpToolConfig[]
}

export interface AgentMcpRegistryView {
  servers: AgentMcpServerConfig[]
  tool_capabilities: AgentToolCapabilitySpec[]
  count: number
  enabled_count: number
}

export interface WorkspaceCommandResult {
  command: string
  exit_code: number
  stdout: string
  stderr: string
  duration_ms: number
  workdir: string
}

export interface WorkspacePatchResult {
  session_id: string
  path: string
  size_before: number
  size_after: number
  replacements: number
  diff_preview: string
  content_preview: string
}

export interface WorkspaceDiffResult extends WorkspacePatchResult {
  change_id: string
  group_id?: string
  status: string
  created_at?: string
  updated_at?: string
}

export interface WorkspaceApplyResult extends WorkspaceDiffResult {}
export interface WorkspaceDiscardResult extends WorkspaceDiffResult {}

export interface WorkspaceDiffManyInput {
  path: string
  old: string
  new: string
}

export interface WorkspaceChangeGroupResult {
  session_id: string
  group_id: string
  action?: string
  status: string
  count?: number
  requested_count?: number
  succeeded_count?: number
  failed_count?: number
  changes?: WorkspaceDiffResult[]
  succeeded?: WorkspaceDiffResult[]
  failed?: Array<{change_id: string; code?: string; message: string}>
}

export interface WorkspaceFileEntry {
  name: string
  path: string
  is_dir: boolean
  size?: number
}

export type AgentControlAction = 'continue' | 'revise' | 'pause' | 'stop' | 'cancel' | 'new_task'

export interface AgentControlRequest {
  action: AgentControlAction
  operator: string
  language: 'zh' | 'en'
  operation_level?: AgentOperationLevel
  reason?: string
  prompt?: string
  run_id?: string
  job_id?: string
  clear_input?: boolean
}

export interface AgentControlResponse {
  session: AgentSession
  message: AgentMessage
}

export interface AgentAuthorizationDecisionRequest extends Partial<AgentProviderConfig> {
  action: 'approve' | 'deny'
  authorization_id?: string
  operator: string
  language: 'zh' | 'en'
  operation_level?: AgentOperationLevel
  reason?: string
  skip_synthesis?: boolean
}

export interface AgentSessionStreamEvent {
  session: AgentSession
}

export interface AgentActivity {
  kind: 'message' | 'tool' | 'state' | 'todo' | string
  id: string
  title: string
  detail?: string
  status?: string
  timestamp: string
}

export interface AgentSessionStreamHandlers {
  onOpen?: () => void
  onSession?: (session: AgentSession) => void
  onActivity?: (activity: AgentActivity) => void
  onPing?: (time: string) => void
  onError?: (error: ApiErrorPayload) => void
}

export interface AgentProviderConfig {
  provider: string
  model: string
  api_key: string
  base_url: string
  instructions: string
  operation_level: AgentOperationLevel
  auth_profiles: Array<{
    id: string
    name?: string
    type?: ApiAuthProfileType
    base_url?: string
    token?: string
    username?: string
    password?: string
    header_name?: string
    header_value?: string
    cookie?: string
    enabled?: boolean
  }>
  default_auth_profile_id: string
}

export interface Provider {
  id: string
  name: string
  default_model: string
}

export interface AgentProviderTestRequest {
  type: AgentProviderType
  model: string
  api_key: string
  base_url?: string
}

export interface AgentProviderTestResponse {
  ok: boolean
  provider: string
  model: string
  latency_ms: number
  output?: string
  error?: string
  request_url?: string
}

export interface ToolDef {
  name: string
  description: string
  input_schema: Record<string, unknown>
}

export interface ScanListItem {
  id: string
  target: string
  status: ScanStatus
  started_at: string
  duration: string
  findings: number
  turns: number
  max_turns: number
  provider: string
  event_count?: number
  artifact_count?: number
  approval_count?: number
  pending_approvals?: number
}

export interface UIEvent {
  type: string
  message: string
  tool: string
  id?: string
  job_id?: string
  step_id?: string
  data?: Record<string, unknown>
  severity: string
  turn: number
  max_turns: number
  timestamp: number
  phase?: string
}

export interface ScanDetail extends ScanListItem {
  tokens_in: number
  tokens_out: number
  est_cost: number
  findings_detail: Finding[]
  summary: string
  model: string
  events: UIEvent[]
  approvals?: ApprovalRequest[]
  artifacts?: Artifact[]
  interactive: boolean
  waiting: boolean
}

export interface StartScanRequest {
  target: string
  scope: string
  provider: string
  model: string
  api_key: string
  base_url: string
  instructions: string
  max_turns: number
  multi_agent: boolean
  no_confirm: boolean
  interactive: boolean
}

export interface ApiClientConfig {
  baseUrl: string
  token: string
}

function joinUrl(baseUrl: string, path: string): string {
  const base = baseUrl.trim().replace(/\/+$/, '')
  return `${base}${path}`
}

export class ApiClient {
  private baseUrl: string
  private token: string

  constructor(config: ApiClientConfig) {
    this.baseUrl = config.baseUrl
    this.token = config.token
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers = new Headers(options.headers)
    if (options.body) headers.set('Content-Type', 'application/json')
    if (this.token.trim()) headers.set('Authorization', `Bearer ${this.token.trim()}`)

    const response = await fetch(joinUrl(this.baseUrl, path), { ...options, headers })
    const body = (await response.json().catch(() => null)) as Envelope<T> | T | null

    if (!response.ok) {
      const envelope = body as Envelope<T> | null
      const payload = envelope && 'error' in envelope && envelope.error ? envelope.error : {
        code: response.status === 401 ? 'unauthorized' : 'request_failed',
        message: response.statusText || 'request failed',
      }
      throw new ApiError(response.status, payload)
    }

    if (body && typeof body === 'object' && 'ok' in body && 'data' in body) {
      const envelope = body as Envelope<T>
      if (!envelope.ok) {
        throw new ApiError(response.status, envelope.error || { code: 'request_failed', message: 'request failed' })
      }
      return envelope.data as T
    }
    return body as T
  }

  async streamAgentSession(
    sessionId: string,
    handlers: AgentSessionStreamHandlers,
    signal?: AbortSignal,
  ): Promise<void> {
    const headers = new Headers()
    headers.set('Accept', 'text/event-stream')
    if (this.token.trim()) headers.set('Authorization', `Bearer ${this.token.trim()}`)
    const response = await fetch(joinUrl(this.baseUrl, `/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/stream`), {
      headers,
      signal,
    })
    if (!response.ok) {
      const body = (await response.json().catch(() => null)) as Envelope<unknown> | null
      throw new ApiError(response.status, body?.error || {
        code: response.status === 401 ? 'unauthorized' : 'stream_failed',
        message: response.statusText || 'stream failed',
      })
    }
    if (!response.body) throw new ApiError(0, { code: 'stream_unavailable', message: 'stream body unavailable' })

    handlers.onOpen?.()
    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let sep = buffer.indexOf('\n\n')
      while (sep >= 0) {
        const raw = buffer.slice(0, sep)
        buffer = buffer.slice(sep + 2)
        this.handleSSEBlock(raw, handlers)
        sep = buffer.indexOf('\n\n')
      }
    }
  }

  private handleSSEBlock(raw: string, handlers: AgentSessionStreamHandlers) {
    const lines = raw.split(/\r?\n/)
    let event = 'message'
    const dataLines: string[] = []
    for (const line of lines) {
      if (line.startsWith('event:')) event = line.slice('event:'.length).trim()
      if (line.startsWith('data:')) dataLines.push(line.slice('data:'.length).trimStart())
    }
    if (dataLines.length === 0) return
    let parsed: Record<string, unknown>
    try {
      parsed = JSON.parse(dataLines.join('\n')) as Record<string, unknown>
    } catch {
      return
    }
    if (event === 'session') {
      const payload = parsed as unknown as AgentSessionStreamEvent
      if (payload.session) handlers.onSession?.(payload.session)
      return
    }
    if (event === 'activity') {
      const activity = parsed.activity as AgentActivity | undefined
      if (activity) handlers.onActivity?.(activity)
      return
    }
    if (event === 'ping') {
      const time = typeof parsed.time === 'string' ? parsed.time : new Date().toISOString()
      handlers.onPing?.(time)
      return
    }
    if (event === 'error') {
      const error = parsed.error as ApiErrorPayload | undefined
      handlers.onError?.(error || { code: 'stream_error', message: 'stream error' })
    }
  }

  async pingStatus(): Promise<DaemonConnectionStatus> {
    try {
      await this.healthz()
      return 'online'
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) return 'unauthorized'
      try {
        await this.listScans()
        return 'online'
      } catch (fallbackErr) {
        if (fallbackErr instanceof ApiError && fallbackErr.status === 401) return 'unauthorized'
        return 'offline'
      }
    }
  }

  async ping(): Promise<boolean> {
    return (await this.pingStatus()) === 'online'
  }

  healthz() {
    return this.request<Healthz>('/api/v1/healthz')
  }

  version() {
    return this.request<VersionInfo>('/api/v1/version')
  }

  doctor() {
    return this.request<DoctorReport>('/api/v1/doctor')
  }

  plugins() {
    return this.request<{ plugins: PluginManifest[] }>('/api/v1/plugins')
  }

  pluginsDoctor() {
    return this.request<PluginDoctorReport>('/api/v1/plugins/doctor')
  }

  listRuns(limit = 50, cursor = '') {
    const params = new URLSearchParams()
    params.set('limit', String(limit))
    if (cursor) params.set('cursor', cursor)
    return this.request<RunListResponse>(`/api/v1/runs?${params}`)
  }

  getRun(runId: string) {
    return this.request<RunDetail>(`/api/v1/runs/${encodeURIComponent(runId)}`)
  }

  getEvents(runId: string, cursor = '', limit = 100) {
    const params = new URLSearchParams()
    params.set('limit', String(limit))
    if (cursor) params.set('cursor', cursor)
    return this.request<EventPage>(`/api/v1/runs/${encodeURIComponent(runId)}/events?${params}`)
  }

  getReport(runId: string, format: 'json' | 'md' = 'json') {
    return this.request<ReportPayload>(`/api/v1/runs/${encodeURIComponent(runId)}/report?format=${format}`)
  }

  getArtifacts(runId: string) {
    return this.request<ArtifactIndex>(`/api/v1/runs/${encodeURIComponent(runId)}/artifacts`)
  }

  getApprovals(runId: string) {
    return this.request<{ approvals: ApprovalRequest[] }>(`/api/v1/runs/${encodeURIComponent(runId)}/approvals`)
  }

  approveApproval(runId: string, approvalId: string, operator: string, comment = '') {
    return this.request<ApprovalRequest>(
      `/api/v1/runs/${encodeURIComponent(runId)}/approvals/${encodeURIComponent(approvalId)}/approve`,
      { method: 'POST', body: JSON.stringify({ operator, comment }) },
    )
  }

  denyApproval(runId: string, approvalId: string, operator: string, comment = '') {
    return this.request<ApprovalRequest>(
      `/api/v1/runs/${encodeURIComponent(runId)}/approvals/${encodeURIComponent(approvalId)}/deny`,
      { method: 'POST', body: JSON.stringify({ operator, comment }) },
    )
  }

  resumeApproved(runId: string, operator: string) {
    return this.request<ResumeApprovedResult>(`/api/v1/runs/${encodeURIComponent(runId)}/resume-approved`, {
      method: 'POST',
      body: JSON.stringify({ operator }),
    })
  }

  startWebScan(request: StartWebScanRequest) {
    return this.request<StartWebScanResponse>('/api/v1/scans/web', {
      method: 'POST',
      body: JSON.stringify(request),
    })
  }

  getJob(jobId: string) {
    return this.request<DaemonJob>(`/api/v1/jobs/${encodeURIComponent(jobId)}`)
  }

  cancelJob(jobId: string) {
    return this.request<{ job_id: string; status: JobStatus }>(`/api/v1/jobs/${encodeURIComponent(jobId)}/cancel`, {
      method: 'POST',
    })
  }

  getAgentCapabilities(language: 'zh' | 'en') {
    const params = new URLSearchParams()
    params.set('language', language)
    return this.request<AgentCapabilitiesResponse>(`/api/v1/agent/capabilities?${params}`)
  }

  getAgentSkills(language: 'zh' | 'en') {
    const params = new URLSearchParams()
    params.set('language', language)
    return this.request<AgentSkillRegistryResponse>(`/api/v1/agent/skills?${params}`)
  }

  getAgentMcpRegistry() {
    return this.request<AgentMcpRegistryView>('/api/v1/agent/mcp')
  }

  setAgentMcpServerEnabled(serverId: string, enabled: boolean) {
    return this.request<{ server: AgentMcpServerConfig }>(`/api/v1/agent/mcp/servers/${encodeURIComponent(serverId)}/${enabled ? 'enable' : 'disable'}`, {
      method: 'POST',
    })
  }

  createAgentSession(operator: string, language: 'zh' | 'en', operationLevel: AgentOperationLevel = 'read_only') {
    return this.request<CreateAgentSessionResponse>('/api/v1/agent/sessions', {
      method: 'POST',
      body: JSON.stringify({ operator, language, operation_level: operationLevel }),
    })
  }

  listAgentSessions(limit = 8) {
    const params = new URLSearchParams()
    params.set('limit', String(limit))
    return this.request<ListAgentSessionsResponse>(`/api/v1/agent/sessions?${params}`)
  }

  getAgentSession(sessionId: string) {
    return this.request<{ session: AgentSession }>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}`)
  }

  deleteAgentSession(sessionId: string) {
    return this.request<DeleteAgentSessionResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}`, {
      method: 'DELETE',
    })
  }

  getAgentSessionState(sessionId: string) {
    return this.request<AgentSessionStateResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/state`)
  }

  getAgentSessionContext(sessionId: string) {
    return this.request<AgentSessionContextResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/context`)
  }

  getAgentSessionTrace(sessionId: string) {
    return this.request<AgentTraceResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/trace`)
  }

  sendAgentMessage(
    sessionId: string,
    message: string,
    operator: string,
    language: 'zh' | 'en',
    providerConfig?: Partial<AgentProviderConfig>,
  ) {
    return this.request<SendAgentMessageResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/messages`, {
      method: 'POST',
      body: JSON.stringify({ message, operator, language, ...(providerConfig || {}) }),
    })
  }

  controlAgentSession(sessionId: string, request: AgentControlRequest) {
    return this.request<AgentControlResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/control`, {
      method: 'POST',
      body: JSON.stringify(request),
    })
  }

  continueAgentSession(sessionId: string, request: Omit<AgentControlRequest, 'action'>) {
    return this.request<AgentControlResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/continue`, {
      method: 'POST',
      body: JSON.stringify(request),
    })
  }

  approveAgentAuthorization(sessionId: string, request: Omit<AgentAuthorizationDecisionRequest, 'action'>) {
    return this.request<AgentControlResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/authorization/approve`, {
      method: 'POST',
      body: JSON.stringify({ ...request, action: 'approve' }),
    })
  }

  denyAgentAuthorization(sessionId: string, request: Omit<AgentAuthorizationDecisionRequest, 'action'>) {
    return this.request<AgentControlResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/authorization/deny`, {
      method: 'POST',
      body: JSON.stringify({ ...request, action: 'deny' }),
    })
  }

  stopAgentSession(sessionId: string, request: Omit<AgentControlRequest, 'action'>) {
    return this.request<AgentControlResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/stop`, {
      method: 'POST',
      body: JSON.stringify(request),
    })
  }

  pauseAgentSession(sessionId: string, request: Omit<AgentControlRequest, 'action'>) {
    return this.request<AgentControlResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/pause`, {
      method: 'POST',
      body: JSON.stringify(request),
    })
  }

  cancelAgentSession(sessionId: string, request: Omit<AgentControlRequest, 'action'>) {
    return this.request<AgentControlResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/cancel`, {
      method: 'POST',
      body: JSON.stringify(request),
    })
  }

  newAgentTask(sessionId: string, request: Omit<AgentControlRequest, 'action'>) {
    return this.request<AgentControlResponse>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/new-task`, {
      method: 'POST',
      body: JSON.stringify(request),
    })
  }

  agentWorkspaceCommand(sessionId: string, command: string, workdir = '') {
    return this.request<WorkspaceCommandResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/command`, {
      method: 'POST',
      body: JSON.stringify({ command, workdir }),
    })
  }

  agentWorkspaceListFiles(sessionId: string, path = '') {
    return this.request<{ path: string; files: WorkspaceFileEntry[]; count: number }>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/list`, {
      method: 'POST',
      body: JSON.stringify({ path }),
    })
  }

  agentWorkspaceReadFile(sessionId: string, path: string) {
    return this.request<{ path: string; content: string; size: number }>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/read`, {
      method: 'POST',
      body: JSON.stringify({ path }),
    })
  }

  agentWorkspaceWriteFile(sessionId: string, path: string, content: string) {
    return this.request<{ path: string; size: number; status: string }>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/write`, {
      method: 'POST',
      body: JSON.stringify({ path, content }),
    })
  }

  agentWorkspacePatchFile(sessionId: string, path: string, oldText: string, newText: string) {
    return this.request<WorkspacePatchResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/patch`, {
      method: 'POST',
      body: JSON.stringify({ path, old: oldText, new: newText }),
    })
  }

  agentWorkspaceDiffFile(sessionId: string, path: string, oldText: string, newText: string) {
    return this.request<WorkspaceDiffResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/diff`, {
      method: 'POST',
      body: JSON.stringify({ path, old: oldText, new: newText }),
    })
  }

  agentWorkspaceDiffMany(sessionId: string, changes: WorkspaceDiffManyInput[]) {
    return this.request<WorkspaceChangeGroupResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/diff-many`, {
      method: 'POST',
      body: JSON.stringify({ changes }),
    })
  }

  agentWorkspaceApplyChange(sessionId: string, changeId: string) {
    return this.request<WorkspaceApplyResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/apply`, {
      method: 'POST',
      body: JSON.stringify({ change_id: changeId }),
    })
  }

  agentWorkspaceApplyGroup(sessionId: string, groupId: string) {
    return this.request<WorkspaceChangeGroupResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/apply-group`, {
      method: 'POST',
      body: JSON.stringify({ group_id: groupId }),
    })
  }

  agentWorkspaceRebaseChange(sessionId: string, changeId: string, oldText: string, newText = '') {
    return this.request<WorkspaceDiffResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/rebase`, {
      method: 'POST',
      body: JSON.stringify({ change_id: changeId, old: oldText, new: newText }),
    })
  }

  agentWorkspaceDiscardChange(sessionId: string, changeId: string) {
    return this.request<WorkspaceDiscardResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/discard`, {
      method: 'POST',
      body: JSON.stringify({ change_id: changeId }),
    })
  }

  agentWorkspaceDiscardGroup(sessionId: string, groupId: string) {
    return this.request<WorkspaceChangeGroupResult>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/discard-group`, {
      method: 'POST',
      body: JSON.stringify({ group_id: groupId }),
    })
  }

  agentWorkspaceDeleteFile(sessionId: string, path: string) {
    return this.request<{ path: string; status: string }>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/delete`, {
      method: 'POST',
      body: JSON.stringify({ path }),
    })
  }

  agentWorkspaceMoveFile(sessionId: string, from: string, to: string) {
    return this.request<{ from: string; path: string; status: string }>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/move`, {
      method: 'POST',
      body: JSON.stringify({ from, to }),
    })
  }

  agentWorkspaceCopyFile(sessionId: string, from: string, to: string) {
    return this.request<{ from: string; path: string; status: string }>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/copy`, {
      method: 'POST',
      body: JSON.stringify({ from, to }),
    })
  }

  agentWorkspaceMkdir(sessionId: string, path: string) {
    return this.request<{ path: string; status: string }>(`/api/v1/agent/sessions/${encodeURIComponent(sessionId)}/workspace/files/mkdir`, {
      method: 'POST',
      body: JSON.stringify({ path }),
    })
  }

  async listScans() {
    return this.request<ScanListItem[]>('/api/scans')
  }

  async getScan(id: string) {
    return this.request<ScanDetail>(`/api/scans/${encodeURIComponent(id)}`)
  }

  async startScan(req: Partial<StartScanRequest>) {
    return this.request<{ id: string }>('/api/scans', {
      method: 'POST',
      body: JSON.stringify(req),
    })
  }

  async stopScan(id: string) {
    return this.request<{ status: string }>(`/api/scans/${encodeURIComponent(id)}/stop`, {
      method: 'POST',
    })
  }

  async deleteScan(id: string) {
    return this.request<{ status: string }>(`/api/scans/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    })
  }

  async sendMessage(id: string, message: string) {
    return this.request<{ status: string }>(`/api/scans/${encodeURIComponent(id)}/message`, {
      method: 'POST',
      body: JSON.stringify({ message }),
    })
  }

  getProviders() {
    return this.request<Provider[]>('/api/providers')
  }

  async getProviderProfiles(): Promise<{ providers: AgentProviderProfile[]; activeId: string }> {
    const cfg = await this.request<{ providers?: unknown; active_id?: string }>('/api/provider-profiles')
    return {
      providers: normalizeAgentProviderProfiles(cfg?.providers, defaultAgentProvider),
      activeId: stringOrDefault(cfg?.active_id, ''),
    }
  }

  async saveProviderProfiles(providers: AgentProviderProfile[], activeId: string) {
    const body = {
      providers: providers.map((p) => ({
        id: p.id,
        name: p.name,
        type: p.type,
        model: p.model,
        api_key: p.apiKey,
        base_url: p.baseUrl,
        instructions: p.instructions,
        max_turns: p.maxTurns,
      })),
      active_id: activeId,
    }
    return this.request<unknown>('/api/provider-profiles', { method: 'PUT', body: JSON.stringify(body) })
  }

  testProvider(req: AgentProviderTestRequest) {
    return this.request<AgentProviderTestResponse>('/api/provider-profiles/test', {
      method: 'POST',
      body: JSON.stringify(req),
    })
  }

  getTools() {
    return this.request<ToolDef[]>('/api/tools')
  }

  eventsUrl(id: string) {
    return `${this.baseUrl.replace(/\/+$/, '')}/api/scans/${encodeURIComponent(id)}/events`
  }
}

export function createApiClient(settings: Pick<ConsoleSettings, 'baseUrl' | 'token'>) {
  return new ApiClient({ baseUrl: settings.baseUrl, token: settings.token })
}
