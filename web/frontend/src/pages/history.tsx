import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Crosshair,
  Database,
  FileWarning,
  Filter,
  RefreshCcw,
  ShieldCheck,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { RunStatus, RunSummary } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'
import { formatDate, hostLabel, shortId, statusLabel, statusTone } from '@/lib/view'

type FilterValue = 'all' | 'running' | 'succeeded' | 'failed'

const filters: Array<{ id: FilterValue; status?: RunStatus }> = [
  { id: 'all' },
  { id: 'running', status: 'running' },
  { id: 'succeeded', status: 'succeeded' },
  { id: 'failed', status: 'failed' },
]

export function HistoryPage() {
  const { client, language, t } = useConsole()
  const [runs, setRuns] = useState<RunSummary[]>([])
  const [filter, setFilter] = useState<FilterValue>('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const page = await client.listRuns(100)
      setRuns(page.items || [])
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

  const visibleRuns = useMemo(() => {
    if (filter === 'all') return runs
    return runs.filter((run) => run.status === filter)
  }, [filter, runs])

  const counts = useMemo(
    () => ({
      all: runs.length,
      running: runs.filter((run) => run.status === 'running').length,
      succeeded: runs.filter((run) => run.status === 'succeeded').length,
      failed: runs.filter((run) => run.status === 'failed').length,
    }),
    [runs],
  )

  return (
    <div className="space-y-6">
      <header className="flex flex-col justify-between gap-4 rounded-[2rem] border border-white/10 bg-white/[0.035] p-5 sm:flex-row sm:items-center sm:p-7">
        <div>
          <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
            <Database className="h-3.5 w-3.5" />
            {t('runs')}
          </div>
          <h1 className="text-3xl font-semibold tracking-tight sm:text-5xl">{text(language, '运行索引', 'Run index')}</h1>
          <p className="mt-3 text-sm text-muted-foreground">{text(language, '读取 daemon runs-dir 中的已知 run。', 'Read known runs from daemon runs-dir.')}</p>
        </div>
        <Button type="button" variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
          <RefreshCcw className={cn('mr-2 h-4 w-4', loading && 'animate-spin')} />
          {t('refresh')}
        </Button>
      </header>

      {error && (
        <div className="flex items-center gap-3 rounded-2xl border border-red-400/20 bg-red-500/10 px-4 py-3 text-sm text-red-100">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Counter icon={Database} label={text(language, '全部', 'All')} value={counts.all} />
        <Counter icon={Activity} label={t('running')} value={counts.running} tone="cyan" />
        <Counter icon={CheckCircle2} label={t('succeeded')} value={counts.succeeded} tone="emerald" />
        <Counter icon={FileWarning} label={t('failed')} value={counts.failed} tone="red" />
      </section>

      <section className="rounded-[2rem] border border-white/10 bg-white/[0.03] p-4">
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <span className="mr-1 inline-flex items-center gap-2 text-xs text-muted-foreground">
            <Filter className="h-4 w-4" />
            {text(language, '筛选', 'Filter')}
          </span>
          {filters.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => setFilter(item.id)}
              className={cn(
                'rounded-full border px-3 py-1 text-xs transition',
                filter === item.id ? 'border-primary/35 bg-primary/10 text-primary' : 'border-white/10 bg-black/20 text-muted-foreground hover:text-foreground',
              )}
            >
              {item.id === 'all' ? text(language, '全部', 'All') : statusLabel(item.status, language)}
            </button>
          ))}
        </div>

        <div className="space-y-2">
          {visibleRuns.map((run) => (
            <Link
              key={run.run_id}
              to={`/scan/${run.run_id}`}
              className="group grid gap-4 rounded-3xl border border-white/10 bg-black/20 p-4 transition hover:border-primary/30 hover:bg-primary/5 lg:grid-cols-[minmax(0,1fr)_auto]"
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <Crosshair className="h-4 w-4 text-primary" />
                  <span className="truncate font-mono text-sm text-foreground">{hostLabel(run.target)}</span>
                  <span className={cn('rounded-full border px-2 py-0.5 text-[11px]', statusTone(run.status))}>
                    {statusLabel(run.status, language)}
                  </span>
                  {run.profile && (
                    <span className="rounded-full border border-emerald-400/20 bg-emerald-400/10 px-2 py-0.5 text-[11px] text-emerald-200">
                      {run.profile}
                    </span>
                  )}
                </div>
                <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 font-mono text-xs text-muted-foreground">
                  <span>{shortId(run.run_id, 28)}</span>
                  <span>{run.scope || 'scope:—'}</span>
                  <span>{run.run_dir}</span>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-2 sm:flex sm:items-center">
                <Pill icon={Activity} value={run.counts?.events || 0} label={t('events')} />
                <Pill icon={ShieldCheck} value={run.counts?.approvals || 0} label={t('approvals')} />
                <Pill icon={AlertTriangle} value={run.counts?.findings || 0} label={t('findings')} />
                <Pill icon={Clock3} value={formatDate(run.updated_at || run.created_at, language)} label="" />
              </div>
            </Link>
          ))}
          {!visibleRuns.length && (
            <div className="grid min-h-48 place-items-center rounded-3xl border border-dashed border-white/10 bg-black/10 text-sm text-muted-foreground">
              {loading ? t('loading') : t('noData')}
            </div>
          )}
        </div>
      </section>
    </div>
  )
}

function Counter({
  icon: Icon,
  label,
  value,
  tone = 'blue',
}: {
  icon: React.ElementType
  label: string
  value: number
  tone?: 'blue' | 'cyan' | 'emerald' | 'red'
}) {
  const tones = {
    blue: 'border-blue-400/20 bg-blue-400/10 text-blue-300',
    cyan: 'border-cyan-400/20 bg-cyan-400/10 text-cyan-300',
    emerald: 'border-emerald-400/20 bg-emerald-400/10 text-emerald-300',
    red: 'border-red-400/20 bg-red-400/10 text-red-300',
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

function Pill({ icon: Icon, value, label }: { icon: React.ElementType; value: number | string; label: string }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.03] px-3 py-2 font-mono text-xs text-muted-foreground">
      <Icon className="h-3.5 w-3.5" />
      <span className="text-foreground">{value}</span>
      {label && <span>{label}</span>}
    </span>
  )
}
