import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  ChevronDown,
  ClipboardList,
  Code2,
  Download,
  Loader2,
  MessageSquare,
  Pause,
  PanelLeftClose,
  PanelLeftOpen,
  Play,
  Plus,
  RefreshCcw,
  Send,
  ShieldCheck,
  Sparkles,
  Trash2,
  Wrench,
  XCircle,
  Zap,
} from 'lucide-react'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import type {
  AgentAuthorizationRequest,
  AgentMessage,
  AgentNextAction,
  AgentOperationLevel,
  AgentProviderConfig,
  AgentSession,
  AgentSessionRunSummary,
  AgentSessionSummary,
  AgentTodo,
  AgentToolCall,
  AgentTraceResponse,
} from '@/lib/api'
import { ApiError, normalizeAgentOperationLevel } from '@/lib/api'
import { consumeAgentDraft } from '@/lib/agent-draft'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

type StreamStatus = 'offline' | 'connecting' | 'online' | 'error'

interface DisplayContent {
  text: string
  command?: CommandSummary
  suggestions?: string[]
  recommendationOptions?: RecommendationOption[]
}

interface RecommendationOption {
  id: string
  title: string
  detail: string
  prompt: string
  command?: string
}

interface CommandSummary {
  command: string
  exitCode?: string
  workdir?: string
  duration?: string
  stdout?: string
  stderr?: string
}

interface AuthorizationGate {
  id: string
  action: string
  prompt?: string
  target?: string
  scope?: string
  policy?: string
  reason?: string
  arguments?: Record<string, unknown>
  requiredLevel: AgentOperationLevel
  source: 'pending' | 'legacy'
  messageIndex?: number
}

const QUICK_PROMPTS = {
  zh: [
    '检查 daemon 健康状态',
    '列出最近运行并总结状态',
    '读取当前任务上下文并给下一步',
    '列出可用插件和内置工具',
    '扫描 <目标> scope=<范围> profile=safe',
  ],
  en: [
    'Check daemon health',
    'List recent runs and summarize status',
    'Read current task context and suggest the next step',
    'List available plugins and built-in tools',
    'scan <target> scope=<scope> profile=safe',
  ],
} as const

const OPERATION_LEVELS: AgentOperationLevel[] = ['read_only', 'workspace', 'safe_scan', 'approval_ops', 'advanced', 'bypass']

export function AgentCockpitPage() {
  const { client, settings, updateSettings, language, t } = useConsole()
  const [sessions, setSessions] = useState<AgentSessionSummary[]>([])
  const [activeSessionId, setActiveSessionId] = useState<string>('')
  const [session, setSession] = useState<AgentSession | null>(null)
  const [loadingSessions, setLoadingSessions] = useState(false)
  const [loadingSession, setLoadingSession] = useState(false)
  const [sending, setSending] = useState(false)
  const [authBusy, setAuthBusy] = useState(false)
  const [input, setInput] = useState('')
  const [error, setError] = useState('')
  const [dismissedAuthorizationGates, setDismissedAuthorizationGates] = useState<string[]>([])
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const [quickOpen, setQuickOpen] = useState(false)
  const [taskOpen, setTaskOpen] = useState(false)
  const [permissionOpen, setPermissionOpen] = useState(false)
  const [streamStatus, setStreamStatus] = useState<StreamStatus>('offline')
  const [traceOpen, setTraceOpen] = useState(false)
  const [traceLoading, setTraceLoading] = useState(false)
  const [traceData, setTraceData] = useState<AgentTraceResponse | null>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const chatEndRef = useRef<HTMLDivElement>(null)
  const draftConsumedRef = useRef(false)

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
      token: profile.token,
      username: profile.username,
      password: profile.password,
      header_name: profile.headerName,
      header_value: profile.headerValue,
      cookie: profile.cookie,
      enabled: profile.enabled,
    })),
    default_auth_profile_id: settings.defaultApiAuthProfileId,
  }), [settings.agentProvider, settings.operationLevel, settings.apiAuthProfiles, settings.defaultApiAuthProfileId])

  const refreshSessions = useCallback(async () => {
    setLoadingSessions(true)
    setError('')
    try {
      const response = await client.listAgentSessions(40)
      const items = response.items || []
      setSessions(items)
      return items
    } catch (err) {
      setError(formatError(err, language))
      setSessions([])
      return []
    } finally {
      setLoadingSessions(false)
    }
  }, [client, language])

  const loadSession = useCallback(async (sessionId: string) => {
    if (!sessionId) return null
    setLoadingSession(true)
    setError('')
    try {
      const response = await client.getAgentSession(sessionId)
      setSession(response.session)
      setActiveSessionId(response.session.session_id)
      return response.session
    } catch (err) {
      setError(formatError(err, language))
      return null
    } finally {
      setLoadingSession(false)
    }
  }, [client, language])

  const createSession = useCallback(async () => {
    setError('')
    setLoadingSession(true)
    try {
      const response = await client.createAgentSession(settings.operator, language, settings.operationLevel)
      setSession(response.session)
      setActiveSessionId(response.session.session_id)
      await refreshSessions()
      return response.session
    } catch (err) {
      setError(formatError(err, language))
      return null
    } finally {
      setLoadingSession(false)
    }
  }, [client, language, refreshSessions, settings.operationLevel, settings.operator])

  useEffect(() => {
    let cancelled = false
    refreshSessions().then((items) => {
      if (cancelled || activeSessionId) return
      if (items[0]?.session_id) {
        void loadSession(items[0].session_id)
      }
    })
    return () => { cancelled = true }
  }, [activeSessionId, loadSession, refreshSessions])

  useEffect(() => {
    if (draftConsumedRef.current) return
    draftConsumedRef.current = true
    const draft = consumeAgentDraft()
    if (draft?.prompt) {
      setInput(draft.prompt)
      setQuickOpen(false)
    }
  }, [])

  useEffect(() => {
    if (!activeSessionId) return
    const id = window.setInterval(() => {
      void loadSession(activeSessionId)
      void refreshSessions()
    }, 5_000)
    return () => window.clearInterval(id)
  }, [activeSessionId, loadSession, refreshSessions])

  useEffect(() => {
    if (!activeSessionId) {
      setStreamStatus('offline')
      return
    }
    const controller = new AbortController()
    setStreamStatus('connecting')
    client.streamAgentSession(activeSessionId, {
      onOpen: () => setStreamStatus('online'),
      onSession: (nextSession) => setSession(nextSession),
      onError: () => setStreamStatus('error'),
    }, controller.signal).catch((err) => {
      if (!controller.signal.aborted) {
        setStreamStatus(formatError(err, language).includes('401') ? 'error' : 'offline')
      }
    })
    return () => controller.abort()
  }, [activeSessionId, client, language])

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [session?.messages?.length, sending])

  const currentOperationLevel = normalizeAgentOperationLevel(settings.operationLevel || session?.operation_level)
  const messages = session?.messages || []
  const activeTask = session?.active_task
  const todos = session?.todos || []
  const plannedTodos = useMemo(() => todos.filter((todo) => isPlannedTodoStatus(todo.status)), [todos])
  const nextAction = session?.next_action
  const suggestedNextPrompt = nextActionSuggestedPrompt(nextAction)
  const runSummary = session?.run_summary || sessions.find((item) => item.session_id === activeSessionId)?.run_summary
  const phases = useMemo(() => buildPhaseProgress(messages), [messages])
  const authorizationGate = useMemo(() => {
    const pendingGate = buildPendingAuthorizationGate(session?.pending_authorization, language)
    if (pendingGate) return pendingGate
    const gate = buildAuthorizationGate(messages, language)
    if (!gate) return null
    return dismissedAuthorizationGates.includes(authorizationGateKey(gate)) ? null : gate
  }, [dismissedAuthorizationGates, language, messages, session?.pending_authorization])

  const sendMessage = useCallback(async (
    override?: string,
    providerOverride?: Partial<AgentProviderConfig>,
    options?: { force?: boolean; keepInput?: boolean },
  ) => {
    const content = (override || input).trim()
    if (!content || (sending && !options?.force)) return
    if (!options?.keepInput) setInput('')
    setSending(true)
    setError('')
    try {
      const ensured = session || await createSession()
      if (!ensured) return
      setSession((current) => current ? {
        ...current,
        messages: [
          ...current.messages,
          {
            id: `optimistic_${Date.now()}`,
            role: 'user',
            content,
            timestamp: new Date().toISOString(),
          },
        ],
      } : current)
      await client.sendAgentMessage(ensured.session_id, content, settings.operator, language, {
        ...providerConfig,
        ...(providerOverride || {}),
      })
      await loadSession(ensured.session_id)
      await refreshSessions()
    } catch (err) {
      setError(formatError(err, language))
    } finally {
      setSending(false)
    }
  }, [client, createSession, input, language, loadSession, providerConfig, refreshSessions, sending, session, settings.operator])

  const runControl = useCallback(async (kind: 'continue' | 'pause' | 'stop' | 'cancel' | 'new-task', prompt = '') => {
    if (!session) return
    setSending(true)
    setError('')
    try {
      const request = {
        operator: settings.operator,
        language,
        operation_level: settings.operationLevel,
        prompt,
      }
      if (kind === 'continue') await client.continueAgentSession(session.session_id, request)
      if (kind === 'pause') await client.pauseAgentSession(session.session_id, request)
      if (kind === 'stop') await client.stopAgentSession(session.session_id, request)
      if (kind === 'cancel') await client.cancelAgentSession(session.session_id, request)
      if (kind === 'new-task') await client.newAgentTask(session.session_id, request)
      await loadSession(session.session_id)
      await refreshSessions()
    } catch (err) {
      setError(formatError(err, language))
    } finally {
      setSending(false)
    }
  }, [client, language, loadSession, refreshSessions, session, settings.operationLevel, settings.operator])

  const deleteSession = useCallback(async (sessionId: string) => {
    setError('')
    try {
      await client.deleteAgentSession(sessionId)
      const items = await refreshSessions()
      if (activeSessionId === sessionId) {
        setSession(null)
        setActiveSessionId('')
        if (items[0]?.session_id) await loadSession(items[0].session_id)
      }
    } catch (err) {
      setError(formatError(err, language))
    }
  }, [activeSessionId, client, language, loadSession, refreshSessions])

  const loadTrace = useCallback(async () => {
    if (!session) return
    setError('')
    setTraceLoading(true)
    try {
      const trace = await client.getAgentSessionTrace(session.session_id)
      setTraceData(trace)
      return trace
    } catch (err) {
      setError(formatError(err, language))
      return null
    } finally {
      setTraceLoading(false)
    }
  }, [client, language, session])

  const openTraceViewer = useCallback(async () => {
    setTraceOpen(true)
    await loadTrace()
  }, [loadTrace])

  const downloadTrace = useCallback(async () => {
    if (!session) return
    setError('')
    try {
      const trace = traceData?.session_id === session.session_id ? traceData : await client.getAgentSessionTrace(session.session_id)
      const blob = new Blob([JSON.stringify(trace, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `cyberagent-trace-${session.session_id}.json`
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      URL.revokeObjectURL(url)
    } catch (err) {
      setError(formatError(err, language))
    }
  }, [client, language, session, traceData])

  const fillInputAndFocus = (value?: string) => {
    if (!value) return
    setInput(value)
    window.requestAnimationFrame(() => inputRef.current?.focus())
  }

  const applySuggestedPrompt = (prompt?: string) => {
    if (!prompt) return
    fillInputAndFocus(prompt)
    setTaskOpen(false)
  }

  const authorizeAndRetry = useCallback(async (gate: AuthorizationGate) => {
    const requiredLevel = gate.requiredLevel
    const nextLevel = agentOperationLevelAtLeast(settings.operationLevel, requiredLevel) ? settings.operationLevel : requiredLevel
    const gateKey = authorizationGateKey(gate)
    setAuthBusy(true)
    setError('')
    try {
      if (nextLevel !== settings.operationLevel) {
        updateSettings({ operationLevel: nextLevel })
      }
      if (session && gate.source === 'pending') {
        await client.approveAgentAuthorization(session.session_id, {
          authorization_id: gate.id,
          operator: settings.operator,
          language,
          ...providerConfig,
          operation_level: nextLevel,
        })
        await loadSession(session.session_id)
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
  }, [client, language, loadSession, providerConfig, refreshSessions, sendMessage, session, settings.operationLevel, settings.operator, updateSettings])

  const denyAuthorizationGate = useCallback(async (gate: AuthorizationGate) => {
    setAuthBusy(true)
    setError('')
    try {
      if (session && gate.source === 'pending') {
        await client.denyAgentAuthorization(session.session_id, {
          authorization_id: gate.id,
          operator: settings.operator,
          language,
          operation_level: settings.operationLevel,
          reason: text(language, `拒绝执行 ${gate.action}`, `Deny ${gate.action}`),
        })
        await loadSession(session.session_id)
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
  }, [client, language, loadSession, refreshSessions, session, settings.operationLevel, settings.operator])

  return (
    <div className="grid h-[calc(100dvh-6.5rem)] min-h-0 gap-3 xl:grid-cols-[auto_minmax(0,1fr)_320px]">
      <aside
        className={cn(
          'glass-panel flex min-h-0 shrink-0 flex-col overflow-hidden transition-[width] duration-200',
          sidebarOpen ? 'w-[248px]' : 'w-12',
        )}
      >
        <div className="flex h-12 shrink-0 items-center justify-between border-b border-white/10 px-2">
          {sidebarOpen && (
            <div className="flex min-w-0 items-center gap-2 px-1">
              <MessageSquare className="h-4 w-4 text-primary" />
              <span className="truncate text-sm font-medium">{text(language, '会话', 'Sessions')}</span>
            </div>
          )}
          <button
            type="button"
            onClick={() => setSidebarOpen((value) => !value)}
            className="grid h-8 w-8 place-items-center rounded-xl text-muted-foreground hover:bg-white/[0.06] hover:text-foreground"
            aria-label={sidebarOpen ? 'Collapse sessions' : 'Expand sessions'}
          >
            {sidebarOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeftOpen className="h-4 w-4" />}
          </button>
        </div>

        {sidebarOpen ? (
          <>
            <div className="shrink-0 border-b border-white/10 p-2">
              <Button type="button" onClick={createSession} disabled={loadingSession} className="h-9 w-full rounded-xl">
                {loadingSession ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Plus className="mr-2 h-4 w-4" />}
                {text(language, '新会话', 'New session')}
              </Button>
            </div>
            <div className="agent-scroll-area min-h-0 flex-1 overflow-y-auto p-2">
              {loadingSessions && !sessions.length ? (
                <div className="flex items-center gap-2 p-2 text-xs text-muted-foreground">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  {t('loading')}
                </div>
              ) : sessions.length ? (
                <div className="space-y-1">
                  {sessions.map((item) => (
                    <div
                      key={item.session_id}
                      className={cn(
                        'group flex items-center gap-1 rounded-xl border px-2 py-2 transition',
                        activeSessionId === item.session_id
                          ? 'border-primary/35 bg-primary/10 text-foreground'
                          : 'border-transparent text-muted-foreground hover:bg-white/[0.05] hover:text-foreground',
                      )}
                    >
                      <button
                        type="button"
                        onClick={() => loadSession(item.session_id)}
                        className="min-w-0 flex-1 truncate text-left text-xs"
                        title={sessionTitle(item, language)}
                      >
                        {sessionTitle(item, language)}
                      </button>
                      <button
                        type="button"
                        onClick={(event) => {
                          event.stopPropagation()
                          void deleteSession(item.session_id)
                        }}
                        className="grid h-7 w-7 shrink-0 place-items-center rounded-lg opacity-70 hover:bg-red-500/15 hover:text-red-200 group-hover:opacity-100"
                        aria-label={text(language, '删除会话', 'Delete session')}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="rounded-2xl border border-dashed border-white/10 p-3 text-xs leading-5 text-muted-foreground">
                  {text(language, '暂无会话，点击新会话开始。', 'No sessions yet. Create one to start.')}
                </div>
              )}
            </div>
          </>
        ) : (
          <div className="flex flex-1 flex-col items-center gap-2 py-2">
            <button
              type="button"
              onClick={createSession}
              className="grid h-8 w-8 place-items-center rounded-xl text-muted-foreground hover:bg-white/[0.06] hover:text-foreground"
              aria-label={text(language, '新会话', 'New session')}
            >
              <Plus className="h-4 w-4" />
            </button>
            <button
              type="button"
              onClick={refreshSessions}
              className="grid h-8 w-8 place-items-center rounded-xl text-muted-foreground hover:bg-white/[0.06] hover:text-foreground"
              aria-label={t('refresh')}
            >
              <RefreshCcw className={cn('h-4 w-4', loadingSessions && 'animate-spin')} />
            </button>
          </div>
        )}
      </aside>

      <section className="agent-console flex min-h-0 min-w-0 flex-col overflow-hidden">
        <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b border-white/10 px-3 sm:px-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="relative grid h-9 w-9 shrink-0 place-items-center rounded-xl border border-primary/30 bg-primary/10">
              <Bot className="h-4 w-4 text-primary" />
              <span className={cn('absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full', streamDotClass(streamStatus))} />
            </span>
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold">{session ? sessionTitle(session, language) : text(language, 'Cybersecurity Agent', 'Cybersecurity Agent')}</div>
              <div className="flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
                <span>{streamLabel(streamStatus, language)}</span>
                <span>·</span>
                <span>{operationLevelLabel(currentOperationLevel, language)}</span>
                {activeTask?.status && (
                  <>
                    <span>·</span>
                    <span>{statusLabel(activeTask.status, language)}</span>
                  </>
                )}
              </div>
            </div>
          </div>

          <div className="flex shrink-0 items-center gap-1.5">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => runControl('continue', suggestedNextPrompt || '')}
              disabled={!session || sending}
              className="h-8 rounded-xl bg-transparent px-2.5"
              title={text(language, '继续', 'Continue')}
            >
              <Play className="h-3.5 w-3.5" />
              <span className="ml-1.5 hidden sm:inline">{text(language, '继续', 'Continue')}</span>
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => runControl('pause')}
              disabled={!session || sending}
              className="h-8 rounded-xl px-2.5 text-muted-foreground"
              title={text(language, '暂停', 'Pause')}
            >
              <Pause className="h-3.5 w-3.5" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => runControl('cancel')}
              disabled={!session || sending}
              className="h-8 rounded-xl px-2.5 text-muted-foreground hover:text-red-200"
              title={text(language, '取消任务', 'Cancel task')}
            >
              <XCircle className="h-3.5 w-3.5" />
            </Button>
          </div>
        </header>

        {error && (
          <div className="flex shrink-0 items-center gap-2 border-b border-red-400/20 bg-red-500/10 px-4 py-2 text-xs text-red-100">
            <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
            <span className="min-w-0 truncate">{error}</span>
          </div>
        )}

        {runSummary && (
          <ActiveRunStatusBanner summary={runSummary} language={language} />
        )}

        <div className="agent-scroll-area min-h-0 flex-1 overflow-y-auto overscroll-contain p-3 sm:p-4">
          {!session && !loadingSession ? (
            <EmptyAgentState language={language} onCreate={createSession} />
          ) : loadingSession && !session ? (
            <div className="grid h-full place-items-center text-sm text-muted-foreground">
              <span className="inline-flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />{t('loading')}</span>
            </div>
          ) : (
            <div className="mx-auto flex min-h-full w-full max-w-5xl flex-col justify-end">
              {messages.map((message) => (
                <MessageBubble
                  key={message.id}
                  message={message}
                  language={language}
                  disabled={sending}
                  onFillPrompt={(prompt) => fillInputAndFocus(prompt)}
                  onExecutePrompt={(prompt) => sendMessage(prompt, undefined, { force: true, keepInput: true })}
                />
              ))}
              {sending && (
                <div className="mb-3 flex justify-start">
                  <div className="inline-flex items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.05] px-3 py-2 text-xs text-muted-foreground">
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    {text(language, 'Agent 正在处理...', 'Agent is working...')}
                  </div>
                </div>
              )}
              <div ref={chatEndRef} />
            </div>
          )}
        </div>

        <footer className="shrink-0 border-t border-white/10 bg-[#050816]/70 p-2.5">
          {authorizationGate && (
            <AuthorizationGateBanner
              gate={authorizationGate}
              language={language}
              busy={authBusy}
              running={sending}
              onAuthorize={() => authorizeAndRetry(authorizationGate)}
              onFill={() => authorizationGate.prompt && fillInputAndFocus(authorizationGate.prompt)}
              onDeny={() => denyAuthorizationGate(authorizationGate)}
            />
          )}

          {quickOpen && (
            <div className="mb-2 flex flex-wrap gap-1.5 rounded-2xl border border-white/10 bg-white/[0.025] p-2">
              {QUICK_PROMPTS[language].map((prompt) => (
                <button
                  key={prompt}
                  type="button"
                  onClick={() => {
                    fillInputAndFocus(prompt)
                    setQuickOpen(false)
                  }}
                  className="rounded-xl border border-white/10 bg-white/[0.04] px-2.5 py-1.5 text-xs text-muted-foreground hover:border-primary/30 hover:text-foreground"
                >
                  {prompt}
                </button>
              ))}
            </div>
          )}

          {taskOpen && (
            <div className="mb-2 grid gap-2 rounded-2xl border border-white/10 bg-white/[0.025] p-2 text-xs sm:grid-cols-[minmax(0,1fr)_auto]">
              <div className="min-w-0">
                <div className="truncate font-medium">{activeTask?.goal || text(language, '暂无活动任务', 'No active task')}</div>
                <div className="mt-1 truncate text-muted-foreground">{activeTask?.summary || nextAction?.reason || text(language, '发送自然语言指令即可创建任务。', 'Send a natural-language instruction to create a task.')}</div>
              </div>
              <div className="flex items-center gap-1.5">
                {suggestedNextPrompt && (
                  <Button type="button" variant="outline" size="sm" onClick={() => applySuggestedPrompt(suggestedNextPrompt)} className="h-8 rounded-xl bg-transparent">
                    <Zap className="mr-1.5 h-3.5 w-3.5" />
                    {text(language, '填入下一步', 'Use next')}
                  </Button>
                )}
                <Button type="button" variant="ghost" size="sm" onClick={() => runControl('new-task', input)} disabled={!session} className="h-8 rounded-xl">
                  <Plus className="mr-1.5 h-3.5 w-3.5" />
                  {text(language, '新任务', 'New task')}
                </Button>
              </div>
            </div>
          )}

          <div className="mb-2 flex h-9 items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.03] px-2">
            <button
              type="button"
              onClick={() => {
                setQuickOpen((value) => !value)
                setTaskOpen(false)
                setPermissionOpen(false)
              }}
              className={cn(
                'inline-flex h-7 items-center gap-1.5 rounded-xl px-2 text-xs transition',
                quickOpen ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:bg-white/[0.06] hover:text-foreground',
              )}
            >
              <Sparkles className="h-3.5 w-3.5" />
              {text(language, '快捷指令', 'Quick')}
              <ChevronDown className={cn('h-3 w-3 transition', quickOpen && 'rotate-180')} />
            </button>
            <button
              type="button"
              onClick={() => {
                setTaskOpen((value) => !value)
                setQuickOpen(false)
                setPermissionOpen(false)
              }}
              className={cn(
                'inline-flex h-7 items-center gap-1.5 rounded-xl px-2 text-xs transition',
                taskOpen ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:bg-white/[0.06] hover:text-foreground',
              )}
            >
              <ClipboardList className="h-3.5 w-3.5" />
              Task
              <ChevronDown className={cn('h-3 w-3 transition', taskOpen && 'rotate-180')} />
            </button>
            <div className="relative shrink-0">
              <button
                type="button"
                onClick={() => {
                  setPermissionOpen((value) => !value)
                  setQuickOpen(false)
                  setTaskOpen(false)
                }}
                className={cn(
                  'inline-flex h-7 items-center gap-1.5 rounded-xl px-2 text-xs transition',
                  permissionOpen || currentOperationLevel === 'bypass'
                    ? currentOperationLevel === 'bypass'
                      ? 'bg-amber-300/15 text-amber-100'
                      : 'bg-primary/15 text-primary'
                    : 'text-muted-foreground hover:bg-white/[0.06] hover:text-foreground',
                )}
                title={text(language, '权限模式', 'Permission mode')}
              >
                <ShieldCheck className="h-3.5 w-3.5" />
                <span>{operationLevelLabel(currentOperationLevel, language)}</span>
                <ChevronDown className={cn('h-3 w-3 transition', permissionOpen && 'rotate-180')} />
              </button>
              {permissionOpen && (
                <div className="absolute bottom-full left-0 z-30 mb-2 w-60 overflow-hidden rounded-2xl border border-white/10 bg-[#050816] p-1.5 shadow-2xl shadow-black/40">
                  <div className="mb-1 px-2 py-1 text-[10px] font-medium text-muted-foreground">
                    {text(language, '权限模式', 'Permission mode')}
                  </div>
                  <div className="grid gap-1">
                    {OPERATION_LEVELS.map((level) => (
                      <button
                        key={level}
                        type="button"
                        title={permissionModeDetail(level, language)}
                        onClick={() => {
                          updateSettings({ operationLevel: level })
                          setSession((current) => current ? { ...current, operation_level: level } : current)
                          setPermissionOpen(false)
                        }}
                        className={cn(
                          'flex h-8 w-full items-center gap-2 rounded-xl border px-2 text-left text-xs transition',
                          currentOperationLevel === level
                            ? 'border-primary/35 bg-primary/10 text-foreground'
                            : 'border-transparent text-muted-foreground hover:bg-white/[0.055] hover:text-foreground',
                          level === 'bypass' && currentOperationLevel === level && 'border-amber-300/35 bg-amber-300/10',
                        )}
                      >
                        <span className={cn('grid h-5 w-5 shrink-0 place-items-center rounded-lg border', level === 'bypass' ? 'border-amber-300/25 bg-amber-300/10 text-amber-100' : 'border-white/10 bg-white/[0.04]')}>
                          <ShieldCheck className="h-3 w-3" />
                        </span>
                        <span className="min-w-0 flex-1 truncate font-medium">{operationLevelLabel(level, language)}</span>
                        {currentOperationLevel === level && <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-primary" />}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
            {suggestedNextPrompt && (
              <div className="hidden items-center gap-1.5 rounded-xl border border-primary/20 bg-primary/10 px-1.5 py-1 sm:flex">
                <button
                  type="button"
                  onClick={() => fillInputAndFocus(suggestedNextPrompt)}
                  disabled={sending}
                  className="rounded-lg px-1.5 py-0.5 text-[11px] text-primary hover:bg-primary/10 disabled:opacity-50"
                  title={suggestedNextPrompt}
                >
                  {text(language, '填入建议', 'Fill suggestion')}
                </button>
                <button
                  type="button"
                  onClick={() => sendMessage(suggestedNextPrompt, undefined, { force: true, keepInput: true })}
                  disabled={sending}
                  className="inline-flex items-center gap-1 rounded-lg bg-primary px-1.5 py-0.5 text-[11px] text-primary-foreground disabled:opacity-50"
                  title={suggestedNextPrompt}
                >
                  <Play className="h-3 w-3" />
                  {text(language, '执行', 'Run')}
                </button>
              </div>
            )}
            <div className="min-w-0 flex-1 truncate text-right font-mono text-[10px] text-muted-foreground">
              {nextAction?.type ? `${statusLabel(nextAction.type, language)} · ${nextAction.reason || ''}` : text(language, '等待指令', 'Waiting for instruction')}
            </div>
          </div>

          <div className="flex items-end gap-2">
            <textarea
              value={input}
              ref={inputRef}
              onChange={(event) => setInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault()
                  void sendMessage()
                }
              }}
              rows={1}
              className="command-input max-h-28 min-h-11 flex-1 resize-none rounded-2xl px-3 py-3 text-sm leading-5 text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
            />
            <Button type="button" onClick={() => sendMessage()} disabled={sending || !input.trim()} className="h-11 rounded-2xl px-4">
              {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
            </Button>
          </div>
        </footer>
      </section>

      <aside className="glass-panel hidden min-h-0 flex-col overflow-hidden xl:flex">
        <div className="flex h-12 shrink-0 items-center justify-between border-b border-white/10 px-4">
          <div className="flex items-center gap-2 text-sm font-medium">
            <ClipboardList className="h-4 w-4 text-primary" />
            Track List
          </div>
          <div className="hidden items-center gap-2">
            <button type="button" onClick={openTraceViewer} disabled={!session || traceLoading} className="text-muted-foreground hover:text-foreground disabled:opacity-40" title={text(language, '查看 trace', 'View trace')}>
              {traceLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Code2 className="h-4 w-4" />}
            </button>
            <button type="button" onClick={downloadTrace} disabled={!session} className="text-muted-foreground hover:text-foreground disabled:opacity-40" title={text(language, '下载 trace', 'Download trace')}>
              <Download className="h-4 w-4" />
            </button>
            <button type="button" onClick={refreshSessions} className="text-muted-foreground hover:text-foreground" title={t('refresh')}>
              <RefreshCcw className={cn('h-4 w-4', loadingSessions && 'animate-spin')} />
            </button>
          </div>
        </div>
        <div className="agent-scroll-area min-h-0 flex-1 space-y-3 overflow-y-auto p-3">
          <InspectorBlock icon={Sparkles} title={text(language, '执行阶段', 'Agent phases')}>
            <PhaseTimeline phases={phases} language={language} />
          </InspectorBlock>

          <InspectorBlock icon={ClipboardList} title={`TODO List · ${plannedTodos.length}`}>
            {plannedTodos.length ? (
              <div className="space-y-2">
                {plannedTodos.slice(0, 8).map((todo) => (
                  <TodoItem key={todo.id || todo.title} todo={todo} language={language} />
                ))}
              </div>
            ) : (
              <div className="text-xs text-muted-foreground">{text(language, '暂无待执行 TODO。', 'No planned TODOs.')}</div>
            )}
          </InspectorBlock>
        </div>
      </aside>

      {traceOpen && (
        <TraceViewerDrawer
          trace={traceData}
          loading={traceLoading}
          language={language}
          onClose={() => setTraceOpen(false)}
          onRefresh={loadTrace}
          onDownload={downloadTrace}
        />
      )}
    </div>
  )
}

function EmptyAgentState({ language, onCreate }: { language: 'zh' | 'en'; onCreate: () => void }) {
  return (
    <div className="grid h-full place-items-center">
      <div className="max-w-md text-center">
        <div className="mx-auto mb-5 grid h-16 w-16 place-items-center rounded-3xl border border-primary/25 bg-primary/10">
          <Bot className="h-8 w-8 text-primary" />
        </div>
        <h1 className="text-xl font-semibold">{text(language, '用对话驱动安全工作流', 'Drive security workflows with chat')}</h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {text(language, '像 Claude Code / Codex 一样，让 Agent 维护任务、调用内置工具、读取运行上下文并给出下一步。', 'Let the Agent maintain tasks, call built-in tools, inspect runtime context and suggest next steps like Claude Code / Codex.')}
        </p>
        <Button type="button" onClick={onCreate} className="mt-5 rounded-2xl">
          <Plus className="mr-2 h-4 w-4" />
          {text(language, '创建会话', 'Create session')}
        </Button>
      </div>
    </div>
  )
}

function AuthorizationGateBanner({
  gate,
  language,
  busy,
  running,
  onAuthorize,
  onFill,
  onDeny,
}: {
  gate: AuthorizationGate
  language: 'zh' | 'en'
  busy: boolean
  running: boolean
  onAuthorize: () => void
  onFill: () => void
  onDeny: () => void
}) {
  return (
    <div className="mb-2 rounded-2xl border border-amber-300/25 bg-amber-400/10 px-3 py-2 text-xs text-amber-50 shadow-[0_0_24px_rgba(251,191,36,0.08)]">
      <div className="flex flex-wrap items-center gap-2">
        <ShieldCheck className="h-4 w-4 text-amber-200" />
        <span className="font-medium">{text(language, '需要授权确认', 'Authorization required')}</span>
        <span className="rounded-full border border-amber-200/20 bg-black/15 px-2 py-0.5 font-mono text-[10px] text-amber-100/85">
          {gate.action}
        </span>
        {gate.target && (
          <span className="min-w-0 truncate font-mono text-[10px] text-amber-100/80">
            {gate.target}
          </span>
        )}
        {running && !busy && (
          <span className="rounded-full border border-amber-200/20 bg-black/15 px-2 py-0.5 text-[10px] text-amber-100/70">
            {text(language, '后台仍在收尾，可直接授权', 'Background request is finishing; you can authorize now')}
          </span>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-1.5">
          <Button type="button" variant="ghost" size="sm" onClick={onDeny} disabled={busy} className="h-7 rounded-xl px-2 text-amber-100/80 hover:bg-red-300/10 hover:text-red-100">
            {text(language, '拒绝', 'Deny')}
          </Button>
          {gate.source === 'legacy' && gate.prompt && (
            <Button type="button" variant="ghost" size="sm" onClick={onFill} disabled={busy} className="h-7 rounded-xl px-2 text-amber-100 hover:bg-amber-300/10">
              {text(language, '填入声明', 'Fill')}
            </Button>
          )}
          <Button type="button" size="sm" onClick={onAuthorize} disabled={busy} className="h-7 rounded-xl bg-amber-300 px-2 text-black hover:bg-amber-200">
            {busy ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <ShieldCheck className="mr-1.5 h-3.5 w-3.5" />}
            {text(language, '允许并继续', 'Allow & continue')}
          </Button>
        </div>
      </div>
      <div className="mt-1 line-clamp-3 text-[11px] leading-5 text-amber-100/75">
        {text(
          language,
          `${gate.reason || 'Agent 已暂停执行，等待操作者确认。'} 本次需要「${operationLevelLabel(gate.requiredLevel, language)}」权限。`,
          `${gate.reason || 'The Agent paused execution and is waiting for operator approval.'} Required: ${operationLevelLabel(gate.requiredLevel, language)}.`,
        )}
      </div>
      {gate.arguments && (
        <details className="mt-1.5 rounded-xl border border-amber-200/15 bg-black/10 px-2 py-1.5 text-[11px]">
          <summary className="cursor-pointer text-amber-100/75">{text(language, '动作参数', 'Action args')}</summary>
          <pre className="mt-1 max-h-24 overflow-auto whitespace-pre-wrap font-mono text-amber-50/80">{safeJson(gate.arguments)}</pre>
        </details>
      )}
    </div>
  )
}

function TraceViewerDrawer({
  trace,
  loading,
  language,
  onClose,
  onRefresh,
  onDownload,
}: {
  trace: AgentTraceResponse | null
  loading: boolean
  language: 'zh' | 'en'
  onClose: () => void
  onRefresh: () => void
  onDownload: () => void
}) {
  const items = (trace?.timeline || []).slice(0, 160)
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/55 backdrop-blur-sm">
      <aside className="flex h-full w-full max-w-2xl flex-col border-l border-white/10 bg-[#050816] shadow-2xl">
        <header className="flex h-14 shrink-0 items-center justify-between border-b border-white/10 px-4">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-sm font-semibold">
              <Code2 className="h-4 w-4 text-primary" />
              Agent Trace
            </div>
            <div className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
              {trace?.session_id || text(language, '加载中', 'Loading')}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button type="button" variant="ghost" size="sm" onClick={onRefresh} disabled={loading} className="h-8 rounded-xl px-2">
              <RefreshCcw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={onDownload} disabled={!trace} className="h-8 rounded-xl bg-transparent px-2">
              <Download className="h-3.5 w-3.5" />
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={onClose} className="h-8 rounded-xl px-2">
              ×
            </Button>
          </div>
        </header>

        <div className="agent-scroll-area min-h-0 flex-1 overflow-y-auto p-4">
          {loading && !trace ? (
            <div className="grid h-full place-items-center text-sm text-muted-foreground">
              <span className="inline-flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />{text(language, '正在加载 trace...', 'Loading trace...')}</span>
            </div>
          ) : trace ? (
            <div className="space-y-4">
              <div className="grid gap-2 sm:grid-cols-4">
                <TraceStat label={text(language, '消息', 'Messages')} value={traceSummaryValue(trace, 'message_count')} />
                <TraceStat label={text(language, '工具', 'Tools')} value={traceSummaryValue(trace, 'tool_call_count')} />
                <TraceStat label={text(language, '上下文', 'Context')} value={traceSummaryValue(trace, 'context_ref_count')} />
                <TraceStat label={text(language, '变更', 'Changes')} value={traceSummaryValue(trace, 'pending_change_count')} />
              </div>

              {trace.active_task && (
                <section className="rounded-2xl border border-white/10 bg-white/[0.035] p-3">
                  <div className="text-xs font-medium">{trace.active_task.goal || trace.active_task.summary}</div>
                  <div className="mt-1 font-mono text-[10px] text-muted-foreground">{trace.active_task.kind} · {trace.active_task.status}</div>
                </section>
              )}

              <section className="space-y-2">
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span>{text(language, '时间线', 'Timeline')}</span>
                  <span className="font-mono">{items.length}/{trace.timeline?.length || 0}</span>
                </div>
                <div className="relative space-y-2 pl-3 before:absolute before:left-[7px] before:top-2 before:h-[calc(100%-1rem)] before:w-px before:bg-white/10">
                  {items.map((item, index) => (
                    <TraceTimelineItem key={String(item.id || `${item.kind}_${index}`)} item={item} language={language} />
                  ))}
                </div>
              </section>
            </div>
          ) : (
            <div className="rounded-2xl border border-dashed border-white/10 p-4 text-sm text-muted-foreground">
              {text(language, '暂无 trace 数据。', 'No trace data.')}
            </div>
          )}
        </div>
      </aside>
    </div>
  )
}

function TraceStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.035] p-3">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="mt-1 font-mono text-lg text-foreground">{value}</div>
    </div>
  )
}

function TraceTimelineItem({ item, language }: { item: Record<string, unknown>; language: 'zh' | 'en' }) {
  const kind = String(item.kind || '')
  const phase = String(item.phase || '')
  const status = String(item.status || '')
  const title = traceItemTitle(item, language)
  const detail = traceItemDetail(item)
  return (
    <div className="relative rounded-2xl border border-white/10 bg-white/[0.035] p-3">
      <span className={cn('absolute -left-[9px] top-4 h-3.5 w-3.5 rounded-full border', status ? toolStatusClass(status) : phaseClass(phase))} />
      <div className="flex items-start gap-2">
        {kind === 'tool_call' ? <Wrench className="mt-0.5 h-3.5 w-3.5 shrink-0 text-primary" /> : <MessageSquare className="mt-0.5 h-3.5 w-3.5 shrink-0 text-primary" />}
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs font-medium">{title}</div>
          <div className="mt-1 flex flex-wrap gap-1.5 font-mono text-[10px] text-muted-foreground">
            <span>{formatTime(String(item.timestamp || ''))}</span>
            {phase && <span className={cn('rounded-full border px-1.5 py-0.5', phaseClass(phase))}>{toolPhaseLabel(phase, language)}</span>}
            {status && <span className={cn('rounded-full border px-1.5 py-0.5', toolStatusClass(status))}>{status}</span>}
          </div>
          {detail && <div className="mt-2 line-clamp-3 whitespace-pre-wrap text-xs leading-5 text-muted-foreground">{detail}</div>}
        </div>
      </div>
    </div>
  )
}

function traceSummaryValue(trace: AgentTraceResponse, key: string) {
  const value = trace.summary?.[key]
  if (typeof value === 'number' || typeof value === 'string' || typeof value === 'boolean') return String(value)
  return '0'
}

function traceItemTitle(item: Record<string, unknown>, language: 'zh' | 'en') {
  const kind = String(item.kind || '')
  if (kind === 'tool_call') {
    const tool = item.tool_call && typeof item.tool_call === 'object'
      ? item.tool_call as { name?: unknown; id?: unknown }
      : undefined
    return String(item.name || tool?.name || item.id || 'tool')
  }
  const role = String(item.role || 'message')
  const id = String(item.id || '')
  return id ? `${roleLabel(role, language)} · ${shortId(id)}` : roleLabel(role, language)
}

function traceItemDetail(item: Record<string, unknown>) {
  const kind = String(item.kind || '')
  if (kind === 'tool_call') {
    const tool = item.tool_call && typeof item.tool_call === 'object'
      ? item.tool_call as { detail?: unknown; result?: unknown; error?: { message?: unknown } }
      : undefined
    const error = tool?.error?.message
    if (typeof error === 'string' && error.trim()) return error
    if (typeof tool?.detail === 'string' && tool.detail.trim()) return tool.detail
    if (tool?.result) return safeJson(tool.result)
    return String(item.detail || '')
  }
  const content = item.content
  if (typeof content === 'string') {
    return displayTextFromContent(content)
  }
  return ''
}

function ActiveRunStatusBanner({ summary, language }: { summary: AgentSessionRunSummary; language: 'zh' | 'en' }) {
  const status = String(summary.status || 'running')
  const running = isRunActiveStatus(status)
  const terminal = isRunTerminalStatus(status)
  const reportReady = Boolean(summary.report_json)
  const summaryText = runSummaryLine(summary, language)
  return (
    <div className="shrink-0 border-b border-white/10 bg-[#050816]/75 px-3 py-2 sm:px-4">
      <div className="mx-auto flex max-w-5xl flex-wrap items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.035] px-3 py-2 text-xs">
        <span className={cn('relative grid h-7 w-7 shrink-0 place-items-center rounded-xl border', running ? 'border-cyan-300/30 bg-cyan-400/10' : terminal ? 'border-emerald-300/25 bg-emerald-400/10' : 'border-amber-300/25 bg-amber-400/10')}>
          {running ? <Loader2 className="h-3.5 w-3.5 animate-spin text-cyan-200" /> : terminal ? <CheckCircle2 className="h-3.5 w-3.5 text-emerald-200" /> : <AlertTriangle className="h-3.5 w-3.5 text-amber-200" />}
          {running && <span className="absolute -right-0.5 -top-0.5 h-2 w-2 animate-pulse rounded-full bg-cyan-300" />}
        </span>
        <div className="min-w-[180px] flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{text(language, '当前任务', 'Active run')}</span>
            <span className={cn('rounded-full border px-2 py-0.5 font-mono text-[10px]', runStatusPillClass(status))}>
              {statusLabel(status, language)}
            </span>
            <Link to={`/scan/${summary.run_id}`} className="font-mono text-[10px] text-primary hover:underline">
              {shortId(summary.run_id)}
            </Link>
          </div>
          <div className="mt-0.5 line-clamp-1 text-[11px] text-muted-foreground">{summaryText}</div>
        </div>
        <div className="flex flex-wrap items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
          <span className="rounded-full border border-white/10 bg-black/15 px-2 py-1">events {summary.events || 0}</span>
          <span className="rounded-full border border-white/10 bg-black/15 px-2 py-1">artifacts {summary.artifacts || 0}</span>
          <span className="rounded-full border border-white/10 bg-black/15 px-2 py-1">findings {summary.findings || 0}</span>
          <span className={cn('rounded-full border px-2 py-1', reportReady ? 'border-emerald-300/25 bg-emerald-400/10 text-emerald-100' : 'border-amber-300/20 bg-amber-400/10 text-amber-100')}>
            {reportReady ? text(language, '报告已生成', 'Report ready') : text(language, '报告生成中', 'Report pending')}
          </span>
        </div>
      </div>
    </div>
  )
}

function MessageBubble({
  message,
  language,
  disabled,
  onFillPrompt,
  onExecutePrompt,
}: {
  message: AgentMessage
  language: 'zh' | 'en'
  disabled?: boolean
  onFillPrompt?: (prompt: string) => void
  onExecutePrompt?: (prompt: string) => void
}) {
  const display = useMemo(() => displayContentForMessage(message), [message])
  const isUser = message.role === 'user'
  const isTool = message.role === 'tool'
  const isSystem = message.role === 'system'
  const isAgent = message.role === 'agent'
  const authorizationDeniedCall = message.tool_calls?.find(isExplicitOperatorIntentDenied)

  return (
    <div className={cn('mb-3 flex', isUser ? 'justify-end' : 'justify-start')}>
      <div
        className={cn(
          'max-w-[88%] rounded-2xl px-3.5 py-2.5 text-sm leading-6 shadow-sm',
          isUser && 'bg-primary/20 text-foreground',
          isAgent && 'border border-white/10 bg-white/[0.055] text-foreground/90',
          isTool && 'border border-amber-400/20 bg-amber-500/10 text-amber-100',
          isSystem && 'border border-white/10 bg-white/[0.04] text-muted-foreground',
        )}
      >
        <div className="mb-1 flex items-center gap-2 font-mono text-[10px] text-muted-foreground">
          {isUser ? <MessageSquare className="h-3 w-3" /> : isTool ? <Wrench className="h-3 w-3" /> : <Bot className="h-3 w-3" />}
          <span>{roleLabel(message.role, language)}</span>
          <span>·</span>
          <span>{formatTime(message.timestamp)}</span>
          {message.phase && (
            <span className={cn('ml-auto rounded-full border px-1.5 py-0.5', phaseClass(message.phase))}>
              {phaseLabel(message.phase, language)}
            </span>
          )}
        </div>

        {authorizationDeniedCall ? (
          <div className="rounded-xl border border-amber-300/20 bg-amber-300/10 px-3 py-2">
            <div className="flex items-center gap-2 font-medium text-amber-50">
              <ShieldCheck className="h-4 w-4 text-amber-200" />
              {text(language, '该动作需要明确授权确认', 'This action needs explicit authorization')}
            </div>
            <div className="mt-1 text-xs leading-5 text-amber-100/75">
              {text(
                language,
                '不是永久拒绝；请使用下方“授权并重试”或在输入框明确写出授权目标与 scope 后重新发送。',
                'This is not a permanent denial; use “Authorize & retry” below or explicitly include the authorized target and scope in the input.',
              )}
            </div>
          </div>
        ) : display.command ? (
          <CommandResultView command={display.command} language={language} />
        ) : (
          <div className="whitespace-pre-wrap break-words">{display.text || '—'}</div>
        )}

        {isAgent && display.suggestions?.length ? (
          <SuggestedPromptActions
            prompts={display.suggestions}
            language={language}
            disabled={Boolean(disabled)}
            onFill={onFillPrompt}
            onExecute={onExecutePrompt}
          />
        ) : null}

        {isAgent && display.recommendationOptions?.length ? (
          <RecommendationOptionsCard
            options={display.recommendationOptions}
            language={language}
            disabled={Boolean(disabled)}
            onFill={onFillPrompt}
          />
        ) : null}

        {message.reasoning && (
          <details className="mt-2 rounded-xl border border-white/10 bg-black/15 px-3 py-2 text-xs">
            <summary className="cursor-pointer text-muted-foreground">{text(language, '推理摘要', 'Reasoning')}</summary>
            <div className="mt-2 whitespace-pre-wrap text-muted-foreground">{message.reasoning}</div>
          </details>
        )}

        {message.tool_calls?.length ? (
          <div className="mt-2 space-y-1.5">
            {message.tool_calls.map((tool) => (
              <ToolCallDetails key={tool.id} call={tool} language={language} />
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}

function SuggestedPromptActions({
  prompts,
  language,
  disabled,
  onFill,
  onExecute,
}: {
  prompts: string[]
  language: 'zh' | 'en'
  disabled: boolean
  onFill?: (prompt: string) => void
  onExecute?: (prompt: string) => void
}) {
  const items = prompts.map((prompt) => prompt.trim()).filter(Boolean).slice(0, 4)
  if (!items.length) return null
  return (
    <div className="mt-2 rounded-2xl border border-primary/20 bg-primary/10 p-2 text-xs shadow-[0_0_24px_rgba(34,211,238,0.05)]">
      <div className="mb-1.5 flex items-center gap-2 text-primary">
        <Sparkles className="h-3.5 w-3.5" />
        <span className="font-medium">{text(language, 'Agent 建议执行', 'Agent suggested prompts')}</span>
      </div>
      <div className="space-y-1.5">
        {items.map((prompt, index) => (
          <div key={`${prompt}_${index}`} className="group rounded-xl border border-white/10 bg-black/15 p-2">
            <button
              type="button"
              onClick={() => onFill?.(prompt)}
              disabled={disabled || !onFill}
              className="w-full cursor-pointer text-left text-[11px] leading-5 text-foreground/90 hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
              title={prompt}
            >
              {prompt}
            </button>
            <div className="mt-1.5 flex items-center justify-end gap-1.5">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={disabled || !onFill}
                onClick={() => onFill?.(prompt)}
                className="h-7 rounded-lg px-2 text-[11px] text-muted-foreground hover:text-foreground"
              >
                {text(language, '填入', 'Fill')}
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={disabled || !onExecute}
                onClick={() => onExecute?.(prompt)}
                className="h-7 rounded-lg px-2 text-[11px]"
              >
                <Play className="mr-1 h-3 w-3" />
                {text(language, '执行', 'Run')}
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function RecommendationOptionsCard({
  options,
  language,
  disabled,
  onFill,
}: {
  options: RecommendationOption[]
  language: 'zh' | 'en'
  disabled: boolean
  onFill?: (prompt: string) => void
}) {
  const [selectedId, setSelectedId] = useState('')
  const items = options.slice(0, 6)
  if (!items.length) return null
  return (
    <div className="mt-2 rounded-2xl border border-cyan-300/20 bg-cyan-400/10 p-2 text-xs shadow-[0_0_24px_rgba(34,211,238,0.06)]">
      <div className="mb-1.5 flex items-center gap-2 text-cyan-50">
        <ClipboardList className="h-3.5 w-3.5 text-cyan-200" />
        <span className="font-medium">{text(language, '可选建议', 'Recommendation options')}</span>
        <span className="ml-auto text-[10px] text-cyan-100/60">
          {text(language, '点击选择，回车发送编号', 'Click to select, Enter sends number')}
        </span>
      </div>
      <div className="space-y-1.5">
        {items.map((item, index) => {
          const selected = selectedId === item.id
          return (
            <button
              key={item.id}
              type="button"
              disabled={disabled || !onFill}
              onClick={() => {
                setSelectedId(item.id)
                onFill?.(String(index + 1))
              }}
              title={`${index + 1}. ${item.detail}`}
              className={cn(
                'group w-full rounded-xl border px-2.5 py-2 text-left transition disabled:cursor-not-allowed disabled:opacity-60',
                selected
                  ? 'border-primary/45 bg-primary/15 text-foreground shadow-[0_0_18px_rgba(34,211,238,0.08)]'
                  : 'border-white/10 bg-black/15 text-muted-foreground hover:border-primary/25 hover:bg-white/[0.055] hover:text-foreground',
              )}
            >
              <div className="flex items-start gap-2">
                <span className={cn(
                  'mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded-full border font-mono text-[10px]',
                  selected ? 'border-primary/45 bg-primary/20 text-primary' : 'border-white/10 text-cyan-100/70',
                )}>
                  {index + 1}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block text-[11px] font-medium leading-5">{item.title}</span>
                  <span className="mt-0.5 line-clamp-2 block text-[10px] leading-4 opacity-75">{item.detail}</span>
                  {item.command && (
                    <code className="mt-1.5 block truncate rounded-lg bg-black/25 px-2 py-1 font-mono text-[10px] text-cyan-100">
                      {item.command}
                    </code>
                  )}
                </span>
                {selected && (
                  <span className="shrink-0 rounded-full border border-primary/30 bg-primary/15 px-1.5 py-0.5 text-[10px] text-primary">
                    {text(language, '已选择', 'Selected')}
                  </span>
                )}
              </div>
            </button>
          )
        })}
      </div>
      {selectedId && (
        <div className="mt-1.5 rounded-xl border border-primary/15 bg-primary/10 px-2 py-1.5 text-[10px] text-primary">
          {text(language, '已填入选择编号，按 Enter 发送给 Agent。', 'Selection number filled. Press Enter to send it to the Agent.')}
        </div>
      )}
    </div>
  )
}

function CommandResultView({ command, language }: { command: CommandSummary; language: 'zh' | 'en' }) {
  const ok = command.exitCode === undefined || command.exitCode === '0'
  return (
    <div>
      <div className="flex flex-wrap items-center gap-2">
        {ok ? <CheckCircle2 className="h-4 w-4 text-emerald-300" /> : <AlertTriangle className="h-4 w-4 text-amber-300" />}
        <span className="font-medium">{text(language, '命令已执行', 'Command executed')}</span>
        <code className="rounded-lg bg-black/25 px-2 py-0.5 text-xs">{command.command}</code>
        {command.exitCode !== undefined && <span className="font-mono text-xs text-muted-foreground">exit={command.exitCode}</span>}
      </div>
      <details className="mt-2 rounded-xl border border-white/10 bg-black/15 px-3 py-2 text-xs">
        <summary className="cursor-pointer text-muted-foreground">{text(language, '执行详情', 'Execution details')}</summary>
        <div className="mt-2 grid gap-2">
          {command.workdir && <KeyValue label="workdir" value={command.workdir} />}
          {command.duration && <KeyValue label="duration" value={command.duration} />}
          <OutputBlock label="stdout" value={command.stdout || text(language, '（无标准输出）', '(no stdout)')} />
          <OutputBlock label="stderr" value={command.stderr || text(language, '（无标准错误）', '(no stderr)')} />
        </div>
      </details>
    </div>
  )
}
function ToolCallDetails({ call, language }: { call: AgentToolCall; language: 'zh' | 'en' }) {
  const command = commandFromToolCall(call)
  const explicitIntentDenied = isExplicitOperatorIntentDenied(call)
  return (
    <details className="rounded-xl border border-white/10 bg-black/15 px-3 py-2 text-xs">
      <summary className="flex cursor-pointer list-none items-center gap-2 text-muted-foreground">
        <Wrench className="h-3.5 w-3.5" />
        <span className="font-mono">{call.name}</span>
        <span className={cn('ml-auto rounded-full border px-1.5 py-0.5 text-[10px]', toolStatusClass(String(call.status)))}>
          {call.status}
        </span>
      </summary>
      <div className="mt-2 space-y-2">
        {explicitIntentDenied ? (
          <div className="rounded-lg border border-amber-300/20 bg-amber-300/10 px-2.5 py-2 text-amber-100">
            {text(language, '等待操作者确认授权后重试。', 'Waiting for operator authorization before retrying.')}
          </div>
        ) : call.detail && <KeyValue label={text(language, '详情', 'Detail')} value={call.detail} />}
        {!explicitIntentDenied && call.error?.message && <KeyValue label="error" value={call.error.message} />}
        {command ? (
          <CommandResultView command={command} language={language} />
        ) : !explicitIntentDenied && call.result ? (
          <OutputBlock label="result" value={safeJson(call.result)} />
        ) : null}
      </div>
    </details>
  )
}

function isPlannedTodoStatus(status: string) {
  const normalized = String(status || '').trim().toLowerCase()
  return ['pending', 'todo', 'planned', 'running', 'in_progress', 'blocked', 'waiting', 'approval_required'].includes(normalized)
}

function TodoItem({ todo, language }: { todo: AgentTodo; language: 'zh' | 'en' }) {
  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.035] p-2">
      <div className="flex items-start gap-2">
        <span className={cn('mt-1.5 h-2 w-2 shrink-0 rounded-full', todoDotClass(todo.status))} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-xs text-foreground">{todo.title}</div>
          {todo.detail && <div className="mt-1 line-clamp-2 text-[11px] text-muted-foreground">{todo.detail}</div>}
          {todo.blocked_reason && <div className="mt-1 text-[11px] text-amber-200">{todo.blocked_reason}</div>}
        </div>
        <span className="shrink-0 rounded-full border border-white/10 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
          {statusLabel(todo.status, language)}
        </span>
      </div>
    </div>
  )
}

interface PhaseProgress {
  key: string
  label: string
  count: number
  active: boolean
}

function PhaseTimeline({ phases, language }: { phases: PhaseProgress[]; language: 'zh' | 'en' }) {
  return (
    <div className="space-y-2">
      {phases.map((phase, index) => (
        <div key={phase.key} className="flex items-center gap-2">
          <span className={cn(
            'grid h-6 w-6 shrink-0 place-items-center rounded-full border text-[10px]',
            phase.active ? phaseClass(phase.key) : 'border-white/10 bg-white/[0.03] text-muted-foreground',
          )}>
            {phase.count > 0 ? <CheckCircle2 className="h-3.5 w-3.5" /> : index + 1}
          </span>
          <div className="min-w-0 flex-1">
            <div className={cn('text-xs', phase.active ? 'text-foreground' : 'text-muted-foreground')}>
              {phaseLabel(phase.key, language)}
            </div>
            <div className="font-mono text-[10px] text-muted-foreground">
              {phase.count ? text(language, `${phase.count} 条记录`, `${phase.count} event(s)`) : text(language, '等待', 'Waiting')}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function InspectorBlock({ icon: Icon, title, children }: { icon: React.ElementType; title: string; children: React.ReactNode }) {
  return (
    <section className="rounded-2xl border border-white/10 bg-white/[0.025] p-3">
      <div className="mb-3 flex items-center gap-2 text-sm font-medium">
        <Icon className="h-4 w-4 text-primary" />
        <span>{title}</span>
      </div>
      {children}
    </section>
  )
}

function KeyValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="mb-1.5 grid gap-1 rounded-xl bg-white/[0.03] px-2.5 py-2 text-xs">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</span>
      <span className="break-words font-mono text-foreground/90">{value}</span>
    </div>
  )
}

function OutputBlock({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="mb-1 font-mono text-[10px] text-muted-foreground">{label}</div>
      <pre className="max-h-56 overflow-auto rounded-xl bg-black/25 p-2 font-mono text-[11px] leading-5 text-foreground/85">{value}</pre>
    </div>
  )
}

function displayContentForMessage(message: AgentMessage): DisplayContent {
  const text = displayTextFromContent(message.content || '')
  const command = parseCommandSummary(message.content || '')
  return {
    text: command ? `${command.command} executed` : text,
    command: command || undefined,
    suggestions: (message.suggested_prompts || []).map((value) => String(value || '').trim()).filter(Boolean),
    recommendationOptions: (message.recommendation_options || [])
      .filter((opt) => opt && (opt.title || opt.detail || opt.prompt))
      .map((opt) => ({
        id: opt.id || `recommendation_${hashString(opt.title || opt.detail || opt.prompt || '')}`,
        title: String(opt.title || opt.detail || '').trim(),
        detail: String(opt.detail || '').trim(),
        prompt: String(opt.prompt || opt.detail || opt.title || '').trim(),
        command: opt.command ? String(opt.command).trim() : undefined,
      })),
  }
}

function displayTextFromContent(content: string): string {
  return String(content || '').trim()
}

function hashString(value: string): string {
  let hash = 0
  for (let i = 0; i < value.length; i += 1) {
    hash = ((hash << 5) - hash + value.charCodeAt(i)) | 0
  }
  return Math.abs(hash).toString(36)
}

function parseCommandSummary(content: string): CommandSummary | null {
  if (!/^(命令已执行|Command executed)\b/.test(content.trim())) return null
  const command = matchLine(content, /-\s*command:\s*`([^`]+)`/i)
  if (!command) return null
  return {
    command,
    exitCode: matchLine(content, /-\s*exit_code:\s*([^\n]+)/i),
    workdir: matchLine(content, /-\s*workdir:\s*`([^`]+)`/i),
    duration: matchLine(content, /-\s*duration:\s*([^\n]+)/i),
    stdout: matchFence(content, /stdout:\s*```(?:text)?\s*([\s\S]*?)```/i),
    stderr: matchFence(content, /stderr:\s*```(?:text)?\s*([\s\S]*?)```/i),
  }
}

function commandFromToolCall(call: AgentToolCall): CommandSummary | null {
  if (!call.result) return null
  const command = String(call.result.command || '').trim()
  if (!command) return null
  return {
    command,
    exitCode: call.result.exit_code === undefined ? undefined : String(call.result.exit_code),
    workdir: String(call.result.workdir || ''),
    duration: call.result.duration_ms === undefined ? undefined : `${call.result.duration_ms}ms`,
    stdout: String(call.result.stdout || ''),
    stderr: String(call.result.stderr || ''),
  }
}

function matchLine(value: string, pattern: RegExp) {
  const match = value.match(pattern)
  return match?.[1]?.trim()
}

function matchFence(value: string, pattern: RegExp) {
  const match = value.match(pattern)
  return match?.[1]?.replace(/\n$/, '').trim()
}

function sessionTitle(item: AgentSessionSummary | AgentSession, language: 'zh' | 'en') {
  const activeTask = 'active_task' in item ? item.active_task : undefined
  const lastMessage = 'last_message' in item ? item.last_message : ''
  const goal = activeTask?.goal || activeTask?.summary || lastMessage
  if (goal?.trim()) return truncate(goal.trim(), 34)
  const created = Date.parse(item.created_at)
  const label = Number.isFinite(created) ? new Date(created).toLocaleString() : item.session_id
  return text(language, `会话 ${label}`, `Session ${label}`)
}

function buildPendingAuthorizationGate(auth: AgentAuthorizationRequest | undefined, language: 'zh' | 'en'): AuthorizationGate | null {
  if (!auth || String(auth.status || '').toLowerCase() !== 'pending') return null
  const args = recordFromUnknown(auth.arguments)
  const action = String(auth.action_name || 'agent.action')
  const target = agentAuthorizationTargetFromArgs(args)
  const scope = String(args.scope || args.scope_url || '').trim()
  return {
    id: auth.id,
    action,
    target,
    scope,
    policy: auth.policy,
    reason: auth.reason,
    arguments: args,
    requiredLevel: normalizeAgentOperationLevel(auth.required_level || requiredLevelForDeniedAction(action)),
    prompt: auth.original_prompt || buildAuthorizationRetryPrompt('', action, target, scope, language),
    source: 'pending',
  }
}

function agentAuthorizationTargetFromArgs(args: Record<string, unknown>) {
  const values = [
    args.target,
    args.url,
    args.input,
    args.path,
    args.file,
    args.command,
    args.run_id,
  ]
  for (const value of values) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function nextActionSuggestedPrompt(nextAction: AgentNextAction | undefined) {
  if (!nextAction) return ''
  const record = nextAction as AgentNextAction & { suggest_prompt?: unknown; prompt?: unknown }
  for (const value of [record.suggested_prompt, record.suggest_prompt, record.prompt]) {
    if (typeof value === 'string' && value.trim()) return value.trim()
  }
  return ''
}

function buildAuthorizationGate(messages: AgentMessage[], language: 'zh' | 'en'): AuthorizationGate | null {
  let latestUserIndex = -1
  for (let i = messages.length - 1; i >= 0; i -= 1) {
    if (messages[i].role === 'user') {
      latestUserIndex = i
      break
    }
  }

  for (let i = messages.length - 1; i >= 0; i -= 1) {
    const calls = messages[i].tool_calls || []
    for (let j = calls.length - 1; j >= 0; j -= 1) {
      const call = calls[j]
      if (!isExplicitOperatorIntentDenied(call)) continue
      if (latestUserIndex > i) return null

      const originalPrompt = lastUserPromptBefore(messages, i)
      const action = toolCallAction(call)
      const target = inferNetworkTargetForPrompt(originalPrompt)
      const scope = inferScopeForPrompt(originalPrompt, target)
      const legacyGate: AuthorizationGate = {
        id: ['legacy', i, action, target || '', scope || ''].join('|'),
        action,
        target,
        scope,
        requiredLevel: requiredLevelForDeniedAction(action),
        prompt: buildAuthorizationRetryPrompt(originalPrompt, action, target, scope, language),
        source: 'legacy',
        messageIndex: i,
      }
      return legacyGate
    }
  }
  return null
}

function authorizationGateKey(gate: AuthorizationGate) {
  return gate.id || [gate.source, gate.messageIndex ?? '', gate.action, gate.target || '', gate.scope || ''].join('|')
}

function isExplicitOperatorIntentDenied(call: AgentToolCall) {
  const result = recordFromUnknown(call.result)
  const details = recordFromUnknown(result.details)
  const policy = String(details.policy || result.policy || '').trim()
  return String(call.status || '').toLowerCase() === 'failed' && policy === 'explicit_operator_intent_required'
}

function toolCallAction(call: AgentToolCall) {
  const result = recordFromUnknown(call.result)
  const details = recordFromUnknown(result.details)
  const metadata = recordFromUnknown(call.metadata)
  return String(result.action || details.action || metadata.action_name || call.name || 'agent.action')
}

function lastUserPromptBefore(messages: AgentMessage[], index: number) {
  for (let i = Math.min(index, messages.length - 1); i >= 0; i -= 1) {
    if (messages[i].role === 'user' && messages[i].content?.trim()) return messages[i].content.trim()
  }
  return ''
}

function buildAuthorizationRetryPrompt(originalPrompt: string, action: string, target: string, scope: string, language: 'zh' | 'en') {
  const targetText = target || text(language, '当前目标', 'the current target')
  const scopeText = scope || target
  const actionText = action.includes('http.request')
    ? text(language, '低风险 HTTP 请求', 'low-risk HTTP request')
    : action.includes('scan')
      ? text(language, 'safe profile Web/API 扫描', 'safe-profile Web/API scan')
      : action

  if (language === 'en') {
    return [
      `I confirm that I am authorized to operate on ${targetText}${scopeText ? ` within scope=${scopeText}` : ''}.`,
      `Please execute ${actionText} now and continue the original task: ${originalPrompt || action}.`,
    ].join(' ')
  }
  return [
    `我确认已获得授权，可以在${scopeText ? ` scope=${scopeText} 内` : '当前授权范围内'}对授权目标 ${targetText} 执行${actionText}。`,
    `请立即继续执行原任务：${originalPrompt || action}。`,
  ].join('')
}

const FRONTEND_HTTP_URL_PATTERN = /https?:\/\/[^\s"'<>，。)]+/i
const FRONTEND_BARE_NETWORK_TARGET_PATTERN = /(?:^|[\s（(])((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}(?::\d{1,5})?(?:\/[^\s"'<>，。)]*)?)/ig

function inferNetworkTargetForPrompt(prompt: string) {
  const explicit = prompt.match(FRONTEND_HTTP_URL_PATTERN)?.[0]
  if (explicit) return cleanPromptToken(explicit)
  FRONTEND_BARE_NETWORK_TARGET_PATTERN.lastIndex = 0
  for (const match of prompt.matchAll(FRONTEND_BARE_NETWORK_TARGET_PATTERN)) {
    const candidate = cleanPromptToken(match[1] || '')
    if (looksLikeBareNetworkTarget(candidate)) return `https://${candidate}`
  }
  return ''
}

function inferScopeForPrompt(prompt: string, target: string) {
  const scopeMatch = prompt.match(/(?:scope|范围)\s*[:=：]\s*([^\s,，;；]+)/i)
  if (scopeMatch?.[1]) return cleanPromptToken(scopeMatch[1])
  try {
    return target ? new URL(target).host : ''
  } catch {
    return target.replace(/^https?:\/\//i, '').split('/')[0] || ''
  }
}

function cleanPromptToken(value: string) {
  return String(value || '').trim().replace(/^[`"'“”]+|[`"'“”，。,)\]]+$/g, '')
}

function looksLikeBareNetworkTarget(value: string) {
  const clean = cleanPromptToken(value)
  if (!clean || clean.includes('://') || /\s/.test(clean)) return false
  const host = clean.split('/')[0].replace(/:\d{1,5}$/, '')
  if (!host.includes('.') || host.startsWith('.') || host.endsWith('.')) return false
  const tld = host.slice(host.lastIndexOf('.') + 1).toLowerCase()
  if (tld.length < 2 || looksLikeWorkspaceExtension(tld)) return false
  return /^[a-z0-9.-]+$/i.test(host)
}

function looksLikeWorkspaceExtension(ext: string) {
  return new Set([
    'md', 'txt', 'log', 'json', 'yaml', 'yml', 'toml', 'xml', 'csv',
    'go', 'py', 'js', 'jsx', 'ts', 'tsx', 'css', 'scss', 'html', 'htm',
    'sh', 'bash', 'zsh', 'sql', 'rs', 'java', 'c', 'cc', 'cpp', 'h',
    'png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'pdf', 'zip', 'gz', 'tar',
    'list', 'context', 'draft', 'safe', 'command', 'files', 'approvals', 'plugins', 'runs', 'web', 'request', 'health',
  ]).has(ext.toLowerCase())
}

function requiredLevelForDeniedAction(action: string): AgentOperationLevel {
  const normalized = action.toLowerCase()
  if (normalized.includes('approval') || normalized.includes('resume_approved')) return 'approval_ops'
  if (normalized.includes('scan.web') || normalized.includes('http.request')) return 'safe_scan'
  if (normalized.includes('workspace.files.write') || normalized.includes('workspace.files.patch') ||
    normalized.includes('workspace.files.diff') || normalized.includes('workspace.files.apply') ||
    normalized.includes('workspace.files.delete') || normalized.includes('workspace.files.move') ||
    normalized.includes('workspace.files.copy') || normalized.includes('workspace.files.mkdir') ||
    normalized.includes('reports.draft')) return 'workspace_write'
  return 'read_only'
}

function agentOperationLevelAtLeast(level: AgentOperationLevel, required: AgentOperationLevel) {
  return agentOperationLevelRank(level) >= agentOperationLevelRank(required)
}

function agentOperationLevelRank(level: AgentOperationLevel) {
  const normalized = normalizeAgentOperationLevel(level)
  const ranks: Record<AgentOperationLevel, number> = {
    read_only: 0,
    workspace: 1,
    workspace_write: 1,
    safe_scan: 2,
    approval_ops: 3,
    advanced: 4,
    bypass: 5,
  }
  return ranks[normalized] ?? 0
}

function recordFromUnknown(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function statusLabel(status: string, language: 'zh' | 'en') {
  const normalized = status.toLowerCase()
  const zh: Record<string, string> = {
    idle: '待命',
    running: '运行中',
    pending: '待处理',
    done: '完成',
    completed: '完成',
    succeeded: '成功',
    partially_succeeded: '部分成功',
    finished: '完成',
    blocked: '阻塞',
    skipped: '跳过',
    wait_user: '等输入',
    continue: '可继续',
    approval_required: '待审批',
    authorization_required: '待授权',
    waiting_authorization: '待授权',
    paused: '已暂停',
    cancelled: '已取消',
    stopped: '已停止',
    failed: '失败',
  }
  const en: Record<string, string> = {
    idle: 'Idle',
    running: 'Running',
    pending: 'Pending',
    done: 'Done',
    completed: 'Completed',
    succeeded: 'Succeeded',
    partially_succeeded: 'Partial',
    finished: 'Finished',
    blocked: 'Blocked',
    skipped: 'Skipped',
    wait_user: 'Waiting',
    continue: 'Ready',
    approval_required: 'Approval',
    authorization_required: 'Authorization',
    waiting_authorization: 'Waiting auth',
    paused: 'Paused',
    cancelled: 'Cancelled',
    stopped: 'Stopped',
    failed: 'Failed',
  }
  return language === 'zh' ? zh[normalized] || status : en[normalized] || status
}

function isRunActiveStatus(status: string) {
  const normalized = String(status || '').toLowerCase()
  return ['running', 'pending', 'queued', 'starting'].includes(normalized)
}

function isRunTerminalStatus(status: string) {
  const normalized = String(status || '').toLowerCase()
  return ['succeeded', 'partially_succeeded', 'completed', 'done', 'finished'].includes(normalized)
}

function runStatusPillClass(status: string) {
  const normalized = String(status || '').toLowerCase()
  if (isRunActiveStatus(normalized)) return 'border-cyan-300/25 bg-cyan-400/10 text-cyan-100'
  if (isRunTerminalStatus(normalized)) return 'border-emerald-300/25 bg-emerald-400/10 text-emerald-100'
  if (['failed', 'error', 'cancelled', 'stopped'].includes(normalized)) return 'border-red-300/25 bg-red-400/10 text-red-100'
  return 'border-amber-300/25 bg-amber-400/10 text-amber-100'
}

function runSummaryLine(summary: AgentSessionRunSummary, language: 'zh' | 'en') {
  if (summary.summary?.trim()) return summary.summary.trim()
  const target = summary.target || '—'
  const status = statusLabel(String(summary.status || 'running'), language)
  if (isRunTerminalStatus(String(summary.status || ''))) {
    return text(
      language,
      `任务已完成：target=${target}，发现 ${summary.findings || 0}，证据 ${summary.artifacts || 0}，事件 ${summary.events || 0}。`,
      `Task completed: target=${target}, findings=${summary.findings || 0}, artifacts=${summary.artifacts || 0}, events=${summary.events || 0}.`,
    )
  }
  return text(
    language,
    `任务${status}：target=${target}，正在收集事件、证据与报告。`,
    `Task ${status}: target=${target}, collecting events, evidence, and report.`,
  )
}

function roleLabel(role: string, language: 'zh' | 'en') {
  if (role === 'user') return text(language, '你', 'You')
  if (role === 'tool') return text(language, '工具', 'Tool')
  if (role === 'system') return text(language, '系统', 'System')
  return 'Agent'
}

function phaseLabel(phase: string, language: 'zh' | 'en') {
  const normalized = phase.toLowerCase()
  const zh: Record<string, string> = {
    plan: 'Plan',
    step: 'Step',
    step_replan: 'StepReplan',
    final_answer: 'FinalAnswer',
  }
  const en = zh
  return language === 'zh' ? zh[normalized] || phase : en[normalized] || phase
}

function buildPhaseProgress(messages: AgentMessage[]): PhaseProgress[] {
  const order = ['plan', 'step', 'step_replan', 'final_answer']
  const counts: Record<string, number> = { plan: 0, step: 0, step_replan: 0, final_answer: 0 }
  for (const message of messages) {
    const phase = String(message.phase || '').toLowerCase()
    if (phase && counts[phase] !== undefined) counts[phase] += 1
    for (const call of message.tool_calls || []) {
      const toolPhase = String(call.metadata?.agent_phase || '').toLowerCase()
      if (toolPhase && counts[toolPhase] !== undefined) counts[toolPhase] += 1
    }
  }
  return order.map((key) => ({
    key,
    label: key,
    count: counts[key],
    active: counts[key] > 0,
  }))
}

function phaseClass(phase: string) {
  const normalized = phase.toLowerCase()
  if (normalized === 'plan') return 'border-blue-400/25 bg-blue-400/10 text-blue-200'
  if (normalized === 'step') return 'border-amber-400/25 bg-amber-400/10 text-amber-200'
  if (normalized === 'step_replan') return 'border-purple-400/25 bg-purple-400/10 text-purple-200'
  if (normalized === 'final_answer') return 'border-emerald-400/25 bg-emerald-400/10 text-emerald-200'
  return 'border-white/10 bg-white/[0.04] text-muted-foreground'
}

function toolPhaseLabel(phase: string, language: 'zh' | 'en') {
  const normalized = phase.toLowerCase()
  if (['plan', 'step', 'step_replan', 'final_answer'].includes(normalized)) return phaseLabel(normalized, language)
  const zh: Record<string, string> = {
    planner: '规划',
    workspace: '工作区',
    tool_synthesis: '总结',
    http_probe: 'HTTP',
    daemon_job: '任务',
    approval_check: '审批',
    approval_decision: '审批',
    approval_resume: '恢复',
    inventory: '盘点',
    history: '历史',
    context: '上下文',
  }
  const en: Record<string, string> = {
    planner: 'Planner',
    workspace: 'Workspace',
    tool_synthesis: 'Synthesis',
    http_probe: 'HTTP',
    daemon_job: 'Job',
    approval_check: 'Approval',
    approval_decision: 'Approval',
    approval_resume: 'Resume',
    inventory: 'Inventory',
    history: 'History',
    context: 'Context',
  }
  return language === 'zh' ? zh[normalized] || phase : en[normalized] || phase
}

function operationLevelLabel(level: string, language: 'zh' | 'en') {
  const normalized = normalizeAgentOperationLevel(level)
  const labels: Record<AgentOperationLevel, { zh: string; en: string }> = {
    read_only: { zh: '只读', en: 'Read only' },
    workspace: { zh: '工作区', en: 'Workspace' },
    workspace_write: { zh: '工作区', en: 'Workspace' },
    safe_scan: { zh: '安全扫描', en: 'Safe scan' },
    approval_ops: { zh: '审批操作', en: 'Approval ops' },
    advanced: { zh: '高级', en: 'Advanced' },
    bypass: { zh: 'Bypass 权限', en: 'Bypass' },
  }
  return labels[normalized][language]
}

function permissionModeDetail(level: string, language: 'zh' | 'en') {
  const normalized = normalizeAgentOperationLevel(level)
  const details: Record<AgentOperationLevel, { zh: string; en: string }> = {
    read_only: { zh: '只读分析，不执行网络、扫描、写入或审批动作。', en: 'Read-only analysis; no network, scan, write, or approval actions.' },
    workspace: { zh: '允许工作区读写、diff、apply 等本地能力。', en: 'Allow workspace read/write, diff, apply, and local workspace actions.' },
    workspace_write: { zh: '允许工作区读写、diff、apply 等本地能力。', en: 'Allow workspace read/write, diff, apply, and local workspace actions.' },
    safe_scan: { zh: '允许授权目标内的低风险 HTTP 请求与 safe scan。', en: 'Allow low-risk HTTP requests and safe scans for authorized targets.' },
    approval_ops: { zh: '允许处理审批、恢复已批准运行。', en: 'Allow approval handling and resuming approved runs.' },
    advanced: { zh: '高级能力，仍会触发必要授权确认。', en: 'Advanced capabilities; still prompts for required authorization.' },
    bypass: { zh: '本地沙箱模式，跳过权限确认弹窗直接执行内置工具。', en: 'Local sandbox mode; skip permission prompts for built-in tools.' },
  }
  return details[normalized][language]
}

function streamLabel(status: StreamStatus, language: 'zh' | 'en') {
  if (status === 'online') return text(language, '实时在线', 'Live')
  if (status === 'connecting') return text(language, '连接中', 'Connecting')
  if (status === 'error') return text(language, '流异常', 'Stream error')
  return text(language, '轮询同步', 'Polling')
}

function streamDotClass(status: StreamStatus) {
  if (status === 'online') return 'bg-emerald-300 shadow-[0_0_12px_rgba(110,231,183,0.75)]'
  if (status === 'connecting') return 'bg-primary shadow-[0_0_12px_rgba(96,165,250,0.75)]'
  if (status === 'error') return 'bg-red-300 shadow-[0_0_12px_rgba(252,165,165,0.75)]'
  return 'bg-slate-500'
}

function toolStatusClass(status: string) {
  const normalized = status.toLowerCase()
  if (['succeeded', 'completed', 'done', 'ok'].includes(normalized)) return 'border-emerald-400/25 bg-emerald-400/10 text-emerald-200'
  if (['failed', 'error'].includes(normalized)) return 'border-red-400/25 bg-red-400/10 text-red-200'
  if (['running', 'queued'].includes(normalized)) return 'border-primary/25 bg-primary/10 text-primary'
  return 'border-white/10 bg-white/[0.04] text-muted-foreground'
}

function todoDotClass(status: string) {
  const normalized = status.toLowerCase()
  if (['done', 'finished', 'completed'].includes(normalized)) return 'bg-emerald-300'
  if (['blocked', 'failed'].includes(normalized)) return 'bg-red-300'
  if (['running'].includes(normalized)) return 'bg-primary'
  return 'bg-slate-500'
}

function formatTime(value: string) {
  const parsed = Date.parse(value)
  if (!Number.isFinite(parsed)) return value
  return new Date(parsed).toLocaleTimeString()
}

function truncate(value: string, max: number) {
  return value.length > max ? `${value.slice(0, max)}…` : value
}

function shortId(value: string) {
  return value.length > 12 ? `${value.slice(0, 10)}…` : value
}

function safeJson(value: unknown) {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function formatError(err: unknown, language: 'zh' | 'en') {
  if (err instanceof ApiError) {
    return err.status === 401
      ? text(language, 'Daemon token 无效或缺失，请在设置页配置。', 'Daemon token is invalid or missing. Configure it in Settings.')
      : `${err.message}${err.code ? ` (${err.code})` : ''}`
  }
  return err instanceof Error ? err.message : text(language, '请求失败', 'Request failed')
}
