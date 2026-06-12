import type { AgentTraceResponse } from '@/lib/api'

export const TRACE_JSON_ARRAY_CHUNK_SIZE = 50

export interface TraceJsonSection {
  id: string
  title: string
  path: string
  value: unknown
  json: string
  lineCount: number
  byteCount: number
  itemCount?: number
  range?: {
    start: number
    end: number
    total: number
  }
  hasError: boolean
  errorText?: string
}

export interface TraceErrorMarker {
  id: string
  sectionId: string
  label: string
  path: string
  message: string
}

export function buildTraceJsonSections(trace: AgentTraceResponse | null, chunkSize = TRACE_JSON_ARRAY_CHUNK_SIZE): TraceJsonSection[] {
  if (!trace) return []

  const sections: TraceJsonSection[] = []
  const appendValue = (id: string, title: string, path: string, value: unknown) => {
    sections.push(createTraceJsonSection(id, title, path, value))
  }
  const appendArray = (id: string, title: string, path: string, value: unknown[] | undefined) => {
    const items = Array.isArray(value) ? value : []
    const safeChunkSize = Math.max(1, Math.floor(chunkSize))
    if (items.length <= safeChunkSize) {
      appendValue(id, `${title} (${items.length})`, path, items)
      return
    }
    for (let start = 0; start < items.length; start += safeChunkSize) {
      const end = Math.min(items.length, start + safeChunkSize)
      const section = createTraceJsonSection(
        `${id}_${start + 1}_${end}`,
        `${title} ${start + 1}-${end} / ${items.length}`,
        `${path}[${start}..${end - 1}]`,
        items.slice(start, end),
      )
      section.itemCount = end - start
      section.range = { start, end, total: items.length }
      sections.push(section)
    }
  }

  appendValue('overview', 'Overview', '$', traceOverview(trace))
  appendValue('summary', 'Summary', '$.summary', trace.summary || {})
  appendValue('active_task', 'Active task', '$.active_task', trace.active_task || null)
  appendArray('messages', 'Messages', '$.messages', trace.messages)
  appendArray('tool_calls', 'Tool calls', '$.tool_calls', trace.tool_calls)
  appendArray('timeline', 'Timeline', '$.timeline', trace.timeline)
  appendArray('todos', 'TODOs', '$.todos', trace.todos || [])
  appendArray('context_refs', 'Context refs', '$.context_refs', trace.context_refs || [])
  appendArray('pending_changes', 'Pending changes', '$.pending_changes', trace.pending_changes || [])
  appendValue('pending_authorization', 'Pending authorization', '$.pending_authorization', trace.pending_authorization || null)
  appendValue('approval_state', 'Approval state', '$.approval_state', trace.approval_state || null)
  appendValue('ui_state', 'UI state', '$.ui_state', trace.ui_state || null)
  appendValue('memory_summary', 'Memory summary', '$.memory_summary', trace.memory_summary || null)
  appendValue('next_action', 'Next action', '$.next_action', trace.next_action || null)
  appendValue('run_summary', 'Run summary', '$.run_summary', trace.run_summary || null)
  appendArray('replay_hints', 'Replay hints', '$.replay_hints', trace.replay_hints || [])

  return sections
}

export function buildTraceErrorMarkers(sections: TraceJsonSection[]): TraceErrorMarker[] {
  return sections
    .filter((section) => section.hasError)
    .map((section) => ({
      id: `error_${section.id}`,
      sectionId: section.id,
      label: section.title,
      path: section.path,
      message: section.errorText || 'error-like trace data',
    }))
}

export function traceSectionMatchesQuery(section: TraceJsonSection, query: string) {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return true
  return [
    section.id,
    section.title,
    section.path,
    section.errorText || '',
    section.json,
  ].some((value) => value.toLowerCase().includes(normalized))
}

export function matchingJsonLines(json: string, query: string, maxLines = 5): Array<{ line: number; text: string }> {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return []
  const matches: Array<{ line: number; text: string }> = []
  const lines = json.split('\n')
  for (let index = 0; index < lines.length; index += 1) {
    if (!lines[index].toLowerCase().includes(normalized)) continue
    matches.push({ line: index + 1, text: lines[index] })
    if (matches.length >= maxLines) break
  }
  return matches
}

export function formatTraceJson(value: unknown) {
  try {
    return { json: JSON.stringify(value, null, 2), error: '' }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    return { json: `/* JSON serialization failed: ${message} */\n${String(value)}`, error: message }
  }
}

function createTraceJsonSection(id: string, title: string, path: string, value: unknown): TraceJsonSection {
  const formatted = formatTraceJson(value)
  const errorText = formatted.error || detectErrorText(value)
  return {
    id,
    title,
    path,
    value,
    json: formatted.json,
    lineCount: formatted.json.split('\n').length,
    byteCount: new Blob([formatted.json]).size,
    itemCount: Array.isArray(value) ? value.length : undefined,
    hasError: Boolean(errorText),
    errorText: errorText || undefined,
  }
}

function traceOverview(trace: AgentTraceResponse) {
  return {
    trace_version: trace.trace_version,
    generated_at: trace.generated_at,
    session_id: trace.session_id,
    operator: trace.operator,
    language: trace.language,
    operation_level: trace.operation_level,
    status: trace.status,
    created_at: trace.created_at,
    updated_at: trace.updated_at,
    counts: {
      messages: trace.messages?.length || 0,
      tool_calls: trace.tool_calls?.length || 0,
      timeline: trace.timeline?.length || 0,
      todos: trace.todos?.length || 0,
      context_refs: trace.context_refs?.length || 0,
      pending_changes: trace.pending_changes?.length || 0,
      replay_hints: trace.replay_hints?.length || 0,
    },
  }
}

function detectErrorText(value: unknown) {
  const seen = new Set<unknown>()
  const queue: Array<{ value: unknown; path: string }> = [{ value, path: '$' }]
  while (queue.length) {
    const current = queue.shift()
    if (!current) continue
    if (!current.value || typeof current.value !== 'object') continue
    if (seen.has(current.value)) continue
    seen.add(current.value)
    for (const [key, child] of Object.entries(current.value as Record<string, unknown>)) {
      const childPath = `${current.path}.${key}`
      const lowerKey = key.toLowerCase()
      if (lowerKey.includes('error') && hasMeaningfulValue(child)) return `${childPath}: ${shortValue(child)}`
      if (typeof child === 'string' && isErrorLikeString(child) && isErrorSignalKey(lowerKey)) return `${childPath}: ${child}`
      if (child && typeof child === 'object') queue.push({ value: child, path: childPath })
    }
  }
  return ''
}

function isErrorSignalKey(key: string) {
  return ['status', 'state', 'type', 'kind', 'phase', 'message', 'detail', 'reason'].some((part) => key.includes(part))
}

function isErrorLikeString(value: string) {
  return /error|failed|failure|exception|panic|denied|blocked/i.test(value)
}

function hasMeaningfulValue(value: unknown) {
  if (value === null || value === undefined || value === false) return false
  if (typeof value === 'string') return value.trim().length > 0
  if (Array.isArray(value)) return value.length > 0
  if (typeof value === 'object') return Object.keys(value).length > 0
  return true
}

function shortValue(value: unknown) {
  const formatted = formatTraceJson(value).json.replace(/\s+/g, ' ').trim()
  return formatted.length > 160 ? `${formatted.slice(0, 157)}...` : formatted
}
