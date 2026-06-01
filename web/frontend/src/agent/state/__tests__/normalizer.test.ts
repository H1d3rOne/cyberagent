import { describe, expect, it } from 'vitest'
import { normalizeRuntimeEvent } from '../store'

describe('normalizeRuntimeEvent V1 envelope compatibility', () => {
  it('unwraps current nested data payload fields', () => {
    const event = normalizeRuntimeEvent('model_token', {
      seq: 7,
      type: 'model_token',
      task_id: 'task-nested',
      run_id: 'run-nested',
      turn_id: 'turn-nested',
      timestamp: '2026-06-01T01:00:00.000Z',
      data: { session_id: 'session-nested', delta: 'hello', index: 1 },
    })
    expect(event.seq).toBe(7)
    expect(event.session_id).toBe('session-nested')
    expect(event.delta).toBe('hello')
    expect(event.index).toBe(1)
  })

  it('keeps frozen V1 top-level fields while preserving data', () => {
    const event = normalizeRuntimeEvent('tool_started', {
      seq: 8,
      type: 'tool_started',
      task_id: 'task-top',
      run_id: 'run-top',
      turn_id: 'turn-top',
      session_id: 'session-top',
      timestamp: '2026-06-01T01:00:01.000Z',
      data: { tool: 'http.request', args: { url: 'https://example.com' }, status: 'running' },
    })
    expect(event.task_id).toBe('task-top')
    expect(event.session_id).toBe('session-top')
    expect(event.tool_name).toBe('http.request')
    expect(event.data?.args).toEqual({ url: 'https://example.com' })
  })

  it('maps approval and authorization aliases into approval', () => {
    const event = normalizeRuntimeEvent('approval_required', {
      seq: 9,
      type: 'approval_required',
      data: {
        session_id: 'session-1',
        approval_id: 'approval-1',
        authorization: { id: 'approval-1', action_name: 'http.request', status: 'pending', created_at: '2026-06-01T01:00:02.000Z', updated_at: '2026-06-01T01:00:02.000Z' },
      },
    })
    expect(event.approval?.id).toBe('approval-1')
    expect(event.session_id).toBe('session-1')
  })

  it('repairs bad timestamps to a valid ISO-compatible timestamp', () => {
    const event = normalizeRuntimeEvent('turn_started', { type: 'turn_started', timestamp: 'not-a-date', data: {} })
    expect(Number.isFinite(Date.parse(event.timestamp || ''))).toBe(true)
  })

  it('accepts empty data without throwing and leaves optional fields blank', () => {
    const event = normalizeRuntimeEvent('tool_finished', { seq: 10, type: 'tool_finished' })
    expect(event.seq).toBe(10)
    expect(event.session_id).toBe('')
    expect(event.type).toBe('tool_finished')
  })
})
