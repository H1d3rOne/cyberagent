import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  ChevronRight,
  Database,
  Download,
  FileText,
  ListChecks,
  MessageSquare,
  RefreshCcw,
  Search,
  Send,
  ShieldCheck,
  Square,
  Trash2,
  X,
  XCircle,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PageHeader, PageTitle, SectionTitle } from '@/components/layout-primitives'
import type { ApprovalRequest, Artifact, Finding, ScanDetail, UIEvent } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'
import { hostLabel, severityTone, statusLabel, statusTone } from '@/lib/view'

type AuditFilter = 'all' | 'error' | 'warning' | 'finding' | 'artifact' | 'approval'
type EvidenceView = 'overview' | 'findings' | 'artifacts' | 'approvals' | 'report'

export function ScanDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { client, language, t } = useConsole()
  const [scan, setScan] = useState<ScanDetail | null>(null)
  const [liveEvents, setLiveEvents] = useState<UIEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [sending, setSending] = useState(false)
  const [auditSearch, setAuditSearch] = useState('')
  const [auditFilter, setAuditFilter] = useState<AuditFilter>('all')
  const [evidenceView, setEvidenceView] = useState<EvidenceView>('overview')
  const eventsEndRef = useRef<HTMLDivElement>(null)
  const eventRefs = useRef<Record<string, HTMLDivElement | null>>({})
  const focusStep = searchParams.get('step') || ''

  const fetchDetail = useCallback(async () => {
    if (!id) return
    try {
      const detail = await client.getScan(id)
      setScan(detail)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed')
    } finally {
      setLoading(false)
    }
  }, [client, id])

  useEffect(() => {
    fetchDetail()
    const interval = window.setInterval(fetchDetail, 5_000)
    return () => window.clearInterval(interval)
  }, [fetchDetail])

  useEffect(() => {
    // Daemon v1 run events are loaded by fetchDetail/getScan polling.
    // Keep liveEvents empty to avoid calling legacy /api/scans SSE endpoints.
    setLiveEvents([])
  }, [id])

  useEffect(() => {
    eventsEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [liveEvents])

  useEffect(() => {
    if (!focusStep || !scan) return
    const target = eventRefs.current[focusStep]
    if (!target) return
    window.setTimeout(() => {
      target.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }, 120)
  }, [focusStep, scan])

  const handleStop = async () => {
    if (!id) return
    try {
      await client.stopScan(id)
      fetchDetail()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'stop failed')
    }
  }

  const handleDelete = async () => {
    if (!id) return
    try {
      await client.deleteScan(id)
      navigate('/history')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'delete failed')
    }
  }

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id || !message.trim() || sending) return
    setSending(true)
    try {
      await client.sendMessage(id, message.trim())
      setMessage('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'send failed')
    } finally {
      setSending(false)
    }
  }

  const handleClearFocus = () => {
    if (!id) return
    navigate(`/scan/${id}`)
  }

  const handleFocusStep = (stepID: string) => {
    if (!id || !stepID) return
    navigate(`/scan/${id}?step=${encodeURIComponent(stepID)}`)
  }

  const handleEvidenceAuditFilter = (filter: AuditFilter) => {
    setAuditSearch('')
    setAuditFilter(filter)
  }

  const handleExportEvidence = (format: 'md' | 'json') => {
    if (!scan || !evidenceSummary) return
    exportRunEvidence(scan, evidenceSummary, stepGroups, format)
  }

  if (loading && !scan) {
    return (
      <div className="grid min-h-[400px] place-items-center text-muted-foreground">
        {t('loading')}
      </div>
    )
  }

  const allEvents = scan ? [...scan.events, ...liveEvents] : liveEvents
  const stepGroups = buildStepGroups(allEvents, scan)
  const evidenceSummary = scan ? buildRunEvidenceSummary(scan, stepGroups) : null
  const selectedStepGroup = focusStep ? stepGroups.find((group) => group.stepID === focusStep) || null : null
  const visibleStepGroups = stepGroups.filter((group) => stepGroupMatchesAudit(group, auditSearch, auditFilter))
  const visibleEvents = allEvents.filter((event) => eventMatchesAudit(event, auditSearch, auditFilter))
  const auditActive = auditSearch.trim().length > 0 || auditFilter !== 'all'
  const auditFilterOptions = buildAuditFilterOptions(language)
  const focusEvents = focusStep ? allEvents.filter((event) => eventMatchesStep(event, focusStep)) : []
  const focusApprovals = focusStep ? (scan?.approvals || []).filter((approval) => approval.step_id === focusStep) : []
  const isRunning = scan?.status === 'running'
  const isInteractive = scan?.interactive
  const isWaiting = scan?.waiting

  return (
    <div className="space-y-4">
      <PageHeader>
        <div className="flex min-w-0 items-center gap-3">
          <Button variant="ghost" size="icon" className="rounded-xl" onClick={() => navigate(-1)}>
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <PageTitle className="truncate">{hostLabel(scan?.target)}</PageTitle>
        </div>
        {scan && (
          <span className={cn('rounded-full border px-2 py-0.5 text-xs', statusTone(scan.status))}>
            {statusLabel(scan.status, language)}
          </span>
        )}
      </PageHeader>

      {error && (
        <div className="flex items-center gap-3 rounded-2xl border border-red-400/20 bg-red-500/10 px-4 py-3 text-sm text-red-100">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      {scan && (
        <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard label={t('turns')} value={`${scan.turns} / ${scan.max_turns}`} />
          <StatCard label={t('duration')} value={scan.duration} />
          <StatCard label={t('tokens')} value={`${scan.tokens_in} in / ${scan.tokens_out} out`} />
          <StatCard label={t('cost')} value={scan.est_cost > 0 ? `$${scan.est_cost.toFixed(4)}` : '—'} />
        </section>
      )}

      <div className="flex flex-wrap gap-2">
        {isRunning && (
          <Button variant="outline" onClick={handleStop} className="rounded-2xl bg-transparent text-red-300 border-red-400/30 hover:bg-red-400/10">
            <Square className="mr-2 h-4 w-4" />
            {t('stop')}
          </Button>
        )}
        {!isRunning && (
          <Button variant="outline" onClick={handleDelete} className="rounded-2xl bg-transparent text-red-300 border-red-400/30 hover:bg-red-400/10">
            <Trash2 className="mr-2 h-4 w-4" />
            {t('delete')}
          </Button>
        )}
        <Button variant="outline" onClick={fetchDetail} className="rounded-2xl bg-transparent">
          <RefreshCcw className="mr-2 h-4 w-4" />
          {t('refresh')}
        </Button>
      </div>

      {scan && evidenceSummary && (
        <RunEvidenceWorkbench
          scan={scan}
          summary={evidenceSummary}
          view={evidenceView}
          onViewChange={setEvidenceView}
          onFocusStep={handleFocusStep}
          onAuditFilter={handleEvidenceAuditFilter}
          onExport={handleExportEvidence}
          language={language}
        />
      )}

      {focusStep && (
        <section className={cn(
          'rounded-[1.5rem] border p-4',
          focusEvents.length || focusApprovals.length
            ? 'border-primary/25 bg-primary/10'
            : 'border-amber-400/25 bg-amber-400/10',
        )}>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="font-mono text-[10px] uppercase tracking-wide text-muted-foreground">
                {text(language, '定位 step', 'Focused step')}
              </div>
              <div className="mt-1 truncate font-mono text-sm text-foreground">{focusStep}</div>
            </div>
            <div className="flex flex-wrap items-center gap-2 font-mono text-[10px]">
              <span className="rounded-full border border-white/10 bg-white/[0.04] px-2 py-1">
                {focusEvents.length} {text(language, '事件', 'events')}
              </span>
              <span className="rounded-full border border-white/10 bg-white/[0.04] px-2 py-1">
                {focusApprovals.length} {text(language, '审批', 'approvals')}
              </span>
              {selectedStepGroup && (
                <span className="rounded-full border border-primary/20 bg-primary/10 px-2 py-1 text-primary">
                  {text(language, '详情已打开', 'detail open')}
                </span>
              )}
            </div>
          </div>
          {!focusEvents.length && !focusApprovals.length && (
            <p className="mt-2 text-xs leading-5 text-amber-50/80">
              {text(language, '未在当前已加载事件中找到该 step，可能事件已被分页截断或运行尚未产生相关记录。', 'This step was not found in the currently loaded events; it may be paginated out or not emitted yet.')}
            </p>
          )}
        </section>
      )}

      <section className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-3 sm:p-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={auditSearch}
              onChange={(event) => setAuditSearch(event.target.value)}
              placeholder={text(language, '搜索 step / 事件 / 证据 / 审批...', 'Search step / event / evidence / approval...')}
              className="h-10 rounded-2xl border-white/10 bg-black/20 pl-9 pr-9 font-mono text-xs"
            />
            {auditSearch && (
              <button
                type="button"
                aria-label={text(language, '清空搜索', 'Clear search')}
                onClick={() => setAuditSearch('')}
                className="absolute right-2 top-1/2 grid h-7 w-7 -translate-y-1/2 place-items-center rounded-full text-muted-foreground transition hover:bg-white/10 hover:text-foreground"
              >
                <XCircle className="h-4 w-4" />
              </button>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {auditFilterOptions.map((option) => (
              <button
                key={option.value}
                type="button"
                onClick={() => setAuditFilter(option.value)}
                className={cn(
                  'h-9 rounded-full border px-3 font-mono text-[11px] transition',
                  auditFilter === option.value
                    ? 'border-primary/45 bg-primary/15 text-primary shadow-[0_0_22px_rgba(59,130,246,0.15)]'
                    : 'border-white/10 bg-white/[0.03] text-muted-foreground hover:border-white/20 hover:text-foreground',
                )}
              >
                {option.label}
              </button>
            ))}
          </div>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2 font-mono text-[10px] text-muted-foreground">
          <span className="rounded-full border border-white/10 bg-white/[0.03] px-2 py-1">
            {visibleStepGroups.length}/{stepGroups.length} steps
          </span>
          <span className="rounded-full border border-white/10 bg-white/[0.03] px-2 py-1">
            {visibleEvents.length}/{allEvents.length} events
          </span>
          {auditActive && (
            <button
              type="button"
              onClick={() => {
                setAuditSearch('')
                setAuditFilter('all')
              }}
              className="rounded-full border border-white/10 bg-white/[0.03] px-2 py-1 transition hover:border-primary/30 hover:text-foreground"
            >
              {text(language, '重置过滤', 'Reset filters')}
            </button>
          )}
        </div>
      </section>

      {stepGroups.length > 0 && (
        <section className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
            <SectionTitle icon={Activity} className="mb-0">{text(language, '执行链', 'Execution Chain')}</SectionTitle>
            <span className="rounded-full border border-white/10 bg-white/[0.04] px-2 py-1 font-mono text-[10px] text-muted-foreground">
              {visibleStepGroups.length}/{stepGroups.length} steps
            </span>
          </div>
          <div className="grid gap-2 lg:grid-cols-2">
            {visibleStepGroups.map((group) => (
              <StepChainCard
                key={group.stepID}
                group={group}
                focused={focusStep === group.stepID}
                language={language}
              />
            ))}
            {!visibleStepGroups.length && (
              <div className="col-span-full grid min-h-[140px] place-items-center rounded-2xl border border-dashed border-white/10 bg-black/10 text-sm text-muted-foreground">
                {text(language, '没有匹配的执行步骤', 'No matching execution steps')}
              </div>
            )}
          </div>
        </section>
      )}

      <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
        <section className="flex max-h-[600px] flex-col rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03]">
          <div className="border-b border-white/10 px-4 py-3">
            <SectionTitle icon={Activity} className="mb-0">{t('events')}</SectionTitle>
          </div>
          <div className="flex-1 space-y-1 overflow-y-auto p-3">
            {visibleEvents.map((ev, i) => {
              const focused = !!focusStep && eventMatchesStep(ev, focusStep)
              return (
                <EventRow
                  key={`${ev.timestamp}-${i}`}
                  event={ev}
                  focused={focused}
                  refCallback={(node) => {
                    if (focused && !eventRefs.current[focusStep]) eventRefs.current[focusStep] = node
                  }}
                />
              )
            })}
            {!visibleEvents.length && (
              <div className="grid min-h-[200px] place-items-center text-sm text-muted-foreground">
                {allEvents.length
                  ? text(language, '没有匹配的事件', 'No matching events')
                  : isRunning
                    ? text(language, '等待事件...', 'Waiting for events...')
                    : t('noData')}
              </div>
            )}
            <div ref={eventsEndRef} />
          </div>

          {isInteractive && (
            <form onSubmit={handleSend} className="border-t border-white/10 p-3">
              {isWaiting && (
                <div className="mb-2 flex items-center gap-2 rounded-xl border border-amber-400/20 bg-amber-400/10 px-3 py-2 text-xs text-amber-200">
                  <MessageSquare className="h-3.5 w-3.5" />
                  {text(language, 'Agent 等待你的输入...', 'Agent is waiting for your input...')}
                </div>
              )}
              <div className="flex gap-2">
                <Input
                  value={message}
                  onChange={(e) => setMessage(e.target.value)}
                  placeholder={text(language, '发送消息给 Agent...', 'Send message to Agent...')}
                  className="h-10 flex-1 rounded-xl font-mono text-sm"
                  disabled={!isRunning}
                />
                <Button type="submit" disabled={sending || !message.trim() || !isRunning} className="h-10 rounded-xl px-4">
                  <Send className="h-4 w-4" />
                </Button>
              </div>
            </form>
          )}
        </section>

        <aside className="space-y-4">
          {scan && scan.findings_detail && scan.findings_detail.length > 0 && (
            <section className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
              <SectionTitle icon={AlertTriangle} className="mb-3">{t('findings')} ({scan.findings_detail.length})</SectionTitle>
              <div className="space-y-2">
                {scan.findings_detail.map((f, i) => (
                  <div key={i} className={cn('rounded-2xl border p-3', severityTone(f.severity), focusStep && findingMatchesStep(f, focusStep) && 'ring-2 ring-primary/50')}>
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm font-medium">{f.title || f.category || 'Finding'}</span>
                      <span className="rounded-full border border-current/20 px-2 py-0.5 text-[10px] uppercase">
                        {f.severity}
                      </span>
                    </div>
                    {f.description && (
                      <p className="mt-2 text-xs leading-5 opacity-80">{f.description}</p>
                    )}
                    {f.url && (
                      <p className="mt-1 truncate font-mono text-[11px] opacity-60">{f.url}</p>
                    )}
                    {findingMatchesStep(f, focusStep) && (
                      <p className="mt-2 rounded-lg border border-primary/20 bg-primary/10 px-2 py-1 font-mono text-[10px] text-primary">
                        {focusStep}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            </section>
          )}

          {scan?.approvals && scan.approvals.length > 0 && (
            <section className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
              <SectionTitle icon={MessageSquare} className="mb-3">{text(language, '审批', 'Approvals')} ({scan.approvals.length})</SectionTitle>
              <div className="space-y-2">
                {scan.approvals.map((approval) => {
                  const focused = !!focusStep && approval.step_id === focusStep
                  return (
                    <div key={approval.id} className={cn('rounded-2xl border p-3 text-xs', focused ? 'border-amber-300/35 bg-amber-400/10 ring-2 ring-amber-300/30' : 'border-white/10 bg-white/[0.03]')}>
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate font-mono text-foreground">{approval.requested_action || approval.id}</span>
                        <span className="rounded-full border border-white/10 px-2 py-0.5 font-mono text-[10px] text-muted-foreground">{approval.status}</span>
                      </div>
                      {approval.step_id && <p className="mt-1 font-mono text-[10px] text-amber-100/80">{approval.step_id}</p>}
                      {approval.reason && <p className="mt-1 line-clamp-2 leading-5 text-muted-foreground">{approval.reason}</p>}
                    </div>
                  )
                })}
              </div>
            </section>
          )}

          {scan?.summary && (
            <section className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
              <SectionTitle icon={MessageSquare} className="mb-3">{text(language, '总结', 'Summary')}</SectionTitle>
              <p className="whitespace-pre-wrap text-sm leading-6 text-muted-foreground">{scan.summary}</p>
            </section>
          )}

          {scan && (
            <section className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
              <SectionTitle icon={Activity} className="mb-3">{text(language, '详情', 'Details')}</SectionTitle>
              <div className="space-y-2 font-mono text-xs text-muted-foreground">
                <DetailRow label="ID" value={scan.id} />
                <DetailRow label={t('provider')} value={scan.provider} />
                <DetailRow label={t('model')} value={scan.model || '—'} />
                <DetailRow label={t('target')} value={scan.target} />
                <DetailRow label={text(language, '模式', 'Mode')} value={scan.interactive ? 'interactive' : 'autonomous'} />
              </div>
            </section>
          )}
        </aside>
      </div>

      {selectedStepGroup && (
        <StepDetailDrawer
          group={selectedStepGroup}
          language={language}
          onClose={handleClearFocus}
        />
      )}
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-3">
      <div className="text-[11px] text-muted-foreground">{label}</div>
      <div className="mt-1 truncate font-mono text-sm">{value}</div>
    </div>
  )
}

type StepGroup = {
  stepID: string
  events: UIEvent[]
  approvals: ApprovalRequest[]
  findings: Finding[]
  artifacts: Artifact[]
  firstTimestamp: number
}

type RunEvidenceSummary = {
  leadFinding: Finding | null
  latestArtifact: Artifact | null
  pendingApprovals: ApprovalRequest[]
  highImpactFindings: Finding[]
  evidenceSteps: StepGroup[]
  reportText: string
}

function buildRunEvidenceSummary(scan: ScanDetail, stepGroups: StepGroup[]): RunEvidenceSummary {
  const findings = scan.findings_detail || []
  const artifacts = scan.artifacts || []
  const approvals = scan.approvals || []
  const highImpactFindings = findings.filter((finding) => {
    const severity = String(finding.severity || '').toLowerCase()
    return ['critical', 'high', 'medium'].includes(severity)
  })
  const leadFinding = highImpactFindings[0] || findings[0] || null
  const latestArtifact = [...artifacts].sort((a, b) => Date.parse(b.created_at || '') - Date.parse(a.created_at || ''))[0] || artifacts[0] || null
  const pendingApprovals = approvals.filter((approval) => approval.status === 'pending')
  const evidenceSteps = stepGroups
    .filter((group) => group.findings.length || group.artifacts.length || group.approvals.length)
    .sort((a, b) => (b.findings.length + b.artifacts.length + b.approvals.length) - (a.findings.length + a.artifacts.length + a.approvals.length))
    .slice(0, 6)
  return {
    leadFinding,
    latestArtifact,
    pendingApprovals,
    highImpactFindings,
    evidenceSteps,
    reportText: scan.summary || '',
  }
}

function exportRunEvidence(scan: ScanDetail, summary: RunEvidenceSummary, stepGroups: StepGroup[], format: 'md' | 'json') {
  const filenameBase = `cyberagent-evidence-${safeFilename(scan.id)}`
  const content = format === 'json'
    ? JSON.stringify(buildEvidenceExportPayload(scan, summary, stepGroups), null, 2)
    : buildEvidenceMarkdown(scan, summary, stepGroups)
  downloadTextFile(
    `${filenameBase}.${format === 'json' ? 'json' : 'md'}`,
    content,
    format === 'json' ? 'application/json;charset=utf-8' : 'text/markdown;charset=utf-8',
  )
}

function buildEvidenceExportPayload(scan: ScanDetail, summary: RunEvidenceSummary, stepGroups: StepGroup[]) {
  return {
    schema_version: 'cyberagent.run_evidence.v1',
    exported_at: new Date().toISOString(),
    run: {
      id: scan.id,
      target: scan.target,
      status: scan.status,
      started_at: scan.started_at,
      duration: scan.duration,
      provider: scan.provider,
      model: scan.model,
      counts: {
        findings: scan.findings_detail.length,
        artifacts: (scan.artifacts || []).length,
        approvals: (scan.approvals || []).length,
        pending_approvals: summary.pendingApprovals.length,
        events: scan.event_count || scan.events.length,
      },
    },
    lead_finding: summary.leadFinding,
    latest_artifact: summary.latestArtifact,
    pending_approvals: summary.pendingApprovals,
    findings: scan.findings_detail,
    artifacts: scan.artifacts || [],
    approvals: scan.approvals || [],
    evidence_steps: summary.evidenceSteps.map(stepGroupExport),
    step_groups: stepGroups.map(stepGroupExport),
    report: summary.reportText,
  }
}

function stepGroupExport(group: StepGroup) {
  return {
    step_id: group.stepID,
    events: group.events.length,
    approvals: group.approvals.map((approval) => approval.id),
    findings: group.findings.map((finding) => finding.id || finding.title || finding.category || 'finding'),
    artifacts: group.artifacts.map((artifact) => artifact.path || artifact.id),
  }
}

function buildEvidenceMarkdown(scan: ScanDetail, summary: RunEvidenceSummary, stepGroups: StepGroup[]) {
  const lines = [
    `# CyberAgent Run Evidence: ${scan.id}`,
    '',
    `- Target: ${scan.target || '—'}`,
    `- Status: ${scan.status}`,
    `- Started: ${scan.started_at || '—'}`,
    `- Duration: ${scan.duration || '—'}`,
    `- Findings: ${scan.findings_detail.length}`,
    `- Artifacts: ${(scan.artifacts || []).length}`,
    `- Approvals: ${(scan.approvals || []).length}`,
    `- Pending approvals: ${summary.pendingApprovals.length}`,
    '',
    '## Lead Finding',
    '',
    summary.leadFinding ? findingMarkdown(summary.leadFinding) : 'No finding.',
    '',
    '## Latest Artifact',
    '',
    summary.latestArtifact ? artifactMarkdown(summary.latestArtifact) : 'No artifact.',
    '',
    '## Pending Approvals',
    '',
    ...markdownList(summary.pendingApprovals.map(approvalMarkdown), 'No pending approval.'),
    '',
    '## Evidence Steps',
    '',
    ...markdownList(summary.evidenceSteps.map((group) => `- \`${group.stepID}\`: ${group.findings.length} findings, ${group.artifacts.length} artifacts, ${group.approvals.length} approvals, ${group.events.length} events`), 'No evidence steps.'),
    '',
    '## Findings',
    '',
    ...markdownList(scan.findings_detail.map(findingMarkdown), 'No findings.'),
    '',
    '## Artifacts',
    '',
    ...markdownList((scan.artifacts || []).map(artifactMarkdown), 'No artifacts.'),
    '',
    '## Step Groups',
    '',
    ...markdownList(stepGroups.map((group) => `- \`${group.stepID}\`: ${group.events.length} events, ${group.findings.length} findings, ${group.artifacts.length} artifacts, ${group.approvals.length} approvals`), 'No step groups.'),
    '',
    '## Report',
    '',
    summary.reportText || 'No report summary.',
    '',
  ]
  return lines.join('\n')
}

function markdownList(items: string[], empty: string) {
  return items.length ? items : [empty]
}

function findingMarkdown(finding: Finding) {
  const stepID = findingStepID(finding)
  return [
    `### ${finding.title || finding.category || finding.id || 'Finding'}`,
    '',
    `- Severity: ${finding.severity || '—'}`,
    `- Category: ${finding.category || '—'}`,
    `- URL: ${finding.url || '—'}`,
    `- Step: ${stepID || '—'}`,
    finding.description ? `- Description: ${finding.description}` : '',
    finding.remediation ? `- Remediation: ${finding.remediation}` : '',
  ].filter(Boolean).join('\n')
}

function artifactMarkdown(artifact: Artifact) {
  return [
    `- Path: \`${artifact.path}\``,
    `  - ID: ${artifact.id || '—'}`,
    `  - Step: ${artifact.step_id || '—'}`,
    `  - Type: ${artifact.type || '—'}`,
    `  - MIME: ${artifact.mime_type || '—'}`,
    `  - Size: ${typeof artifact.size_bytes === 'number' ? `${artifact.size_bytes}B` : '—'}`,
    `  - SHA256: ${artifact.sha256 || '—'}`,
  ].join('\n')
}

function approvalMarkdown(approval: ApprovalRequest) {
  return `- ${approval.requested_action || approval.id} · status=${approval.status}${approval.step_id ? ` · step=${approval.step_id}` : ''}${approval.reason ? ` · reason=${approval.reason}` : ''}`
}

function downloadTextFile(filename: string, content: string, mime: string) {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

function safeFilename(value: string) {
  return (value || 'run').replace(/[^a-zA-Z0-9._-]+/g, '-').slice(0, 80)
}

function RunEvidenceWorkbench({
  scan,
  summary,
  view,
  onViewChange,
  onFocusStep,
  onAuditFilter,
  onExport,
  language,
}: {
  scan: ScanDetail
  summary: RunEvidenceSummary
  view: EvidenceView
  onViewChange: (view: EvidenceView) => void
  onFocusStep: (stepID: string) => void
  onAuditFilter: (filter: AuditFilter) => void
  onExport: (format: 'md' | 'json') => void
  language: 'zh' | 'en'
}) {
  const tabs = buildEvidenceTabs(scan, language)
  const hasEvidence = scan.findings_detail.length > 0 || (scan.artifacts || []).length > 0 || (scan.approvals || []).length > 0 || !!summary.reportText

  return (
    <section className="overflow-hidden rounded-[1.75rem] border border-primary/15 bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.16),transparent_34%),rgba(255,255,255,0.035)] shadow-[0_18px_70px_rgba(0,0,0,0.22)]">
      <div className="border-b border-white/10 p-4 sm:p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2 font-mono text-[10px] uppercase tracking-[0.22em] text-primary">
              <Database className="h-3.5 w-3.5" />
              {text(language, 'Run Evidence 工作台', 'Run Evidence Workbench')}
            </div>
            <h2 className="mt-2 text-lg font-semibold text-foreground">
              {text(language, '证据、发现、审批与报告集中复盘', 'Evidence, findings, approvals and report in one place')}
            </h2>
            <p className="mt-1 max-w-3xl text-xs leading-5 text-muted-foreground">
              {text(language, '把 Agent 运行产物整理成可快速决策的证据面板，方便从发现跳回 step、从审批追踪风险、从 artifact 回看原始证据。', 'A decision-focused evidence board for jumping from findings to steps, tracking approval risk, and reviewing raw artifacts.')}
            </p>
          </div>
          <div className="grid min-w-[260px] grid-cols-3 gap-2">
            <EvidenceKpi label={text(language, '发现', 'Findings')} value={scan.findings_detail.length} tone={scan.findings_detail.length ? 'finding' : 'muted'} />
            <EvidenceKpi label={text(language, '证据', 'Artifacts')} value={(scan.artifacts || []).length} tone={(scan.artifacts || []).length ? 'artifact' : 'muted'} />
            <EvidenceKpi label={text(language, '待审批', 'Pending')} value={summary.pendingApprovals.length} tone={summary.pendingApprovals.length ? 'approval' : 'muted'} />
          </div>
        </div>

        <div className="mt-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex flex-wrap gap-2">
            {tabs.map((tab) => (
              <button
                key={tab.value}
                type="button"
                onClick={() => onViewChange(tab.value)}
                className={cn(
                  'rounded-full border px-3 py-1.5 font-mono text-[11px] transition',
                  view === tab.value
                    ? 'border-primary/45 bg-primary/15 text-primary'
                    : 'border-white/10 bg-white/[0.03] text-muted-foreground hover:border-white/20 hover:text-foreground',
                )}
              >
                {tab.label}
              </button>
            ))}
          </div>
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => onExport('md')}
              className="inline-flex items-center gap-2 rounded-full border border-cyan-300/20 bg-cyan-300/10 px-3 py-1.5 font-mono text-[11px] text-cyan-100 transition hover:bg-cyan-300/15"
            >
              <Download className="h-3.5 w-3.5" />
              {text(language, '导出 MD', 'Export MD')}
            </button>
            <button
              type="button"
              onClick={() => onExport('json')}
              className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.03] px-3 py-1.5 font-mono text-[11px] text-muted-foreground transition hover:border-white/20 hover:text-foreground"
            >
              <Download className="h-3.5 w-3.5" />
              JSON
            </button>
          </div>
        </div>
      </div>

      <div className="p-4 sm:p-5">
        {!hasEvidence ? (
          <div className="grid min-h-[140px] place-items-center rounded-2xl border border-dashed border-white/10 bg-black/10 text-sm text-muted-foreground">
            {text(language, '当前运行还没有可展示的证据产物', 'No evidence artifacts are available for this run yet')}
          </div>
        ) : view === 'overview' ? (
          <EvidenceOverview
            summary={summary}
            language={language}
            onFocusStep={onFocusStep}
            onAuditFilter={onAuditFilter}
          />
        ) : view === 'findings' ? (
          <EvidenceFindingList findings={scan.findings_detail} language={language} onFocusStep={onFocusStep} />
        ) : view === 'artifacts' ? (
          <EvidenceArtifactList artifacts={scan.artifacts || []} language={language} onFocusStep={onFocusStep} />
        ) : view === 'approvals' ? (
          <EvidenceApprovalList approvals={scan.approvals || []} language={language} onFocusStep={onFocusStep} />
        ) : (
          <EvidenceReport summary={summary} language={language} />
        )}
      </div>
    </section>
  )
}

function buildEvidenceTabs(scan: ScanDetail, language: 'zh' | 'en'): Array<{ value: EvidenceView; label: string }> {
  return [
    { value: 'overview', label: text(language, '总览', 'Overview') },
    { value: 'findings', label: `${text(language, '发现', 'Findings')} · ${scan.findings_detail.length}` },
    { value: 'artifacts', label: `${text(language, '证据', 'Artifacts')} · ${(scan.artifacts || []).length}` },
    { value: 'approvals', label: `${text(language, '审批', 'Approvals')} · ${(scan.approvals || []).length}` },
    { value: 'report', label: text(language, '报告', 'Report') },
  ]
}

function EvidenceKpi({ label, value, tone = 'muted' }: { label: string; value: number; tone?: 'muted' | 'approval' | 'finding' | 'artifact' }) {
  const toneClass = tone === 'approval'
    ? 'border-amber-400/20 bg-amber-400/10 text-amber-100'
    : tone === 'finding'
      ? 'border-red-400/20 bg-red-400/10 text-red-100'
      : tone === 'artifact'
        ? 'border-cyan-400/20 bg-cyan-400/10 text-cyan-100'
        : 'border-white/10 bg-white/[0.03] text-muted-foreground'
  return (
    <div className={cn('rounded-2xl border p-3', toneClass)}>
      <div className="text-[10px]">{label}</div>
      <div className="mt-1 font-mono text-xl text-foreground">{value}</div>
    </div>
  )
}

function EvidenceOverview({
  summary,
  language,
  onFocusStep,
  onAuditFilter,
}: {
  summary: RunEvidenceSummary
  language: 'zh' | 'en'
  onFocusStep: (stepID: string) => void
  onAuditFilter: (filter: AuditFilter) => void
}) {
  return (
    <div className="grid gap-4 xl:grid-cols-[1fr_1fr]">
      <div className="space-y-3">
        <EvidenceLeadCard
          title={text(language, '首要发现', 'Lead finding')}
          empty={text(language, '暂无发现', 'No finding yet')}
          tone="finding"
          actionLabel={text(language, '筛选发现', 'Filter findings')}
          onAction={() => onAuditFilter('finding')}
        >
          {summary.leadFinding && <FindingEvidenceBody finding={summary.leadFinding} onFocusStep={onFocusStep} />}
        </EvidenceLeadCard>
        <EvidenceLeadCard
          title={text(language, '最新证据', 'Latest artifact')}
          empty={text(language, '暂无证据文件', 'No artifact yet')}
          tone="artifact"
          actionLabel={text(language, '筛选证据', 'Filter artifacts')}
          onAction={() => onAuditFilter('artifact')}
        >
          {summary.latestArtifact && <ArtifactEvidenceBody artifact={summary.latestArtifact} onFocusStep={onFocusStep} />}
        </EvidenceLeadCard>
      </div>
      <div className="space-y-3">
        <EvidenceLeadCard
          title={text(language, '审批风险', 'Approval risk')}
          empty={text(language, '无待审批项', 'No pending approvals')}
          tone="approval"
          actionLabel={text(language, '筛选审批', 'Filter approvals')}
          onAction={() => onAuditFilter('approval')}
        >
          {summary.pendingApprovals[0] && <ApprovalEvidenceBody approval={summary.pendingApprovals[0]} onFocusStep={onFocusStep} />}
        </EvidenceLeadCard>
        <div className="rounded-2xl border border-white/10 bg-black/10 p-3">
          <div className="mb-3 flex items-center justify-between">
            <StepPanelTitle icon={Activity}>{text(language, '关键步骤', 'Evidence steps')}</StepPanelTitle>
            <span className="font-mono text-[10px] text-muted-foreground">{summary.evidenceSteps.length} steps</span>
          </div>
          <div className="space-y-2">
            {summary.evidenceSteps.map((group) => (
              <button
                key={group.stepID}
                type="button"
                onClick={() => onFocusStep(group.stepID)}
                className="flex w-full items-center gap-3 rounded-xl border border-white/10 bg-white/[0.03] px-3 py-2 text-left transition hover:border-primary/35 hover:bg-primary/10"
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate font-mono text-xs text-foreground">{group.stepID}</div>
                  <div className="mt-1 font-mono text-[10px] text-muted-foreground">
                    {group.findings.length} findings · {group.artifacts.length} artifacts · {group.approvals.length} approvals
                  </div>
                </div>
                <ChevronRight className="h-4 w-4 text-muted-foreground" />
              </button>
            ))}
            {!summary.evidenceSteps.length && <StepEmpty>{text(language, '暂无关键证据步骤', 'No evidence steps yet')}</StepEmpty>}
          </div>
        </div>
      </div>
    </div>
  )
}

function EvidenceLeadCard({
  title,
  empty,
  tone,
  actionLabel,
  onAction,
  children,
}: {
  title: string
  empty: string
  tone: 'approval' | 'finding' | 'artifact'
  actionLabel: string
  onAction: () => void
  children?: React.ReactNode
}) {
  const toneClass = tone === 'approval'
    ? 'border-amber-400/20 bg-amber-400/10'
    : tone === 'finding'
      ? 'border-red-400/20 bg-red-400/10'
      : 'border-cyan-400/20 bg-cyan-400/10'
  return (
    <div className={cn('rounded-2xl border p-3', toneClass)}>
      <div className="mb-3 flex items-center justify-between gap-3">
        <div className="text-xs font-medium text-foreground">{title}</div>
        <button type="button" onClick={onAction} className="rounded-full border border-white/10 bg-black/15 px-2 py-1 font-mono text-[10px] text-muted-foreground transition hover:text-foreground">
          {actionLabel}
        </button>
      </div>
      {children || <StepEmpty>{empty}</StepEmpty>}
    </div>
  )
}

function EvidenceFindingList({ findings, language, onFocusStep }: { findings: Finding[]; language: 'zh' | 'en'; onFocusStep: (stepID: string) => void }) {
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      {findings.map((finding, index) => (
        <StepFindingCard key={finding.id || `${finding.title}-${index}`} finding={finding} onFocusStep={onFocusStep} />
      ))}
      {!findings.length && <StepEmpty>{text(language, '暂无发现', 'No findings')}</StepEmpty>}
    </div>
  )
}

function EvidenceArtifactList({ artifacts, language, onFocusStep }: { artifacts: Artifact[]; language: 'zh' | 'en'; onFocusStep: (stepID: string) => void }) {
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      {artifacts.map((artifact) => (
        <StepArtifactCard key={artifact.id || artifact.path} artifact={artifact} onFocusStep={onFocusStep} />
      ))}
      {!artifacts.length && <StepEmpty>{text(language, '暂无证据文件', 'No artifacts')}</StepEmpty>}
    </div>
  )
}

function EvidenceApprovalList({ approvals, language, onFocusStep }: { approvals: ApprovalRequest[]; language: 'zh' | 'en'; onFocusStep: (stepID: string) => void }) {
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      {approvals.map((approval) => (
        <StepApprovalCard key={approval.id} approval={approval} onFocusStep={onFocusStep} />
      ))}
      {!approvals.length && <StepEmpty>{text(language, '暂无审批请求', 'No approvals')}</StepEmpty>}
    </div>
  )
}

function EvidenceReport({ summary, language }: { summary: RunEvidenceSummary; language: 'zh' | 'en' }) {
  return summary.reportText ? (
    <pre className="max-h-[420px] overflow-auto whitespace-pre-wrap rounded-2xl border border-white/10 bg-black/20 p-4 text-xs leading-5 text-muted-foreground">
      {summary.reportText}
    </pre>
  ) : (
    <StepEmpty>{text(language, '暂无报告摘要', 'No report summary')}</StepEmpty>
  )
}

function FindingEvidenceBody({ finding, onFocusStep }: { finding: Finding; onFocusStep: (stepID: string) => void }) {
  return <StepFindingCard finding={finding} onFocusStep={onFocusStep} />
}

function ArtifactEvidenceBody({ artifact, onFocusStep }: { artifact: Artifact; onFocusStep: (stepID: string) => void }) {
  return <StepArtifactCard artifact={artifact} onFocusStep={onFocusStep} />
}

function ApprovalEvidenceBody({ approval, onFocusStep }: { approval: ApprovalRequest; onFocusStep: (stepID: string) => void }) {
  return <StepApprovalCard approval={approval} onFocusStep={onFocusStep} />
}

function buildStepGroups(events: UIEvent[], scan: ScanDetail | null): StepGroup[] {
  const groups = new Map<string, StepGroup>()
  const ensure = (stepID: string) => {
    const id = stepID || 'run'
    const existing = groups.get(id)
    if (existing) return existing
    const created: StepGroup = { stepID: id, events: [], approvals: [], findings: [], artifacts: [], firstTimestamp: Number.POSITIVE_INFINITY }
    groups.set(id, created)
    return created
  }
  for (const event of events) {
    const stepID = event.step_id || event.tool || stepFromData(event.data) || 'run'
    const group = ensure(stepID)
    group.events.push(event)
    group.firstTimestamp = Math.min(group.firstTimestamp, event.timestamp || Number.POSITIVE_INFINITY)
  }
  for (const approval of scan?.approvals || []) {
    if (!approval.step_id) continue
    ensure(approval.step_id).approvals.push(approval)
  }
  for (const finding of scan?.findings_detail || []) {
    const stepID = findingStepID(finding)
    if (!stepID) continue
    ensure(stepID).findings.push(finding)
  }
  for (const artifact of scan?.artifacts || []) {
    if (!artifact.step_id) continue
    ensure(artifact.step_id).artifacts.push(artifact)
  }
  return [...groups.values()].sort((a, b) => {
    const aTime = Number.isFinite(a.firstTimestamp) ? a.firstTimestamp : Number.MAX_SAFE_INTEGER
    const bTime = Number.isFinite(b.firstTimestamp) ? b.firstTimestamp : Number.MAX_SAFE_INTEGER
    if (aTime !== bTime) return aTime - bTime
    return a.stepID.localeCompare(b.stepID)
  })
}

function stepFromData(data?: Record<string, unknown>) {
  if (!data) return ''
  for (const [key, value] of Object.entries(data)) {
    if (key.toLowerCase().includes('step') && typeof value === 'string') return value
  }
  return ''
}

function findingStepID(finding: Finding) {
  const source = finding.source
  if (source?.step_id) return source.step_id
  const metadataStep = finding.metadata?.step_id
  return typeof metadataStep === 'string' ? metadataStep : ''
}

function StepChainCard({ group, focused, language }: { group: StepGroup; focused: boolean; language: 'zh' | 'en' }) {
  const failed = group.events.some((event) => event.severity === 'error' || event.type.includes('failed'))
  const warning = group.approvals.some((approval) => approval.status === 'pending') || group.events.some((event) => event.severity === 'warn')
  const succeeded = group.events.some((event) => event.type.includes('finished') || event.type === 'done')
  const tone = failed
    ? 'border-red-400/30 bg-red-400/10'
    : warning
      ? 'border-amber-400/25 bg-amber-400/10'
      : succeeded
        ? 'border-emerald-400/20 bg-emerald-400/10'
        : 'border-white/10 bg-black/10'

  return (
    <a
      href={`?step=${encodeURIComponent(group.stepID)}`}
      className={cn('rounded-2xl border p-3 transition hover:border-primary/35', tone, focused && 'ring-2 ring-primary/50')}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-mono text-xs text-foreground">{group.stepID}</span>
        <span className="rounded-full border border-white/10 bg-white/[0.04] px-2 py-0.5 font-mono text-[10px] text-muted-foreground">
          {group.events.length} evt
        </span>
      </div>
      <div className="mt-3 grid grid-cols-4 gap-1.5">
        <ChainMetric label="ev" value={group.events.length} />
        <ChainMetric label={text(language, '审', 'ap')} value={group.approvals.length} tone={group.approvals.some((item) => item.status === 'pending') ? 'approval' : 'muted'} />
        <ChainMetric label={text(language, '发', 'fd')} value={group.findings.length} tone={group.findings.length ? 'finding' : 'muted'} />
        <ChainMetric label="art" value={group.artifacts.length} tone={group.artifacts.length ? 'artifact' : 'muted'} />
      </div>
      <div className="mt-2 space-y-1">
        {group.events.slice(-2).map((event, idx) => (
          <p key={`${event.timestamp}-${idx}`} className="line-clamp-1 text-[11px] leading-4 text-muted-foreground">
            <span className="font-mono text-primary">{event.type}</span>
            {event.message ? ` · ${event.message}` : ''}
          </p>
        ))}
        {group.findings.slice(0, 1).map((finding) => (
          <p key={finding.id || finding.title} className="line-clamp-1 text-[11px] leading-4 text-amber-50/85">
            {finding.title || finding.category}
          </p>
        ))}
      </div>
    </a>
  )
}

function ChainMetric({ label, value, tone = 'muted' }: { label: string; value: number; tone?: 'muted' | 'approval' | 'finding' | 'artifact' }) {
  const toneClass = tone === 'approval'
    ? 'border-amber-400/20 bg-amber-400/10 text-amber-100'
    : tone === 'finding'
      ? 'border-red-400/20 bg-red-400/10 text-red-100'
      : tone === 'artifact'
        ? 'border-cyan-400/20 bg-cyan-400/10 text-cyan-100'
        : 'border-white/10 bg-white/[0.03] text-muted-foreground'
  return (
    <span className={cn('rounded-lg border px-1.5 py-1 text-center font-mono text-[10px]', toneClass)}>
      {value}<span className="ml-1 opacity-70">{label}</span>
    </span>
  )
}

function StepDetailDrawer({ group, language, onClose }: { group: StepGroup; language: 'zh' | 'en'; onClose: () => void }) {
  const failed = group.events.some(isErrorEvent)
  const pendingApproval = group.approvals.some((approval) => approval.status === 'pending')
  const tone = failed
    ? 'border-red-400/25 bg-red-500/10 text-red-100'
    : pendingApproval
      ? 'border-amber-400/25 bg-amber-400/10 text-amber-100'
      : 'border-primary/20 bg-primary/10 text-primary'

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/55 p-3 backdrop-blur-sm sm:items-center">
      <section
        role="dialog"
        aria-modal="true"
        aria-label={text(language, 'Step 详情', 'Step detail')}
        className="flex max-h-[88dvh] w-full max-w-5xl flex-col overflow-hidden rounded-[1.75rem] border border-white/10 bg-[#080d18]/95 shadow-2xl shadow-black/40"
      >
        <header className="border-b border-white/10 bg-white/[0.03] p-4 sm:p-5">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className={cn('rounded-full border px-2 py-1 font-mono text-[10px]', tone)}>
                  {failed ? text(language, '异常步骤', 'Error step') : pendingApproval ? text(language, '等待审批', 'Approval required') : text(language, '执行步骤', 'Execution step')}
                </span>
                <span className="rounded-full border border-white/10 bg-white/[0.04] px-2 py-1 font-mono text-[10px] text-muted-foreground">
                  {group.events.length} events
                </span>
              </div>
              <h2 className="mt-3 truncate font-mono text-base font-semibold text-foreground">{group.stepID}</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                {text(language, '集中查看该步骤的事件、审批、发现与证据，便于复盘 Agent 的执行链。', 'Review this step’s events, approvals, findings and artifacts in one place.')}
              </p>
            </div>
            <button
              type="button"
              onClick={onClose}
              aria-label={text(language, '关闭', 'Close')}
              className="grid h-10 w-10 shrink-0 place-items-center rounded-2xl border border-white/10 bg-white/[0.04] text-muted-foreground transition hover:border-white/20 hover:text-foreground"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4">
            <StepDetailMetric icon={Activity} label={text(language, '事件', 'Events')} value={group.events.length} />
            <StepDetailMetric icon={ShieldCheck} label={text(language, '审批', 'Approvals')} value={group.approvals.length} tone={pendingApproval ? 'approval' : 'muted'} />
            <StepDetailMetric icon={AlertTriangle} label={text(language, '发现', 'Findings')} value={group.findings.length} tone={group.findings.length ? 'finding' : 'muted'} />
            <StepDetailMetric icon={FileText} label={text(language, '证据', 'Artifacts')} value={group.artifacts.length} tone={group.artifacts.length ? 'artifact' : 'muted'} />
          </div>
        </header>

        <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto p-4 lg:grid-cols-[1.1fr_0.9fr]">
          <section className="min-w-0 space-y-3">
            <StepPanelTitle icon={Activity}>{text(language, '事件流', 'Event stream')}</StepPanelTitle>
            <div className="space-y-2">
              {group.events.map((event, index) => (
                <EventRow key={`${event.timestamp}-${index}`} event={event} />
              ))}
              {!group.events.length && <StepEmpty>{text(language, '该 step 暂无事件', 'No events for this step')}</StepEmpty>}
            </div>
          </section>

          <section className="min-w-0 space-y-4">
            <div>
              <StepPanelTitle icon={ShieldCheck}>{text(language, '审批', 'Approvals')}</StepPanelTitle>
              <div className="mt-2 space-y-2">
                {group.approvals.map((approval) => (
                  <StepApprovalCard key={approval.id} approval={approval} />
                ))}
                {!group.approvals.length && <StepEmpty>{text(language, '无审批请求', 'No approvals')}</StepEmpty>}
              </div>
            </div>

            <div>
              <StepPanelTitle icon={AlertTriangle}>{text(language, '发现', 'Findings')}</StepPanelTitle>
              <div className="mt-2 space-y-2">
                {group.findings.map((finding, index) => (
                  <StepFindingCard key={finding.id || `${finding.title}-${index}`} finding={finding} />
                ))}
                {!group.findings.length && <StepEmpty>{text(language, '无安全发现', 'No findings')}</StepEmpty>}
              </div>
            </div>

            <div>
              <StepPanelTitle icon={FileText}>{text(language, '证据文件', 'Artifacts')}</StepPanelTitle>
              <div className="mt-2 space-y-2">
                {group.artifacts.map((artifact) => (
                  <StepArtifactCard key={artifact.id || artifact.path} artifact={artifact} />
                ))}
                {!group.artifacts.length && <StepEmpty>{text(language, '无证据文件', 'No artifacts')}</StepEmpty>}
              </div>
            </div>
          </section>
        </div>
      </section>
    </div>
  )
}

function StepDetailMetric({ icon: Icon, label, value, tone = 'muted' }: { icon: typeof Activity; label: string; value: number; tone?: 'muted' | 'approval' | 'finding' | 'artifact' }) {
  const toneClass = tone === 'approval'
    ? 'text-amber-100'
    : tone === 'finding'
      ? 'text-red-100'
      : tone === 'artifact'
        ? 'text-cyan-100'
        : 'text-muted-foreground'
  return (
    <div className="rounded-2xl border border-white/10 bg-black/15 p-3">
      <div className={cn('flex items-center gap-2 text-[11px]', toneClass)}>
        <Icon className="h-3.5 w-3.5" />
        {label}
      </div>
      <div className="mt-1 font-mono text-lg text-foreground">{value}</div>
    </div>
  )
}

function StepPanelTitle({ icon: Icon, children }: { icon: typeof Activity; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
      <Icon className="h-3.5 w-3.5" />
      {children}
    </div>
  )
}

function StepEmpty({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.02] px-3 py-4 text-center text-xs text-muted-foreground">
      {children}
    </div>
  )
}

function StepApprovalCard({ approval, onFocusStep }: { approval: ApprovalRequest; onFocusStep?: (stepID: string) => void }) {
  return (
    <div className={cn('rounded-2xl border p-3 text-xs', approval.status === 'pending' ? 'border-amber-300/30 bg-amber-400/10' : 'border-white/10 bg-white/[0.03]')}>
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-mono text-foreground">{approval.requested_action || approval.id}</span>
        <span className="rounded-full border border-white/10 px-2 py-0.5 font-mono text-[10px] text-muted-foreground">{approval.status}</span>
      </div>
      {approval.reason && <p className="mt-2 line-clamp-3 leading-5 text-muted-foreground">{approval.reason}</p>}
      <div className="mt-2 flex flex-wrap items-center gap-2 font-mono text-[10px] text-amber-100/80">
        {approval.policy_decision_id && <span className="truncate">{approval.policy_decision_id}</span>}
        {approval.step_id && (
          <button
            type="button"
            onClick={() => onFocusStep?.(approval.step_id || '')}
            className="rounded-full border border-amber-300/20 px-2 py-0.5 transition hover:bg-amber-300/10"
          >
            {approval.step_id}
          </button>
        )}
      </div>
    </div>
  )
}

function StepFindingCard({ finding, onFocusStep }: { finding: Finding; onFocusStep?: (stepID: string) => void }) {
  const stepID = findingStepID(finding)
  return (
    <div className={cn('rounded-2xl border p-3', severityTone(finding.severity))}>
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm font-medium">{finding.title || finding.category || 'Finding'}</span>
        {finding.severity && (
          <span className="rounded-full border border-current/20 px-2 py-0.5 text-[10px] uppercase">{finding.severity}</span>
        )}
      </div>
      {finding.description && <p className="mt-2 line-clamp-4 text-xs leading-5 opacity-85">{finding.description}</p>}
      {finding.remediation && <p className="mt-2 line-clamp-3 text-xs leading-5 opacity-70">{finding.remediation}</p>}
      {stepID && (
        <button
          type="button"
          onClick={() => onFocusStep?.(stepID)}
          className="mt-2 rounded-full border border-current/20 px-2 py-0.5 font-mono text-[10px] opacity-80 transition hover:bg-white/10 hover:opacity-100"
        >
          {stepID}
        </button>
      )}
    </div>
  )
}

function StepArtifactCard({ artifact, onFocusStep }: { artifact: Artifact; onFocusStep?: (stepID: string) => void }) {
  return (
    <div className="rounded-2xl border border-cyan-400/20 bg-cyan-400/10 p-3 text-xs">
      <div className="flex items-center gap-2">
        <ListChecks className="h-3.5 w-3.5 text-cyan-100" />
        <span className="truncate font-mono text-cyan-50">{artifact.path}</span>
      </div>
      <div className="mt-2 flex flex-wrap gap-2 font-mono text-[10px] text-cyan-50/70">
        {artifact.type && <span>{artifact.type}</span>}
        {artifact.mime_type && <span>{artifact.mime_type}</span>}
        {typeof artifact.size_bytes === 'number' && <span>{artifact.size_bytes}B</span>}
        {artifact.redacted && <span>redacted</span>}
      </div>
      {artifact.step_id && (
        <button
          type="button"
          onClick={() => onFocusStep?.(artifact.step_id || '')}
          className="mt-2 rounded-full border border-cyan-300/20 px-2 py-0.5 font-mono text-[10px] text-cyan-50/80 transition hover:bg-cyan-300/10"
        >
          {artifact.step_id}
        </button>
      )}
    </div>
  )
}

function buildAuditFilterOptions(language: 'zh' | 'en'): Array<{ value: AuditFilter; label: string }> {
  return [
    { value: 'all', label: text(language, '全部', 'All') },
    { value: 'error', label: text(language, '错误', 'Errors') },
    { value: 'warning', label: text(language, '警告', 'Warnings') },
    { value: 'finding', label: text(language, '发现', 'Findings') },
    { value: 'artifact', label: text(language, '证据', 'Artifacts') },
    { value: 'approval', label: text(language, '审批', 'Approvals') },
  ]
}

function stepGroupMatchesAudit(group: StepGroup, query: string, filter: AuditFilter) {
  if (!auditTypeMatchesGroup(group, filter)) return false
  return auditTextMatches(stepGroupAuditText(group), query)
}

function eventMatchesAudit(event: UIEvent, query: string, filter: AuditFilter) {
  if (!auditTypeMatchesEvent(event, filter)) return false
  return auditTextMatches(eventAuditText(event), query)
}

function auditTypeMatchesGroup(group: StepGroup, filter: AuditFilter) {
  if (filter === 'all') return true
  if (filter === 'error') return group.events.some(isErrorEvent)
  if (filter === 'warning') {
    return group.events.some(isWarningEvent) || group.approvals.some((approval) => approval.status === 'pending')
  }
  if (filter === 'finding') return group.findings.length > 0
  if (filter === 'artifact') return group.artifacts.length > 0
  if (filter === 'approval') return group.approvals.length > 0
  return true
}

function auditTypeMatchesEvent(event: UIEvent, filter: AuditFilter) {
  if (filter === 'all') return true
  if (filter === 'error') return isErrorEvent(event)
  if (filter === 'warning') return isWarningEvent(event)
  if (filter === 'finding') return event.type.toLowerCase().includes('finding')
  if (filter === 'artifact') {
    const haystack = eventAuditText(event)
    return event.type.toLowerCase().includes('artifact') || haystack.includes('artifact') || haystack.includes('evidence')
  }
  if (filter === 'approval') {
    const haystack = eventAuditText(event)
    return event.type.toLowerCase().includes('approval') || haystack.includes('approval') || haystack.includes('pending')
  }
  return true
}

function isErrorEvent(event: UIEvent) {
  const type = event.type.toLowerCase()
  const severity = event.severity.toLowerCase()
  return severity === 'error' || type.includes('failed') || type.includes('error') || type.includes('cancel')
}

function isWarningEvent(event: UIEvent) {
  const type = event.type.toLowerCase()
  const severity = event.severity.toLowerCase()
  return severity === 'warn' || severity === 'warning' || type.includes('warning') || type.includes('approval')
}

function auditTextMatches(haystack: string, query: string) {
  const trimmed = query.trim().toLowerCase()
  if (!trimmed) return true
  return trimmed.split(/\s+/).every((part) => haystack.includes(part))
}

function stepGroupAuditText(group: StepGroup) {
  return [
    group.stepID,
    ...group.events.map(eventAuditText),
    ...group.approvals.map(approvalAuditText),
    ...group.findings.map(findingAuditText),
    ...group.artifacts.map(artifactAuditText),
  ].join('\n').toLowerCase()
}

function eventAuditText(event: UIEvent) {
  return [
    event.id,
    event.job_id,
    event.step_id,
    event.type,
    event.tool,
    event.severity,
    event.message,
    stringifyAuditValue(event.data),
  ].join(' ').toLowerCase()
}

function approvalAuditText(approval: ApprovalRequest) {
  return [
    approval.id,
    approval.job_id,
    approval.step_id,
    approval.policy_decision_id,
    approval.status,
    approval.reason,
    approval.requested_action,
    approval.options?.join(' '),
    stringifyAuditValue(approval.risk_summary),
    stringifyAuditValue(approval.resolution),
  ].join(' ').toLowerCase()
}

function findingAuditText(finding: Finding) {
  return [
    finding.id,
    finding.job_id,
    finding.title,
    finding.severity,
    finding.confidence,
    finding.category,
    finding.url,
    finding.affected_targets?.join(' '),
    finding.evidence_ids?.join(' '),
    finding.description,
    finding.evidence,
    finding.impact,
    finding.remediation,
    finding.source?.tool,
    finding.source?.step_id,
    stringifyAuditValue(finding.metadata),
  ].join(' ').toLowerCase()
}

function artifactAuditText(artifact: Artifact) {
  return [
    artifact.id,
    artifact.job_id,
    artifact.step_id,
    artifact.type,
    artifact.path,
    artifact.sha256,
    artifact.mime_type,
    artifact.created_at,
    artifact.redacted ? 'redacted' : '',
  ].join(' ').toLowerCase()
}

function stringifyAuditValue(value: unknown) {
  if (value === undefined || value === null) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function eventMatchesStep(event: UIEvent, step: string) {
  if (!step) return false
  if (event.step_id === step || event.tool === step) return true
  const data = event.data || {}
  return Object.values(data).some((value) => String(value) === step)
}

function findingMatchesStep(finding: ScanDetail['findings_detail'][number], step: string) {
  if (!step) return false
  if (findingStepID(finding) === step) return true
  const evidenceIds = finding.evidence_ids
  return Array.isArray(evidenceIds) && evidenceIds.some((id) => String(id) === step)
}

function EventRow({ event, focused = false, refCallback }: { event: UIEvent; focused?: boolean; refCallback?: (node: HTMLDivElement | null) => void }) {
  const typeColor = event.type === 'finding'
    ? 'text-amber-300'
    : event.type === 'error'
      ? 'text-red-300'
      : event.type === 'done'
        ? 'text-emerald-300'
        : 'text-muted-foreground'

  return (
    <div
      ref={refCallback}
      className={cn(
        'rounded-xl px-3 py-2 transition',
        focused
          ? 'border border-primary/40 bg-primary/10 shadow-[0_0_28px_rgba(59,130,246,0.18)]'
          : 'subtle-surface',
      )}
    >
      <div className="flex items-center gap-2">
        <span className={cn('font-mono text-[11px]', typeColor)}>{event.type}</span>
        {event.tool && <span className={cn('font-mono text-[11px]', focused ? 'text-primary' : 'text-primary')}>{event.tool}</span>}
        {event.turn > 0 && (
          <span className="ml-auto font-mono text-[10px] text-muted-foreground">
            {event.turn}/{event.max_turns}
          </span>
        )}
      </div>
      {event.message && (
        <p className="mt-1 text-xs leading-5 text-muted-foreground">{event.message}</p>
      )}
    </div>
  )
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-xl subtle-surface px-3 py-2">
      <span>{label}</span>
      <span className="truncate text-right text-foreground">{value}</span>
    </div>
  )
}
