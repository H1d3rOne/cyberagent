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
  | 'task_created'
  | 'run_started'
  | 'turn_started'
  | 'turn_finished'
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
  seq?: number
  id?: string
  task_id?: string
  run_id?: string
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
  origin?: string
  index?: number
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
  runId?: string
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

export interface AgentTaskTurnTimeline {
  id: string
  taskId: string
  runId?: string
  sessionId?: string
  status: string
  startedAt: string
  finishedAt?: string
  events: TimelineEventView[]
}

export interface AgentTaskTimeline {
  id: string
  sessionId?: string
  origin?: string
  title: string
  status: string
  createdAt: string
  updatedAt: string
  runIds: string[]
  turnIds: string[]
  events: TimelineEventView[]
  turns: Record<string, AgentTaskTurnTimeline>
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
  lastSeq: number
  seenEventKeys: string[]
  sessions: AgentSessionSummary[]
  activeSessionId: string
  session: AgentSession | null
  events: TimelineEventView[]
  tasks: Record<string, AgentTaskTimeline>
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
  lastSeq: 0,
  seenEventKeys: [],
  sessions: [],
  activeSessionId: '',
  session: null,
  events: [],
  tasks: {},
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
      return applyRuntimeEvent(state, normalizeRuntimeEvent(action.event.type, action.event as unknown as Record<string, unknown>))
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
    return { ...state, streamStatus: 'online', lastHeartbeatAt: timestamp, lastSeq: Math.max(state.lastSeq, incoming.seq || 0) }
  }
  if (incoming.type === 'snapshot' && incoming.session) {
    return agentCockpitReducer(state, { type: 'session/loaded', session: incoming.session })
  }

  const event = normalizeTimelineEvent(incoming, timestamp)
  const key = eventKey(event)
  if (state.seenEventKeys.includes(key)) return state

  const seenEventKeys = appendSeenEventKey(state.seenEventKeys, key)
  const events = appendUniqueEvent(state.events, event)
  const lastSeq = event.seq ? Math.max(state.lastSeq, event.seq) : state.lastSeq
  let session = state.session
  let turns = state.turns
  let selectedToolCallId = state.selectedToolCallId
  let error = state.error

  if (incoming.activity) {
    turns = applyActivityToTurns(turns, incoming.activity, session)
  }

  switch (event.type) {
    case 'task_created':
    case 'run_started':
      break
    case 'turn_started':
      turns = upsertTurn(turns, {
        id: event.turn_id || fallbackTurnId(event),
        taskId: event.task_id,
        runId: event.run_id,
        sessionId: event.session_id || session?.session_id,
        title: event.title || event.message || stringFrom(event.data?.prompt) || stringFrom(event.data?.goal) || 'Turn',
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
    case 'turn_finished':
      turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({
        ...turn,
        status: event.status || 'done',
        finishedAt: timestamp,
        eventIds: unique([...turn.eventIds, event.id]),
      }), timestamp, event)
      break
    case 'model_token':
      turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({
        ...turn,
        modelText: `${turn.modelText}${event.delta || event.text || event.message || ''}`,
        eventIds: unique([...turn.eventIds, event.id]),
      }), timestamp, event)
      break
    case 'model_reasoning':
      turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({
        ...turn,
        reasoning: `${turn.reasoning}${event.delta || event.reasoning || event.text || event.message || ''}`,
        eventIds: unique([...turn.eventIds, event.id]),
      }), timestamp, event)
      break
    case 'tool_started':
    case 'tool_progress':
    case 'tool_finished': {
      const tool = toolCallFromEvent(event)
      if (tool) {
        session = upsertSessionTool(session, tool)
        selectedToolCallId = selectedToolCallId || tool.id
        turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({
          ...turn,
          toolCallIds: unique([...turn.toolCallIds, tool.id]),
          eventIds: unique([...turn.eventIds, event.id]),
          status: event.type === 'tool_finished' ? turn.status : 'running',
        }), timestamp, event)
      }
      break
    }
    case 'todo_updated': {
      const todos = todosFromEvent(event)
      for (const todo of todos) {
        session = upsertSessionTodo(session, todo)
        turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({
          ...turn,
          todoIds: unique([...turn.todoIds, todo.id]),
          eventIds: unique([...turn.eventIds, event.id]),
        }), timestamp, event)
      }
      break
    }
    case 'approval_required': {
      const approval = event.approval || approvalFromEvent(event, 'pending')
      session = approval && session ? { ...session, pending_authorization: approval, updated_at: timestamp } : session
      turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({
        ...turn,
        status: 'approval_required',
        eventIds: unique([...turn.eventIds, event.id]),
      }), timestamp, event)
      break
    }
    case 'approval_resolved':
      session = shouldClearAgentPendingAuthorization(event) && session
        ? { ...session, pending_authorization: undefined, updated_at: timestamp }
        : session
      turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({
        ...turn,
        status: event.status || 'running',
        eventIds: unique([...turn.eventIds, event.id]),
      }), timestamp, event)
      break
    case 'task_finished':
    case 'task_failed':
      if (event.type === 'task_failed') error = eventMessage(event) || error
      turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({
        ...turn,
        status: event.type === 'task_failed' ? 'failed' : 'done',
        finishedAt: timestamp,
        eventIds: unique([...turn.eventIds, event.id]),
      }), timestamp, event)
      break
    case 'memory_compacted':
      session = applyMemorySummary(session, event, timestamp)
      turns = patchTurn(turns, eventTurnId(event, turns), (turn) => ({ ...turn, eventIds: unique([...turn.eventIds, event.id]) }), timestamp, event)
      break
    case 'error':
      error = eventMessage(event) || error
      break
  }

  return {
    ...state,
    session,
    turns,
    events,
    tasks: applyEventToTasks(state.tasks, event),
    streamStatus: state.streamStatus === 'connecting' ? 'online' : state.streamStatus,
    selectedToolCallId,
    lastSeq,
    seenEventKeys,
    error,
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
  const record = recordFrom(payload) || {}
  const data = recordFrom(record.data) || {}
  const eventType = stringFrom(record.type) || type

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
    return { type: 'heartbeat', seq: optionalNumberFrom(record.seq), timestamp: validTimestamp(record.time), data: record }
  }
  if (type === 'error') {
    return { type: 'error', seq: optionalNumberFrom(record.seq), timestamp: validTimestamp(record.timestamp), error: errorFrom(record.error), data: record, message: stringFrom(record.message) }
  }

  const toolCall = objectFromFirst(record.tool_call, data.tool_call) as AgentToolCall | undefined
  const todo = objectFromFirst(record.todo, data.todo) as AgentTodo | undefined
  const approval = objectFromFirst(record.approval, record.authorization, data.approval, data.authorization) as AgentAuthorizationRequest | undefined
  let normalizedData = Object.keys(data).length ? data : payloadDataFromEnvelope(record)
  if (record.approval_kind !== undefined && normalizedData.approval_kind === undefined) {
    normalizedData = { ...normalizedData, approval_kind: record.approval_kind }
  }
  if (record.approval_state !== undefined && normalizedData.approval_state === undefined) {
    normalizedData = { ...normalizedData, approval_state: record.approval_state }
  }
  const timestamp = validTimestamp(record.timestamp ?? data.timestamp ?? record.created_at ?? data.created_at ?? record.time ?? data.time)

  return {
    type: eventType,
    seq: optionalNumberFrom(record.seq ?? data.seq),
    id: stringFrom(record.id ?? data.id),
    task_id: stringFrom(record.task_id ?? record.taskId ?? data.task_id ?? data.taskId),
    run_id: stringFrom(record.run_id ?? record.runId ?? data.run_id ?? data.runId),
    session_id: stringFrom(record.session_id ?? record.sessionId ?? data.session_id ?? data.sessionId),
    turn_id: stringFrom(record.turn_id ?? record.turnId ?? data.turn_id ?? data.turnId),
    tool_call_id: stringFrom(record.tool_call_id ?? record.toolCallId ?? data.tool_call_id ?? data.toolCallId ?? toolCall?.id),
    tool_name: stringFrom(record.tool_name ?? record.tool ?? record.name ?? data.tool_name ?? data.tool ?? data.name ?? data.action_name ?? toolCall?.name),
    timestamp,
    message: stringFrom(record.message ?? data.message ?? data.reason),
    delta: textFrom(record.delta ?? data.delta ?? record.text_delta ?? data.text_delta ?? record.text ?? data.text),
    text: textFrom(record.text ?? data.text ?? record.content ?? data.content ?? record.delta ?? data.delta),
    reasoning: textFrom(record.reasoning ?? data.reasoning ?? record.thinking_delta ?? data.thinking_delta),
    status: stringFrom(record.status ?? data.status ?? toolCall?.status),
    title: stringFrom(record.title ?? data.title ?? data.goal ?? data.prompt),
    detail: stringFrom(record.detail ?? data.detail ?? data.message),
    origin: stringFrom(record.origin ?? data.origin),
    index: optionalNumberFrom(record.index ?? data.index ?? record.i ?? data.i),
    data: normalizedData,
    session: objectFromFirst(record.session, data.session) as AgentSession | undefined,
    activity: objectFromFirst(record.activity, data.activity) as AgentActivity | undefined,
    tool_call: toolCall,
    todo,
    approval,
    error: errorFrom(record.error ?? data.error),
  }
}

function normalizeTimelineEvent(event: AgentRuntimeEvent, timestamp: string): TimelineEventView {
  return {
    ...event,
    id: event.id || (event.seq ? `seq_${event.seq}` : '') || `${event.type}_${event.task_id || ''}_${event.run_id || ''}_${event.turn_id || ''}_${timestamp}_${hashString(JSON.stringify(event.data || event.message || event.delta || ''))}`,
    timestamp,
  }
}

function eventTimestamp(event: AgentRuntimeEvent) {
  return validTimestamp(event.timestamp || event.activity?.timestamp)
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
    startedAt: validTimestamp(activity.timestamp),
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

function patchTurn(turns: AgentTurnView[], id: string | undefined, patch: (turn: AgentTurnView) => AgentTurnView, timestamp?: string, event?: TimelineEventView) {
  if (!id) return turns
  const index = turns.findIndex((turn) => turn.id === id)
  if (index < 0) return upsertTurn(turns, patch({
    id,
    taskId: event?.task_id,
    runId: event?.run_id,
    sessionId: event?.session_id,
    title: event?.title || event?.message || 'Live turn',
    status: 'running',
    startedAt: timestamp || new Date().toISOString(),
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

function eventTurnId(event: TimelineEventView, turns: AgentTurnView[]) {
  return event.turn_id || latestTurnId(turns) || fallbackTurnId(event)
}

function fallbackTurnId(event: Pick<AgentRuntimeEvent, 'turn_id' | 'run_id' | 'task_id' | 'id'>) {
  return event.turn_id || (event.run_id ? `${event.run_id}_turn` : '') || (event.task_id ? `${event.task_id}_turn` : '') || event.id || 'stream_turn'
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
  const tools = mergeToolById(session.tool_calls || [], tool)
  return { ...session, tool_calls: tools, updated_at: tool.finished_at || tool.started_at || session.updated_at }
}

function upsertSessionTodo(session: AgentSession | null, todo: AgentTodo): AgentSession | null {
  if (!session) return session
  return { ...session, todos: upsertById(session.todos || [], todo), updated_at: todo.updated_at || session.updated_at }
}

function mergeToolById(items: AgentToolCall[], item: AgentToolCall) {
  return items.some((entry) => entry.id === item.id)
    ? items.map((entry) => entry.id === item.id ? {
      ...entry,
      ...item,
      started_at: entry.started_at || item.started_at,
      result: item.result || entry.result,
      error: item.error || entry.error,
      metadata: { ...(entry.metadata || {}), ...(item.metadata || {}) },
    } : entry)
    : [...items, item]
}

function upsertById<T extends { id: string }>(items: T[], item: T) {
  return items.some((entry) => entry.id === item.id)
    ? items.map((entry) => entry.id === item.id ? { ...entry, ...item } : entry)
    : [...items, item]
}

function toolCallFromEvent(event: TimelineEventView): AgentToolCall | null {
  const payloadTool = event.tool_call
  const payload = event.data || {}
  const name = event.tool_name || payloadTool?.name || stringFrom(payload.tool) || 'tool'
  const id = event.tool_call_id || payloadTool?.id || stringFrom(payload.tool_call_id) || `${event.turn_id || event.task_id || event.id}:${name}`
  if (!id || !name) return null
  const status = event.type === 'tool_finished'
    ? (event.status || (event.error ? 'failed' : 'succeeded'))
    : event.type === 'tool_progress'
      ? (event.status || 'running')
      : (event.status || 'running')
  const result = recordFrom(payload.result ?? payload.output ?? payloadTool?.result)
  const args = recordFrom(payload.args ?? payload.arguments)
  const metadata = compactRecord({
    ...payload,
    args,
    progress: payload.progress,
    latency_ms: payload.latency_ms,
  })
  return {
    ...(payloadTool || {}),
    id,
    name,
    status,
    detail: event.detail || event.message || payloadTool?.detail || stringFrom(payload.message),
    started_at: payloadTool?.started_at || event.timestamp,
    finished_at: event.type === 'tool_finished' ? (payloadTool?.finished_at || event.timestamp) : payloadTool?.finished_at,
    result,
    error: errorObjectFrom(event.error ?? payloadTool?.error),
    metadata,
  }
}

function todosFromEvent(event: TimelineEventView): AgentTodo[] {
  const payload = event.data || {}
  const rawItems = arrayFromFirst(payload.items, payload.todos, payload.todo, event.todo)
  return rawItems.map((raw, index) => todoFromRaw(raw, event, index)).filter((todo): todo is AgentTodo => Boolean(todo))
}

function todoFromRaw(value: unknown, event: TimelineEventView, index: number): AgentTodo | null {
  const raw = recordFrom(value)
  if (!raw) return null
  const id = stringFrom(raw.id ?? event.id)
  const title = stringFrom(raw.title ?? raw.content ?? event.title ?? event.message)
  if (!id || !title) return null
  return {
    id,
    title,
    detail: stringFrom(raw.detail ?? raw.message),
    status: stringFrom(raw.status) || event.status || 'pending',
    priority: numberFrom(raw.priority, index + 1),
    tool_call_id: stringFrom(raw.tool_call_id ?? raw.toolCallId),
    updated_at: validTimestamp(raw.updated_at ?? raw.updatedAt ?? event.timestamp),
  }
}

function approvalFromEvent(event: TimelineEventView, fallbackStatus: string): AgentAuthorizationRequest | undefined {
  const payload = event.data || {}
  const args = recordFrom(payload.args ?? payload.arguments) || {}
  const id = stringFrom(payload.approval_id ?? payload.authorization_id ?? event.id)
  const action = event.tool_name || stringFrom(payload.tool ?? payload.action_name ?? payload.action)
  if (!id && !action) return undefined
  return {
    id: id || `${event.id}:approval`,
    action_name: action || 'agent.action',
    arguments: args,
    required_level: stringFrom(payload.required_level),
    policy: stringFrom(payload.policy),
    reason: event.message || stringFrom(payload.reason),
    original_prompt: stringFrom(payload.original_prompt ?? payload.prompt),
    status: stringFrom(payload.status ?? payload.decision) || fallbackStatus,
    created_at: event.timestamp,
    updated_at: event.timestamp,
  }
}

function shouldClearAgentPendingAuthorization(event: TimelineEventView) {
  const kind = approvalKindFromEvent(event)
  if (kind === 'run_hitl') return false
  return kind === 'agent_authorization' || !kind
}

function approvalKindFromEvent(event: TimelineEventView) {
  const payload = event.data || {}
  const approvalState = recordFrom(payload.approval_state)
  return stringFrom(payload.approval_kind ?? approvalState?.approval_kind).toLowerCase()
}

function applyMemorySummary(session: AgentSession | null, event: TimelineEventView, timestamp: string): AgentSession | null {
  if (!session) return session
  const memory = recordFrom(event.data?.memory_summary)
  if (!memory) return { ...session, updated_at: timestamp }
  return { ...session, memory_summary: memory as unknown as AgentSession['memory_summary'], updated_at: timestamp }
}

function applyEventToTasks(tasks: Record<string, AgentTaskTimeline>, event: TimelineEventView): Record<string, AgentTaskTimeline> {
  const taskId = event.task_id
  if (!taskId) return tasks
  const timestamp = event.timestamp
  const existing = tasks[taskId]
  let task: AgentTaskTimeline = existing || {
    id: taskId,
    sessionId: event.session_id,
    origin: event.origin || stringFrom(event.data?.origin),
    title: event.title || stringFrom(event.data?.goal ?? event.data?.prompt) || taskId,
    status: 'created',
    createdAt: timestamp,
    updatedAt: timestamp,
    runIds: [],
    turnIds: [],
    events: [],
    turns: {},
  }

  task = {
    ...task,
    sessionId: event.session_id || task.sessionId,
    origin: event.origin || task.origin,
    title: task.title || event.title || stringFrom(event.data?.goal ?? event.data?.prompt) || taskId,
    updatedAt: timestamp,
    events: appendUniqueEvent(task.events, event),
  }

  if (event.run_id) task = { ...task, runIds: unique([...task.runIds, event.run_id]) }

  switch (event.type) {
    case 'task_created':
      task = { ...task, status: event.status || 'created', createdAt: timestamp }
      break
    case 'run_started':
      task = { ...task, status: event.status || 'running' }
      break
    case 'approval_required':
      task = { ...task, status: 'approval_required' }
      break
    case 'task_finished':
      task = { ...task, status: event.status || 'done' }
      break
    case 'task_failed':
      task = { ...task, status: event.status || 'failed' }
      break
  }

  const turnId = taskTurnIdForEvent(event, task)
  if (turnId && isTurnScopedEvent(event.type)) {
    const existingTurn = task.turns[turnId]
    let turn: AgentTaskTurnTimeline = existingTurn || {
      id: turnId,
      taskId,
      runId: event.run_id,
      sessionId: event.session_id,
      status: 'running',
      startedAt: timestamp,
      events: [],
    }
    turn = {
      ...turn,
      runId: event.run_id || turn.runId,
      sessionId: event.session_id || turn.sessionId,
      status: statusForTaskTurnEvent(event, turn.status),
      finishedAt: event.type === 'turn_finished' || event.type === 'task_finished' || event.type === 'task_failed' ? timestamp : turn.finishedAt,
      events: appendUniqueEvent(turn.events, event),
    }
    task = {
      ...task,
      turnIds: unique([...task.turnIds, turnId]),
      turns: { ...task.turns, [turnId]: turn },
    }
  }

  return { ...tasks, [taskId]: task }
}

function taskTurnIdForEvent(event: TimelineEventView, task: AgentTaskTimeline) {
  if (event.turn_id) return event.turn_id
  if (task.turnIds.length) return task.turnIds[task.turnIds.length - 1]
  if (event.run_id) return `${event.run_id}_turn`
  return event.type === 'task_created' || event.type === 'run_started' ? '' : `${task.id}_turn`
}

function isTurnScopedEvent(type: string) {
  return [
    'turn_started',
    'turn_finished',
    'model_token',
    'model_reasoning',
    'tool_started',
    'tool_progress',
    'tool_finished',
    'todo_updated',
    'approval_required',
    'approval_resolved',
    'memory_compacted',
    'task_finished',
    'task_failed',
  ].includes(type)
}

function statusForTaskTurnEvent(event: TimelineEventView, current: string) {
  if (event.type === 'turn_started') return event.status || 'running'
  if (event.type === 'turn_finished') return event.status || 'done'
  if (event.type === 'approval_required') return 'approval_required'
  if (event.type === 'approval_resolved') return event.status || 'running'
  if (event.type === 'task_failed') return 'failed'
  if (event.type === 'task_finished') return 'done'
  return current || event.status || 'running'
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

function textFrom(value: unknown): string {
  return typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value)
}

function numberFrom(value: unknown, fallback: number) {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

function optionalNumberFrom(value: unknown): number | undefined {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : undefined
}

function recordFrom(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined
}

function objectFromFirst(...values: unknown[]): Record<string, unknown> | undefined {
  for (const value of values) {
    const record = recordFrom(value)
    if (record) return record
  }
  return undefined
}

function arrayFromFirst(...values: unknown[]): unknown[] {
  for (const value of values) {
    if (Array.isArray(value)) return value
    if (value !== undefined && value !== null) return [value]
  }
  return []
}

function validTimestamp(value: unknown): string {
  const raw = stringFrom(value)
  if (raw && Number.isFinite(Date.parse(raw))) return raw
  return new Date().toISOString()
}

function payloadDataFromEnvelope(record: Record<string, unknown>): Record<string, unknown> {
  const skip = new Set(['seq', 'type', 'id', 'task_id', 'taskId', 'run_id', 'runId', 'turn_id', 'turnId', 'session_id', 'sessionId', 'timestamp', 'created_at', 'time', 'data'])
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(record)) {
    if (!skip.has(key)) out[key] = value
  }
  return out
}

function compactRecord(record: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(record)) {
    if (value !== undefined && value !== null && value !== '') out[key] = value
  }
  return out
}

function errorFrom(value: unknown): AgentRuntimeEvent['error'] {
  if (typeof value === 'string') return value
  const record = recordFrom(value)
  return record ? { code: stringFrom(record.code), message: stringFrom(record.message) || 'error' } : undefined
}

function errorObjectFrom(value: unknown): AgentToolCall['error'] {
  if (!value) return null
  if (typeof value === 'string') return { code: 'error', message: value }
  const record = recordFrom(value)
  if (!record) return null
  return { code: stringFrom(record.code) || 'error', message: stringFrom(record.message) || 'tool error' }
}

function eventMessage(event: AgentRuntimeEvent) {
  if (event.message) return event.message
  if (typeof event.error === 'string') return event.error
  if (event.error && typeof event.error === 'object') return event.error.message || event.error.code || ''
  return stringFrom(event.data?.message ?? event.data?.reason)
}

function eventKey(event: TimelineEventView) {
  return event.seq ? `seq:${event.seq}` : `${event.type}:${event.id}`
}

function appendSeenEventKey(keys: string[], key: string) {
  return unique([...keys, key]).slice(-1000)
}

function truncate(value: string, max: number) {
  return value.length > max ? `${value.slice(0, max)}…` : value
}

function hashString(value: string): string {
  let hash = 0
  for (let i = 0; i < value.length; i += 1) hash = ((hash << 5) - hash + value.charCodeAt(i)) | 0
  return Math.abs(hash).toString(36)
}
