import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  ChevronRight,
  Crosshair,
  Loader2,
  MessageSquare,
  PanelLeftClose,
  PanelLeftOpen,
  PlayCircle,
  RefreshCcw,
  Send,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Square,
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
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const chatEndRef = useRef<HTMLDivElement>(null)
  const eventSourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    client.getTools().then(setTools).catch(() => {})
  }, [client])

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

  const lastEventCountRef = useRef(0)
  useEffect(() => {
    if (!scanId) return
    lastEventCountRef.current = 0
    const poll = async () => {
      try {
        const detail = await client.getScan(scanId)
        setScan(detail)

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

  const [toolsExpanded, setToolsExpanded] = useState(false)

  return (
    <div className="flex h-[calc(100vh-7rem)] gap-3">
      {/* Session Sidebar */}
      <div
        className={cn(
          'flex shrink-0 flex-col rounded-3xl border border-white/10 bg-white/[0.02] transition-all duration-200',
          sidebarOpen ? 'w-60' : 'w-12',
        )}
      >
        <div className="flex items-center justify-between border-b border-white/10 p-3">
          {sidebarOpen && (
            <span className="flex items-center gap-2 text-sm font-medium">
              <MessageSquare className="h-4 w-4 text-primary" />
              {t('history')}
            </span>
          )}
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="grid h-7 w-7 place-items-center rounded-lg text-muted-foreground hover:bg-white/[0.06] hover:text-foreground"
          >
            {sidebarOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeftOpen className="h-4 w-4" />}
          </button>
        </div>
        {sidebarOpen && (
          <div className="flex-1 overflow-y-auto p-2">
            {recentScans.length === 0 ? (
              <p className="p-2 text-xs text-muted-foreground">{t('noData')}</p>
            ) : (
              <div className="space-y-0.5">
                {recentScans.map((s) => (
                  <button
                    key={s.id}
                    onClick={() => resumeScan(s.id)}
                    className={cn(
                      'flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-xs transition hover:bg-white/[0.06]',
                      scanId === s.id && 'bg-white/[0.08]',
                    )}
                  >
                    {s.status === 'running' && <Loader2 className="h-3 w-3 shrink-0 animate-spin text-emerald-400" />}
                    {s.status === 'completed' && <CheckCircle2 className="h-3 w-3 shrink-0 text-blue-400" />}
                    {s.status === 'failed' && <AlertTriangle className="h-3 w-3 shrink-0 text-red-400" />}
                    <span className="truncate">{s.target}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Main Chat Area */}
      <div className="flex min-w-0 flex-1 flex-col rounded-3xl border border-white/10 bg-white/[0.02]">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/10 px-5 py-3">
          <div className="flex items-center gap-3">
            <span className="grid h-9 w-9 place-items-center rounded-xl border border-primary/30 bg-primary/10">
              <Bot className="h-4 w-4 text-primary" />
            </span>
            <div>
              <h1 className="text-sm font-semibold">
                {text(language, '安全 Agent', 'Security Agent')}
              </h1>
              <p className="font-mono text-[10px] text-muted-foreground">
                {text(language, '规划 → 执行 → 重规划 → 总结', 'Plan → Step → Replan → Final')}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {/* Phase Indicator */}
            {scanId && isRunning && (
              <div className="flex items-center gap-0.5 rounded-xl border border-white/10 bg-white/[0.03] px-2 py-1">
                {phaseOrder.map((phase, i) => {
                  const cfg = phaseConfig[phase]
                  const active = currentPhase === phase
                  const done = currentPhase ? phaseOrder.indexOf(currentPhase) > i : false
                  return (
                    <div key={phase} className="flex items-center gap-0.5">
                      <span
                        className={cn(
                          'flex items-center gap-1 rounded-lg px-2 py-1 font-mono text-[10px] transition-all',
                          active && 'bg-white/10 text-foreground',
                          done && 'text-emerald-400',
                          !active && !done && 'text-muted-foreground',
                        )}
                      >
                        <cfg.icon className={cn('h-3 w-3', active && cfg.color)} />
                        {text(language, cfg.labelZh, cfg.label)}
                      </span>
                      {i < phaseOrder.length - 1 && (
                        <ChevronRight className="h-2.5 w-2.5 text-muted-foreground" />
                      )}
                    </div>
                  )
                })}
              </div>
            )}

            {/* Scan Status Chips */}
            {scan && (
              <div className="flex items-center gap-1.5 font-mono text-[10px] text-muted-foreground">
                <span className={cn(
                  'rounded-lg px-2 py-1',
                  scan.status === 'running' && 'bg-emerald-500/10 text-emerald-400',
                  scan.status === 'completed' && 'bg-blue-500/10 text-blue-400',
                  scan.status === 'failed' && 'bg-red-500/10 text-red-400',
                )}>{scan.status}</span>
                <span className="rounded-lg bg-white/[0.04] px-2 py-1">{scan.turns}/{scan.max_turns}</span>
                <span className="rounded-lg bg-white/[0.04] px-2 py-1">{scan.duration}</span>
                {scan.findings > 0 && (
                  <span className="rounded-lg bg-amber-500/10 px-2 py-1 text-amber-400">{scan.findings} findings</span>
                )}
              </div>
            )}

            {/* Tools Toggle */}
            {tools.length > 0 && (
              <button
                onClick={() => setToolsExpanded(!toolsExpanded)}
                className={cn(
                  'grid h-8 w-8 place-items-center rounded-xl transition hover:bg-white/[0.06]',
                  toolsExpanded ? 'bg-white/[0.08] text-foreground' : 'text-muted-foreground',
                )}
                title={`${tools.length} tools`}
              >
                <Wrench className="h-4 w-4" />
              </button>
            )}
          </div>
        </div>

        {/* Tools Dropdown */}
        {toolsExpanded && tools.length > 0 && (
          <div className="border-b border-white/10 bg-white/[0.01] px-5 py-3">
            <div className="flex flex-wrap gap-1.5">
              {tools.map((tool) => (
                <span
                  key={tool.name}
                  className="flex items-center gap-1 rounded-lg bg-white/[0.04] px-2.5 py-1 font-mono text-[11px] text-muted-foreground"
                  title={tool.description}
                >
                  <Wrench className="h-3 w-3" />
                  {tool.name}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Findings Bar */}
        {findings.length > 0 && (
          <div className="border-b border-white/10 bg-white/[0.01] px-5 py-2.5">
            <div className="flex items-center gap-2 text-xs">
              <ShieldAlert className="h-3.5 w-3.5 text-amber-400" />
              <span className="font-medium">{t('findings')} ({findings.length})</span>
              <div className="flex flex-1 gap-1.5 overflow-x-auto">
                {findings.map((f, i) => (
                  <span
                    key={f.id || i}
                    className={cn(
                      'shrink-0 rounded-lg px-2 py-0.5 font-mono text-[10px]',
                      f.severity === 'critical' && 'bg-red-500/20 text-red-300',
                      f.severity === 'high' && 'bg-orange-500/20 text-orange-300',
                      f.severity === 'medium' && 'bg-amber-500/20 text-amber-300',
                      f.severity === 'low' && 'bg-blue-500/20 text-blue-300',
                      f.severity === 'info' && 'bg-white/[0.06] text-muted-foreground',
                    )}
                  >
                    [{f.severity}] {f.title}
                  </span>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* Chat Messages */}
        <div className="flex-1 overflow-y-auto p-5">
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
              <div className="grid grid-cols-3 gap-2 text-xs text-muted-foreground">
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
          <div className="flex items-center gap-2 border-t border-red-400/20 bg-red-500/10 px-5 py-2 text-xs text-red-200">
            <AlertTriangle className="h-3 w-3" />
            {error}
          </div>
        )}

        {/* Input Area */}
        <div className="border-t border-white/10 p-4">
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
    </div>
  )
}
