import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Crosshair,
  Database,
  FileWarning,
  Fingerprint,
  PlugZap,
  Radar,
  RefreshCcw,
  ShieldCheck,
  TerminalSquare,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { DoctorReport, Healthz, PluginDoctorReport, PluginManifest, RunSummary, VersionInfo } from '@/lib/api'
import { useConsole, text } from '@/lib/console'
import { cn } from '@/lib/utils'
import { formatDate, hostLabel, shortId, statusLabel, statusTone } from '@/lib/view'

interface DashboardData {
  health?: Healthz
  version?: VersionInfo
  doctor?: DoctorReport
  plugins: PluginManifest[]
  pluginDoctor?: PluginDoctorReport
  runs: RunSummary[]
}

const emptyData: DashboardData = {
  plugins: [],
  runs: [],
}

export function DashboardPage() {
  const { client, language, t } = useConsole()
  const [data, setData] = useState<DashboardData>(emptyData)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [health, version, doctor, plugins, pluginDoctor, runs] = await Promise.all([
        client.healthz(),
        client.version(),
        client.doctor(),
        client.plugins(),
        client.pluginsDoctor(),
        client.listRuns(12),
      ])
      setData({
        health,
        version,
        doctor,
        plugins: plugins.plugins || [],
        pluginDoctor,
        runs: runs.items || [],
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    } finally {
      setLoading(false)
    }
  }, [client])

  useEffect(() => {
    refresh()
    const id = window.setInterval(refresh, 10_000)
    return () => window.clearInterval(id)
  }, [refresh])

  const metrics = useMemo(() => {
    const running = data.runs.filter((run) => run.status === 'running').length
    const succeeded = data.runs.filter((run) => run.status === 'succeeded').length
    const failed = data.runs.filter((run) => run.status === 'failed').length
    const findings = data.runs.reduce((sum, run) => sum + (run.counts?.findings || 0), 0)
    const approvals = data.runs.reduce((sum, run) => sum + (run.counts?.approvals || 0), 0)
    const pluginIssues = data.pluginDoctor?.plugins.filter((plugin) => !plugin.ok).length || 0
    return { running, succeeded, failed, findings, approvals, pluginIssues }
  }, [data])

  return (
    <div className="space-y-6">
      <section className="relative overflow-hidden rounded-[2rem] border border-white/10 bg-white/[0.035] p-5 shadow-2xl shadow-black/30 sm:p-7">
        <div className="absolute inset-y-0 right-0 hidden w-1/2 bg-[radial-gradient(circle_at_center,rgba(37,99,235,0.20),transparent_58%)] lg:block" />
        <div className="relative grid gap-7 lg:grid-cols-[1fr_420px]">
          <div className="flex min-h-[300px] flex-col justify-between">
            <div>
              <div className="mb-5 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
                <ShieldCheck className="h-3.5 w-3.5" />
                {text(language, '授权 Web/API 安全分析', 'Authorized Web/API analysis')}
              </div>
              <h1 className="max-w-3xl text-4xl font-semibold tracking-tight sm:text-6xl">CyberAgent</h1>
              <p className="mt-4 max-w-2xl text-sm leading-7 text-muted-foreground sm:text-base">
                {text(
                  language,
                  '用一个本地控制台调度 daemon、扫描、审批、证据与报告。',
                  'Operate daemon, scans, approvals, evidence and reports from one local console.',
                )}
              </p>
            </div>

            <div className="mt-8 flex flex-wrap items-center gap-3">
              <Button asChild className="rounded-2xl">
                <Link to="/scan/new">
                  <Crosshair className="mr-2 h-4 w-4" />
                  {t('start')}
                </Link>
              </Button>
              <Button type="button" variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
                <RefreshCcw className={cn('mr-2 h-4 w-4', loading && 'animate-spin')} />
                {t('refresh')}
              </Button>
              <span className="inline-flex items-center gap-2 rounded-2xl border border-white/10 bg-black/20 px-3 py-2 font-mono text-xs text-muted-foreground">
                <Fingerprint className="h-4 w-4" />
                {data.version?.version || 'v0.1.2'}
              </span>
            </div>
          </div>

          <div className="relative grid min-h-[300px] place-items-center">
            <div className="radar-orb">
              <div className="radar-sweep" />
              <Radar className="relative z-10 h-20 w-20 text-primary" />
            </div>
            <div className="absolute bottom-0 left-0 right-0 grid grid-cols-3 gap-2">
              <Signal icon={Activity} label={t('health')} value={data.health?.status || (error ? t('offline') : t('loading'))} active={!error} />
              <Signal icon={PlugZap} label={t('plugins')} value={String(data.plugins.length)} active={metrics.pluginIssues === 0} />
              <Signal icon={TerminalSquare} label={t('runs')} value={String(data.runs.length)} active={metrics.failed === 0} />
            </div>
          </div>
        </div>
      </section>

      {error && (
        <div className="flex items-center gap-3 rounded-2xl border border-red-400/20 bg-red-500/10 px-4 py-3 text-sm text-red-100">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-6">
        <Metric icon={Activity} label={t('running')} value={metrics.running} tone="cyan" />
        <Metric icon={CheckCircle2} label={t('succeeded')} value={metrics.succeeded} tone="emerald" />
        <Metric icon={FileWarning} label={t('failed')} value={metrics.failed} tone="red" />
        <Metric icon={AlertTriangle} label={t('findings')} value={metrics.findings} tone="amber" />
        <Metric icon={ShieldCheck} label={t('approvals')} value={metrics.approvals} tone="violet" />
        <Metric icon={PlugZap} label={t('plugins')} value={metrics.pluginIssues} tone="blue" />
      </section>

      <section className="grid gap-4 xl:grid-cols-[1.2fr_0.8fr]">
        <div className="rounded-[1.5rem] border border-white/10 bg-white/[0.03] p-4">
          <div className="mb-4 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Database className="h-4 w-4 text-primary" />
              <h2 className="text-sm font-semibold">{t('runs')}</h2>
            </div>
            <Link to="/history" className="text-xs text-muted-foreground hover:text-foreground">
              {text(language, '全部', 'All')}
            </Link>
          </div>
          <div className="space-y-2">
            {data.runs.slice(0, 8).map((run) => (
              <Link
                key={run.run_id}
                to={`/scan/${run.run_id}`}
                className="group grid gap-3 rounded-2xl border border-white/10 bg-black/20 p-3 transition hover:border-primary/30 hover:bg-primary/5 sm:grid-cols-[1fr_auto]"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-mono text-sm text-foreground">{hostLabel(run.target)}</span>
                    <span className={cn('rounded-full border px-2 py-0.5 text-[11px]', statusTone(run.status))}>
                      {statusLabel(run.status, language)}
                    </span>
                  </div>
                  <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{shortId(run.run_id, 24)}</div>
                </div>
                <div className="flex items-center gap-3 text-xs text-muted-foreground">
                  <span>{run.counts?.events || 0} {t('events')}</span>
                  <span>{run.counts?.artifacts || 0} {t('artifacts')}</span>
                  <span>{formatDate(run.updated_at || run.created_at, language)}</span>
                </div>
              </Link>
            ))}
            {!data.runs.length && !loading && <EmptyLine label={t('noData')} />}
          </div>
        </div>

        <div className="grid gap-4">
          <HealthPanel
            title={t('doctor')}
            ok={data.doctor?.ok}
            items={(data.doctor?.checks || []).map((check) => ({
              id: check.name,
              label: check.name,
              ok: check.ok,
              message: check.message || check.status || '',
            }))}
          />
          <HealthPanel
            title={t('plugins')}
            ok={data.pluginDoctor?.ok}
            items={(data.pluginDoctor?.plugins || []).slice(0, 6).map((plugin) => ({
              id: plugin.id,
              label: plugin.id,
              ok: plugin.ok,
              message: plugin.dependencies?.map((dep) => `${dep.name}:${dep.available ? 'ok' : 'missing'}`).join(' · ') || '',
            }))}
          />
        </div>
      </section>
    </div>
  )
}

function Signal({
  icon: Icon,
  label,
  value,
  active,
}: {
  icon: React.ElementType
  label: string
  value: string
  active: boolean
}) {
  return (
    <div className="rounded-2xl border border-white/10 bg-black/35 p-3 backdrop-blur">
      <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
        <Icon className={cn('h-3.5 w-3.5', active ? 'text-emerald-300' : 'text-amber-300')} />
        {label}
      </div>
      <div className="mt-1 truncate font-mono text-sm">{value}</div>
    </div>
  )
}

function Metric({
  icon: Icon,
  label,
  value,
  tone,
}: {
  icon: React.ElementType
  label: string
  value: number
  tone: 'cyan' | 'emerald' | 'red' | 'amber' | 'violet' | 'blue'
}) {
  const tones = {
    cyan: 'text-cyan-300 bg-cyan-400/10 border-cyan-400/20',
    emerald: 'text-emerald-300 bg-emerald-400/10 border-emerald-400/20',
    red: 'text-red-300 bg-red-400/10 border-red-400/20',
    amber: 'text-amber-300 bg-amber-400/10 border-amber-400/20',
    violet: 'text-violet-300 bg-violet-400/10 border-violet-400/20',
    blue: 'text-blue-300 bg-blue-400/10 border-blue-400/20',
  }
  return (
    <div className="rounded-3xl border border-white/10 bg-white/[0.03] p-4">
      <div className="flex items-center justify-between">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span className={cn('grid h-9 w-9 place-items-center rounded-2xl border', tones[tone])}>
          <Icon className="h-4 w-4" />
        </span>
      </div>
      <div className="mt-4 font-mono text-3xl font-semibold">{value}</div>
    </div>
  )
}

function HealthPanel({
  title,
  ok,
  items,
}: {
  title: string
  ok?: boolean
  items: Array<{ id: string; label: string; ok: boolean; message: string }>
}) {
  return (
    <div className="rounded-[1.5rem] border border-white/10 bg-white/[0.03] p-4">
      <div className="mb-4 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <ShieldCheck className={cn('h-4 w-4', ok === false ? 'text-amber-300' : 'text-emerald-300')} />
          <h2 className="text-sm font-semibold">{title}</h2>
        </div>
        <span className={cn('rounded-full border px-2 py-0.5 text-[11px]', ok === false ? statusTone('pending') : statusTone('succeeded'))}>
          {ok === false ? 'warn' : 'ok'}
        </span>
      </div>
      <div className="space-y-2">
        {items.map((item) => (
          <div key={item.id} className="rounded-2xl border border-white/10 bg-black/20 p-3">
            <div className="flex items-center justify-between gap-3">
              <span className="truncate font-mono text-xs">{item.label}</span>
              <span className={cn('h-2 w-2 rounded-full', item.ok ? 'bg-emerald-300' : 'bg-amber-300')} />
            </div>
            {item.message && <div className="mt-1 truncate text-xs text-muted-foreground">{item.message}</div>}
          </div>
        ))}
        {!items.length && <EmptyLine label="—" />}
      </div>
    </div>
  )
}

function EmptyLine({ label }: { label: string }) {
  return (
    <div className="grid min-h-28 place-items-center rounded-2xl border border-dashed border-white/10 bg-black/10 text-sm text-muted-foreground">
      {label}
    </div>
  )
}
