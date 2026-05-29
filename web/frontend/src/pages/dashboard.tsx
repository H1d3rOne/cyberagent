import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  BarChart3,
  CheckCircle2,
  FileWarning,
  RefreshCcw,
  ShieldCheck,
  Target,
  TrendingUp,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { MetricCard, PageDescription, PageHeader, PageTitle, Section, SectionHeader, SectionTitle, EmptyState } from '@/components/layout-primitives'
import { ApiError, type ScanListItem } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

interface ChartPoint {
  label: string
  value: number
}

export function DashboardPage() {
  const { client, language, t } = useConsole()
  const [runs, setRuns] = useState<ScanListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const items = await client.listScans()
      setRuns(items || [])
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError(text(language, '需要令牌：请在设置页填写 token 后刷新。', 'Token required: set the token in Settings and refresh.'))
      } else {
        setError(err instanceof Error ? err.message : 'request failed')
      }
    } finally {
      setLoading(false)
    }
  }, [client, language])

  useEffect(() => {
    refresh()
    const id = window.setInterval(refresh, 8_000)
    return () => window.clearInterval(id)
  }, [refresh])

  const metrics = useMemo(() => {
    const active = runs.filter((run) => run.status === 'running').length
    const completed = runs.filter((run) => run.status === 'completed').length
    const failed = runs.filter((run) => run.status === 'failed').length
    const findings = runs.reduce((sum, run) => sum + (run.findings || 0), 0)
    const successRate = runs.length ? Math.round((completed / runs.length) * 100) : 0
    return { active, completed, failed, findings, total: runs.length, successRate }
  }, [runs])

  const statusPoints = useMemo<ChartPoint[]>(() => [
    { label: t('running'), value: metrics.active },
    { label: t('completed'), value: metrics.completed },
    { label: t('failed'), value: metrics.failed },
  ], [metrics.active, metrics.completed, metrics.failed, t])

  const activityPoints = useMemo<ChartPoint[]>(() => {
    const buckets = new Map<string, number>()
    const now = new Date()
    for (let i = 6; i >= 0; i--) {
      const day = new Date(now)
      day.setDate(now.getDate() - i)
      const key = day.toISOString().slice(5, 10)
      buckets.set(key, 0)
    }
    for (const run of runs) {
      const raw = run.started_at
      if (!raw) continue
      const key = new Date(raw).toISOString().slice(5, 10)
      if (buckets.has(key)) buckets.set(key, (buckets.get(key) || 0) + 1)
    }
    return Array.from(buckets.entries()).map(([label, value]) => ({ label, value }))
  }, [runs])

  const findingPoints = useMemo<ChartPoint[]>(() => {
    return runs
      .filter((run) => (run.findings || 0) > 0)
      .sort((a, b) => (b.findings || 0) - (a.findings || 0))
      .slice(0, 6)
      .map((run) => ({ label: run.target || run.id.slice(0, 12), value: run.findings || 0 }))
  }, [runs])

  return (
    <div className="space-y-6">
      <Section className="mission-panel p-5 sm:p-7">
        <div className="absolute inset-y-0 right-0 hidden w-1/2 bg-[radial-gradient(circle_at_center,rgba(37,99,235,0.22),transparent_58%)] lg:block" />
        <PageHeader className="relative">
          <div>
            <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
              <ShieldCheck className="h-3.5 w-3.5" />
              {text(language, 'Agent 指挥面板', 'Agent Command Surface')}
            </div>
            <PageTitle>{t('overview')}</PageTitle>
            <PageDescription className="mt-2">
              {text(
                language,
                '这里不发起任务，只给安全 Agent 的运行态势：任务数量、成功率、发现密度、状态分布和近 7 日活动趋势。',
                'No task launching here: this surface only shows the security agent posture — tasks, success rate, finding density, status distribution and 7-day activity.',
              )}
            </PageDescription>
          </div>
          <Button type="button" variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
            <RefreshCcw className={cn('mr-2 h-4 w-4', loading && 'animate-spin')} />
            {t('refresh')}
          </Button>
        </PageHeader>
      </Section>

      {error && (
        <div className="flex items-center gap-3 rounded-2xl border border-red-400/20 bg-red-500/10 px-4 py-3 text-sm text-red-100">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>{error}</span>
        </div>
      )}

      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <MetricCard icon={Target} label={text(language, '任务总数', 'Total tasks')} value={metrics.total} tone="blue" />
        <MetricCard icon={Activity} label={t('running')} value={metrics.active} tone="cyan" />
        <MetricCard icon={CheckCircle2} label={t('completed')} value={metrics.completed} tone="emerald" />
        <MetricCard icon={FileWarning} label={t('failed')} value={metrics.failed} tone="red" />
        <MetricCard icon={AlertTriangle} label={t('findings')} value={metrics.findings} tone="amber" />
      </section>

      <div className="grid gap-4 xl:grid-cols-[1fr_0.85fr]">
        <Section>
          <SectionHeader>
            <SectionTitle icon={BarChart3}>{text(language, '任务状态分布', 'Task status distribution')}</SectionTitle>
            <span className="font-mono text-xs text-muted-foreground">{metrics.successRate}% success</span>
          </SectionHeader>
          <BarList points={statusPoints} emptyLabel={t('noData')} />
        </Section>

        <Section>
          <SectionHeader>
            <SectionTitle icon={TrendingUp}>{text(language, '近 7 日活动', '7-day activity')}</SectionTitle>
          </SectionHeader>
          <SparkBars points={activityPoints} />
        </Section>
      </div>

      <Section>
        <SectionHeader>
          <SectionTitle icon={AlertTriangle}>{text(language, '发现密度 Top 目标', 'Top finding density')}</SectionTitle>
        </SectionHeader>
        {findingPoints.length ? (
          <BarList points={findingPoints} emptyLabel={t('noData')} />
        ) : (
          <EmptyState>{text(language, '暂无发现数据', 'No finding data yet')}</EmptyState>
        )}
      </Section>
    </div>
  )
}

function BarList({ points, emptyLabel }: { points: ChartPoint[]; emptyLabel: string }) {
  const max = Math.max(1, ...points.map((point) => point.value))
  if (!points.length) return <EmptyState>{emptyLabel}</EmptyState>
  return (
    <div className="space-y-3">
      {points.map((point) => (
        <div key={point.label} className="grid gap-2 sm:grid-cols-[150px_1fr_56px] sm:items-center">
          <div className="truncate text-xs text-muted-foreground">{point.label}</div>
          <div className="h-3 overflow-hidden rounded-full subtle-surface">
            <div className="h-full rounded-full bg-primary/70" style={{ width: `${Math.max(4, (point.value / max) * 100)}%` }} />
          </div>
          <div className="font-mono text-xs text-foreground sm:text-right">{point.value}</div>
        </div>
      ))}
    </div>
  )
}

function SparkBars({ points }: { points: ChartPoint[] }) {
  const max = Math.max(1, ...points.map((point) => point.value))
  return (
    <div className="flex h-48 items-end gap-2 rounded-3xl subtle-surface p-4">
      {points.map((point) => (
        <div key={point.label} className="flex min-w-0 flex-1 flex-col items-center gap-2">
          <div className="flex h-32 w-full items-end rounded-xl bg-white/[0.03] p-1">
            <div className="w-full rounded-lg bg-cyan-400/70" style={{ height: `${Math.max(5, (point.value / max) * 100)}%` }} />
          </div>
          <span className="font-mono text-[10px] text-muted-foreground">{point.label}</span>
        </div>
      ))}
    </div>
  )
}
