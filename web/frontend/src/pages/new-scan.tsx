import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  Bot,
  CheckCircle2,
  Crosshair,
  FileWarning,
  FileSearch,
  Radar,
  RefreshCcw,
  X,
  ShieldCheck,
  Sparkles,
  Target,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { MetricCard, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import { rememberJob, type ScanListItem } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { saveAgentDraft } from '@/lib/agent-draft'
import { cn } from '@/lib/utils'
import { hostLabel, statusLabel, statusTone } from '@/lib/view'

type TaskTab = 'running' | 'completed' | 'failed'

export function NewScanPage() {
  const navigate = useNavigate()
  const { client, language, settings, updateSettings, t } = useConsole()
  const savedDraft = settings.scanDraft
  const [target, setTarget] = useState(savedDraft.target)
  const [scope, setScope] = useState(savedDraft.scope)
  const [multiAgent, setMultiAgent] = useState(savedDraft.multiAgent)
  const [interactive, setInteractive] = useState(savedDraft.interactive)
  const [loading, setLoading] = useState(false)
  const [tasksLoading, setTasksLoading] = useState(true)
  const [tasks, setTasks] = useState<ScanListItem[]>([])
  const [taskTab, setTaskTab] = useState<TaskTab>('running')
  const [createOpen, setCreateOpen] = useState(false)
  const [error, setError] = useState('')

  const refreshTasks = useCallback(async () => {
    setTasksLoading(true)
    try {
      const items = await client.listScans()
      setTasks(items || [])
    } catch {
      setTasks([])
    } finally {
      setTasksLoading(false)
    }
  }, [client])

  useEffect(() => {
    updateSettings({
      scanDraft: {
        target,
        scope,
        multiAgent,
        interactive,
      },
    })
  }, [interactive, multiAgent, scope, target, updateSettings])

  useEffect(() => {
    refreshTasks()
    const id = window.setInterval(refreshTasks, 8_000)
    return () => window.clearInterval(id)
  }, [refreshTasks])

  const mode = interactive ? 'interactive' : multiAgent ? 'multi-agent' : 'single'

  const prompt = buildAgentPrompt(language, target, scope, multiAgent, interactive)

  const taskMetrics = useMemo(() => {
    const running = tasks.filter((task) => task.status === 'running' || task.status === 'queued').length
    const completed = tasks.filter((task) => task.status === 'completed' || task.status === 'succeeded').length
    const failed = tasks.filter((task) => task.status === 'failed').length
    return { total: tasks.length, running, completed, failed }
  }, [tasks])

  const visibleTasks = useMemo(() => {
    if (taskTab === 'running') return tasks.filter((task) => ['running', 'queued', 'created', 'planning'].includes(task.status))
    if (taskTab === 'completed') return tasks.filter((task) => task.status === 'completed' || task.status === 'succeeded')
    return tasks.filter((task) => task.status === 'failed')
  }, [taskTab, tasks])

  const start = async (event: React.FormEvent) => {
    event.preventDefault()
    setLoading(true)
    setError('')
    try {
      const response = await client.startWebScan({
        target,
        scope,
        profile: 'safe',
        no_external_tools: true,
        enabled_plugins: [],
      })
      rememberJob({
        job_id: response.job_id,
        run_id: response.run_id,
        created_at: new Date().toISOString(),
      })
      navigate(`/scan/${response.run_id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed')
    } finally {
      setLoading(false)
    }
  }

  const handoffToAgent = () => {
    saveAgentDraft({
      prompt,
      source: 'new-scan',
      target,
    })
    navigate('/agent')
  }

  return (
    <div className="space-y-6">
      <Section>
        <SectionHeader>
          <SectionTitle icon={Target}>{t('tasks')}</SectionTitle>
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" onClick={refreshTasks} className="rounded-2xl bg-transparent">
              <RefreshCcw className={cn('mr-2 h-4 w-4', tasksLoading && 'animate-spin')} />
              {t('refresh')}
            </Button>
            <Button type="button" onClick={() => setCreateOpen(true)} className="rounded-2xl">
              <Crosshair className="mr-2 h-4 w-4" />
              {text(language, '新增任务', 'New task')}
            </Button>
          </div>
        </SectionHeader>
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <MetricCard icon={Target} label={text(language, '全部任务', 'All tasks')} value={taskMetrics.total} tone="blue" />
          <MetricCard icon={Activity} label={t('running')} value={taskMetrics.running} tone="cyan" />
          <MetricCard icon={CheckCircle2} label={t('completed')} value={taskMetrics.completed} tone="emerald" />
          <MetricCard icon={FileWarning} label={t('failed')} value={taskMetrics.failed} tone="red" />
        </div>
      </Section>

      <Section>
        <SectionHeader>
          <SectionTitle icon={FileSearch}>{text(language, '任务列表', 'Task list')}</SectionTitle>
          <span className="font-mono text-xs text-muted-foreground">{tasksLoading ? t('loading') : `${visibleTasks.length} items`}</span>
        </SectionHeader>
        <div className="mb-4 flex flex-wrap gap-2">
          {([
            ['running', t('running')],
            ['completed', t('completed')],
            ['failed', t('failed')],
          ] as Array<[TaskTab, string]>).map(([id, label]) => (
            <button
              key={id}
              type="button"
              onClick={() => setTaskTab(id)}
              className={cn(
                'rounded-full border px-3 py-1 text-xs transition',
                taskTab === id ? 'border-primary/35 bg-primary/10 text-primary' : 'list-item rounded-full px-3 py-1 text-muted-foreground hover:text-foreground',
              )}
            >
              {label}
            </button>
          ))}
        </div>
        <div className="space-y-2">
          {visibleTasks.map((task) => (
            <Link key={task.id} to={`/scan/${task.id}`} className="group grid gap-3 rounded-2xl p-3 list-item sm:grid-cols-[minmax(0,1fr)_auto]">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="truncate font-mono text-sm text-foreground">{hostLabel(task.target)}</span>
                  <span className={cn('rounded-full border px-2 py-0.5 text-[11px]', statusTone(task.status))}>
                    {statusLabel(task.status, language)}
                  </span>
                </div>
                <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{task.id}</div>
              </div>
              <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <span>{task.findings} {t('findings')}</span>
                <span>{task.duration}</span>
              </div>
            </Link>
          ))}
          {!visibleTasks.length && !tasksLoading && (
            <div className="grid min-h-28 place-items-center rounded-2xl border border-dashed border-white/10 bg-white/[0.02] text-sm text-muted-foreground">
              {t('noData')}
            </div>
          )}
        </div>
      </Section>

      {createOpen && (
        <div className="fixed inset-0 z-50 overflow-y-auto bg-black/70 p-4 backdrop-blur-sm">
          <div className="mx-auto my-8 max-w-6xl rounded-[2rem] border border-white/10 bg-background p-4 shadow-2xl shadow-black/40 sm:p-5">
            <div className="mb-4 flex items-center justify-between gap-3">
              <div>
                <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
                  <Crosshair className="h-3.5 w-3.5" />
                  {text(language, '新增任务', 'New task')}
                </div>
                <h2 className="text-xl font-semibold tracking-tight">{text(language, '手动发起 Scan', 'Launch scan manually')}</h2>
              </div>
              <Button type="button" variant="ghost" size="icon" className="rounded-2xl" onClick={() => setCreateOpen(false)}>
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_400px]">
      <form onSubmit={start} className="space-y-6">
        <section className="relative overflow-hidden rounded-[1.75rem] border border-white/[0.08] bg-white/[0.03] p-5 sm:p-7">
          <div className="absolute inset-0 bg-[radial-gradient(circle_at_15%_0%,rgba(59,130,246,0.22),transparent_38%)]" />
          <div className="relative mb-7 flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
                <Crosshair className="h-3.5 w-3.5" />
                {text(language, '任务输入', 'Task input')}
              </div>
              <h1 className="text-3xl font-semibold tracking-tight sm:text-5xl">{t('start')}</h1>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
                {text(
                  language,
                  '这里只填写本次授权目标、范围和执行模式。Provider、模型、密钥等全局能力统一在设置页维护。',
                  'Only enter the authorized target, scope and run mode here. Provider, model, key and other global capabilities are maintained in Settings.',
                )}
              </p>
            </div>
            <div className="hidden h-16 w-16 place-items-center rounded-[1.5rem] border border-primary/25 bg-primary/10 sm:grid">
              <Radar className="h-8 w-8 text-primary" />
            </div>
          </div>

          <div className="relative grid gap-5">
            <Field
              label={t('target')}
              htmlFor="target"
              helper={text(
                language,
                '本次要分析的授权 Web/API 入口 URL。',
                'Authorized Web/API entry URL for this run.',
              )}
            >
              <Input
                id="target"
                value={target}
                onChange={(event) => setTarget(event.target.value)}
                placeholder="http://127.0.0.1:18080"
                required
                className="h-12 rounded-2xl font-mono"
              />
            </Field>
            <Field
              label={t('scope')}
              htmlFor="scope"
              helper={text(
                language,
                '授权边界，用于限制扫描范围，例如主机、域名或 URL 前缀。',
                'Authorization boundary used to constrain scanning, such as host, domain or URL prefix.',
              )}
            >
              <Input
                id="scope"
                value={scope}
                onChange={(event) => setScope(event.target.value)}
                placeholder="127.0.0.1"
                className="h-12 rounded-2xl font-mono"
              />
            </Field>
          </div>
        </section>

        <section className="rounded-[1.75rem] border border-white/[0.08] bg-white/[0.03] p-5 sm:p-7">
          <div className="mb-4 flex items-center gap-3">
            <span className="grid h-11 w-11 place-items-center rounded-2xl border border-emerald-400/20 bg-emerald-400/10 text-emerald-300">
              <Bot className="h-5 w-5" />
            </span>
            <div>
              <h2 className="text-sm font-semibold">{text(language, 'Agent 模式', 'Agent mode')}</h2>
              <p className="font-mono text-xs text-muted-foreground">safe profile / HITL</p>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            <ModeButton
              active={!multiAgent && !interactive}
              icon={ShieldCheck}
              label={text(language, '单 Agent', 'Single Agent')}
              detail={text(language, '标准自主扫描', 'Standard autonomous run')}
              onClick={() => {
                setMultiAgent(false)
                setInteractive(false)
              }}
            />
            <ModeButton
              active={multiAgent}
              icon={Sparkles}
              label={t('multiAgent')}
              detail={text(language, '后续编排扩展', 'Future orchestration')}
              onClick={() => {
                setMultiAgent(true)
                setInteractive(false)
              }}
            />
            <ModeButton
              active={interactive}
              icon={Bot}
              label={t('interactive')}
              detail={text(language, '交给对话驾驶', 'Drive via chat')}
              onClick={() => {
                setInteractive(true)
                setMultiAgent(false)
              }}
            />
          </div>

          <div className="mt-4 rounded-3xl subtle-surface p-4 text-sm leading-6 text-muted-foreground">
            {text(
              language,
              'MVP 会直接调用 daemon 的 safe Web Scan；交互/多 Agent 模式会作为 Agent Cockpit 的任务意图继续演进。',
              'MVP calls the daemon safe Web Scan directly; interactive / multi-agent modes continue as Agent Cockpit task intents.',
            )}
          </div>
        </section>

        {error && (
          <div className="flex items-center gap-3 rounded-2xl border border-red-400/20 bg-red-500/10 px-4 py-3 text-sm text-red-100">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
          <Button type="submit" disabled={loading || !target.trim()} className="h-14 rounded-2xl text-base">
            <Sparkles className={cn('mr-2 h-5 w-5', loading && 'animate-pulse')} />
            {loading ? text(language, '启动中...', 'Starting...') : t('start')}
          </Button>
          <Button type="button" variant="outline" onClick={handoffToAgent} disabled={!target.trim()} className="h-14 rounded-2xl bg-transparent px-5">
            <Bot className="mr-2 h-5 w-5" />
            {text(language, '交给 Agent', 'Ask Agent')}
          </Button>
        </div>
      </form>

      <aside className="space-y-4">
        <div className="rounded-[2rem] border border-white/10 bg-black/25 p-5">
          <div className="mb-4 flex items-center gap-2">
            <Target className="h-4 w-4 text-primary" />
            <h2 className="text-sm font-semibold">{text(language, '请求预览', 'Request Preview')}</h2>
          </div>
          <div className="space-y-2 font-mono text-xs text-muted-foreground">
            <PreviewLine label="POST" value="/api/v1/scans/web" />
            <PreviewLine label={t('target')} value={target || '—'} />
            <PreviewLine label={t('scope')} value={scope || '—'} />
            <PreviewLine label={t('profile')} value="safe" />
            <PreviewLine
              label={text(language, '外部工具', 'External tools')}
              value={text(language, '默认禁用', 'disabled by default')}
            />
            <PreviewLine label={text(language, '模式', 'Mode')} value={mode} />
          </div>
        </div>

        <div className="rounded-[2rem] subtle-surface p-5">
          <div className="mb-3 flex items-center gap-2 text-sm font-semibold">
            <FileSearch className="h-4 w-4 text-primary" />
            Agent Prompt
          </div>
          <p className="font-mono text-xs leading-6 text-muted-foreground">{prompt}</p>
        </div>
            </aside>
          </div>
        </div>
      </div>
      )}
    </div>
  )
}

function buildAgentPrompt(language: 'zh' | 'en', target: string, scope: string, multiAgent: boolean, interactive: boolean) {
  const mode = interactive ? 'interactive' : multiAgent ? 'multi-agent' : 'single-agent'
  if (language === 'en') {
    return `scan ${target || '<target>'}${scope ? ` scope=${scope}` : ''} profile=safe mode=${mode}`
  }
  return `扫描 ${target || '<目标>'}${scope ? ` scope=${scope}` : ''} profile=safe mode=${mode}`
}

function Field({
  label,
  htmlFor,
  helper,
  children,
}: {
  label: string
  htmlFor: string
  helper: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor} className="text-xs text-muted-foreground">
        {label}
      </Label>
      {children}
      <p className="text-xs leading-5 text-muted-foreground">{helper}</p>
    </div>
  )
}

function ModeButton({
  active,
  icon: Icon,
  label,
  detail,
  onClick,
}: {
  active: boolean
  icon: React.ElementType
  label: string
  detail: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex min-h-24 items-center gap-3 rounded-3xl border p-4 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        active
          ? 'border-primary/35 bg-primary/10 text-foreground'
          : 'border-white/10 bg-black/20 text-muted-foreground hover:bg-white/[0.04] hover:text-foreground',
      )}
    >
      <span className="grid h-10 w-10 shrink-0 place-items-center rounded-2xl border border-primary/20 bg-primary/10 text-primary">
        <Icon className="h-4 w-4" />
      </span>
      <span className="min-w-0">
        <span className="block text-sm font-medium">{label}</span>
        <span className="mt-1 block truncate text-xs text-muted-foreground">{detail}</span>
      </span>
    </button>
  )
}

function PreviewLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-xl border border-white/10 bg-white/[0.03] px-3 py-2">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate text-right text-foreground">{value}</span>
    </div>
  )
}
