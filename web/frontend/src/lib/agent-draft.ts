export const AGENT_DRAFT_STORAGE_KEY = 'cyberagent_agent_draft'

export interface AgentDraft {
  prompt: string
  source?: string
  created_at: string
  run_id?: string
  target?: string
}

export function saveAgentDraft(draft: Omit<AgentDraft, 'created_at'> & { created_at?: string }) {
  if (typeof window === 'undefined') return
  const prompt = draft.prompt.trim()
  if (!prompt) return
  window.localStorage.setItem(
    AGENT_DRAFT_STORAGE_KEY,
    JSON.stringify({
      ...draft,
      prompt,
      created_at: draft.created_at || new Date().toISOString(),
    }),
  )
}

export function consumeAgentDraft(): AgentDraft | null {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem(AGENT_DRAFT_STORAGE_KEY)
  if (!raw) return null
  window.localStorage.removeItem(AGENT_DRAFT_STORAGE_KEY)

  try {
    const parsed = JSON.parse(raw) as Partial<AgentDraft>
    const prompt = String(parsed.prompt || '').trim()
    if (!prompt) return null
    return {
      prompt,
      source: parsed.source,
      created_at: parsed.created_at || new Date().toISOString(),
      run_id: parsed.run_id,
      target: parsed.target,
    }
  } catch {
    const prompt = raw.trim()
    if (!prompt) return null
    return {
      prompt,
      source: 'legacy',
      created_at: new Date().toISOString(),
    }
  }
}
