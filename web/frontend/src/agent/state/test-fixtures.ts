import type { AgentSession } from '@/lib/api'
import type { AgentRuntimeEvent } from './store'

export function sampleSession(overrides: Partial<AgentSession> = {}): AgentSession {
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

export function runtimeEvent(type: string, seq: number, data: Record<string, unknown> = {}, overrides: Partial<AgentRuntimeEvent> = {}): AgentRuntimeEvent {
  return {
    seq,
    type,
    task_id: 'task-1',
    run_id: 'run-1',
    turn_id: 'turn-1',
    timestamp: `2026-06-01T00:00:${String(seq).padStart(2, '0')}.000Z`,
    data: { session_id: 'session-1', ...data },
    ...overrides,
  }
}

export function toSseBlock(event: AgentRuntimeEvent): string {
  return `id: ${event.seq || ''}\nevent: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`
}
