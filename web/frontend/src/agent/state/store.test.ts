import { describe, expect, it } from 'vitest'
import { agentCockpitReducer, buildTurnsFromSession, initialAgentCockpitState, normalizeRuntimeEvent, sessionToolCalls } from './store'
import type { AgentSession } from '@/lib/api'

function sampleSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    session_id: 'session-1',
    created_at: '2026-06-01T00:00:00.000Z',
    updated_at: '2026-06-01T00:01:00.000Z',
    operator: 'tester',
    language: 'zh',
    operation_level: 'read_only',
    messages: [
      {
        id: 'msg-user-1',
        role: 'user',
        content: 'scan example.com',
        timestamp: '2026-06-01T00:00:01.000Z',
      },
      {
        id: 'msg-agent-1',
        role: 'agent',
        content: 'running',
        timestamp: '2026-06-01T00:00:02.000Z',
        reasoning: 'thinking',
        tool_calls: [
          {
            id: 'tool-1',
            name: 'scan.web.safe',
            status: 'running',
            started_at: '2026-06-01T00:00:03.000Z',
            metadata: { agent_phase: 'step' },
          },
        ],
      },
    ],
    todos: [
      {
        id: 'todo-1',
        title: 'collect evidence',
        status: 'pending',
        priority: 1,
        updated_at: '2026-06-01T00:00:02.500Z',
      },
    ],
    tool_calls: [
      {
        id: 'tool-1',
        name: 'scan.web.safe',
        status: 'running',
        started_at: '2026-06-01T00:00:03.000Z',
        metadata: { agent_phase: 'step' },
      },
    ],
    ...overrides,
  }
}

describe('agent cockpit store', () => {
  it('builds turns from session and deduplicates tool calls', () => {
    const turns = buildTurnsFromSession(sampleSession())
    expect(turns).toHaveLength(1)
    expect(turns[0].messages).toHaveLength(2)
    expect(turns[0].toolCallIds).toEqual(['tool-1'])
    expect(sessionToolCalls(sampleSession())).toHaveLength(1)
  })

  it('hydrates from snapshot and applies live stream events', () => {
    const session = sampleSession()
    let state = agentCockpitReducer(initialAgentCockpitState, { type: 'session/loaded', session })
    expect(state.activeSessionId).toBe('session-1')
    expect(state.turns).toHaveLength(1)

    state = agentCockpitReducer(state, { type: 'stream/event', event: { type: 'turn_started', session_id: 'session-1', turn_id: 'turn-live', title: 'Live turn', timestamp: '2026-06-01T00:01:01.000Z' } })
    state = agentCockpitReducer(state, { type: 'stream/event', event: { type: 'model_token', session_id: 'session-1', turn_id: 'turn-live', delta: 'Hel', timestamp: '2026-06-01T00:01:02.000Z' } })
    state = agentCockpitReducer(state, { type: 'stream/event', event: { type: 'model_reasoning', session_id: 'session-1', turn_id: 'turn-live', delta: 'think', timestamp: '2026-06-01T00:01:03.000Z' } })
    state = agentCockpitReducer(state, {
      type: 'stream/event',
      event: {
        type: 'tool_started',
        session_id: 'session-1',
        turn_id: 'turn-live',
        tool_call_id: 'tool-live',
        tool_name: 'http.request',
        detail: 'GET /',
        timestamp: '2026-06-01T00:01:04.000Z',
      },
    })
    state = agentCockpitReducer(state, {
      type: 'stream/event',
      event: {
        type: 'todo_updated',
        session_id: 'session-1',
        turn_id: 'turn-live',
        todo: { id: 'todo-live', title: 'write report', status: 'running', priority: 2, updated_at: '2026-06-01T00:01:05.000Z' },
        timestamp: '2026-06-01T00:01:05.000Z',
      },
    })
    state = agentCockpitReducer(state, {
      type: 'stream/event',
      event: {
        type: 'approval_required',
        session_id: 'session-1',
        turn_id: 'turn-live',
        approval: { id: 'auth-1', action_name: 'http.request', status: 'pending', created_at: '2026-06-01T00:01:06.000Z', updated_at: '2026-06-01T00:01:06.000Z' } as never,
        timestamp: '2026-06-01T00:01:06.000Z',
      },
    })
    state = agentCockpitReducer(state, {
      type: 'stream/event',
      event: {
        type: 'tool_finished',
        session_id: 'session-1',
        turn_id: 'turn-live',
        tool_call_id: 'tool-live',
        tool_name: 'http.request',
        status: 'succeeded',
        data: { result: { ok: true } },
        timestamp: '2026-06-01T00:01:07.000Z',
      },
    })
    state = agentCockpitReducer(state, { type: 'stream/event', event: { type: 'memory_compacted', session_id: 'session-1', turn_id: 'turn-live', message: 'summary injected', timestamp: '2026-06-01T00:01:08.000Z' } })

    expect(state.events.map((event) => event.type)).toContain('turn_started')
    expect(state.turns.some((turn) => turn.id === 'turn-live')).toBe(true)
    const liveTurn = state.turns.find((turn) => turn.id === 'turn-live')
    expect(liveTurn?.modelText).toBe('Hel')
    expect(liveTurn?.reasoning).toBe('think')
    expect(liveTurn?.toolCallIds).toContain('tool-live')
    expect(liveTurn?.todoIds).toContain('todo-live')
    expect(state.session?.pending_authorization?.id).toBe('auth-1')
    expect(state.session?.tool_calls?.some((tool) => tool.id === 'tool-live' && tool.status === 'succeeded')).toBe(true)
  })

  it('normalizes legacy stream payloads into snapshot and heartbeat events', () => {
    const snapshot = normalizeRuntimeEvent('session', { session: sampleSession() })
    const ping = normalizeRuntimeEvent('ping', { time: '2026-06-01T00:02:00.000Z' })
    expect(snapshot.type).toBe('snapshot')
    expect(snapshot.session?.session_id).toBe('session-1')
    expect(ping.type).toBe('heartbeat')
    expect(ping.timestamp).toBe('2026-06-01T00:02:00.000Z')
  })
})
