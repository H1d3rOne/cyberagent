import { describe, expect, it } from 'vitest'
import { agentCockpitReducer, initialAgentCockpitState } from '../store'
import { runtimeEvent } from '../test-fixtures'

describe('agent cockpit reducer edge cases', () => {
  it('keeps unknown event types in the timeline without mutating tasks', () => {
    const state = agentCockpitReducer(initialAgentCockpitState, { type: 'stream/event', event: runtimeEvent('unknown_type', 1, { foo: 'bar' }, { task_id: '' }) })
    expect(state.events.map((event) => event.type)).toEqual(['unknown_type'])
    expect(Object.keys(state.tasks)).toHaveLength(0)
  })

  it('deduplicates duplicate seq events', () => {
    let state = agentCockpitReducer(initialAgentCockpitState, { type: 'stream/event', event: runtimeEvent('turn_started', 1) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('model_token', 2, { delta: 'A' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('model_token', 2, { delta: 'A' }) })
    expect(state.lastSeq).toBe(2)
    expect(state.turns.find((turn) => turn.id === 'turn-1')?.modelText).toBe('A')
    expect(state.events.filter((event) => event.seq === 2)).toHaveLength(1)
  })

  it('deduplicates replayed no-seq events by generated id/type', () => {
    const event = runtimeEvent('model_token', 0, { delta: 'A' }, { seq: undefined, id: 'evt-a' })
    let state = agentCockpitReducer(initialAgentCockpitState, { type: 'stream/event', event: runtimeEvent('turn_started', 0, {}, { seq: undefined, id: 'evt-turn' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event })
    state = agentCockpitReducer(state, { type: 'stream/event', event })
    expect(state.turns.find((turn) => turn.id === 'turn-1')?.modelText).toBe('A')
  })

  it('uses from_seq style replay by ignoring already seen records and accepting increments', () => {
    let state = initialAgentCockpitState
    for (const event of [runtimeEvent('turn_started', 1), runtimeEvent('model_token', 2, { delta: 'one' })]) {
      state = agentCockpitReducer(state, { type: 'stream/event', event })
    }
    const replayed = [runtimeEvent('model_token', 2, { delta: 'one' }), runtimeEvent('model_token', 3, { delta: ' two' })]
    for (const event of replayed) state = agentCockpitReducer(state, { type: 'stream/event', event })
    expect(state.lastSeq).toBe(3)
    expect(state.turns.find((turn) => turn.id === 'turn-1')?.modelText).toBe('one two')
  })
})
