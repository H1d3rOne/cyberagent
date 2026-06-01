import { describe, expect, it, vi } from 'vitest'
import type { Mock } from 'vitest'
import { ApiClient, agentSessionStreamPath, handleAgentSessionSSEBlock, parseAgentSessionSSEBlock } from '../api'

describe('agent session SSE helpers', () => {
  it('builds stream paths with optional from_seq', () => {
    expect(agentSessionStreamPath('session id')).toBe('/api/v1/agent/sessions/session%20id/stream')
    expect(agentSessionStreamPath('session id', 42)).toBe('/api/v1/agent/sessions/session%20id/stream?from_seq=42')
  })

  it('parses SSE id, event and seq metadata', () => {
    const parsed = parseAgentSessionSSEBlock('id: 12\nevent: model_token\ndata: {"seq":12,"type":"model_token","data":{"delta":"hi"}}\n\n')
    expect(parsed?.event).toBe('model_token')
    expect(parsed?.meta.id).toBe('12')
    expect(parsed?.meta.seq).toBe(12)
    expect(parsed?.payload.data).toEqual({ delta: 'hi' })
  })

  it('dispatches onLastSeq and onEvent from runtime SSE blocks', () => {
    const onLastSeq = vi.fn()
    const onEvent = vi.fn()
    handleAgentSessionSSEBlock('id: 13\nevent: tool_finished\ndata: {"seq":13,"type":"tool_finished","data":{"session_id":"s"}}\n\n', { onLastSeq, onEvent })
    expect(onLastSeq).toHaveBeenCalledWith(13)
    expect(onEvent).toHaveBeenCalledWith('tool_finished', expect.objectContaining({ seq: 13 }), { id: '13', seq: 13 })
  })

  it('asks fetch stream endpoint with from_seq and exposes runtime events', async () => {
    const originalFetch = globalThis.fetch
    const onEvent = vi.fn()
    const onLastSeq = vi.fn()
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('id: 21\nevent: model_token\ndata: {"seq":21,"type":"model_token","data":{"session_id":"s","delta":"x"}}\n\n'))
        controller.close()
      },
    })
    const fetchMock = vi.fn(async () => new Response(stream, { status: 200, headers: { 'Content-Type': 'text/event-stream' } }))
    globalThis.fetch = fetchMock as typeof fetch
    try {
      const client = new ApiClient({ baseUrl: 'http://daemon.local', token: 'tok' })
      await client.streamAgentSession('s', { onEvent, onLastSeq }, undefined, { fromSeq: 20 })
      const calls = (fetchMock as Mock<typeof fetch>).mock.calls
      expect(String(calls[0]?.[0])).toBe('http://daemon.local/api/v1/agent/sessions/s/stream?from_seq=20')
      expect(onLastSeq).toHaveBeenCalledWith(21)
      expect(onEvent).toHaveBeenCalledWith('model_token', expect.objectContaining({ seq: 21 }), { id: '21', seq: 21 })
    } finally {
      globalThis.fetch = originalFetch
    }
  })
})
