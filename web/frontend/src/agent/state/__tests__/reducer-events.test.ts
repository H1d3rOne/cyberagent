import { describe, expect, it } from 'vitest'
import { agentCockpitReducer, initialAgentCockpitState } from '../store'
import { runtimeEvent, sampleSession } from '../test-fixtures'

describe('agent cockpit reducer runtime event handling', () => {
  it('handles task/run/turn lifecycle and task timeline ordering', () => {
    let state = agentCockpitReducer(initialAgentCockpitState, { type: 'stream/event', event: runtimeEvent('task_created', 1, { goal: 'scan target', origin: 'user' }, { turn_id: '' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('run_started', 2, { state: 'running' }, { turn_id: '' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('turn_started', 3, { prompt: 'go' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('turn_finished', 4, { status: 'done' }) })
    expect(state.tasks['task-1'].status).toBe('running')
    expect(state.tasks['task-1'].turns['turn-1'].status).toBe('done')
    expect(state.tasks['task-1'].events.map((event) => event.type)).toEqual(['task_created', 'run_started', 'turn_started', 'turn_finished'])
  })

  it('streams model text and reasoning deltas', () => {
    let state = initialAgentCockpitState
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('turn_started', 1) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('model_token', 2, { delta: 'Hello' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('model_token', 3, { text: ' world' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('model_reasoning', 4, { delta: 'think' }) })
    const turn = state.turns.find((item) => item.id === 'turn-1')
    expect(turn?.modelText).toBe('Hello world')
    expect(turn?.reasoning).toBe('think')
  })

  it('upserts tool progress and final result into session tool calls', () => {
    let state = agentCockpitReducer(initialAgentCockpitState, { type: 'session/loaded', session: sampleSession() })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('turn_started', 1) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('tool_started', 2, { tool: 'http.request', args: { url: 'https://example.com' }, status: 'running' }, { tool_call_id: 'tool-live' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('tool_progress', 3, { tool: 'http.request', progress: 50, message: 'half' }, { tool_call_id: 'tool-live' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('tool_finished', 4, { tool: 'http.request', result: { ok: true }, latency_ms: 123 }, { tool_call_id: 'tool-live' }) })
    const tool = state.session?.tool_calls?.find((item) => item.id === 'tool-live')
    expect(tool?.status).toBe('succeeded')
    expect(tool?.result).toEqual({ ok: true })
    expect(tool?.metadata?.progress).toBe(50)
  })

  it('updates todos, approvals, memory compaction and terminal task status', () => {
    let state = agentCockpitReducer(initialAgentCockpitState, { type: 'session/loaded', session: sampleSession() })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('turn_started', 1) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('todo_updated', 2, { items: [{ id: 'todo-live', status: 'running', content: 'write report' }] }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('approval_required', 3, { approval_id: 'approval-1', tool: 'http.request', args: { method: 'POST' }, reason: 'needs approval' }) })
    expect(state.session?.todos?.some((todo) => todo.id === 'todo-live' && todo.title === 'write report')).toBe(true)
    expect(state.session?.pending_authorization?.id).toBe('approval-1')
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('approval_resolved', 4, { approval_id: 'approval-1', decision: 'approved' }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('memory_compacted', 5, { kept_turns: 5, dropped_turns: 40, summary_tokens: 99, memory_summary: { summary: 'kept context', message_count: 2, updated_at: '2026-06-01T00:00:05.000Z' } }) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('task_finished', 6) })
    expect(state.session?.pending_authorization).toBeUndefined()
    expect(state.session?.memory_summary?.summary).toBe('kept context')
    expect(state.tasks['task-1'].status).toBe('done')
  })

  it('does not clear agent pending authorization for run HITL approval resolution', () => {
    const pendingAuthorization = {
      id: 'agent-approval-1',
      action_name: 'http.request',
      status: 'pending',
      created_at: '2026-06-01T00:00:00.000Z',
      updated_at: '2026-06-01T00:00:00.000Z',
    }
    let state = agentCockpitReducer(initialAgentCockpitState, {
      type: 'session/loaded',
      session: sampleSession({ pending_authorization: pendingAuthorization }),
    })

    state = agentCockpitReducer(state, {
      type: 'stream/event',
      event: runtimeEvent('approval_resolved', 7, {
        approval_id: 'run-hitl-approval-1',
        decision: 'approved',
        approval_state: {
          approval_kind: 'run_hitl',
          state: 'approved',
          approval_id: 'run-hitl-approval-1',
        },
      }),
    })

    expect(state.session?.pending_authorization?.id).toBe('agent-approval-1')
  })

  it('records task_failed as error state', () => {
    let state = initialAgentCockpitState
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('turn_started', 1) })
    state = agentCockpitReducer(state, { type: 'stream/event', event: runtimeEvent('task_failed', 2, { message: 'boom' }) })
    expect(state.tasks['task-1'].status).toBe('failed')
    expect(state.error).toBe('boom')
  })
})
