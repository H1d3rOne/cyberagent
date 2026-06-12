import { describe, expect, it } from 'vitest'
import type { AgentTraceResponse } from '@/lib/api'
import {
  buildTraceErrorMarkers,
  buildTraceJsonSections,
  formatTraceJson,
  matchingJsonLines,
  traceSectionMatchesQuery,
} from './trace-json'

describe('trace json helpers', () => {
  it('splits large arrays into bounded JSON sections', () => {
    const trace = sampleTrace({
      timeline: Array.from({ length: 55 }, (_, index) => ({ id: `evt_${index}`, kind: 'event', timestamp: `t${index}` })),
    })

    const sections = buildTraceJsonSections(trace, 20)
    const timelineSections = sections.filter((section) => section.id.startsWith('timeline_'))

    expect(timelineSections.map((section) => section.range)).toEqual([
      { start: 0, end: 20, total: 55 },
      { start: 20, end: 40, total: 55 },
      { start: 40, end: 55, total: 55 },
    ])
    expect(timelineSections.every((section) => section.itemCount && section.itemCount <= 20)).toBe(true)
  })

  it('matches query against section metadata and JSON content', () => {
    const sections = buildTraceJsonSections(sampleTrace({
      tool_calls: [toolCall('tool_1', 'workspace.files.write', 'succeeded')],
    }))
    const tools = sections.find((section) => section.id === 'tool_calls')

    expect(tools).toBeTruthy()
    expect(traceSectionMatchesQuery(tools!, 'workspace.files.write')).toBe(true)
    expect(traceSectionMatchesQuery(tools!, '$.tool_calls')).toBe(true)
    expect(traceSectionMatchesQuery(tools!, 'not-present')).toBe(false)
  })

  it('builds error markers for failed or error-like trace sections', () => {
    const trace = sampleTrace({
      tool_calls: [toolCall('tool_failed', 'shell', 'failed', { code: 'exit_1', message: 'command failed' })],
      timeline: [{ id: 'evt_error', kind: 'tool_finished', status: 'failed', message: 'command failed' }],
    })

    const sections = buildTraceJsonSections(trace)
    const markers = buildTraceErrorMarkers(sections)

    expect(markers.some((marker) => marker.sectionId === 'tool_calls')).toBe(true)
    expect(markers.some((marker) => marker.sectionId === 'timeline')).toBe(true)
    expect(markers.map((marker) => marker.message).join('\n')).toContain('failed')
  })

  it('returns matching JSON lines with one-based line numbers', () => {
    const lines = matchingJsonLines('{\n  "status": "failed",\n  "message": "boom"\n}', 'failed')

    expect(lines).toEqual([{ line: 2, text: '  "status": "failed",' }])
  })

  it('surfaces JSON serialization failures instead of throwing', () => {
    const cyclic: Record<string, unknown> = { id: 'cyclic' }
    cyclic.self = cyclic

    const result = formatTraceJson(cyclic)

    expect(result.error).toContain('circular')
    expect(result.json).toContain('JSON serialization failed')
  })
})

function sampleTrace(overrides: Partial<AgentTraceResponse> = {}): AgentTraceResponse {
  return {
    trace_version: 'v1',
    generated_at: '2026-06-09T00:00:00Z',
    session_id: 'session_1',
    operator: 'tester',
    language: 'zh',
    operation_level: 'safe',
    status: 'running',
    created_at: '2026-06-09T00:00:00Z',
    updated_at: '2026-06-09T00:00:01Z',
    messages: [],
    tool_calls: [],
    timeline: [],
    summary: {},
    ...overrides,
  }
}

function toolCall(id: string, name: string, status: string, error?: { code: string; message: string }) {
  return {
    id,
    name,
    status,
    started_at: '2026-06-09T00:00:00Z',
    error: error || null,
  }
}
