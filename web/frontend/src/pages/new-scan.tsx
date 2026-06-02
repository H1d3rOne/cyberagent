import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  Activity,
  AlertTriangle,
  Bot,
  CheckCircle2,
  Clock,
  Crosshair,
  FileWarning,
  FileSearch,
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
import { SchedulesPanel } from '@/components/schedules-panel'
import { rememberJob, type ScanListItem } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { saveAgentDraft } from '@/lib/agent-draft'
import { cn } from '@/lib/utils'
import { hostLabel, statusLabel, statusTone } from '@/lib/view'

type TaskTab = 'running' | 'completed' | 'failed' | 'scheduled'
type CreateTab = 'scan' | 'schedule'

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
  const [createTab, setCreateTab] = useState<CreateTab>('scan')
  const [cron, setCron] = useState('0 * * * *')
  const [scheduleName, setScheduleName] = useState('')
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
      if (createTab === 'schedule') {
        if (!cron.trim()) {
          throw new Error(text(language, '请填写 Cron 表达式', 'Cron expression is required'))
        }
        await client.createSchedule({
          name: scheduleName.trim() || (target ? `scan ${target}` : text(language, '定时扫描', 'scheduled scan')),
          cron: cron.trim(),
          action: 'scan_web',
          enabled: true,
          target,
          scope,
          profile: 'safe',
          language,
          operator: 'web-console',
        })
        setCreateOpen(false)
        setTaskTab('scheduled')
        return
      }
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
      prompt: buildAgentPrompt(language, target, scope, multiAgent, interactive),
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
            ['scheduled', text(language, '定时', 'Scheduled')],
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
        {taskTab === 'scheduled' ? (
          <SchedulesPanel hideCreate />
        ) : (
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
        )}
      </Section>

      {createOpen && (
        <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/70 p-4 backdrop-blur-sm sm:items-center">
          <div className="flex max-h-[88vh] w-full max-w-2xl flex-col overflow-hidden rounded-3xl border border-white/10 bg-background shadow-2xl shadow-black/40">
            <div className="flex items-center justify-between gap-3 border-b border-white/5 px-5 py-3">
              <div className="flex items-center gap-2">
                <Crosshair className="h-4 w-4 text-primary" />
                <h2 className="text-sm font-semibold tracking-tight">
                  {createTab === 'schedule'
                    ? text(language, '定时任务', 'Scheduled task')
                    : text(language, '新增任务', 'New task')}
                </h2>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-8 w-8 rounded-xl"
                onClick={() => setCreateOpen(false)}
                aria-label={text(language, '关闭创建任务弹窗', 'Close create task dialog')}
              >
                <X className="h-4 w-4" />
              </Button>
            </div>
            <div className="flex-1 overflow-y-auto px-5 py-4">
            <div className="mb-4 grid grid-cols-2 gap-2 rounded-2xl border border-white/10 bg-white/[0.03] p-1">
              {([
                ['scan', text(language, '普通任务', 'One-off'), text(language, '立即执行一次', 'Run immediately')],
                ['schedule', text(language, '定时任务', 'Scheduled'), text(language, '按 Cron 周期触发', 'Run on a cron schedule')],
              ] as Array<[CreateTab, string, string]>).map(([id, label, hint]) => (
                <button
                  key={id}
                  type="button"
                  onClick={() => setCreateTab(id)}
                  className={cn(
                    'rounded-xl px-3 py-2 text-left transition',
                    createTab === id ? 'bg-primary/15 text-primary' : 'text-muted-foreground hover:text-foreground',
                  )}
                >
                  <div className="text-sm font-semibold">{label}</div>
                  <div className="font-mono text-[11px] opacity-80">{hint}</div>
                </button>
              ))}
            </div>
            <form onSubmit={start} className="space-y-4">
              <p className="text-xs text-muted-foreground">
                {createTab === 'schedule'
                  ? text(
                      language,
                      '配置周期触发的安全扫描；目标/范围与普通任务一致，由 daemon 内部执行 scan_web。',
                      'Configure a recurring safe scan. Target/scope are the same as a one-off task; executed as scan_web inside the daemon.',
                    )
                  : text(
                      language,
                      '只填本次授权目标、范围和执行模式。Provider/模型/密钥在设置页配置。',
                      'Enter authorized target, scope and run mode. Provider/model/keys live in Settings.',
                    )}
              </p>
              <Field
                label={t('target')}
                htmlFor="target"
                helper={text(language, '本次授权 Web/API 入口 URL。', 'Authorized Web/API entry URL.')}
              >
                <Input
                  id="target"
                  value={target}
                  onChange={(event) => setTarget(event.target.value)}
                  placeholder="http://127.0.0.1:18080"
                  required
                  className="h-10 rounded-xl font-mono"
                />
              </Field>
              <Field
                label={t('scope')}
                htmlFor="scope"
                helper={text(language, '授权边界，例如主机/域名/URL 前缀。', 'Authorization boundary: host / domain / URL prefix.')}
              >
                <Input
                  id="scope"
                  value={scope}
                  onChange={(event) => setScope(event.target.value)}
                  placeholder="127.0.0.1"
                  className="h-10 rounded-xl font-mono"
                />
              </Field>
              {createTab === 'schedule' ? (
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field
                    label={text(language, '名称', 'Name')}
                    htmlFor="schedule-name"
                    helper={text(language, '可选；缺省按目标自动生成。', 'Optional; defaults to target.')}
                  >
                    <Input
                      id="schedule-name"
                      value={scheduleName}
                      onChange={(event) => setScheduleName(event.target.value)}
                      placeholder={text(language, '夜间巡检', 'nightly scan')}
                      className="h-10 rounded-xl"
                    />
                  </Field>
                  <Field
                    label="Cron"
                    htmlFor="schedule-cron"
                    helper={text(language, '5 字段或 @every 1h / @daily。', '5-field cron, or @every 1h / @daily.')}
                  >
                    <Input
                      id="schedule-cron"
                      value={cron}
                      onChange={(event) => setCron(event.target.value)}
                      placeholder="0 * * * *"
                      required
                      className="h-10 rounded-xl font-mono"
                    />
                  </Field>
                </div>
              ) : (
                <div>
                  <div className="mb-2 flex items-center gap-2">
                    <Bot className="h-4 w-4 text-emerald-300" />
                    <span className="text-xs font-semibold">{text(language, 'Agent 模式', 'Agent mode')}</span>
                    <span className="font-mono text-[11px] text-muted-foreground">safe / HITL</span>
                  </div>
                  <div className="grid gap-2 sm:grid-cols-3">
                    <ModeButton
                      active={!multiAgent && !interactive}
                      icon={ShieldCheck}
                      label={text(language, '单 Agent', 'Single Agent')}
                      detail={text(language, '标准自主扫描', 'Autonomous')}
                      onClick={() => {
                        setMultiAgent(false)
                        setInteractive(false)
                      }}
                    />
                    <ModeButton
                      active={multiAgent}
                      icon={Sparkles}
                      label={t('multiAgent')}
                      detail={text(language, '编排扩展', 'Orchestration')}
                      onClick={() => {
                        setMultiAgent(true)
                        setInteractive(false)
                      }}
                    />
                    <ModeButton
                      active={interactive}
                      icon={Bot}
                      label={t('interactive')}
                      detail={text(language, '对话驾驶', 'Chat driven')}
                      onClick={() => {
                        setInteractive(true)
                        setMultiAgent(false)
                      }}
                    />
                  </div>
                </div>
              )}
              {error && (
                <div className="flex items-center gap-2 rounded-xl border border-red-400/20 bg-red-500/10 px-3 py-2 text-xs text-red-100">
                  <AlertTriangle className="h-4 w-4 shrink-0" />
                  <span>{error}</span>
                </div>
              )}
              <div className="grid gap-2 sm:grid-cols-[1fr_auto]">
                <Button
                  type="submit"
                  disabled={loading || !target.trim() || (createTab === 'schedule' && !cron.trim())}
                  className="h-11 rounded-xl text-sm"
                >
                  {createTab === 'schedule' ? (
                    <Clock className={cn('mr-2 h-4 w-4', loading && 'animate-pulse')} />
                  ) : (
                    <Sparkles className={cn('mr-2 h-4 w-4', loading && 'animate-pulse')} />
                  )}
                  {loading
                    ? text(language, '处理中...', 'Working...')
                    : createTab === 'schedule'
                      ? text(language, '创建定时任务', 'Create schedule')
                      : t('start')}
                </Button>
                {createTab === 'scan' && (
                  <Button type="button" variant="outline" onClick={handoffToAgent} disabled={!target.trim()} className="h-11 rounded-xl bg-transparent px-4 text-sm">
                    <Bot className="mr-2 h-4 w-4" />
                    {text(language, '交给 Agent', 'Ask Agent')}
                  </Button>
                )}
              </div>
            </form>
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
