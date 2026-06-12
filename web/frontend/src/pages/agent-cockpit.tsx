import { useCallback, useEffect, useId, useMemo, useReducer, useRef, useState } from 'react'
import { AlertTriangle, Bot, Code2, Download, Loader2, MessageSquare, Plus, RefreshCcw, ShieldCheck, X } from 'lucide-react'
import { GlassAlert, GlassPanel, GlassToolbar } from '@/components/glass'
import { Button } from '@/components/ui/button'
import type {
  AgentOperationLevel,
  AgentProviderConfig,
  AgentSession,
  AgentSessionSummary,
  AgentSessionStreamMeta,
  AgentTraceResponse,
  SendAgentMessageResponse,
} from '@/lib/api'
import { normalizeAgentOperationLevel } from '@/lib/api'
import { consumeAgentDraft } from '@/lib/agent-draft'
import { text, type Language, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'
import { AuthorizationGateBanner } from '@/agent/components/authorization-gate-banner'
import { Composer } from '@/agent/components/composer'
import { useDialogA11y } from '@/agent/components/dialog-a11y'
import { TaskTree } from '@/agent/components/task-tree'
import { ToolInspector } from '@/agent/components/tool-inspector'
import { TraceViewerDrawer } from '@/agent/components/trace-viewer'
import { TurnTimeline } from '@/agent/components/turn-timeline'
import {
  agentCockpitReducer,
  initialAgentCockpitState,
  sessionRunSummary,
  sessionToolCalls,
} from '@/agent/state/store'
import type { AgentRuntimeEvent, CockpitStreamStatus, OptimisticMessage } from '@/agent/state/store'
import {
  authorizationGateKey,
  buildLegacyAuthorizationGate,
  buildPendingAuthorizationGate,
  formatError,
  nextActionSuggestedPrompt,
  operationLevelLabel,
  sessionDisplayHint,
  sessionTitle,
  statusLabel,
} from '@/agent/utils'
import type { AuthorizationGate } from '@/agent/utils'

export function AgentCockpitPage() {
  const { client, settings, updateSettings, language } = useConsole()
  const [state, dispatch] = useReducer(agentCockpitReducer, initialAgentCockpitState)
  const [input, setInput] = useState('')
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [mobileTasksOpen, setMobileTasksOpen] = useState(false)
  const [authBusy, setAuthBusy] = useState(false)
  const [dismissedAuthorizationGates, setDismissedAuthorizationGates] = useState<string[]>([])
  const [traceOpen, setTraceOpen] = useState(false)
  const [traceLoading, setTraceLoading] = useState(false)
  const [traceData, setTraceData] = useState<AgentTraceResponse | null>(null)
  const chatEndRef = useRef<HTMLDivElement>(null)
  const draftConsumedRef = useRef(false)
  const sessionRef = useRef<AgentSession | null>(null)
  const streamSessionRef = useRef('')
  const lastSeqRef = useRef(0)

  useEffect(() => {
    sessionRef.current = state.session
  }, [state.session])

  const providerConfig = useMemo(() => ({
    provider: settings.agentProvider.provider,
    model: settings.agentProvider.model,
    api_key: settings.agentProvider.apiKey,
    base_url: settings.agentProvider.providerBaseUrl,
    instructions: settings.agentProvider.instructions,
    operation_level: settings.operationLevel,
    auth_profiles: (settings.apiAuthProfiles || []).filter((profile) => profile.enabled).map((profile) => ({
      id: profile.id,
      name: profile.name,
      type: profile.type,
      base_url: profile.baseUrl,
      token: '',
      username: profile.username,
      password: '',
      header_name: profile.headerName,
      header_value: '',
      cookie: '',
      enabled: profile.enabled,
    })),
    default_auth_profile_id: settings.defaultApiAuthProfileId,
  }), [settings.agentProvider, settings.apiAuthProfiles, settings.defaultApiAuthProfileId, settings.operationLevel])

  const setSessionSnapshot = useCallback((session: AgentSession | null) => {
    sessionRef.current = session
    dispatch({ type: 'session/loaded', session })
  }, [])

  const setError = useCallback((error: string) => {
    dispatch({ type: 'error', error })
  }, [])

  const setStreamStatus = useCallback((status: CockpitStreamStatus) => {
    dispatch({ type: 'stream/status', status })
  }, [])

  const refreshSessions = useCallback(async () => {
    dispatch({ type: 'sessions/loading', loading: true })
    setError('')
    try {
      const response = await client.listAgentSessions(40)
      const items = response.items || []
      dispatch({ type: 'sessions/loaded', sessions: items })
      return items
    } catch (err) {
      setError(formatError(err, language))
      dispatch({ type: 'sessions/loaded', sessions: [] })
      return []
    }
  }, [client, language, setError])

  const loadSession = useCallback(async (sessionId: string) => {
    if (!sessionId) return null
    dispatch({ type: 'session/loading', loading: true })
    setError('')
    try {
      const response = await client.getAgentSession(sessionId)
      setSessionSnapshot(response.session)
      return response.session
    } catch (err) {
      setError(formatError(err, language))
      return null
    } finally {
      dispatch({ type: 'session/loading', loading: false })
    }
  }, [client, language, setError, setSessionSnapshot])

  const createSession = useCallback(async () => {
    dispatch({ type: 'session/loading', loading: true })
    setError('')
    try {
      const response = await client.createAgentSession(settings.operator, language, settings.operationLevel)
      setSessionSnapshot(response.session)
      await refreshSessions()
      return response.session
    } catch (err) {
      setError(formatError(err, language))
      return null
    } finally {
      dispatch({ type: 'session/loading', loading: false })
    }
  }, [client, language, refreshSessions, setError, setSessionSnapshot, settings.operationLevel, settings.operator])

  useEffect(() => {
    let cancelled = false
    void (async () => {
      const items = await refreshSessions()
      if (cancelled || state.activeSessionId) return
      if (items[0]?.session_id) await loadSession(items[0].session_id)
    })()
    return () => { cancelled = true }
    // bootstrap only; active session changes are driven by explicit selection and SSE
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (draftConsumedRef.current) return
    draftConsumedRef.current = true
    const draft = consumeAgentDraft()
    if (draft?.prompt) setInput(draft.prompt)
  }, [])

  const dispatchStreamEvent = useCallback((event: string, payload: Record<string, unknown>, meta?: AgentSessionStreamMeta) => {
    const eventPayload: Record<string, unknown> = { ...payload }
    if (typeof eventPayload.type !== 'string') eventPayload.type = event
    if (eventPayload.id === undefined && meta?.id) eventPayload.id = meta.id
    if (eventPayload.seq === undefined && meta?.seq !== undefined) eventPayload.seq = meta.seq
    dispatch({ type: 'stream/event', event: eventPayload as unknown as AgentRuntimeEvent })
  }, [])

  useEffect(() => {
    if (!state.activeSessionId) {
      streamSessionRef.current = ''
      lastSeqRef.current = 0
      setStreamStatus('offline')
      return
    }

    const controller = new AbortController()
    let stopped = false
    if (streamSessionRef.current !== state.activeSessionId) {
      streamSessionRef.current = state.activeSessionId
      lastSeqRef.current = 0
    }

    const connect = async () => {
      while (!stopped && !controller.signal.aborted) {
        setStreamStatus(lastSeqRef.current > 0 ? 'degraded' : 'connecting')
        try {
          await client.streamAgentSession(state.activeSessionId, {
            onOpen: () => setStreamStatus('online'),
            onSession: (session) => setSessionSnapshot(session),
            onEvent: dispatchStreamEvent,
            onLastSeq: (seq) => {
              lastSeqRef.current = seq
            },
            onError: (streamError) => {
              dispatch({ type: 'stream/error', message: streamError.message || 'stream error' })
            },
          }, controller.signal, { fromSeq: lastSeqRef.current || undefined })
        } catch (err) {
          if (stopped || controller.signal.aborted) return
          const message = formatError(err, language)
          if (message.includes('401')) {
            setError(message)
            setStreamStatus('error')
            return
          }
          setStreamStatus('degraded')
        }
        if (!stopped && !controller.signal.aborted) await delay(1_500)
      }
    }

    void connect()
    return () => {
      stopped = true
      controller.abort()
    }
  }, [client, dispatchStreamEvent, language, setError, setSessionSnapshot, setStreamStatus, state.activeSessionId])

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [state.events.length, state.optimisticMessages.length, state.sending, state.turns.length])

  const currentOperationLevel = normalizeAgentOperationLevel(settings.operationLevel || state.session?.operation_level)
  const messages = state.session?.messages || []
  const nextAction = state.session?.next_action
  const suggestedNextPrompt = nextActionSuggestedPrompt(nextAction)
  const runSummary = sessionRunSummary(state.session, state.sessions, state.activeSessionId)
  const displayHint = sessionDisplayHint(state.session, language)
  const toolCalls = useMemo(() => sessionToolCalls(state.session), [state.session])
  const selectedTool = useMemo(
    () => toolCalls.find((tool) => tool.id === state.selectedToolCallId) || toolCalls[0],
    [state.selectedToolCallId, toolCalls],
  )
  const authorizationGate = useMemo(() => {
    const pendingGate = buildPendingAuthorizationGate(state.session?.pending_authorization, language, {
      approvalState: state.session?.approval_state,
      uiState: state.session?.ui_state,
    })
    if (pendingGate) return pendingGate
    const gate = buildLegacyAuthorizationGate(messages, language)
    if (!gate) return null
    return dismissedAuthorizationGates.includes(authorizationGateKey(gate)) ? null : gate
  }, [dismissedAuthorizationGates, language, messages, state.session?.approval_state, state.session?.pending_authorization, state.session?.ui_state])

  const refreshCurrentSession = useCallback(async () => {
    if (state.activeSessionId) await loadSession(state.activeSessionId)
    await refreshSessions()
  }, [loadSession, refreshSessions, state.activeSessionId])

  const mergeSendResponse = useCallback((base: AgentSession, response: SendAgentMessageResponse): AgentSession => ({
    ...base,
    updated_at: new Date().toISOString(),
    messages: response.messages || base.messages,
    tool_calls: response.tool_calls || base.tool_calls,
    active_task: response.active_task || base.active_task,
    todos: response.todos || base.todos,
    context_refs: response.context_refs || base.context_refs,
    pending_changes: response.pending_changes || base.pending_changes,
    pending_authorization: response.pending_authorization,
    approval_state: response.approval_state,
    ui_state: response.ui_state,
    memory_summary: response.memory_summary || base.memory_summary,
    next_action: response.next_action || base.next_action,
  }), [])

  const sendMessage = useCallback(async (
    override?: string,
    providerOverride?: Partial<AgentProviderConfig>,
    options?: { force?: boolean; keepInput?: boolean },
  ) => {
    const content = (override || input).trim()
    if (!content || (state.sending && !options?.force)) return
    if (!options?.keepInput) setInput('')
    const optimistic: OptimisticMessage = {
      id: `optimistic_${Date.now()}`,
      content,
      timestamp: new Date().toISOString(),
      status: 'sending',
    }
    dispatch({ type: 'optimistic/add', message: optimistic })
    setError('')
    try {
      const ensured = sessionRef.current || await createSession()
      if (!ensured) {
        dispatch({ type: 'optimistic/fail', id: optimistic.id })
        return
      }
      const response = await client.sendAgentMessage(ensured.session_id, content, settings.operator, language, {
        ...providerConfig,
        ...(providerOverride || {}),
      })
      dispatch({ type: 'optimistic/confirm', id: optimistic.id })
      setSessionSnapshot(mergeSendResponse(ensured, response))
      await refreshSessions()
    } catch (err) {
      dispatch({ type: 'optimistic/fail', id: optimistic.id })
      setError(formatError(err, language))
    } finally {
      dispatch({ type: 'sending', sending: false })
    }
  }, [client, createSession, input, language, mergeSendResponse, providerConfig, refreshSessions, setError, setSessionSnapshot, settings.operator, state.sending])

  const runControl = useCallback(async (kind: 'continue' | 'pause' | 'stop' | 'cancel' | 'new-task', prompt = '') => {
    if (!state.session) return
    dispatch({ type: 'sending', sending: true })
    setError('')
    try {
      const request = {
        operator: settings.operator,
        language,
        operation_level: settings.operationLevel,
        prompt,
      }
      const response = kind === 'continue'
        ? await client.continueAgentSession(state.session.session_id, request)
        : kind === 'pause'
          ? await client.pauseAgentSession(state.session.session_id, request)
          : kind === 'stop'
            ? await client.stopAgentSession(state.session.session_id, request)
            : kind === 'cancel'
              ? await client.cancelAgentSession(state.session.session_id, request)
              : await client.newAgentTask(state.session.session_id, request)
      setSessionSnapshot(response.session)
      await refreshSessions()
    } catch (err) {
      setError(formatError(err, language))
    } finally {
      dispatch({ type: 'sending', sending: false })
    }
  }, [client, language, refreshSessions, setError, setSessionSnapshot, settings.operationLevel, settings.operator, state.session])

  const deleteSession = useCallback(async (sessionId: string) => {
    setError('')
    try {
      await client.deleteAgentSession(sessionId)
      const items = await refreshSessions()
      dispatch({ type: 'session/deleted', sessionId, sessions: items })
      if (state.activeSessionId === sessionId) {
        if (items[0]?.session_id) await loadSession(items[0].session_id)
        else setSessionSnapshot(null)
      }
    } catch (err) {
      setError(formatError(err, language))
    }
  }, [client, language, loadSession, refreshSessions, setError, setSessionSnapshot, state.activeSessionId])

  const loadTrace = useCallback(async () => {
    if (!state.session) return null
    setTraceLoading(true)
    setError('')
    try {
      const trace = await client.getAgentSessionTrace(state.session.session_id)
      setTraceData(trace)
      return trace
    } catch (err) {
      setError(formatError(err, language))
      return null
    } finally {
      setTraceLoading(false)
    }
  }, [client, language, setError, state.session])

  const openTraceViewer = useCallback(async () => {
    setTraceOpen(true)
    await loadTrace()
  }, [loadTrace])

  const downloadTrace = useCallback(async () => {
    if (!state.session) return
    setError('')
    try {
      const trace = traceData?.session_id === state.session.session_id ? traceData : await client.getAgentSessionTrace(state.session.session_id)
      const blob = new Blob([JSON.stringify(trace, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `cyberagent-trace-${state.session.session_id}.json`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError(formatError(err, language))
    }
  }, [client, language, setError, state.session, traceData])

  const fillInput = useCallback((value?: string) => {
    if (value) setInput(value)
  }, [])

  const authorizeAndRetry = useCallback(async (gate: AuthorizationGate) => {
    const requiredLevel = gate.requiredLevel
    const nextLevel = agentOperationLevelAtLeast(settings.operationLevel, requiredLevel) ? settings.operationLevel : requiredLevel
    const gateKey = authorizationGateKey(gate)
    setAuthBusy(true)
    setError('')
    try {
      if (nextLevel !== settings.operationLevel) updateSettings({ operationLevel: nextLevel })
      if (state.session && gate.source === 'pending') {
        const response = await client.approveAgentAuthorization(state.session.session_id, {
          authorization_id: gate.id,
          operator: settings.operator,
          language,
          ...providerConfig,
          operation_level: nextLevel,
        })
        setSessionSnapshot(response.session)
        await refreshSessions()
      } else if (gate.prompt) {
        setDismissedAuthorizationGates((items) => Array.from(new Set([...items, gateKey])))
        await sendMessage(gate.prompt, { operation_level: nextLevel }, { force: true })
      }
    } catch (err) {
      setError(formatError(err, language))
    } finally {
      setAuthBusy(false)
    }
  }, [client, language, providerConfig, refreshSessions, sendMessage, setError, setSessionSnapshot, settings.operationLevel, settings.operator, state.session, updateSettings])

  const denyAuthorizationGate = useCallback(async (gate: AuthorizationGate) => {
    setAuthBusy(true)
    setError('')
    try {
      if (state.session && gate.source === 'pending') {
        const response = await client.denyAgentAuthorization(state.session.session_id, {
          authorization_id: gate.id,
          operator: settings.operator,
          language,
          operation_level: settings.operationLevel,
          reason: text(language, `拒绝执行 ${gate.action}`, `Deny ${gate.action}`),
        })
        setSessionSnapshot(response.session)
        await refreshSessions()
      } else {
        setDismissedAuthorizationGates((items) => Array.from(new Set([...items, authorizationGateKey(gate)])))
        setInput(text(language, `拒绝执行 ${gate.action}。请换一个低风险方案。`, `Deny ${gate.action}. Please choose a lower-risk alternative.`))
      }
    } catch (err) {
      setError(formatError(err, language))
    } finally {
      setAuthBusy(false)
    }
  }, [client, language, refreshSessions, setError, setSessionSnapshot, settings.operationLevel, settings.operator, state.session])

  return (
    <div className="flex min-h-[calc(100dvh-6.5rem)] min-w-0 flex-col gap-3 overflow-hidden xl:grid xl:h-[calc(100dvh-6.5rem)] xl:min-h-0 xl:grid-cols-[auto_minmax(0,1fr)_340px]">
      <MobileTaskSwitcher
        sessions={state.sessions}
        activeSessionId={state.activeSessionId}
        activeTitle={state.session ? sessionTitle(state.session, language) : text(language, 'Cybersecurity Agent', 'Cybersecurity Agent')}
        loading={state.loadingSessions || state.loadingSession}
        open={mobileTasksOpen}
        language={language}
        onToggle={() => setMobileTasksOpen((value) => !value)}
        onCreate={() => {
          setMobileTasksOpen(false)
          void createSession()
        }}
        onSelect={(sessionId) => {
          setMobileTasksOpen(false)
          void loadSession(sessionId)
        }}
        onRefresh={() => { void refreshSessions() }}
      />

      <TaskTree
        sessions={state.sessions}
        activeSessionId={state.activeSessionId}
        loading={state.loadingSessions || state.loadingSession}
        collapsed={sidebarCollapsed}
        language={language}
        onToggleCollapsed={() => setSidebarCollapsed((value) => !value)}
        onCreate={() => { void createSession() }}
        onSelect={(sessionId) => { void loadSession(sessionId) }}
        onRefresh={() => { void refreshSessions() }}
        onDelete={(sessionId) => { void deleteSession(sessionId) }}
      />

      <GlassPanel as="section" variant="elevated" className="flex min-h-[72dvh] min-w-0 flex-1 flex-col overflow-hidden xl:min-h-0">
        <GlassToolbar as="header" className="flex min-h-14 shrink-0 items-center justify-between gap-2 border-b px-3 py-2 sm:gap-3 sm:px-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="relative grid h-9 w-9 shrink-0 place-items-center rounded-xl border border-primary/30 bg-primary/10">
              <Bot className="h-4 w-4 text-primary" />
              <span className={cn('absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full', streamDotClass(state.streamStatus))} />
            </span>
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold">
                {state.session ? sessionTitle(state.session, language) : text(language, 'Cybersecurity Agent', 'Cybersecurity Agent')}
              </div>
              <div className="flex min-w-0 items-center gap-1.5 overflow-hidden font-mono text-[10px] text-muted-foreground">
                <span className="shrink-0">{streamLabel(state.streamStatus, language)}</span>
                <span className="shrink-0">·</span>
                <span className="shrink-0 truncate">{operationLevelLabel(currentOperationLevel, language)}</span>
                {state.lastSeq > 0 && <span className="hidden shrink-0 sm:inline">seq={state.lastSeq}</span>}
                {state.session?.active_task?.status && (
                  <>
                    <span className="hidden shrink-0 sm:inline">·</span>
                    <span className="hidden shrink-0 sm:inline">{statusLabel(state.session.active_task.status, language)}</span>
                  </>
                )}
                {displayHint && (
                  <>
                    <span className="hidden shrink-0 md:inline">·</span>
                    <span
                      className={cn('hidden max-w-[13rem] truncate md:inline-block', displayHintClass(displayHint.tone))}
                      title={[displayHint.detail, displayHint.source].filter(Boolean).join(' · ') || undefined}
                    >
                      {displayHint.label}
                    </span>
                  </>
                )}
              </div>
            </div>
          </div>

          <div className="flex shrink-0 items-center gap-1.5">
            <Button type="button" variant="ghost" size="sm" onClick={() => { void refreshCurrentSession() }} disabled={!state.activeSessionId || state.loadingSession} className="h-11 w-11 rounded-xl px-0 text-muted-foreground sm:h-8 sm:w-auto sm:px-2.5">
              <span className="sr-only">{text(language, '刷新当前会话', 'Refresh current session')}</span>
              <RefreshCcw className={cn('h-3.5 w-3.5', state.loadingSession && 'animate-spin')} />
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={openTraceViewer} disabled={!state.session || traceLoading} className="h-11 w-11 rounded-xl px-0 text-muted-foreground sm:h-8 sm:w-auto sm:px-2.5">
              <span className="sr-only">{text(language, '打开 Trace', 'Open trace')}</span>
              {traceLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Code2 className="h-3.5 w-3.5" />}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={downloadTrace} disabled={!state.session} className="h-11 w-11 rounded-xl bg-transparent px-0 sm:h-8 sm:w-auto sm:px-2.5">
              <span className="sr-only">{text(language, '下载 Trace', 'Download trace')}</span>
              <Download className="h-3.5 w-3.5" />
            </Button>
          </div>
        </GlassToolbar>

        {state.error && (
          <GlassAlert tone="danger" className="flex shrink-0 items-center gap-2 rounded-none border-x-0 border-t-0 px-4 py-2 text-xs">
            <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
            <span className="min-w-0 truncate">{state.error}</span>
          </GlassAlert>
        )}

        {runSummary && (
          <GlassAlert tone="info" className="flex shrink-0 items-center gap-2 rounded-none border-x-0 border-t-0 px-4 py-2 text-xs">
            <ShieldCheck className="h-3.5 w-3.5 shrink-0 text-cyan-200" />
            <span className="min-w-0 truncate">{runSummaryLine(runSummary, language)}</span>
          </GlassAlert>
        )}

        <div className="agent-scroll-area min-h-0 flex-1 overflow-y-auto overscroll-contain p-3 sm:p-4">
          <TurnTimeline
            state={state}
            language={language}
            disabled={state.sending}
            onSelectTool={(toolCallId) => dispatch({ type: 'ui/selectTool', toolCallId })}
            onFillPrompt={fillInput}
            onExecutePrompt={(prompt) => { void sendMessage(prompt, undefined, { force: true, keepInput: true }) }}
            onCreateSession={() => { void createSession() }}
          />
          <div ref={chatEndRef} />
        </div>

        {authorizationGate && (
          <div className="shrink-0 px-2.5 pt-2">
            <AuthorizationGateBanner
              gate={authorizationGate}
              language={language}
              busy={authBusy}
              running={state.sending}
              onAuthorize={() => { void authorizeAndRetry(authorizationGate) }}
              onFill={() => fillInput(authorizationGate.prompt)}
              onDeny={() => { void denyAuthorizationGate(authorizationGate) }}
            />
          </div>
        )}

        <Composer
          value={input}
          onChange={setInput}
          onSubmit={() => { void sendMessage() }}
          onQuickPrompt={fillInput}
          onClear={() => setInput('')}
          onContinue={() => { void runControl('continue', suggestedNextPrompt || '') }}
          onPause={() => { void runControl('pause') }}
          onCancel={() => { void runControl('cancel') }}
          onNewTask={() => { void runControl('new-task', input) }}
          language={language}
          streamStatus={state.streamStatus}
          operationLevel={currentOperationLevel}
          suggestedPrompt={suggestedNextPrompt}
          sending={state.sending}
          disabled={state.loadingSession}
        />
      </GlassPanel>

      <GlassPanel as="aside" variant="panel" className="hidden min-h-0 flex-col overflow-hidden xl:flex">
        <GlassToolbar className="flex h-12 shrink-0 items-center justify-between border-b px-4">
          <div className="min-w-0">
            <div className="text-sm font-medium">Tool Inspector</div>
            <div className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
              {selectedTool?.id || text(language, '未选择工具调用', 'No tool selected')}
            </div>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => dispatch({ type: 'ui/selectTool', toolCallId: undefined })}
            className="h-8 rounded-xl px-2 text-muted-foreground"
            aria-label={text(language, '清除工具选择', 'Clear tool selection')}
          >
            ×
          </Button>
        </GlassToolbar>
        <div className="agent-scroll-area min-h-0 flex-1 overflow-y-auto p-3">
          <ToolInspector
            tool={selectedTool}
            language={language}
            onRefresh={() => { void refreshCurrentSession() }}
            onClear={() => dispatch({ type: 'ui/selectTool', toolCallId: undefined })}
          />
        </div>
      </GlassPanel>

      {selectedTool && state.selectedToolCallId && (
        <MobileToolInspectorSheet
          tool={selectedTool}
          language={language}
          onRefresh={() => { void refreshCurrentSession() }}
          onClose={() => dispatch({ type: 'ui/selectTool', toolCallId: undefined })}
        />
      )}

      {traceOpen && (
        <TraceViewerDrawer
          trace={traceData}
          loading={traceLoading}
          language={language}
          onClose={() => setTraceOpen(false)}
          onRefresh={() => { void loadTrace() }}
          onDownload={() => { void downloadTrace() }}
        />
      )}
    </div>
  )
}

function MobileToolInspectorSheet({
  tool,
  language,
  onRefresh,
  onClose,
}: {
  tool: NonNullable<ReturnType<typeof sessionToolCalls>>[number]
  language: Language
  onRefresh: () => void
  onClose: () => void
}) {
  const titleId = useId()
  const descriptionId = useId()
  const sheetRef = useDialogA11y<HTMLElement>({
    onClose,
    modal: false,
    initialFocusSelector: '[data-dialog-close]',
  })
  return (
    <GlassPanel
      ref={sheetRef}
      as="aside"
      variant="elevated"
      role="dialog"
      aria-modal="false"
      aria-labelledby={titleId}
      aria-describedby={descriptionId}
      tabIndex={-1}
      className="fixed inset-x-3 bottom-24 z-40 flex max-h-[68dvh] flex-col overflow-hidden shadow-2xl xl:hidden"
    >
      <GlassToolbar className="flex min-h-12 shrink-0 items-center justify-between gap-2 border-b px-3 py-2">
        <div className="min-w-0">
          <div id={titleId} className="text-sm font-medium">{text(language, '工具详情', 'Tool details')}</div>
          <div id={descriptionId} className="truncate font-mono text-[10px] text-muted-foreground">{tool.id}</div>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onClose}
          className="h-11 w-11 rounded-xl px-0 text-muted-foreground"
          aria-label={text(language, '关闭工具详情', 'Close tool details')}
          data-dialog-close
        >
          <X className="h-4 w-4" />
        </Button>
      </GlassToolbar>
      <div className="agent-scroll-area min-h-0 flex-1 overflow-y-auto p-3">
        <ToolInspector
          tool={tool}
          language={language}
          onRefresh={onRefresh}
          onClear={onClose}
        />
      </div>
    </GlassPanel>
  )
}

function MobileTaskSwitcher({
  sessions,
  activeSessionId,
  activeTitle,
  loading,
  open,
  language,
  onToggle,
  onCreate,
  onSelect,
  onRefresh,
}: {
  sessions: AgentSessionSummary[]
  activeSessionId: string
  activeTitle: string
  loading: boolean
  open: boolean
  language: Language
  onToggle: () => void
  onCreate: () => void
  onSelect: (sessionId: string) => void
  onRefresh: () => void
}) {
  const visibleSessions = sessions.slice(0, 12)
  const panelId = useId()
  return (
    <GlassPanel as="section" variant="panel" className="shrink-0 overflow-hidden xl:hidden">
      <GlassToolbar as="header" className={cn('flex min-h-14 items-center gap-2 px-2 py-2', open && 'border-b')}>
        <button
          type="button"
          onClick={onToggle}
          className="flex min-w-0 flex-1 items-center gap-2 rounded-2xl px-2 py-1.5 text-left hover:bg-white/[0.05] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-expanded={open}
          aria-controls={panelId}
          aria-label={open ? text(language, '收起任务列表', 'Collapse task list') : text(language, '展开任务列表', 'Expand task list')}
        >
          <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-primary/25 bg-primary/10">
            <MessageSquare className="h-4 w-4 text-primary" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-medium">{activeTitle}</span>
            <span className="mt-0.5 block truncate font-mono text-[10px] text-muted-foreground">
              {loading
                ? text(language, '同步任务列表中', 'Syncing task list')
                : text(language, `${sessions.length} 个会话`, `${sessions.length} sessions`)}
            </span>
          </span>
        </button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onRefresh}
          className="h-11 w-11 shrink-0 rounded-xl px-0 text-muted-foreground"
          aria-label={text(language, '刷新任务列表', 'Refresh task list')}
        >
          <RefreshCcw className={cn('h-4 w-4', loading && 'animate-spin')} />
        </Button>
        <Button
          type="button"
          size="sm"
          onClick={onCreate}
          disabled={loading}
          className="h-11 w-11 shrink-0 rounded-xl px-0"
          aria-label={text(language, '新建任务', 'New task')}
        >
          <Plus className="h-4 w-4" />
        </Button>
      </GlassToolbar>

      {open && (
        <div id={panelId} className="agent-scroll-area max-h-[42dvh] overflow-y-auto p-2">
          {visibleSessions.length ? (
            <div className="space-y-1.5">
              {visibleSessions.map((item) => {
                const active = item.session_id === activeSessionId
                return (
                  <button
                    key={item.session_id}
                    type="button"
                    onClick={() => onSelect(item.session_id)}
                    aria-current={active ? 'page' : undefined}
                    className={cn(
                      'flex w-full items-start gap-2 rounded-2xl border p-2 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
                      active
                        ? 'border-primary/35 bg-primary/10 text-foreground'
                        : 'border-white/5 bg-white/[0.025] text-muted-foreground hover:border-white/12 hover:bg-white/[0.05] hover:text-foreground',
                    )}
                  >
                    <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-primary/70" />
                    <span className="min-w-0 flex-1">
                      <span className="line-clamp-2 text-xs font-medium leading-5">{sessionTitle(item, language)}</span>
                      <span className="mt-1 flex flex-wrap items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                        <span>{statusLabel(item.status, language)}</span>
                        <span>·</span>
                        <span>{item.updated_at ? new Date(item.updated_at).toLocaleTimeString() : '—'}</span>
                      </span>
                    </span>
                  </button>
                )
              })}
              {sessions.length > visibleSessions.length && (
                <div className="px-2 py-1 text-center font-mono text-[10px] text-muted-foreground">
                  {text(language, `仅显示最近 ${visibleSessions.length} 个会话`, `Showing the latest ${visibleSessions.length} sessions`)}
                </div>
              )}
            </div>
          ) : (
            <div className="rounded-2xl border border-dashed border-white/10 p-3 text-xs leading-5 text-muted-foreground">
              {text(language, '暂无任务。点击加号创建第一个 Agent 会话。', 'No tasks yet. Tap plus to create the first Agent session.')}
            </div>
          )}
        </div>
      )}
    </GlassPanel>
  )
}

function streamLabel(status: CockpitStreamStatus, language: Language) {
  if (status === 'online') return text(language, '实时在线', 'Live')
  if (status === 'connecting') return text(language, '连接中', 'Connecting')
  if (status === 'error') return text(language, '流异常', 'Stream error')
  if (status === 'degraded') return text(language, '重连中', 'Reconnecting')
  return text(language, '离线', 'Offline')
}

function streamDotClass(status: CockpitStreamStatus) {
  if (status === 'online') return 'bg-emerald-300 shadow-[0_0_12px_rgba(110,231,183,0.75)]'
  if (status === 'connecting') return 'bg-primary shadow-[0_0_12px_rgba(96,165,250,0.75)]'
  if (status === 'degraded') return 'bg-amber-300 shadow-[0_0_12px_rgba(252,211,77,0.65)]'
  if (status === 'error') return 'bg-red-300 shadow-[0_0_12px_rgba(252,165,165,0.75)]'
  return 'bg-slate-500'
}

function displayHintClass(tone: 'muted' | 'info' | 'success' | 'warning' | 'danger') {
  if (tone === 'success') return 'text-emerald-200'
  if (tone === 'warning') return 'text-amber-200'
  if (tone === 'danger') return 'text-red-200'
  if (tone === 'info') return 'text-cyan-200'
  return 'text-muted-foreground'
}

function runSummaryLine(summary: NonNullable<ReturnType<typeof sessionRunSummary>>, language: Language) {
  if (summary.summary?.trim()) return summary.summary.trim()
  const target = summary.target || '—'
  const status = statusLabel(String(summary.status || 'running'), language)
  return text(
    language,
    `任务${status}：target=${target}，发现 ${summary.findings || 0}，证据 ${summary.artifacts || 0}，事件 ${summary.events || 0}。`,
    `Task ${status}: target=${target}, findings=${summary.findings || 0}, artifacts=${summary.artifacts || 0}, events=${summary.events || 0}.`,
  )
}

function agentOperationLevelAtLeast(current: unknown, required: AgentOperationLevel) {
  return operationLevelRank(normalizeAgentOperationLevel(current)) >= operationLevelRank(normalizeAgentOperationLevel(required))
}

function operationLevelRank(level: AgentOperationLevel) {
  const order: AgentOperationLevel[] = ['read_only', 'workspace', 'workspace_write', 'safe_scan', 'approval_ops', 'advanced', 'bypass']
  return order.indexOf(level)
}

function delay(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms))
}
