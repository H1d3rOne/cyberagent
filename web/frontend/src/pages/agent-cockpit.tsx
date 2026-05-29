import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  ChevronRight,
  Crosshair,
  Loader2,
  MessageSquare,
  PlayCircle,
  RefreshCcw,
  Send,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Square,
  Terminal,
  Wrench,
  Zap,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { ScanDetail, ScanListItem, ToolDef, UIEvent } from '@/lib/api'
import { ApiError } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

type AgentPhase = 'planning' | 'executing' | 'replanning' | 'final_answer'
type ChatRole = 'user' | 'agent' | 'system' | 'tool'

interface ChatMessage {
  id: string
  role: ChatRole
  content: string
  timestamp: number
  phase?: AgentPhase
  toolName?: string
}

const phaseConfig: Record<string, { label: string; labelZh: string; icon: typeof Bot; color: string }> = {
  planning: { label: 'Planning', labelZh: '规划', icon: Crosshair, color: 'text-blue-400' },
  executing: { label: 'Executing', labelZh: '执行', icon: Zap, color: 'text-amber-400' },
  replanning: { label: 'Re-planning', labelZh: '重规划', icon: RefreshCcw, color: 'text-purple-400' },
  final_answer: { label: 'Final Answer', labelZh: '总结', icon: ShieldCheck, color: 'text-emerald-400' },
}

const phaseOrder = ['planning', 'executing', 'replanning', 'final_answer'] as const

export function AgentCockpitPage() {
  const { client, settings, language, t } = useConsole()
  const [scanId, setScanId] = useState<string | null>(null)
  const [scan, setScan] = useState<ScanDetail | null>(null)
  const [recentScans, setRecentScans] = useState<ScanListItem[]>([])
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [targetInput, setTargetInput] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [tools, setTools] = useState<ToolDef[]>([])
  const [currentPhase, setCurrentPhase] = useState<AgentPhase | null>(null)
  const chatEndRef = useRef<HTMLDivElement>(null)
  const eventSourceRef = useRef<EventSource | null>(null)

  // Load tools list
  useEffect(() => {
    client.getTools().then(setTools).catch(() => {})
  }, [client])

  // Load recent scans
  const refreshScans = useCallback(async () => {
    try {
      const scans = await client.listScans()
      setRecentScans(scans || [])
    } catch {}
  }, [client])

  useEffect(() => {
    refreshScans()
    const id = window.setInterval(refreshScans, 10_000)
    return () => window.clearInterval(id)
  }, [refreshScans])

  // Subscribe to SSE events when scanId is set
  useEffect(() => {
    if (!scanId) return

    const url = client.eventsUrl(scanId)
    const es = new EventSource(url)
    eventSourceRef.current = es

    es.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as UIEvent
        handleEvent(data)
      } catch {}
    }

    es.onerror = () => {
      es.close()
    }

    return () => {
      es.close()
      eventSourceRef.current = null
    }
  }, [scanId, client])

  const handleEvent = useCallback((event: UIEvent) => {
    const phase = event.phase as AgentPhase | undefined
    if (phase && phase in phaseConfig) setCurrentPhase(phase)

    switch (event.type) {
      case 'text_delta':
      case 'thinking':
        setMessages((prev) => {
          const last = prev[prev.length - 1]
          if (last && last.role === 'agent' && last.phase === phase) {
            return [...prev.slice(0, -1), { ...last, content: last.content + event.message }]
          }
          return [...prev, {
            id: `msg_${event.timestamp}`,
            role: 'agent',
            content: event.message,
            timestamp: event.timestamp,
            phase,
          }]
        })
        break
      case 'tool_call':
        setMessages((prev) => [...prev, {
          id: `tool_${event.timestamp}`,
          role: 'tool',
          content: text(language, `调用工具: ${event.tool}`, `Calling tool: ${event.tool}`),
          timestamp: event.timestamp,
          toolName: event.tool,
          phase: 'executing',
        }])
        break
      case 'tool_result':
        setMessages((prev) => [...prev, {
          id: `result_${event.timestamp}`,
          role: 'tool',
          content: event.message || text(language, `${event.tool} 执行完成`, `${event.tool} completed`),
          timestamp: event.timestamp,
          toolName: event.tool,
          phase: 'executing',
        }])
        break
      case 'finding':
        setMessages((prev) => [...prev, {
          id: `finding_${event.timestamp}`,
          role: 'system',
          content: `[${event.severity?.toUpperCase()}] ${event.message}`,
          timestamp: event.timestamp,
          phase: 'executing',
        }])
        break
      case 'user_message':
        setMessages((prev) => [...prev, {
          id: `user_${event.timestamp}`,
          role: 'user',
          content: event.message,
          timestamp: event.timestamp,
        }])
        break
      case 'done':
        setMessages((prev) => [...prev, {
          id: `done_${event.timestamp}`,
          role: 'agent',
          content: event.message || text(language, '任务完成', 'Task complete'),
          timestamp: event.timestamp,
          phase: 'final_answer',
        }])
        setCurrentPhase('final_answer')
        break
      case 'error':
        setMessages((prev) => [...prev, {
          id: `error_${event.timestamp}`,
          role: 'system',
          content: `Error: ${event.message}`,
          timestamp: event.timestamp,
        }])
        break
    }
  }, [language])

  // Poll scan detail and sync events to messages
  const lastEventCountRef = useRef(0)
  useEffect(() => {
    if (!scanId) return
    lastEventCountRef.current = 0
    const poll = async () => {
      try {
        const detail = await client.getScan(scanId)
        setScan(detail)

        // Sync events from detail into messages (catches events SSE missed)
        const events = detail.events || []
        if (events.length > lastEventCountRef.current) {
          const newEvents = events.slice(lastEventCountRef.current)
          lastEventCountRef.current = events.length
          for (const evt of newEvents) {
            handleEvent(evt)
          }
        }
      } catch {}
    }
    poll()
    const id = window.setInterval(poll, 3_000)
    return () => { window.clearInterval(id) }
  }, [scanId, client, handleEvent])

  // Auto-scroll chat
  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const startScan = useCallback(async () => {
    const target = targetInput.trim()
    if (!target) {
      setError(text(language, '请输入目标 URL', 'Please enter a target URL'))
      return
    }

    if (!settings.agentProvider.apiKey.trim() && settings.agentProvider.provider !== 'ollama') {
      setError(text(language,
        '请先在设置页面配置 API Key',
        'Please configure your API Key in Settings first'))
      return
    }

    setLoading(true)
    setError('')
    setMessages([{
      id: 'init',
      role: 'system',
      content: text(language, `启动安全评估: ${target}`, `Starting security assessment: ${target}`),
      timestamp: Date.now(),
    }])

    try {
      const result = await client.startScan({
        target,
        scope: target,
        provider: settings.agentProvider.provider,
        model: settings.agentProvider.model,
        api_key: settings.agentProvider.apiKey,
        base_url: settings.agentProvider.providerBaseUrl,
        instructions: settings.agentProvider.instructions,
        max_turns: settings.agentProvider.maxTurns,
        interactive: true,
        no_confirm: true,
      })
      setScanId(result.id)
      setCurrentPhase('planning')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to start scan')
    } finally {
      setLoading(false)
    }
  }, [targetInput, client, settings, language])

  const sendMessage = useCallback(async () => {
    if (!scanId || !input.trim()) return
    const msg = input.trim()
    setInput('')

    setMessages((prev) => [...prev, {
      id: `user_${Date.now()}`,
      role: 'user',
      content: msg,
      timestamp: Date.now(),
    }])

    try {
      await client.sendMessage(scanId, msg)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to send message')
    }
  }, [scanId, input, client])

  const stopScan = useCallback(async () => {
    if (!scanId) return
    try {
      await client.stopScan(scanId)
      setMessages((prev) => [...prev, {
        id: `stop_${Date.now()}`,
        role: 'system',
        content: text(language, '扫描已停止', 'Scan stopped'),
        timestamp: Date.now(),
      }])
    } catch {}
  }, [scanId, client, language])

  const resumeScan = useCallback((id: string) => {
    setScanId(id)
    setMessages([{
      id: 'resume',
      role: 'system',
      content: text(language, `恢复扫描: ${id}`, `Resuming scan: ${id}`),
      timestamp: Date.now(),
    }])
  }, [language])

  const isRunning = scan?.status === 'running'
  const isWaiting = scan?.waiting

  const findings = useMemo(() => scan?.findings_detail || [], [scan])

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="mission-panel rounded-3xl p-5 sm:p-6">
        <div className="flex items-center gap-3">
          <span className="grid h-11 w-11 place-items-center rounded-2xl border border-primary/30 bg-primary/10">
            <Bot className="h-5 w-5 text-primary" />
          </span>
          <div>
            <h1 className="text-lg font-semibold">
              {text(language, '安全 Agent 控制台', 'Security Agent Console')}
            </h1>
            <p className="font-mono text-xs text-muted-foreground">
              {text(language, '4 阶段执行: 规划 → 执行 → 重规划 → 总结', '4-Phase: Plan → Step → Replan → Final')}
            </p>
          </div>
        </div>
      </div>

      {/* Phase Indicator */}
      {scanId && isRunning && (
        <div className="flex items-center gap-1 rounded-2xl border border-white/10 bg-white/[0.03] p-3">
          {phaseOrder.map((phase, i) => {
            const cfg = phaseConfig[phase]
            const active = currentPhase === phase
            const done = currentPhase ? phaseOrder.indexOf(currentPhase) > i : false
            return (
              <div key={phase} className="flex items-center gap-1">
                <span
                  className={cn(
                    'flex items-center gap-1.5 rounded-xl px-3 py-1.5 font-mono text-xs transition-all',
                    active && 'bg-white/10 text-foreground shadow-lg',
                    done && 'text-emerald-400',
                    !active && !done && 'text-muted-foreground',
                  )}
                >
                  <cfg.icon className={cn('h-3.5 w-3.5', active && cfg.color)} />
                  {text(language, cfg.labelZh, cfg.label)}
                </span>
                {i < phaseOrder.length - 1 && (
                  <ChevronRight className="h-3 w-3 text-muted-foreground" />
                )}
              </div>
            )
          })}
        </div>
      )}

      <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
        {/* Chat Panel */}
        <div className="flex flex-col rounded-3xl border border-white/10 bg-white/[0.02]">
          {/* Chat Messages */}
          <div className="flex-1 overflow-y-auto p-4" style={{ maxHeight: '60vh', minHeight: '400px' }}>
            {messages.length === 0 && !scanId && (
              <div className="flex h-full flex-col items-center justify-center gap-4 text-center">
                <div className="grid h-16 w-16 place-items-center rounded-3xl border border-primary/20 bg-primary/5">
                  <Shield className="h-8 w-8 text-primary/50" />
                </div>
                <div>
                  <p className="text-sm font-medium text-foreground/80">
                    {text(language, '输入目标 URL 开始安全评估', 'Enter a target URL to start assessment')}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {text(language, 'Agent 将自动规划、执行、评估并给出报告', 'Agent will plan, execute, evaluate and report automatically')}
                  </p>
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                  {tools.slice(0, 6).map((tool) => (
                    <span key={tool.name} className="flex items-center gap-1 rounded-lg bg-white/[0.04] px-2 py-1">
                      <Wrench className="h-3 w-3" />
                      {tool.name}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {messages.map((msg) => (
              <div key={msg.id} className={cn('mb-3 flex', msg.role === 'user' ? 'justify-end' : 'justify-start')}>
                <div
                  className={cn(
                    'max-w-[85%] rounded-2xl px-4 py-2.5 text-sm',
                    msg.role === 'user' && 'bg-primary/20 text-foreground',
                    msg.role === 'agent' && 'bg-white/[0.06] text-foreground/90',
                    msg.role === 'tool' && 'bg-amber-500/10 font-mono text-xs text-amber-200',
                    msg.role === 'system' && msg.content.startsWith('[') && 'bg-red-500/10 text-red-200',
                    msg.role === 'system' && !msg.content.startsWith('[') && 'bg-white/[0.04] text-xs text-muted-foreground',
                  )}
                >
                  {msg.role === 'tool' && msg.toolName && (
                    <div className="mb-1 flex items-center gap-1 text-[10px] text-amber-400/70">
                      <Wrench className="h-3 w-3" />
                      {msg.toolName}
                    </div>
                  )}
                  {msg.role === 'agent' && msg.phase && phaseConfig[msg.phase] && (
                    <div className={cn('mb-1 flex items-center gap-1 text-[10px]', phaseConfig[msg.phase].color)}>
                      {(() => { const Icon = phaseConfig[msg.phase].icon; return <Icon className="h-3 w-3" />; })()}
                      {text(language, phaseConfig[msg.phase].labelZh, phaseConfig[msg.phase].label)}
                    </div>
                  )}
                  <div className="whitespace-pre-wrap">{msg.content}</div>
                </div>
              </div>
            ))}

            {isRunning && (
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <Loader2 className="h-3 w-3 animate-spin" />
                {isWaiting
                  ? text(language, 'Agent 等待输入...', 'Agent waiting for input...')
                  : text(language, 'Agent 执行中...', 'Agent running...')}
              </div>
            )}
            <div ref={chatEndRef} />
          </div>

          {/* Error Bar */}
          {error && (
            <div className="flex items-center gap-2 border-t border-red-400/20 bg-red-500/10 px-4 py-2 text-xs text-red-200">
              <AlertTriangle className="h-3 w-3" />
              {error}
            </div>
          )}

          {/* Input Area */}
          <div className="border-t border-white/10 p-3">
            {!scanId ? (
              <div className="flex gap-2">
                <Input
                  value={targetInput}
                  onChange={(e) => setTargetInput(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && startScan()}
                  placeholder={text(language, '输入目标 URL (如 https://example.com)', 'Enter target URL (e.g. https://example.com)')}
                  className="rounded-xl bg-white/[0.05] border-white/10"
                />
                <Button onClick={startScan} disabled={loading} className="rounded-xl px-4">
                  {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <PlayCircle className="h-4 w-4" />}
                  {t('start')}
                </Button>
              </div>
            ) : (
              <div className="flex gap-2">
                <Input
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && sendMessage()}
                  placeholder={
                    isWaiting
                      ? text(language, 'Agent 等待你的输入...', 'Agent is waiting for your input...')
                      : text(language, '发送消息给 Agent...', 'Send a message to Agent...')
                  }
                  disabled={!isRunning}
                  className="rounded-xl bg-white/[0.05] border-white/10"
                />
                <Button onClick={sendMessage} disabled={!isRunning || !input.trim()} variant="outline" className="rounded-xl px-3">
                  <Send className="h-4 w-4" />
                </Button>
                {isRunning && (
                  <Button onClick={stopScan} variant="destructive" className="rounded-xl px-3">
                    <Square className="h-4 w-4" />
                  </Button>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Right Panel */}
        <div className="space-y-4">
          {/* Scan Status */}
          {scan && (
            <div className="rounded-3xl border border-white/10 bg-white/[0.02] p-4">
              <div className="mb-3 flex items-center gap-2 text-sm font-medium">
                <Terminal className="h-4 w-4 text-primary" />
                {text(language, '扫描状态', 'Scan Status')}
              </div>
              <div className="space-y-2 font-mono text-xs">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('target')}</span>
                  <span className="truncate pl-2 text-right">{scan.target}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{text(language, '状态', 'Status')}</span>
                  <span className={cn(
                    scan.status === 'running' && 'text-emerald-400',
                    scan.status === 'completed' && 'text-blue-400',
                    scan.status === 'failed' && 'text-red-400',
                  )}>{scan.status}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('turns')}</span>
                  <span>{scan.turns}/{scan.max_turns}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('duration')}</span>
                  <span>{scan.duration}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('findings')}</span>
                  <span className={cn(findings.length > 0 && 'text-amber-400')}>{scan.findings}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-muted-foreground">{t('tokens')}</span>
                  <span>{scan.tokens_in + scan.tokens_out}</span>
                </div>
                {scan.est_cost > 0 && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">{t('cost')}</span>
                    <span>${scan.est_cost.toFixed(4)}</span>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Findings */}
          {findings.length > 0 && (
            <div className="rounded-3xl border border-white/10 bg-white/[0.02] p-4">
              <div className="mb-3 flex items-center gap-2 text-sm font-medium">
                <ShieldAlert className="h-4 w-4 text-amber-400" />
                {t('findings')} ({findings.length})
              </div>
              <div className="space-y-2">
                {findings.map((f, i) => (
                  <div
                    key={f.id || i}
                    className={cn(
                      'rounded-xl border p-3 text-xs',
                      f.severity === 'critical' && 'border-red-400/30 bg-red-500/10',
                      f.severity === 'high' && 'border-orange-400/30 bg-orange-500/10',
                      f.severity === 'medium' && 'border-amber-400/30 bg-amber-500/10',
                      f.severity === 'low' && 'border-blue-400/30 bg-blue-500/10',
                      f.severity === 'info' && 'border-white/10 bg-white/[0.03]',
                    )}
                  >
                    <div className="flex items-center gap-2">
                      <span className={cn(
                        'rounded px-1.5 py-0.5 font-mono text-[10px] uppercase',
                        f.severity === 'critical' && 'bg-red-500/30 text-red-200',
                        f.severity === 'high' && 'bg-orange-500/30 text-orange-200',
                        f.severity === 'medium' && 'bg-amber-500/30 text-amber-200',
                        f.severity === 'low' && 'bg-blue-500/30 text-blue-200',
                        f.severity === 'info' && 'bg-white/10 text-muted-foreground',
                      )}>
                        {f.severity}
                      </span>
                      <span className="font-medium text-foreground/90">{f.title}</span>
                    </div>
                    {f.description && (
                      <p className="mt-1 text-muted-foreground line-clamp-2">{f.description}</p>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Tools */}
          {tools.length > 0 && (
            <div className="rounded-3xl border border-white/10 bg-white/[0.02] p-4">
              <div className="mb-3 flex items-center gap-2 text-sm font-medium">
                <Wrench className="h-4 w-4 text-primary" />
                {t('tools')} ({tools.length})
              </div>
              <div className="space-y-1">
                {tools.map((tool) => (
                  <div key={tool.name} className="flex items-start gap-2 rounded-lg p-2 text-xs hover:bg-white/[0.04]">
                    <Wrench className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground" />
                    <div>
                      <div className="font-mono font-medium">{tool.name}</div>
                      <div className="text-muted-foreground line-clamp-1">{tool.description}</div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Recent Scans */}
          <div className="rounded-3xl border border-white/10 bg-white/[0.02] p-4">
            <div className="mb-3 flex items-center gap-2 text-sm font-medium">
              <MessageSquare className="h-4 w-4 text-primary" />
              {t('history')}
            </div>
            {recentScans.length === 0 ? (
              <p className="text-xs text-muted-foreground">{t('noData')}</p>
            ) : (
              <div className="space-y-1">
                {recentScans.slice(0, 8).map((s) => (
                  <button
                    key={s.id}
                    onClick={() => resumeScan(s.id)}
                    className={cn(
                      'flex w-full items-center justify-between rounded-xl px-3 py-2 text-xs transition hover:bg-white/[0.06]',
                      scanId === s.id && 'bg-white/[0.08]',
                    )}
                  >
                    <div className="flex items-center gap-2 truncate">
                      {s.status === 'running' && <Loader2 className="h-3 w-3 animate-spin text-emerald-400" />}
                      {s.status === 'completed' && <CheckCircle2 className="h-3 w-3 text-blue-400" />}
                      {s.status === 'failed' && <AlertTriangle className="h-3 w-3 text-red-400" />}
                      <span className="truncate">{s.target}</span>
                    </div>
                    <span className="font-mono text-muted-foreground">{s.findings}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
