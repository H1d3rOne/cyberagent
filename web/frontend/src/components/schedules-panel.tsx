import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { Clock, Pause, Play, Plus, RefreshCcw, Trash2, Zap } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { EmptyState, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import { type AgentSessionSummary, type Schedule, type ScheduleAction, type ScheduleInput } from '@/lib/api'
import { text, useConsole } from '@/lib/console'

type DraftState = ScheduleInput & { id?: string }

const emptyDraft: DraftState = {
  name: '',
  cron: '*/30 * * * *',
  action: 'agent_prompt',
  enabled: true,
  session_id: '',
  operator: 'scheduler',
  language: 'zh',
  prompt: '',
  target: '',
  scope: '',
  profile: '',
}

export function SchedulesPanel({ hideCreate = false }: { hideCreate?: boolean } = {}) {
  const { client, language } = useConsole()
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

  const startCreate = () => setDraft({ ...emptyDraft, language })
  const startEdit = (s: Schedule) =>
    setDraft({
      id: s.id,
      name: s.name,
      cron: s.cron,
      action: s.action,
      enabled: s.enabled,
      session_id: s.session_id || '',
      operator: s.operator || 'scheduler',
      language: s.language || language,
      prompt: s.prompt || '',
      target: s.target || '',
      scope: s.scope || '',
      profile: s.profile || '',
    })

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!draft) return
    setSubmitting(true)
    setError('')
    try {
      const body: ScheduleInput = {
        name: draft.name,
        cron: draft.cron,
        action: draft.action,
        enabled: draft.enabled,
        session_id: draft.action === 'agent_prompt' ? draft.session_id : undefined,
        operator: draft.action === 'agent_prompt' ? draft.operator : undefined,
        language: draft.action === 'agent_prompt' ? draft.language : undefined,
        prompt: draft.action === 'agent_prompt' ? draft.prompt : undefined,
        target: draft.action === 'scan_web' ? draft.target : undefined,
        scope: draft.action === 'scan_web' ? draft.scope : undefined,
        profile: draft.action === 'scan_web' ? draft.profile : undefined,
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

  const sessionOptions = useMemo(
    () =>
      sessions.map((s) => ({
        value: s.session_id,
        label: `${s.session_id} · ${s.operator || ''}`,
      })),
    [sessions],
  )

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {text(
            language,
            '按 Cron 表达式自动触发：向已存在的 Agent 会话发 prompt，或直接启动一次扫描任务。daemon 在则调度生效。',
            'Cron-driven triggers: send prompts to an existing agent session or start a scan job. Active while the daemon is running.',
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

      {error && <Section className="border-red-400/25 text-sm text-red-200">{error}</Section>}

      {draft && (
        <Section>
          <SectionHeader>
            <SectionTitle icon={Clock}>
              {draft.id ? text(language, '编辑计划', 'Edit schedule') : text(language, '新建计划', 'New schedule')}
            </SectionTitle>
          </SectionHeader>
          <form onSubmit={submit} className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1.5">
              <Label>{text(language, '名称', 'Name')}</Label>
              <Input value={draft.name || ''} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
            </div>
            <div className="space-y-1.5">
              <Label>Cron</Label>
              <Input
                value={draft.cron}
                onChange={(e) => setDraft({ ...draft, cron: e.target.value })}
                placeholder="*/30 * * * *"
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label>{text(language, '动作', 'Action')}</Label>
              <select
                value={draft.action}
                onChange={(e) => setDraft({ ...draft, action: e.target.value as ScheduleAction })}
                className="h-9 w-full rounded-md border border-white/10 bg-white/[0.04] px-3 text-sm"
              >
                <option value="agent_prompt">{text(language, '发送 Agent 提示', 'Send Agent prompt')}</option>
                <option value="scan_web">{text(language, '启动扫描任务', 'Start scan job')}</option>
              </select>
            </div>
            <div className="space-y-1.5">
              <Label>{text(language, '启用', 'Enabled')}</Label>
              <div className="flex h-9 items-center gap-2">
                <input
                  type="checkbox"
                  checked={draft.enabled ?? true}
                  onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
                />
                <span className="text-xs text-muted-foreground">
                  {text(language, '关闭后保留配置但不触发', 'Disabled keeps config but stops firing')}
                </span>
              </div>
            </div>

            {draft.action === 'agent_prompt' ? (
              <>
                <div className="space-y-1.5 md:col-span-2">
                  <Label>{text(language, '目标会话', 'Target session')}</Label>
                  <select
                    value={draft.session_id || ''}
                    onChange={(e) => setDraft({ ...draft, session_id: e.target.value })}
                    className="h-9 w-full rounded-md border border-white/10 bg-white/[0.04] px-3 text-sm"
                    required
                  >
                    <option value="">{text(language, '选择会话…', 'Select a session…')}</option>
                    {sessionOptions.map((s) => (
                      <option key={s.value} value={s.value}>
                        {s.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="space-y-1.5 md:col-span-2">
                  <Label>{text(language, '提示内容', 'Prompt')}</Label>
                  <Textarea
                    value={draft.prompt || ''}
                    onChange={(e) => setDraft({ ...draft, prompt: e.target.value })}
                    rows={4}
                    required
                  />
                </div>
              </>
            ) : (
              <>
                <div className="space-y-1.5">
                  <Label>Target</Label>
                  <Input
                    value={draft.target || ''}
                    onChange={(e) => setDraft({ ...draft, target: e.target.value })}
                    placeholder="https://example.com"
                    required
                  />
                </div>
                <div className="space-y-1.5">
                  <Label>Scope</Label>
                  <Input value={draft.scope || ''} onChange={(e) => setDraft({ ...draft, scope: e.target.value })} />
                </div>
                <div className="space-y-1.5 md:col-span-2">
                  <Label>Profile</Label>
                  <Input value={draft.profile || ''} onChange={(e) => setDraft({ ...draft, profile: e.target.value })} />
                </div>
              </>
            )}

            <div className="flex gap-2 md:col-span-2">
              <Button type="submit" disabled={submitting}>
                {submitting ? text(language, '保存中…', 'Saving…') : text(language, '保存', 'Save')}
              </Button>
              <Button type="button" variant="outline" onClick={() => setDraft(null)}>
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
                  <th className="px-3 py-2 text-left">{text(language, '动作', 'Action')}</th>
                  <th className="px-3 py-2 text-left">{text(language, '状态', 'Status')}</th>
                  <th className="px-3 py-2 text-left">{text(language, '下次运行', 'Next run')}</th>
                  <th className="px-3 py-2 text-left">{text(language, '上次结果', 'Last result')}</th>
                  <th className="px-3 py-2 text-right">{text(language, '操作', 'Actions')}</th>
                </tr>
              </thead>
              <tbody>
                {schedules.map((s) => (
                  <tr key={s.id} className="border-t border-white/5">
                    <td className="px-3 py-2">
                      <div className="font-medium">{s.name}</div>
                      <div className="font-mono text-[10px] text-muted-foreground">{s.id}</div>
                    </td>
                    <td className="px-3 py-2 font-mono">{s.cron}</td>
                    <td className="px-3 py-2">
                      {s.action === 'agent_prompt'
                        ? `${text(language, '会话', 'session')}: ${s.session_id || '—'}`
                        : `scan: ${s.target || '—'}`}
                    </td>
                    <td className="px-3 py-2">
                      <Badge variant={s.enabled ? 'default' : 'outline'}>
                        {s.enabled ? text(language, '启用', 'enabled') : text(language, '停用', 'disabled')}
                      </Badge>
                      {s.in_flight ? (
                        <Badge variant="outline" className="ml-1">
                          {text(language, '运行中', 'in-flight')}
                        </Badge>
                      ) : null}
                    </td>
                    <td className="px-3 py-2 text-xs text-muted-foreground">
                      {s.next_run_at ? new Date(s.next_run_at).toLocaleString() : '—'}
                    </td>
                    <td className="px-3 py-2 text-xs">
                      {s.last_status ? (
                        <span
                          className={
                            s.last_status === 'success'
                              ? 'text-emerald-300'
                              : s.last_status === 'skipped'
                                ? 'text-amber-300'
                                : 'text-red-300'
                          }
                        >
                          {s.last_status}
                          {s.last_run_at ? ` · ${new Date(s.last_run_at).toLocaleString()}` : ''}
                          {s.last_error ? ` · ${s.last_error}` : ''}
                        </span>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </td>
                    <td className="px-3 py-2">
                      <div className="flex justify-end gap-1">
                        <Button size="sm" variant="ghost" onClick={() => runNow(s)} title={text(language, '立即运行', 'Run now')}>
                          <Zap className="h-4 w-4" />
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => toggle(s)}>
                          {s.enabled ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => startEdit(s)}>
                          {text(language, '编辑', 'Edit')}
                        </Button>
                        <Button size="sm" variant="ghost" onClick={() => remove(s)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </div>
  )
}
