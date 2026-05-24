import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  Archive,
  Check,
  ChevronLeft,
  Clock3,
  Crosshair,
  FileJson,
  FileText,
  PauseCircle,
  Play,
  RefreshCcw,
  ShieldCheck,
  Square,
  TerminalSquare,
  X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import type {
  ApprovalRequest,
  ArtifactIndex,
  DaemonJob,
  EventItem,
  Finding,
  ReportPayload,
  RunDetail,
} from '@/lib/api'
import { recentJobForRun } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'
import { formatBytes, formatDate, formatTime, hostLabel, severityTone, shortId, statusLabel, statusTone } from '@/lib/view'

type Panel = 'events' | 'report' | 'artifacts' | 'approvals'

export function ScanDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  const { client, language, settings, t } = useConsole()
  const [detail, setDetail] = useState<RunDetail | null>(null)
  const [events, setEvents] = useState<EventItem[]>([])
  const [jsonReport, setJsonReport] = useState<ReportPayload | null>(null)
  const [markdownReport, setMarkdownReport] = useState<ReportPayload | null>(null)
  const [artifacts, setArtifacts] = useState<ArtifactIndex | null>(null)
  const [approvals, setApprovals] = useState<ApprovalRequest[]>([])
  const [job, setJob] = useState<DaemonJob | null>(null)
  const [active, setActive] = useState<Panel>('events')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState('')

  const recentJob = id ? recentJobForRun(id) : undefined

  const refresh = useCallback(async () => {
    if (!id) return
    setLoading(true)
    setError('')
    try {
      const [runDetail, eventPage, reportJson, reportMd, artifactIndex, approvalsList, daemonJob] = await Promise.all([
        client.getRun(id),
        client.getEvents(id, '', 160).catch(() => ({ items: [], eof: true, limit: 160 })),
        client.getReport(id, 'json').catch(() => null),
        client.getReport(id, 'md').catch(() => null),
        client.getArtifacts(id).catch(() => null),
        client.getApprovals(id).catch(() => ({ approvals: [] })),
        recentJob?.job_id ? client.getJob(recentJob.job_id).catch(() => null) : Promise.resolve(null),
      ])
      setDetail(runDetail)
      setEvents(eventPage.items || [])
      setJsonReport(reportJson)
      setMarkdownReport(reportMd)
      setArtifacts(artifactIndex)
      setApprovals(approvalsList.approvals || [])
      setJob(daemonJob)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    } finally {
      setLoading(false)
    }
  }, [client, id, recentJob?.job_id])

  useEffect(() => {
    refresh()
    const interval = window.setInterval(refresh, 5000)
    return () => window.clearInterval(interval)
  }, [refresh])

  const status = job?.status || detail?.summary.status || detail?.job?.status
  const findings = useMemo(() => (jsonReport?.report?.findings || []) as Finding[], [jsonReport])
  const pendingApprovals = approvals.filter((approval) => approval.status === 'pending')
  const canCancel = Boolean(job?.job_id && job.status === 'running')

  const cancelJob = async () => {
    if (!job?.job_id) return
    setBusy('cancel')
    try {
      await client.cancelJob(job.job_id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'cancel failed')
    } finally {
      setBusy('')
    }
  }

  const resolveApproval = async (approval: ApprovalRequest, decision: 'approve' | 'deny') => {
    setBusy(`${decision}:${approval.id}`)
    try {
      if (decision === 'approve') {
        await client.approveApproval(id, approval.id, settings.operator)
      } else {
        await client.denyApproval(id, approval.id, settings.operator)
      }
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'approval failed')
    } finally {
      setBusy('')
    }
  }

  const resumeApproved = async () => {
    setBusy('resume')
    try {
      await client.resumeApproved(id, settings.operator)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'resume failed')
    } finally {
      setBusy('')
    }
  }

  if (!id) {
    return <EmptyState label="missing run id" />
  }

  return (
    <div className="space-y-6">
      <header className="rounded-[2rem] border border-white/10 bg-white/[0.035] p-5 sm:p-7">
        <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
          <Button asChild variant="ghost" className="rounded-2xl">
            <Link to="/history">
              <ChevronLeft className="mr-2 h-4 w-4" />
              {t('runs')}
            </Link>
          </Button>
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
              <RefreshCcw className={cn('mr-2 h-4 w-4', loading && 'animate-spin')} />
              {t('refresh')}
            </Button>
            {canCancel && (
              <Button type="button" variant="destructive" onClick={cancelJob} disabled={busy === 'cancel'} className="rounded-2xl">
                <Square className="mr-2 h-4 w-4" />
                {t('cancel')}
              </Button>
            )}
          </div>
        </div>

        <div className="grid gap-6 lg:grid-cols-[1fr_auto]">
          <div className="min-w-0">
            <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
              <Crosshair className="h-3.5 w-3.5" />
              {detail?.summary.profile || detail?.job?.profile || 'safe'}
            </div>
            <h1 className="truncate text-3xl font-semibold tracking-tight sm:text-5xl">
              {hostLabel(detail?.summary.target)}
            </h1>
            <div className="mt-3 flex flex-wrap gap-2 font-mono text-xs text-muted-foreground">
              <span>{shortId(id, 34)}</span>
              <span>{detail?.summary.scope || 'scope:—'}</span>
              <span>{detail?.run_dir || 'runs/—'}</span>
            </div>
          </div>

          <div className="grid min-w-72 gap-2 rounded-3xl border border-white/10 bg-black/25 p-4">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">{t('job')}</span>
              <span className={cn('rounded-full border px-2 py-0.5 text-[11px]', statusTone(status))}>
                {statusLabel(status, language)}
              </span>
            </div>
            <div className="font-mono text-xs text-muted-foreground">{job?.job_id || detail?.job?.id || 'job:—'}</div>
          </div>
        </div>
      </header>

      {error && (
        <div className="flex items-center gap-3 rounded-2xl border border-red-400/20 bg-red-500/10 px-4 py-3 text-sm text-red-100">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <Metric icon={Activity} label={t('events')} value={detail?.summary.counts?.events || events.length} />
        <Metric icon={Archive} label={t('artifacts')} value={detail?.summary.counts?.artifacts || artifacts?.count || 0} />
        <Metric icon={AlertTriangle} label={t('findings')} value={findings.length || detail?.summary.counts?.findings || 0} tone="amber" />
        <Metric icon={ShieldCheck} label={t('approvals')} value={pendingApprovals.length || detail?.summary.counts?.approvals || 0} tone="violet" />
        <Metric icon={Clock3} label={text(language, '更新', 'Updated')} value={formatDate(detail?.summary.updated_at || detail?.summary.created_at, language)} textValue />
      </section>

      <section className="grid gap-4 xl:grid-cols-[280px_1fr]">
        <nav className="grid gap-2 rounded-[2rem] border border-white/10 bg-white/[0.03] p-3 xl:self-start">
          <PanelButton icon={TerminalSquare} id="events" active={active} setActive={setActive} label={t('events')} count={events.length} />
          <PanelButton icon={FileText} id="report" active={active} setActive={setActive} label={t('report')} count={findings.length} />
          <PanelButton icon={Archive} id="artifacts" active={active} setActive={setActive} label={t('artifacts')} count={artifacts?.count || 0} />
          <PanelButton icon={PauseCircle} id="approvals" active={active} setActive={setActive} label={t('approvals')} count={approvals.length} />
        </nav>

        <div className="min-h-[560px] rounded-[2rem] border border-white/10 bg-white/[0.03] p-4">
          {active === 'events' && <EventsPanel events={events} language={language} empty={loading ? t('loading') : t('noData')} />}
          {active === 'report' && <ReportPanel findings={findings} markdown={markdownReport?.content || ''} language={language} empty={t('noData')} />}
          {active === 'artifacts' && <ArtifactsPanel artifacts={artifacts} empty={t('noData')} />}
          {active === 'approvals' && (
            <ApprovalsPanel
              approvals={approvals}
              busy={busy}
              language={language}
              empty={t('noData')}
              onApprove={(approval) => resolveApproval(approval, 'approve')}
              onDeny={(approval) => resolveApproval(approval, 'deny')}
              onResume={resumeApproved}
            />
          )}
        </div>
      </section>
    </div>
  )
}

function Metric({
  icon: Icon,
  label,
  value,
  tone = 'blue',
  textValue = false,
}: {
  icon: React.ElementType
  label: string
  value: number | string
  tone?: 'blue' | 'amber' | 'violet'
  textValue?: boolean
}) {
  const tones = {
    blue: 'border-blue-400/20 bg-blue-400/10 text-blue-300',
    amber: 'border-amber-400/20 bg-amber-400/10 text-amber-300',
    violet: 'border-violet-400/20 bg-violet-400/10 text-violet-300',
  }
  return (
    <div className="rounded-3xl border border-white/10 bg-white/[0.03] p-4">
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span className={cn('grid h-9 w-9 place-items-center rounded-2xl border', tones[tone])}>
          <Icon className="h-4 w-4" />
        </span>
      </div>
      <div className={cn('mt-4 font-mono font-semibold', textValue ? 'text-sm' : 'text-3xl')}>{value}</div>
    </div>
  )
}

function PanelButton({
  icon: Icon,
  id,
  active,
  setActive,
  label,
  count,
}: {
  icon: React.ElementType
  id: Panel
  active: Panel
  setActive: (panel: Panel) => void
  label: string
  count: number
}) {
  const selected = active === id
  return (
    <button
      type="button"
      onClick={() => setActive(id)}
      className={cn(
        'flex items-center justify-between gap-3 rounded-2xl border px-4 py-3 text-left transition',
        selected ? 'border-primary/35 bg-primary/10 text-foreground' : 'border-white/10 bg-black/20 text-muted-foreground hover:bg-white/[0.04]',
      )}
    >
      <span className="flex items-center gap-3">
        <Icon className="h-4 w-4" />
        <span className="text-sm">{label}</span>
      </span>
      <span className="font-mono text-xs">{count}</span>
    </button>
  )
}

function EventsPanel({ events, language, empty }: { events: EventItem[]; language: 'zh' | 'en'; empty: string }) {
  if (!events.length) return <EmptyState label={empty} />
  return (
    <div className="space-y-2">
      {events.map((item) => {
        const levelTone =
          item.event.level === 'error'
            ? 'border-red-400/25 bg-red-400/10'
            : item.event.level === 'warn'
              ? 'border-amber-400/25 bg-amber-400/10'
              : 'border-white/10 bg-black/20'
        return (
          <div key={`${item.sequence}-${item.event.id}`} className={cn('rounded-2xl border p-3', levelTone)}>
            <div className="mb-2 flex flex-wrap items-center gap-2 font-mono text-[11px] text-muted-foreground">
              <span>#{item.sequence}</span>
              <span>{formatTime(item.event.timestamp, language)}</span>
              <span>{item.event.type}</span>
              {item.event.step_id && <span>{item.event.step_id}</span>}
            </div>
            <p className="text-sm leading-6">{item.event.message || '—'}</p>
          </div>
        )
      })}
    </div>
  )
}

function ReportPanel({
  findings,
  markdown,
  language,
  empty,
}: {
  findings: Finding[]
  markdown: string
  language: 'zh' | 'en'
  empty: string
}) {
  return (
    <div className="grid gap-4 xl:grid-cols-[360px_1fr]">
      <div className="space-y-2">
        <div className="mb-3 flex items-center gap-2 text-sm font-semibold">
          <FileJson className="h-4 w-4 text-primary" />
          {text(language, '发现', 'Findings')}
        </div>
        {findings.map((finding, index) => (
          <div key={finding.id || index} className="rounded-2xl border border-white/10 bg-black/20 p-3">
            <div className="mb-2 flex items-center gap-2">
              <span className={cn('rounded-full border px-2 py-0.5 text-[11px]', severityTone(finding.severity))}>
                {finding.severity || 'info'}
              </span>
              <span className="truncate text-sm font-medium">{finding.title || finding.id || 'finding'}</span>
            </div>
            <p className="line-clamp-3 text-xs leading-5 text-muted-foreground">{finding.description || finding.category || '—'}</p>
          </div>
        ))}
        {!findings.length && <EmptyState label={empty} />}
      </div>

      <pre className="min-h-[440px] overflow-auto rounded-2xl border border-white/10 bg-black/30 p-4 text-xs leading-6 text-muted-foreground">
        {markdown || empty}
      </pre>
    </div>
  )
}

function ArtifactsPanel({ artifacts, empty }: { artifacts: ArtifactIndex | null; empty: string }) {
  const items = artifacts?.artifacts || []
  if (!items.length) return <EmptyState label={empty} />
  return (
    <div className="space-y-2">
      {items.map((artifact) => (
        <div key={artifact.id} className="grid gap-3 rounded-2xl border border-white/10 bg-black/20 p-3 lg:grid-cols-[1fr_auto]">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <Archive className="h-4 w-4 text-primary" />
              <span className="truncate font-mono text-sm">{artifact.path}</span>
            </div>
            <div className="mt-1 flex flex-wrap gap-3 font-mono text-xs text-muted-foreground">
              <span>{artifact.type || artifact.mime_type || 'artifact'}</span>
              <span>{shortId(artifact.sha256, 18)}</span>
              {artifact.redacted && <span>redacted</span>}
            </div>
          </div>
          <span className="rounded-2xl border border-white/10 bg-white/[0.03] px-3 py-2 font-mono text-xs text-muted-foreground">
            {formatBytes(artifact.size_bytes)}
          </span>
        </div>
      ))}
    </div>
  )
}

function ApprovalsPanel({
  approvals,
  busy,
  language,
  empty,
  onApprove,
  onDeny,
  onResume,
}: {
  approvals: ApprovalRequest[]
  busy: string
  language: 'zh' | 'en'
  empty: string
  onApprove: (approval: ApprovalRequest) => void
  onDeny: (approval: ApprovalRequest) => void
  onResume: () => void
}) {
  if (!approvals.length) return <EmptyState label={empty} />
  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Button type="button" variant="outline" onClick={onResume} disabled={busy === 'resume'} className="rounded-2xl bg-transparent">
          <Play className="mr-2 h-4 w-4" />
          {text(language, '恢复已通过', 'Resume approved')}
        </Button>
      </div>
      {approvals.map((approval) => (
        <div key={approval.id} className="rounded-2xl border border-white/10 bg-black/20 p-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-sm">{approval.id}</span>
                <span className={cn('rounded-full border px-2 py-0.5 text-[11px]', statusTone(approval.status))}>
                  {statusLabel(approval.status, language)}
                </span>
              </div>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{approval.reason || approval.requested_action || '—'}</p>
              <div className="mt-2 flex flex-wrap gap-3 font-mono text-xs text-muted-foreground">
                <span>{approval.step_id || 'step:—'}</span>
                <span>{approval.created_at ? formatDate(approval.created_at, language) : '—'}</span>
              </div>
            </div>
            {approval.status === 'pending' && (
              <div className="flex gap-2">
                <Button type="button" size="sm" onClick={() => onApprove(approval)} disabled={busy === `approve:${approval.id}`} className="rounded-2xl">
                  <Check className="mr-2 h-4 w-4" />
                  {text(language, '通过', 'Approve')}
                </Button>
                <Button type="button" size="sm" variant="destructive" onClick={() => onDeny(approval)} disabled={busy === `deny:${approval.id}`} className="rounded-2xl">
                  <X className="mr-2 h-4 w-4" />
                  {text(language, '拒绝', 'Deny')}
                </Button>
              </div>
            )}
          </div>
        </div>
      ))}
    </div>
  )
}

function EmptyState({ label }: { label: string }) {
  return (
    <div className="grid min-h-64 place-items-center rounded-3xl border border-dashed border-white/10 bg-black/10 text-sm text-muted-foreground">
      {label}
    </div>
  )
}
