import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Database,
  FileWarning,
  Filter,
  RefreshCcw,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { EmptyState, MetricCard, PageActions, PageHeader, PageTitle, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import type { ScanListItem, ScanStatus } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'
import { formatDate, hostLabel, statusLabel, statusTone } from '@/lib/view'

type FilterValue = 'all' | 'running' | 'completed' | 'failed' | 'stopped'

export function HistoryPage() {
  const { client, language, t } = useConsole()
  const [scans, setScans] = useState<ScanListItem[]>([])
  const [filter, setFilter] = useState<FilterValue>('all')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const items = await client.listScans()
      setScans(items || [])
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

  const visible = useMemo(() => {
    if (filter === 'all') return scans
    return scans.filter((s) => s.status === filter)
  }, [filter, scans])

  const counts = useMemo(() => ({
    all: scans.length,
    running: scans.filter((s) => s.status === 'running').length,
    completed: scans.filter((s) => s.status === 'completed').length,
    failed: scans.filter((s) => s.status === 'failed').length,
  }), [scans])

  const filters: Array<{ id: FilterValue; status?: ScanStatus }> = [
    { id: 'all' },
    { id: 'running', status: 'running' },
    { id: 'completed', status: 'completed' },
    { id: 'failed', status: 'failed' },
  ]

  return (
    <div className="space-y-6">
      <Section className="flex flex-col justify-between gap-4 p-5 sm:flex-row sm:items-center sm:p-7">
        <PageHeader>
          <div>
            <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
              <Database className="h-3.5 w-3.5" />
              {t('history')}
            </div>
            <PageTitle>{text(language, '扫描历史', 'Scan History')}</PageTitle>
          </div>
          <PageActions>
            <Button type="button" variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
              <RefreshCcw className={cn('mr-2 h-4 w-4', loading && 'animate-spin')} />
              {t('refresh')}
            </Button>
          </PageActions>
        </PageHeader>
      </Section>

      {error && (
        <div className="flex items-center gap-3 rounded-2xl border border-red-400/20 bg-red-500/10 px-4 py-3 text-sm text-red-100">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard icon={Database} label={text(language, '全部', 'All')} value={counts.all} tone="blue" />
        <MetricCard icon={Activity} label={t('running')} value={counts.running} tone="cyan" />
        <MetricCard icon={CheckCircle2} label={t('completed')} value={counts.completed} tone="emerald" />
        <MetricCard icon={FileWarning} label={t('failed')} value={counts.failed} tone="red" />
      </section>

      <Section>
        <SectionHeader className="mb-3">
          <SectionTitle icon={Filter}>{text(language, '筛选', 'Filter')}</SectionTitle>
        </SectionHeader>
        <div className="mb-4 flex flex-wrap items-center gap-2">
          {filters.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => setFilter(item.id)}
              className={cn(
                'rounded-full border px-3 py-1 text-xs transition',
                filter === item.id ? 'border-primary/35 bg-primary/10 text-primary' : 'list-item rounded-full px-3 py-1 text-muted-foreground hover:text-foreground',
              )}
            >
              {item.id === 'all' ? text(language, '全部', 'All') : statusLabel(item.status, language)}
            </button>
          ))}
        </div>

        <div className="space-y-2">
          {visible.map((scan) => (
            <Link
              key={scan.id}
              to={`/scan/${scan.id}`}
              className="group grid gap-4 rounded-3xl p-4 list-item lg:grid-cols-[minmax(0,1fr)_auto]"
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate font-mono text-sm text-foreground">{hostLabel(scan.target)}</span>
                  <span className={cn('rounded-full border px-2 py-0.5 text-[11px]', statusTone(scan.status))}>
                    {statusLabel(scan.status, language)}
                  </span>
                  <span className="rounded-full border border-white/10 bg-white/[0.04] px-2 py-0.5 text-[11px] text-muted-foreground">
                    {scan.provider}
                  </span>
                </div>
                <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 font-mono text-xs text-muted-foreground">
                  <span>{scan.id}</span>
                </div>
              </div>

              <div className="flex items-center gap-3 text-xs text-muted-foreground">
                <span>{scan.findings} {t('findings')}</span>
                <span>{scan.turns}/{scan.max_turns} {t('turns')}</span>
                <span>{scan.duration}</span>
                <span>{formatDate(scan.started_at, language)}</span>
              </div>
            </Link>
          ))}
          {!visible.length && (
            <EmptyState className="min-h-[12rem] rounded-3xl">
              {loading ? t('loading') : t('noData')}
            </EmptyState>
          )}
        </div>
      </Section>
    </div>
  )
}
