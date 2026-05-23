import { useEffect, useState, useRef } from 'react'
import { useParams } from 'react-router-dom'
import { Square, Clock, Cpu, DollarSign, AlertTriangle, Send } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { useScanEvents } from '@/hooks/use-scan-events'
import { api, type ScanDetail, type UIEvent } from '@/lib/api'

export function ScanDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [scan, setScan] = useState<ScanDetail | null>(null)
  const [stopping, setStopping] = useState(false)
  const [message, setMessage] = useState('')
  const [sending, setSending] = useState(false)
  const { events } = useScanEvents(id)
  const logRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!id) return
    const load = () => api.getScan(id).then(setScan).catch(() => {})
    load()
    const interval = setInterval(load, 2000)
    return () => clearInterval(interval)
  }, [id])

  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight
    }
  }, [events])

  const handleStop = async () => {
    if (!id) return
    setStopping(true)
    try { await api.stopScan(id) } catch { /* ignore */ }
    setStopping(false)
  }

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id || !message.trim()) return
    setSending(true)
    try {
      await api.sendMessage(id, message.trim())
      setMessage('')
    } catch { /* ignore */ }
    setSending(false)
  }

  if (!scan) {
    return <div className="text-muted-foreground">Loading scan...</div>
  }

  const isRunning = scan.status === 'running'
  const allEvents = [...(scan.events || []), ...events]
  const findings = scan.findings_detail || []
  const isInteractive = scan.interactive
  const isWaiting = scan.waiting

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold">{scan.target}</h2>
          <div className="flex items-center gap-3 mt-1">
            <StatusBadge status={scan.status} />
            {isInteractive && <Badge variant="secondary">Interactive</Badge>}
            {isWaiting && <Badge variant="default">Waiting for input</Badge>}
            <span className="text-sm text-muted-foreground">{scan.provider}</span>
            {scan.model && <span className="text-sm text-muted-foreground">/ {scan.model}</span>}
          </div>
        </div>
        {isRunning && (
          <Button variant="destructive" onClick={handleStop} disabled={stopping}>
            <Square className="mr-2 h-4 w-4" />
            {stopping ? 'Stopping...' : 'Stop'}
          </Button>
        )}
      </div>

      <div className="grid gap-4 md:grid-cols-5">
        <MiniStat icon={Cpu} label="Progress" value={`${scan.turns} / ${scan.max_turns}`} />
        <MiniStat icon={Clock} label="Duration" value={scan.duration} />
        <MiniStat icon={Cpu} label="Tokens" value={`${fmtNum(scan.tokens_in)} / ${fmtNum(scan.tokens_out)}`} />
        <MiniStat icon={DollarSign} label="Est. Cost" value={scan.est_cost > 0 ? `$${scan.est_cost.toFixed(4)}` : '-'} />
        <MiniStat icon={AlertTriangle} label="Findings" value={String(findings.length)} />
      </div>

      {isRunning && (
        <div className="h-2 bg-secondary rounded-full overflow-hidden">
          <div
            className="h-full bg-primary rounded-full transition-all duration-500"
            style={{ width: `${(scan.turns / scan.max_turns) * 100}%` }}
          />
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <Card className="flex flex-col">
            <CardHeader className="pb-3">
              <CardTitle className="text-sm">
                {isInteractive ? 'Conversation' : 'Event Log'}
              </CardTitle>
            </CardHeader>
            <CardContent className="flex-1 flex flex-col">
              <div ref={logRef} className="h-[450px] overflow-y-auto font-mono text-xs space-y-1 bg-background rounded p-3 border">
                {allEvents.map((ev, i) => (
                  <EventLine key={i} event={ev} />
                ))}
                {allEvents.length === 0 && (
                  <p className="text-muted-foreground">Waiting for events...</p>
                )}
                {isWaiting && (
                  <div className="text-primary animate-pulse mt-2">Agent is waiting for your input...</div>
                )}
              </div>

              {isInteractive && isRunning && (
                <form onSubmit={handleSend} className="mt-3 flex gap-2">
                  <Input
                    value={message}
                    onChange={(e) => setMessage(e.target.value)}
                    placeholder={isWaiting ? "Type your message to the agent..." : "Agent is working..."}
                    disabled={sending || !isWaiting}
                    className="flex-1"
                    autoFocus
                  />
                  <Button type="submit" disabled={sending || !message.trim() || !isWaiting} size="icon">
                    <Send className="h-4 w-4" />
                  </Button>
                </form>
              )}
            </CardContent>
          </Card>
        </div>

        <div>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm">Findings ({findings.length})</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-3 max-h-[500px] overflow-y-auto">
                {findings.map((f, i) => (
                  <div key={i} className="border rounded-md p-3 space-y-1">
                    <div className="flex items-center gap-2">
                      <Badge variant={f.severity as 'critical' | 'high' | 'medium' | 'low' | 'info'}>
                        {f.severity}
                      </Badge>
                      <span className="text-sm font-medium">{f.title}</span>
                    </div>
                    <p className="text-xs text-muted-foreground">{f.description}</p>
                  </div>
                ))}
                {findings.length === 0 && (
                  <p className="text-sm text-muted-foreground">No findings yet</p>
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      {scan.summary && (
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm">Summary</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm whitespace-pre-wrap">{scan.summary}</p>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  const variant = {
    running: 'default' as const,
    completed: 'secondary' as const,
    failed: 'destructive' as const,
    stopped: 'outline' as const,
  }[status] || 'outline' as const
  return <Badge variant={variant}>{status}</Badge>
}

function MiniStat({ icon: Icon, label, value }: { icon: React.ElementType; label: string; value: string }) {
  return (
    <Card>
      <CardContent className="p-4">
        <div className="flex items-center gap-2">
          <Icon className="h-4 w-4 text-muted-foreground" />
          <span className="text-xs text-muted-foreground">{label}</span>
        </div>
        <p className="text-lg font-semibold mt-1">{value}</p>
      </CardContent>
    </Card>
  )
}

function EventLine({ event }: { event: UIEvent }) {
  const time = new Date(event.timestamp).toLocaleTimeString()

  if (event.type === 'user_message') {
    return (
      <div className="text-cyan-400 leading-relaxed py-1 border-l-2 border-cyan-500 pl-2 my-1">
        <span className="text-muted-foreground">{time}</span>{' '}
        <span className="font-semibold">[you] </span>
        {event.message}
      </div>
    )
  }

  let color = 'text-muted-foreground'
  let prefix = ''

  switch (event.type) {
    case 'tool_call':
      color = 'text-purple-400'
      prefix = `[tool] ${event.tool}`
      break
    case 'finding':
      color = 'text-green-400'
      prefix = `[finding]`
      break
    case 'error':
      color = 'text-red-400'
      prefix = `[error]`
      break
    case 'text_delta':
      color = 'text-foreground'
      break
    case 'progress':
      color = 'text-blue-400'
      prefix = `[turn ${event.turn}/${event.max_turns}]`
      break
    case 'done':
      color = 'text-emerald-400'
      prefix = '[agent]'
      break
    default:
      prefix = `[${event.type}]`
  }

  return (
    <div className={`${color} leading-relaxed`}>
      <span className="text-muted-foreground">{time}</span>{' '}
      {prefix && <span className="font-semibold">{prefix} </span>}
      {event.message}
    </div>
  )
}

function fmtNum(n: number): string {
  if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return String(n)
}
