const BASE = ''

export interface ScanListItem {
  id: string
  target: string
  status: 'running' | 'completed' | 'failed' | 'stopped'
  started_at: string
  duration: string
  findings: number
  turns: number
  max_turns: number
  provider: string
}

export interface ScanDetail extends ScanListItem {
  tokens_in: number
  tokens_out: number
  est_cost: number
  findings_detail: Finding[]
  summary: string
  model: string
  events: UIEvent[]
  interactive: boolean
  waiting: boolean
}

export interface Finding {
  id: string
  title: string
  severity: 'critical' | 'high' | 'medium' | 'low' | 'info'
  description: string
  remediation: string
  affected_targets: string[]
}

export interface UIEvent {
  type: string
  message: string
  tool?: string
  severity?: string
  turn?: number
  max_turns?: number
  timestamp: number
}

export interface ProviderInfo {
  id: string
  name: string
  default_model: string
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

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

export const api = {
  listScans: () => request<ScanListItem[]>('/api/scans'),
  getScan: (id: string) => request<ScanDetail>(`/api/scans/${id}`),
  startScan: (req: StartScanRequest) =>
    request<{ id: string }>('/api/scans', { method: 'POST', body: JSON.stringify(req) }),
  stopScan: (id: string) =>
    request<{ status: string }>(`/api/scans/${id}/stop`, { method: 'POST' }),
  deleteScan: (id: string) =>
    request<{ status: string }>(`/api/scans/${id}`, { method: 'DELETE' }),
  sendMessage: (id: string, message: string) =>
    request<{ status: string }>(`/api/scans/${id}/message`, { method: 'POST', body: JSON.stringify({ message }) }),
  getProviders: () => request<ProviderInfo[]>('/api/providers'),
}
