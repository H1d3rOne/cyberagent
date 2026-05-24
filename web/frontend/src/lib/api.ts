const SETTINGS_KEY = 'cyberagent_console_settings'
const RECENT_JOBS_KEY = 'cyberagent_recent_jobs'

export type ApiStatus = 'idle' | 'loading' | 'ok' | 'error'
export type RunStatus = 'created' | 'running' | 'succeeded' | 'failed' | 'cancelled' | 'skipped' | string
export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled' | string
export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'modified' | 'expired' | 'cancelled' | string
export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info' | string

export interface ConsoleSettings {
  baseUrl: string
  token: string
  operator: string
  language: 'zh' | 'en'
}

export const defaultSettings: ConsoleSettings = {
  baseUrl: '',
  token: '',
  operator: 'local-user',
  language: 'zh',
}

export function loadSettings(): ConsoleSettings {
  const stored = localStorage.getItem(SETTINGS_KEY)
  if (!stored) return defaultSettings
  try {
    return { ...defaultSettings, ...JSON.parse(stored) }
  } catch {
    return defaultSettings
  }
}

export function saveSettings(settings: ConsoleSettings) {
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings))
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
  ok: boolean
  message?: string
  status?: string
}

export interface DoctorReport {
  ok: boolean
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
  title?: string
  severity?: Severity
  confidence?: string
  category?: string
  affected_targets?: string[]
  description?: string
  impact?: string
  remediation?: string
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
    headers.set('Content-Type', 'application/json')
    if (this.token.trim()) headers.set('Authorization', `Bearer ${this.token.trim()}`)

    const response = await fetch(joinUrl(this.baseUrl, path), { ...options, headers })
    const body = (await response.json().catch(() => null)) as Envelope<T> | null

    if (!response.ok || !body?.ok) {
      const payload = body?.error || {
        code: response.status === 401 ? 'unauthorized' : 'request_failed',
        message: response.statusText || 'request failed',
      }
      throw new ApiError(response.status, payload)
    }
    return body.data as T
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
}

export function createApiClient(settings: Pick<ConsoleSettings, 'baseUrl' | 'token'>) {
  return new ApiClient({ baseUrl: settings.baseUrl, token: settings.token })
}
