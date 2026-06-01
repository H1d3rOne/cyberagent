import { describe, expect, it } from 'vitest'
import { agentCockpitReducer, buildTurnsFromSession, initialAgentCockpitState, normalizeRuntimeEvent, sessionToolCalls } from './store'
import { runtimeEvent, sampleSession } from './test-fixtures'

describe('agent cockpit store legacy session helpers', () => {
  it('builds turns from session and deduplicates tool calls', () => {
    const turns = buildTurnsFromSession(sampleSession())
    expect(turns).toHaveLength(1)
    expect(turns[0].messages).toHaveLength(2)
    expect(turns[0].toolCallIds).toEqual(['tool-1'])
    expect(sessionToolCalls(sampleSession())).toHaveLength(1)
  })

  it('hydrates from snapshot and removes confirmed optimistic messages', () => {
    let state = agentCockpitReducer(initialAgentCockpitState, {
      type: 'optimistic/add',
      message: { id: 'optimistic-1', content: 'scan example.com', timestamp: '2026-06-01T00:00:00.000Z', status: 'sending' },
    })
    state = agentCockpitReducer(state, { type: 'session/loaded', session: sampleSession() })
    expect(state.activeSessionId).toBe('session-1')
    expect(state.turns).toHaveLength(1)
    expect(state.optimisticMessages).toHaveLength(0)
  })

  it('normalizes legacy stream payloads into snapshot and heartbeat events', () => {
    const snapshot = normalizeRuntimeEvent('session', { session: sampleSession() })
    const ping = normalizeRuntimeEvent('ping', { time: '2026-06-01T00:02:00.000Z' })
    expect(snapshot.type).toBe('snapshot')
    expect(snapshot.session?.session_id).toBe('session-1')
    expect(ping.type).toBe('heartbeat')
    expect(ping.timestamp).toBe('2026-06-01T00:02:00.000Z')
  })

  it('applies a nested model token to a live turn', () => {
    let state = agentCockpitReducer(initialAgentCockpitState, { type: 'session/loaded', session: sampleSession() })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('turn_started', 1, { prompt: 'hello' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('model_token', 2, { delta: 'Hel' }) })
    expect(state.turns.find((turn) => turn.id === 'turn-1')?.modelText).toBe('Hel')
  })
})
