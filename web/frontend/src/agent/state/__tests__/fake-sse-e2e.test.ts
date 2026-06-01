import { describe, expect, it } from 'vitest'
import { handleAgentSessionSSEBlock, type AgentSessionStreamHandlers } from '@/lib/api'
import { agentCockpitReducer, initialAgentCockpitState, normalizeRuntimeEvent, type AgentCockpitState, type AgentRuntimeEvent } from '../store'
import { runtimeEvent, toSseBlock } from '../test-fixtures'

class FakeEventSource {
  constructor(private readonly events: AgentRuntimeEvent[]) {}

  connect(fromSeq: number, handlers: AgentSessionStreamHandlers) {
    const delivered = this.events.filter((event) => (event.seq || 0) > fromSeq)
    for (const event of delivered) handleAgentSessionSSEBlock(toSseBlock(event), handlers)
    return delivered.length
  }
}

function reduceFromSource(source: FakeEventSource, initial: AgentCockpitState) {
  let state = initial
  const seen: AgentRuntimeEvent[] = []
  const handlers: AgentSessionStreamHandlers = {
    onEvent: (event, payload) => {
      const normalized = normalizeRuntimeEvent(event, payload)
      seen.push(normalized)
      state = agentCockpitReducer(state, { type: 'stream/event', event: normalized })
    },
  }
  const delivered = source.connect(state.lastSeq, handlers)
  return { state, seen, delivered }
}

describe('fake SSE event source reducer e2e', () => {
  it('reduces a full user turn into task and turn timelines', () => {
    const events = [
      runtimeEvent('task_created', 1, { goal: 'scan target', origin: 'user' }, { turn_id: '' }),
      runtimeEvent('run_started', 2, { state: 'running' }, { turn_id: '' }),
      runtimeEvent('turn_started', 3, { prompt: 'scan target' }),
      runtimeEvent('model_token', 4, { delta: 'Checking' }),
      runtimeEvent('model_token', 5, { delta: ' target' }),
      runtimeEvent('tool_started', 6, { tool: 'scan.web.safe', args: { target: 'example.com' } }, { tool_call_id: 'tool-scan' }),
      runtimeEvent('tool_finished', 7, { tool: 'scan.web.safe', result: { ok: true } }, { tool_call_id: 'tool-scan' }),
      runtimeEvent('model_token', 8, { delta: '. Done.' }),
      runtimeEvent('turn_finished', 9, { status: 'done' }),
      runtimeEvent('task_finished', 10),
    ]
    const { state } = reduceFromSource(new FakeEventSource(events), initialAgentCockpitState)
    expect(state.lastSeq).toBe(10)
    expect(state.tasks['task-1'].turns['turn-1'].events.map((event) => event.type)).toEqual([
      'turn_started',
      'model_token',
      'model_token',
      'tool_started',
      'tool_finished',
      'model_token',
      'turn_finished',
      'task_finished',
    ])
    expect(state.turns.find((turn) => turn.id === 'turn-1')?.modelText).toBe('Checking target. Done.')
    expect(state.tasks['task-1'].status).toBe('done')
  })

  it('reconnects with from_seq by only delivering incremental events', () => {
    const source = new FakeEventSource([
      runtimeEvent('turn_started', 1),
      runtimeEvent('model_token', 2, { delta: 'first' }),
      runtimeEvent('model_token', 3, { delta: ' second' }),
    ])
    const first = reduceFromSource(new FakeEventSource([runtimeEvent('turn_started', 1), runtimeEvent('model_token', 2, { delta: 'first' })]), initialAgentCockpitState)
    const second = reduceFromSource(source, first.state)
    expect(first.delivered).toBe(2)
    expect(second.delivered).toBe(1)
    expect(second.state.lastSeq).toBe(3)
    expect(second.state.turns.find((turn) => turn.id === 'turn-1')?.modelText).toBe('first second')
  })
})
