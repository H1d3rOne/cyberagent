import type {
  AgentActivity,
  AgentAuthorizationRequest,
  AgentMessage,
  AgentNextAction,
  AgentSession,
  AgentSessionRunSummary,
  AgentSessionSummary,
  AgentTodo,
  AgentToolCall,
} from '@/lib/api'

export type CockpitStreamStatus = 'offline' | 'connecting' | 'online' | 'degraded' | 'error'

export type AgentRuntimeEventType =
  | 'snapshot'
  | 'activity'
  | 'heartbeat'
  | 'turn_started'
  | 'model_token'
  | 'model_reasoning'
  | 'tool_started'
  | 'tool_progress'
  | 'tool_finished'
  | 'todo_updated'
  | 'approval_required'
  | 'approval_resolved'
  | 'task_finished'
  | 'task_failed'
  | 'memory_compacted'
  | 'error'
  | string

export interface AgentRuntimeEvent {
  type: AgentRuntimeEventType
  id?: string
  task_id?: string
  session_id?: string
  turn_id?: string
  tool_call_id?: string
  tool_name?: string
  timestamp?: string
  message?: string
  delta?: string
  text?: string
  reasoning?: string
  status?: string
  title?: string
  detail?: string
  data?: Record<string, unknown>
  session?: AgentSession
  activity?: AgentActivity
  tool_call?: AgentToolCall
  todo?: AgentTodo
  approval?: AgentAuthorizationRequest
  error?: { code?: string; message?: string } | string
}

export interface AgentTurnView {
  id: string
  taskId?: string
  sessionId?: string
  title: string
  status: string
  startedAt: string
  finishedAt?: string
  modelText: string
  reasoning: string
  messages: AgentMessage[]
  toolCallIds: string[]
  todoIds: string[]
  eventIds: string[]
}

export interface TimelineEventView extends AgentRuntimeEvent {
  id: string
  type: AgentRuntimeEventType
  timestamp: string
}

export interface OptimisticMessage {
  id: string
  content: string
  timestamp: string
  status: 'sending' | 'confirmed' | 'failed'
}

export interface AgentCockpitState {
  streamStatus: CockpitStreamStatus
  healthStatus: 'unknown' | 'online' | 'offline' | 'unauthorized'
  lastHeartbeatAt?: string
  sessions: AgentSessionSummary[]
  activeSessionId: string
  session: AgentSession | null
  events: TimelineEventView[]
  turns: AgentTurnView[]
  selectedToolCallId?: string
  selectedTurnId?: string
  optimisticMessages: OptimisticMessage[]
  loadingSessions: boolean
  loadingSession: boolean
  sending: boolean
  error: string
}

export type AgentCockpitAction =
  | { type: 'sessions/loading'; loading: boolean }
  | { type: 'sessions/loaded'; sessions: AgentSessionSummary[] }
  | { type: 'session/loading'; loading: boolean }
  | { type: 'session/loaded'; session: AgentSession | null }
  | { type: 'session/deleted'; sessionId: string; sessions?: AgentSessionSummary[] }
  | { type: 'session/select'; sessionId: string }
  | { type: 'stream/status'; status: CockpitStreamStatus }
  | { type: 'health/status'; status: AgentCockpitState['healthStatus'] }
  | { type: 'stream/event'; event: AgentRuntimeEvent }
  | { type: 'stream/error'; message: string }
  | { type: 'ui/selectTool'; toolCallId?: string }
  | { type: 'ui/selectTurn'; turnId?: string }
  | { type: 'optimistic/add'; message: OptimisticMessage }
  | { type: 'optimistic/confirm'; id: string }
  | { type: 'optimistic/fail'; id: string }
  | { type: 'sending'; sending: boolean }
  | { type: 'error'; error: string }
  | { type: 'reset' }

export const initialAgentCockpitState: AgentCockpitState = {
  streamStatus: 'offline',
  healthStatus: 'unknown',
  sessions: [],
  activeSessionId: '',
  session: null,
  events: [],
  turns: [],
  optimisticMessages: [],
  loadingSessions: false,
  loadingSession: false,
  sending: false,
  error: '',
}

export function agentCockpitReducer(state: AgentCockpitState, action: AgentCockpitAction): AgentCockpitState {
  switch (action.type) {
    case 'reset':
      return initialAgentCockpitState
    case 'sessions/loading':
      return { ...state, loadingSessions: action.loading }
    case 'sessions/loaded':
      return { ...state, sessions: action.sessions, loadingSessions: false }
    case 'session/loading':
      return { ...state, loadingSession: action.loading }
    case 'session/select':
      return { ...state, activeSessionId: action.sessionId, selectedToolCallId: undefined, selectedTurnId: undefined }
    case 'session/loaded': {
      const session = action.session
      const derived = session ? buildTurnsFromSession(session) : []
      const selectedToolCallId = state.selectedToolCallId && sessionToolCalls(session).some((tool) => tool.id === state.selectedToolCallId)
        ? state.selectedToolCallId
        : sessionToolCalls(session)[0]?.id
      const liveTurns = session
        ? state.turns.filter((turn) => turn.sessionId === session.session_id)
        : []
      return {
        ...state,
        session,
        activeSessionId: session?.session_id || state.activeSessionId,
        loadingSession: false,
        turns: mergeTurns(derived, liveTurns),
        selectedToolCallId,
        optimisticMessages: removeConfirmedOptimistic(state.optimisticMessages, session?.messages || []),
      }
    }
    case 'session/deleted': {
      const sessions = action.sessions || state.sessions.filter((item) => item.session_id !== action.sessionId)
      const activeDeleted = state.activeSessionId === action.sessionId
      return {
        ...state,
        sessions,
        activeSessionId: activeDeleted ? '' : state.activeSessionId,
        session: activeDeleted ? null : state.session,
        turns: activeDeleted ? [] : state.turns,
        events: activeDeleted ? [] : state.events,
        selectedToolCallId: activeDeleted ? undefined : state.selectedToolCallId,
        selectedTurnId: activeDeleted ? undefined : state.selectedTurnId,
      }
    }
    case 'stream/status':
      return { ...state, streamStatus: action.status }
    case 'health/status':
      return { ...state, healthStatus: action.status }
    case 'stream/event':
      return applyRuntimeEvent(state, action.event)
    case 'stream/error':
      return { ...state, streamStatus: 'error', error: action.message }
    case 'ui/selectTool':
      return { ...state, selectedToolCallId: action.toolCallId }
    case 'ui/selectTurn':
      return { ...state, selectedTurnId: action.turnId }
    case 'optimistic/add':
      return { ...state, optimisticMessages: [...state.optimisticMessages, action.message], sending: true }
    case 'optimistic/confirm':
      return { ...state, optimisticMessages: state.optimisticMessages.map((msg) => msg.id === action.id ? { ...msg, status: 'confirmed' } : msg) }
    case 'optimistic/fail':
      return { ...state, optimisticMessages: state.optimisticMessages.map((msg) => msg.id === action.id ? { ...msg, status: 'failed' } : msg) }
    case 'sending':
      return { ...state, sending: action.sending }
    case 'error':
      return { ...state, error: action.error }
    default:
      return state
  }
}

function applyRuntimeEvent(state: AgentCockpitState, incoming: AgentRuntimeEvent): AgentCockpitState {
  const timestamp = eventTimestamp(incoming)
  if (incoming.type === 'heartbeat') {
    return { ...state, streamStatus: 'online', lastHeartbeatAt: timestamp }
  }
  if (incoming.type === 'snapshot' && incoming.session) {
    return agentCockpitReducer(state, { type: 'session/loaded', session: incoming.session })
  }

  const event = normalizeTimelineEvent(incoming, timestamp)
  const events = appendUniqueEvent(state.events, event)
  let session = state.session
  let turns = state.turns
  let selectedToolCallId = state.selectedToolCallId

  if (incoming.activity) {
    turns = applyActivityToTurns(turns, incoming.activity, session)
  }

  switch (incoming.type) {
    case 'turn_started':
      turns = upsertTurn(turns, {
        id: event.turn_id || event.id,
        taskId: event.task_id,
        sessionId: event.session_id || session?.session_id,
        title: event.title || event.message || 'Turn',
        status: event.status || 'running',
        startedAt: timestamp,
        modelText: '',
        reasoning: '',
        messages: [],
        toolCallIds: [],
        todoIds: [],
        eventIds: [event.id],
      })
      break
    case 'model_token':
      turns = patchTurn(turns, event.turn_id || latestTurnId(turns), (turn) => ({
        ...turn,
        modelText: `${turn.modelText}${event.delta || event.text || event.message || ''}`,
        eventIds: unique([...turn.eventIds, event.id]),
      }))
      break
    case 'model_reasoning':
      turns = patchTurn(turns, event.turn_id || latestTurnId(turns), (turn) => ({
        ...turn,
        reasoning: `${turn.reasoning}${event.delta || event.reasoning || event.text || event.message || ''}`,
        eventIds: unique([...turn.eventIds, event.id]),
      }))
      break
    case 'tool_started':
    case 'tool_progress':
    case 'tool_finished': {
      const tool = toolCallFromEvent(event)
      if (tool) {
        session = upsertSessionTool(session, tool)
        selectedToolCallId = selectedToolCallId || tool.id
        turns = patchTurn(turns, event.turn_id || latestTurnId(turns), (turn) => ({
          ...turn,
          toolCallIds: unique([...turn.toolCallIds, tool.id]),
          eventIds: unique([...turn.eventIds, event.id]),
          status: incoming.type === 'tool_finished' ? turn.status : 'running',
        }))
      }
      break
    }
    case 'todo_updated': {
      const todo = event.todo || todoFromEvent(event)
      if (todo) {
        session = upsertSessionTodo(session, todo)
        turns = patchTurn(turns, event.turn_id || latestTurnId(turns), (turn) => ({
          ...turn,
          todoIds: unique([...turn.todoIds, todo.id]),
          eventIds: unique([...turn.eventIds, event.id]),
        }))
      }
      break
    }
    case 'approval_required':
      session = event.approval && session ? { ...session, pending_authorization: event.approval } : session
      turns = patchTurn(turns, event.turn_id || latestTurnId(turns), (turn) => ({
        ...turn,
        status: 'approval_required',
        eventIds: unique([...turn.eventIds, event.id]),
      }))
      break
    case 'approval_resolved':
      session = session ? { ...session, pending_authorization: undefined } : session
      turns = patchTurn(turns, event.turn_id || latestTurnId(turns), (turn) => ({
        ...turn,
        status: event.status || 'running',
        eventIds: unique([...turn.eventIds, event.id]),
      }))
      break
    case 'task_finished':
    case 'task_failed':
      turns = patchTurn(turns, event.turn_id || latestTurnId(turns), (turn) => ({
        ...turn,
        status: incoming.type === 'task_failed' ? 'failed' : 'done',
        finishedAt: timestamp,
        eventIds: unique([...turn.eventIds, event.id]),
      }))
      break
    case 'memory_compacted':
      turns = patchTurn(turns, event.turn_id || latestTurnId(turns), (turn) => ({ ...turn, eventIds: unique([...turn.eventIds, event.id]) }))
      break
  }

  return {
    ...state,
    session,
    turns,
    events,
    streamStatus: state.streamStatus === 'connecting' ? 'online' : state.streamStatus,
    selectedToolCallId,
  }
}

export function buildTurnsFromSession(session: AgentSession): AgentTurnView[] {
  const messages = session.messages || []
  const turns: AgentTurnView[] = []
  let current: AgentTurnView | null = null

  for (const message of messages) {
    if (message.role === 'user' || !current) {
      current = {
        id: `turn_${message.id || turns.length + 1}`,
        taskId: session.active_task?.id,
        sessionId: session.session_id,
        title: titleFromMessage(message),
        status: session.status || 'running',
        startedAt: message.timestamp || session.updated_at || session.created_at,
        modelText: '',
        reasoning: '',
        messages: [],
        toolCallIds: [],
        todoIds: [],
        eventIds: [],
      }
      turns.push(current)
    }
    current.messages.push(message)
    if (message.role === 'agent') {
      current.modelText = [current.modelText, message.content || ''].filter(Boolean).join(current.modelText ? '\n\n' : '')
      if (message.reasoning) current.reasoning = [current.reasoning, message.reasoning].filter(Boolean).join('\n\n')
    }
    for (const tool of message.tool_calls || []) {
      current.toolCallIds = unique([...current.toolCallIds, tool.id])
    }
  }

  if (!turns.length) {
    turns.push({
      id: `session_${session.session_id}`,
      taskId: session.active_task?.id,
      sessionId: session.session_id,
      title: session.active_task?.goal || 'Session ready',
      status: session.status || 'idle',
      startedAt: session.updated_at || session.created_at || new Date().toISOString(),
      modelText: '',
      reasoning: '',
      messages: [],
      toolCallIds: (session.tool_calls || []).map((tool) => tool.id),
      todoIds: (session.todos || []).map((todo) => todo.id),
      eventIds: [],
    })
  }

  const last = turns[turns.length - 1]
  last.toolCallIds = unique([...last.toolCallIds, ...(session.tool_calls || []).map((tool) => tool.id)])
  last.todoIds = unique([...last.todoIds, ...(session.todos || []).map((todo) => todo.id)])
  return turns
}

export function sessionToolCalls(session: AgentSession | null | undefined): AgentToolCall[] {
  if (!session) return []
  const byId = new Map<string, AgentToolCall>()
  for (const tool of session.tool_calls || []) byId.set(tool.id, tool)
  for (const message of session.messages || []) {
    for (const tool of message.tool_calls || []) byId.set(tool.id, tool)
  }
  return [...byId.values()].sort((a, b) => Date.parse(a.started_at || '') - Date.parse(b.started_at || ''))
}

export function sessionTodos(session: AgentSession | null | undefined): AgentTodo[] {
  return [...(session?.todos || [])].sort((a, b) => (a.priority || 0) - (b.priority || 0))
}

export function sessionRunSummary(session: AgentSession | null | undefined, sessions: AgentSessionSummary[], activeSessionId: string): AgentSessionRunSummary | undefined {
  return session?.run_summary || sessions.find((item) => item.session_id === activeSessionId)?.run_summary
}

export function nextActionSuggestedPrompt(nextAction: AgentNextAction | undefined) {
  if (!nextAction) return ''
  const record = nextAction as AgentNextAction & { suggest_prompt?: unknown; prompt?: unknown }
  for (const value of [record.suggested_prompt, record.suggest_prompt, record.prompt]) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

export function normalizeRuntimeEvent(type: string, payload: Record<string, unknown> | undefined): AgentRuntimeEvent {
  const record = payload || {}
  if (type === 'session' && record.session && typeof record.session === 'object') {
    return { type: 'snapshot', session: record.session as AgentSession, timestamp: new Date().toISOString() }
  }
  if (type === 'activity' && record.activity && typeof record.activity === 'object') {
    const activity = record.activity as AgentActivity
    return {
      type: 'activity',
      id: activity.id,
      timestamp: activity.timestamp,
      activity,
      title: activity.title,
      detail: activity.detail,
      status: activity.status,
      data: record,
    }
  }
  if (type === 'ping') {
    return { type: 'heartbeat', timestamp: typeof record.time === 'string' ? record.time : new Date().toISOString(), data: record }
  }
  if (type === 'error') {
    return { type: 'error', timestamp: new Date().toISOString(), error: record.error as AgentRuntimeEvent['error'], data: record }
  }
  return {
    type,
    id: stringFrom(record.id),
    task_id: stringFrom(record.task_id ?? record.taskId),
    session_id: stringFrom(record.session_id ?? record.sessionId),
    turn_id: stringFrom(record.turn_id ?? record.turnId ?? record.run_id),
    tool_call_id: stringFrom(record.tool_call_id ?? record.toolCallId),
    tool_name: stringFrom(record.tool_name ?? record.tool ?? record.name),
    timestamp: stringFrom(record.timestamp ?? record.created_at ?? record.time),
    message: stringFrom(record.message),
    delta: stringFrom(record.delta ?? record.text_delta),
    text: stringFrom(record.text ?? record.content),
    reasoning: stringFrom(record.reasoning ?? record.thinking_delta),
    status: stringFrom(record.status),
    title: stringFrom(record.title),
    detail: stringFrom(record.detail),
    data: record,
    session: record.session as AgentSession | undefined,
    activity: record.activity as AgentActivity | undefined,
    tool_call: record.tool_call as AgentToolCall | undefined,
    todo: record.todo as AgentTodo | undefined,
    approval: record.approval as AgentAuthorizationRequest | undefined,
    error: record.error as AgentRuntimeEvent['error'],
  }
}

function normalizeTimelineEvent(event: AgentRuntimeEvent, timestamp: string): TimelineEventView {
  return {
    ...event,
    id: event.id || event.tool_call_id || event.turn_id || `${event.type}_${timestamp}_${hashString(JSON.stringify(event.data || event.message || event.delta || ''))}`,
    timestamp,
  }
}

function eventTimestamp(event: AgentRuntimeEvent) {
  return event.timestamp || event.activity?.timestamp || new Date().toISOString()
}

function appendUniqueEvent(events: TimelineEventView[], event: TimelineEventView) {
  const next = events.some((item) => item.id === event.id && item.type === event.type) ? events : [...events, event]
  return next.slice(-500)
}

function applyActivityToTurns(turns: AgentTurnView[], activity: AgentActivity, session: AgentSession | null) {
  const latest = latestTurnId(turns) || (session ? `session_${session.session_id}` : 'stream')
  const seed: AgentTurnView = turns.find((turn) => turn.id === latest) || {
    id: latest,
    taskId: session?.active_task?.id,
    sessionId: session?.session_id,
    title: session?.active_task?.goal || activity.title || 'Activity',
    status: activity.status || session?.status || 'running',
    startedAt: activity.timestamp || new Date().toISOString(),
    modelText: '',
    reasoning: '',
    messages: [],
    toolCallIds: [],
    todoIds: [],
    eventIds: [],
  }
  return upsertTurn(turns, {
    ...seed,
    status: activity.status || seed.status,
    eventIds: unique([...seed.eventIds, activity.id]),
  })
}

function upsertTurn(turns: AgentTurnView[], next: AgentTurnView) {
  const index = turns.findIndex((turn) => turn.id === next.id)
  if (index < 0) return [...turns, next]
  return turns.map((turn, i) => i === index ? { ...turn, ...next, eventIds: unique([...turn.eventIds, ...next.eventIds]) } : turn)
}

function patchTurn(turns: AgentTurnView[], id: string | undefined, patch: (turn: AgentTurnView) => AgentTurnView) {
  if (!id) return turns
  const index = turns.findIndex((turn) => turn.id === id)
  if (index < 0) return upsertTurn(turns, patch({
    id,
    title: 'Live turn',
    status: 'running',
    startedAt: new Date().toISOString(),
    modelText: '',
    reasoning: '',
    messages: [],
    toolCallIds: [],
    todoIds: [],
    eventIds: [],
  }))
  return turns.map((turn, i) => i === index ? patch(turn) : turn)
}

function latestTurnId(turns: AgentTurnView[]) {
  return turns[turns.length - 1]?.id
}

function mergeTurns(derived: AgentTurnView[], live: AgentTurnView[]) {
  const byId = new Map<string, AgentTurnView>()
  for (const turn of live) byId.set(turn.id, turn)
  for (const turn of derived) {
    const existing = byId.get(turn.id)
    byId.set(turn.id, existing ? {
      ...existing,
      ...turn,
      modelText: turn.modelText || existing.modelText,
      reasoning: turn.reasoning || existing.reasoning,
      toolCallIds: unique([...existing.toolCallIds, ...turn.toolCallIds]),
      todoIds: unique([...existing.todoIds, ...turn.todoIds]),
      eventIds: unique([...existing.eventIds, ...turn.eventIds]),
    } : turn)
  }
  return [...byId.values()].sort((a, b) => Date.parse(a.startedAt) - Date.parse(b.startedAt))
}

function removeConfirmedOptimistic(items: OptimisticMessage[], messages: AgentMessage[]) {
  if (!items.length) return items
  return items.filter((item) => !messages.some((message) => message.role === 'user' && message.content.trim() === item.content.trim()))
}

function upsertSessionTool(session: AgentSession | null, tool: AgentToolCall): AgentSession | null {
  if (!session) return session
  const tools = upsertById(session.tool_calls || [], tool)
  return { ...session, tool_calls: tools, updated_at: tool.finished_at || tool.started_at || session.updated_at }
}

function upsertSessionTodo(session: AgentSession | null, todo: AgentTodo): AgentSession | null {
  if (!session) return session
  return { ...session, todos: upsertById(session.todos || [], todo), updated_at: todo.updated_at || session.updated_at }
}

function upsertById<T extends { id: string }>(items: T[], item: T) {
  return items.some((entry) => entry.id === item.id)
    ? items.map((entry) => entry.id === item.id ? { ...entry, ...item } : entry)
    : [...items, item]
}

function toolCallFromEvent(event: TimelineEventView): AgentToolCall | null {
  if (event.tool_call) return event.tool_call
  const id = event.tool_call_id || event.id
  const name = event.tool_name || stringFrom(event.data?.tool) || 'tool'
  if (!id || !name) return null
  const status = event.type === 'tool_finished'
    ? (event.status || 'succeeded')
    : event.type === 'tool_progress'
      ? (event.status || 'running')
      : (event.status || 'running')
  return {
    id,
    name,
    status,
    detail: event.detail || event.message,
    started_at: event.timestamp,
    finished_at: event.type === 'tool_finished' ? event.timestamp : undefined,
    result: recordFrom(event.data?.result ?? event.data?.output),
    error: event.error && typeof event.error === 'object' ? { code: event.error.code || 'error', message: event.error.message || 'tool error' } : null,
    metadata: event.data,
  }
}

function todoFromEvent(event: TimelineEventView): AgentTodo | null {
  const raw = recordFrom(event.data?.todo ?? event.data)
  if (!raw) return null
  const id = stringFrom(raw.id ?? event.id)
  const title = stringFrom(raw.title ?? event.title ?? event.message)
  if (!id || !title) return null
  return {
    id,
    title,
    detail: stringFrom(raw.detail),
    status: stringFrom(raw.status) || event.status || 'pending',
    priority: numberFrom(raw.priority, 0),
    tool_call_id: stringFrom(raw.tool_call_id),
    updated_at: stringFrom(raw.updated_at) || event.timestamp,
  }
}

function titleFromMessage(message: AgentMessage) {
  const content = String(message.content || '').trim().replace(/\s+/g, ' ')
  return content ? truncate(content, 64) : message.role
}

function unique<T>(items: T[]): T[] {
  return Array.from(new Set(items))
}

function stringFrom(value: unknown): string {
  return typeof value === 'string' ? value.trim() : value === undefined || value === null ? '' : String(value)
}

function numberFrom(value: unknown, fallback: number) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function recordFrom(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined
}

function truncate(value: string, max: number) {
  return value.length > max ? `${value.slice(0, max)}…` : value
}

function hashString(value: string): string {
  let hash = 0
  for (let i = 0; i < value.length; i += 1) hash = ((hash << 5) - hash + value.charCodeAt(i)) | 0
  return Math.abs(hash).toString(36)
}
