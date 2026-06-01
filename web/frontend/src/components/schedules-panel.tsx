import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { Clock, Pause, Play, Plus, RefreshCcw, ShieldCheck, Trash2, Zap } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { EmptyState, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import { type AgentSessionSummary, type Schedule, type ScheduleInput } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

interface DraftState extends ScheduleInput {
  id?: string
  prompt: string
  ops_level: string
  active: boolean
}

const emptyDraft: DraftState = {
  name: '',
  cron: '0 * * * *',
  action: 'agent_prompt',
  enabled: true,
  active: true,
  session_id: '',
  operator: 'scheduler',
  language: 'zh',
  prompt: '',
  target: '',
  scope: '',
  profile: '',
  ops_level: 'read_only',
}

export function SchedulesPanel({ hideCreate = false }: { hideCreate?: boolean } = {}) {
  const { client, language, settings } = useConsole()
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [sessions, setSessions] = useState<AgentSessionSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [draft, setDraft] = useState<DraftState | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [sched, sess] = await Promise.all([
        client.listSchedules(),
        client.listAgentSessions(50).catch(() => ({ items: [] as AgentSessionSummary[], count: 0, limit: 50 })),
      ])
      setSchedules(sched.schedules || [])
      setSessions(sess.items || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    } finally {
      setLoading(false)
    }
  }, [client])

  useEffect(() => {
    refresh()
    const id = window.setInterval(refresh, 15_000)
    return () => window.clearInterval(id)
  }, [refresh])

  const startCreate = () => setDraft({ ...emptyDraft, language, enabled: true, active: true })
  const startEdit = (s: Schedule) =>
    setDraft({
      id: s.id,
      name: s.name,
      cron: s.cron,
      action: s.action,
      enabled: s.enabled,
      active: s.active ?? s.enabled,
      session_id: s.session_id || '',
      operator: s.operator || 'scheduler',
      language: s.language || language,
      prompt: s.prompt || '',
      target: s.target || '',
      scope: s.scope || '',
      profile: s.profile || '',
      ops_level: s.ops_level || settings.operationLevel || 'read_only',
    })

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!draft) return
    setSubmitting(true)
    setError('')
    try {
      const body: ScheduleInput = {
        name: draft.name?.trim() || undefined,
        cron: draft.cron,
        action: 'agent_prompt',
        enabled: draft.active,
        active: draft.active,
        session_id: draft.session_id || sessions[0]?.session_id || undefined,
        operator: draft.operator || 'scheduler',
        language: draft.language || language,
        prompt: draft.prompt.trim(),
        target: draft.target || undefined,
        scope: draft.scope || undefined,
        profile: draft.profile || undefined,
        ops_level: draft.ops_level || settings.operationLevel || 'read_only',
      }
      if (draft.id) {
        await client.updateSchedule(draft.id, body)
      } else {
        await client.createSchedule(body)
      }
      setDraft(null)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'submit failed')
    } finally {
      setSubmitting(false)
    }
  }

  const toggle = async (s: Schedule) => {
    try {
      if (s.enabled) await client.disableSchedule(s.id)
      else await client.enableSchedule(s.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'toggle failed')
    }
  }

  const runNow = async (s: Schedule) => {
    try {
      await client.runScheduleNow(s.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'run failed')
    }
  }

  const remove = async (s: Schedule) => {
    if (!confirm(text(language, `删除计划 ${s.name}?`, `Delete schedule ${s.name}?`))) return
    try {
      await client.deleteSchedule(s.id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'delete failed')
    }
  }

  const unifiedSummary = useMemo(() => {
    const active = schedules.filter((schedule) => schedule.active ?? schedule.enabled).length
    return { total: schedules.length, active }
  }, [schedules])

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {text(
            language,
            '统一调度入口：只填写 Cron、Prompt、权限级别和启用状态；daemon 负责把它转成实际任务。',
            'Unified scheduling: fill Cron, prompt, permission level and active state only; the daemon turns it into a real task.',
          )}
        </p>
        <div className="flex gap-2">
          <Button variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
            <RefreshCcw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            {text(language, '刷新', 'Refresh')}
          </Button>
          {!hideCreate && (
            <Button onClick={startCreate} className="rounded-2xl">
              <Plus className="mr-2 h-4 w-4" />
              {text(language, '新建', 'New')}
            </Button>
          )}
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Metric label={text(language, '总计划', 'Total schedules')} value={unifiedSummary.total} tone="blue" />
        <Metric label={text(language, '启用中', 'Active')} value={unifiedSummary.active} tone="emerald" />
        <Metric label={text(language, '会话', 'Sessions')} value={sessions.length} tone="cyan" />
        <Metric label={text(language, '权限级别', 'Permission')} value={settings.operationLevel} tone="amber" />
      </div>

      {error && <Section className="border-red-400/25 text-sm text-red-200">{error}</Section>}

      {draft && (
        <Section>
          <SectionHeader>
            <SectionTitle icon={Clock}>
              {draft.id ? text(language, '编辑计划', 'Edit schedule') : text(language, '新建计划', 'New schedule')}
            </SectionTitle>
          </SectionHeader>
          <form onSubmit={submit} className="grid gap-3 md:grid-cols-2">
            <Field label={text(language, '名称', 'Name')}>
              <Input value={draft.name || ''} onChange={(e) => setDraft({ ...draft, name: e.target.value })} placeholder={text(language, '夜间巡检', 'nightly scan')} />
            </Field>
            <Field label="Cron">
              <Input value={draft.cron} onChange={(e) => setDraft({ ...draft, cron: e.target.value })} placeholder="0 * * * *" required className="font-mono" />
            </Field>
            <Field label={text(language, '权限级别', 'Permission level')}>
              <select
                value={draft.ops_level}
                onChange={(e) => setDraft({ ...draft, ops_level: e.target.value })}
                className="h-10 w-full rounded-xl border border-white/10 bg-white/[0.04] px-3 text-sm"
              >
                {['read_only', 'workspace', 'safe_scan', 'approval_ops', 'advanced', 'bypass'].map((level) => (
                  <option key={level} value={level}>{level}</option>
                ))}
              </select>
            </Field>
            <Field label={text(language, '启用状态', 'Active')}>
              <label className="flex h-10 items-center gap-2 rounded-xl border border-white/10 bg-white/[0.04] px-3 text-sm">
                <input type="checkbox" checked={draft.active} onChange={(e) => setDraft({ ...draft, active: e.target.checked, enabled: e.target.checked })} />
                <span>{draft.active ? text(language, '启用', 'Enabled') : text(language, '停用', 'Disabled')}</span>
              </label>
            </Field>
            <Field label={text(language, 'Prompt', 'Prompt')} className="md:col-span-2">
              <Textarea value={draft.prompt} onChange={(e) => setDraft({ ...draft, prompt: e.target.value })} rows={5} required />
            </Field>
            <Field label={text(language, '目标', 'Target')}>
              <Input value={draft.target || ''} onChange={(e) => setDraft({ ...draft, target: e.target.value })} placeholder="https://example.com" />
            </Field>
            <Field label={text(language, '范围', 'Scope')}>
              <Input value={draft.scope || ''} onChange={(e) => setDraft({ ...draft, scope: e.target.value })} placeholder="example.com" />
            </Field>
            <Field label={text(language, 'Operator', 'Operator')}>
              <Input value={draft.operator || ''} onChange={(e) => setDraft({ ...draft, operator: e.target.value })} />
            </Field>
            <Field label={text(language, '语言', 'Language')}>
              <select
                value={draft.language || language}
                onChange={(e) => setDraft({ ...draft, language: e.target.value })}
                className="h-10 w-full rounded-xl border border-white/10 bg-white/[0.04] px-3 text-sm"
              >
                <option value="zh">中文</option>
                <option value="en">English</option>
              </select>
            </Field>
            <div className="flex gap-2 md:col-span-2">
              <Button type="submit" disabled={submitting} className="rounded-xl">
                {submitting ? text(language, '保存中…', 'Saving…') : text(language, '保存', 'Save')}
              </Button>
              <Button type="button" variant="outline" onClick={() => setDraft(null)} className="rounded-xl bg-transparent">
                {text(language, '取消', 'Cancel')}
              </Button>
            </div>
          </form>
        </Section>
      )}

      <Section>
        <SectionHeader>
          <SectionTitle icon={Clock}>{text(language, '已配置计划', 'Configured schedules')}</SectionTitle>
        </SectionHeader>
        {schedules.length === 0 ? (
          <EmptyState>{text(language, '暂无计划', 'No schedules yet')}</EmptyState>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="px-3 py-2 text-left">{text(language, '名称', 'Name')}</th>
                  <th className="px-3 py-2 text-left">Cron</th>
                  <th className="px-3 py-2 text-left">Prompt</th>
                  <th className="px-3 py-2 text-left">ops_level</th>
                  <th className="px-3 py-2 text-left">Active</th>
                  <th className="px-3 py-2 text-left">{text(language, '状态', 'Status')}</th>
                  <th className="px-3 py-2 text-left">{text(language, '上次结果', 'Last result')}</th>
                  <th className="px-3 py-2 text-right">{text(language, '操作', 'Actions')}</th>
                </tr>
              </thead>
              <tbody>
                {schedules.map((s) => {
                  const active = s.active ?? s.enabled
                  return (
                    <tr key={s.id} className="border-t border-white/5">
                      <td className="px-3 py-2">
                        <div className="font-medium">{s.name}</div>
                        <div className="font-mono text-[10px] text-muted-foreground">{s.id}</div>
                      </td>
                      <td className="px-3 py-2 font-mono">{s.cron}</td>
                      <td className="max-w-[340px] px-3 py-2 text-xs text-muted-foreground">
                        <div className="line-clamp-2">{s.prompt || '—'}</div>
                        {s.target ? <div className="mt-1 font-mono text-[10px]">{s.target}</div> : null}
                      </td>
                      <td className="px-3 py-2 font-mono text-xs">{s.ops_level || '—'}</td>
                      <td className="px-3 py-2">
                        <Badge variant={active ? 'default' : 'outline'}>{active ? text(language, '启用', 'Enabled') : text(language, '停用', 'Disabled')}</Badge>
                        {s.in_flight ? <Badge variant="outline" className="ml-1">{text(language, '运行中', 'In flight')}</Badge> : null}
                      </td>
                      <td className="px-3 py-2 text-xs text-muted-foreground">
                        {s.last_status ? `${s.last_status}${s.last_run_at ? ` · ${new Date(s.last_run_at).toLocaleString()}` : ''}` : '—'}
                        {s.last_error ? <div className="mt-1 max-w-[240px] truncate text-red-200">{s.last_error}</div> : null}
                      </td>
                      <td className="px-3 py-2 text-xs text-muted-foreground">{s.last_job_id || s.last_message_id || '—'}</td>
                      <td className="px-3 py-2">
                        <div className="flex justify-end gap-1">
                          <Button size="sm" variant="ghost" onClick={() => runNow(s)} title={text(language, '立即运行', 'Run now')}>
                            <Zap className="h-4 w-4" />
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => toggle(s)} title={active ? text(language, '停用', 'Disable') : text(language, '启用', 'Enable')}>
                            {active ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
                          </Button>
                          {!hideCreate && <Button size="sm" variant="ghost" onClick={() => startEdit(s)}>{text(language, '编辑', 'Edit')}</Button>}
                          <Button size="sm" variant="ghost" onClick={() => remove(s)}>
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </div>
  )
}

function Field({ label, children, className }: { label: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={cn('space-y-1.5', className)}>
      <Label>{label}</Label>
      {children}
    </div>
  )
}

function Metric({ label, value, tone }: { label: string; value: React.ReactNode; tone: 'blue' | 'cyan' | 'emerald' | 'amber' }) {
  const toneMap = {
    blue: 'text-blue-300 bg-blue-500/10 border-blue-500/20',
    cyan: 'text-cyan-300 bg-cyan-400/10 border-cyan-400/20',
    emerald: 'text-emerald-300 bg-emerald-400/10 border-emerald-400/20',
    amber: 'text-amber-300 bg-amber-400/10 border-amber-400/20',
  } as const
  return (
    <div className="glass-panel p-4">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span className={cn('grid h-9 w-9 place-items-center rounded-2xl border', toneMap[tone])}><ShieldCheck className="h-4 w-4" /></span>
      </div>
      <div className="mt-4 font-mono text-2xl font-semibold sm:text-3xl">{value}</div>
    </div>
  )
}
