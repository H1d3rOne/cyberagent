import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  Boxes,
  Clock,
  Layers3,
  Network,
  Power,
  RefreshCcw,
  ShieldAlert,
  ShieldCheck,
  Wrench,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState, MetricCard, PageDescription, PageHeader, PageTitle, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import {
  normalizeAgentOperationLevel,
  type AgentCapabilitiesResponse,
  type AgentMcpRegistryView,
  type AgentOperationLevel,
  type AgentSkillRegistryResponse,
  type Schedule,
} from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

const OPERATION_LEVELS: AgentOperationLevel[] = ['read_only', 'workspace', 'workspace_write', 'safe_scan', 'approval_ops', 'advanced', 'bypass']

type RegistryRowKind = 'tool_capability' | 'security_tool' | 'mcp_capability'

interface RegistryToolRow {
  name: string
  kind: RegistryRowKind
  source: string
  description?: string
  risk?: string
  requiredLevel?: string
  autoRun?: boolean
  tags?: string[]
  path?: string
}

interface RegistrySourceGroup {
  source: string
  rows: RegistryToolRow[]
}

export function OpsObservabilityPage() {
  const { client, language, settings } = useConsole()
  const [skills, setSkills] = useState<AgentSkillRegistryResponse | null>(null)
  const [capabilities, setCapabilities] = useState<AgentCapabilitiesResponse | null>(null)
  const [mcp, setMcp] = useState<AgentMcpRegistryView | null>(null)
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    const [skillsResult, capabilitiesResult, mcpResult, schedulesResult] = await Promise.allSettled([
      client.getAgentSkills(language),
      client.getAgentCapabilities(language),
      client.getAgentMcpRegistry(),
      client.listSchedules(),
    ])

    const failures: string[] = []
    if (skillsResult.status === 'fulfilled') {
      setSkills(skillsResult.value)
    } else {
      failures.push(`skills: ${errorMessage(skillsResult.reason)}`)
    }
    if (capabilitiesResult.status === 'fulfilled') {
      setCapabilities(capabilitiesResult.value)
    } else {
      failures.push(`capabilities: ${errorMessage(capabilitiesResult.reason)}`)
    }
    if (mcpResult.status === 'fulfilled') {
      setMcp(mcpResult.value)
    } else {
      failures.push(`mcp: ${errorMessage(mcpResult.reason)}`)
    }
    if (schedulesResult.status === 'fulfilled') {
      setSchedules(schedulesResult.value.schedules || [])
    } else {
      failures.push(`schedules: ${errorMessage(schedulesResult.reason)}`)
    }
    if (failures.length) setError(failures.join(' · '))
    setLastRefresh(new Date())
    setLoading(false)
  }, [client, language])

  useEffect(() => {
    refresh()
    const id = window.setInterval(refresh, 15_000)
    return () => window.clearInterval(id)
  }, [refresh])

  const registryTools = useMemo(() => collectRegistryTools(skills, mcp), [skills, mcp])
  const sourceGroups = useMemo(() => groupBySource(registryTools), [registryTools])
  const gateGroups = useMemo(() => groupByRequiredLevel(registryTools), [registryTools])

  const metrics = useMemo(() => {
    const activeSchedules = schedules.filter((schedule) => schedule.active ?? schedule.enabled).length
    const inFlightSchedules = schedules.filter((schedule) => schedule.in_flight).length
    const failedSchedules = schedules.filter((schedule) => schedule.last_error || lower(schedule.last_status).includes('fail')).length
    const blockedTools = registryTools.filter((tool) => !agentOperationLevelAtLeast(settings.operationLevel, tool.requiredLevel)).length
    return {
      tools: registryTools.length,
      sources: sourceGroups.length,
      mcpEnabled: mcp?.enabled_count || 0,
      mcpTotal: mcp?.count || 0,
      schedules: schedules.length,
      activeSchedules,
      inFlightSchedules,
      failedSchedules,
      blockedTools,
      builtinTools: capabilities?.builtin_tools.length || 0,
      highestRequiredLevel: highestRequiredLevel(registryTools),
    }
  }, [capabilities, mcp, registryTools, schedules, settings.operationLevel, sourceGroups.length])

  return (
    <div className="space-y-5">
      <PageHeader>
        <div>
          <PageTitle className="flex items-center gap-3">
            <Activity className="h-8 w-8 text-primary" />
            {text(language, 'Operations Observability', 'Operations Observability')}
          </PageTitle>
          <PageDescription>
            {text(
              language,
              '集中查看 Tool Registry 来源、当前操作权限 gate、MCP 注入能力与 Scheduler job 状态。数据来自 daemon 实时 API，页面每 15 秒自动刷新。',
              'View Tool Registry sources, operation-level gates, MCP injected capabilities and scheduler job state in one place. Data comes from live daemon APIs and refreshes every 15 seconds.',
            )}
          </PageDescription>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <span className="rounded-2xl border border-white/10 bg-white/[0.04] px-3 py-2 font-mono text-xs text-muted-foreground">
            {lastRefresh ? formatDate(lastRefresh.toISOString()) : text(language, '尚未刷新', 'Not refreshed')}
          </span>
          <Button variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
            <RefreshCcw className={cn('mr-2 h-4 w-4', loading && 'animate-spin')} />
            {text(language, '刷新', 'Refresh')}
          </Button>
        </div>
      </PageHeader>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard icon={Wrench} label={text(language, 'Registry 工具', 'Registry tools')} value={metrics.tools} tone="cyan" />
        <MetricCard icon={Network} label="MCP" value={`${metrics.mcpEnabled}/${metrics.mcpTotal}`} tone={metrics.mcpEnabled ? 'emerald' : 'amber'} />
        <MetricCard icon={Clock} label={text(language, '活跃计划', 'Active schedules')} value={`${metrics.activeSchedules}/${metrics.schedules}`} tone={metrics.inFlightSchedules ? 'cyan' : 'blue'} />
        <MetricCard icon={ShieldCheck} label={text(language, '当前权限', 'Current level')} value={settings.operationLevel} tone={metrics.blockedTools ? 'amber' : 'emerald'} />
      </div>

      {error && (
        <Section className="border-amber-400/25 text-sm text-amber-100">
          <div className="flex gap-2">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>{text(language, '部分观测数据刷新失败：', 'Some observability data failed to refresh: ')}{error}</span>
          </div>
        </Section>
      )}

      <Section>
        <SectionHeader>
          <SectionTitle icon={Layers3}>{text(language, 'Runtime data sources', 'Runtime data sources')}</SectionTitle>
        </SectionHeader>
        <div className="grid gap-3 text-sm md:grid-cols-2 xl:grid-cols-4">
          <InfoTile label="Tool Registry" value="/api/v1/agent/skills" detail={`${metrics.sources} sources · ${metrics.tools} tools`} />
          <InfoTile label="MCP Registry" value="/api/v1/agent/mcp" detail={`${mcp?.enabled_count || 0} enabled · ${(mcp?.tool_capabilities || []).length} injected`} />
          <InfoTile label="Scheduler" value="/api/v1/schedules" detail={`${metrics.inFlightSchedules} in flight · ${metrics.failedSchedules} failed/error`} />
          <InfoTile label="Capabilities" value={capabilities?.api_version || '—'} detail={`${metrics.builtinTools} builtin tools · highest gate ${metrics.highestRequiredLevel}`} />
        </div>
      </Section>

      <div className="grid gap-4 xl:grid-cols-[1.05fr_0.95fr]">
        <Section>
          <SectionHeader>
            <SectionTitle icon={Boxes}>{text(language, 'Tool Registry sources', 'Tool Registry sources')}</SectionTitle>
            <Badge variant="outline" className="rounded-xl">{metrics.sources}</Badge>
          </SectionHeader>
          {!sourceGroups.length && !loading ? (
            <EmptyState>{text(language, '暂无 registry 工具来源', 'No registry sources')}</EmptyState>
          ) : (
            <div className="grid gap-3">
              {sourceGroups.map((group) => (
                <RegistrySourceBlock key={group.source} group={group} language={language} />
              ))}
            </div>
          )}
        </Section>

        <Section>
          <SectionHeader>
            <SectionTitle icon={ShieldAlert}>{text(language, 'Operation level gate', 'Operation level gate')}</SectionTitle>
            <Badge variant={metrics.blockedTools ? 'medium' : 'low'} className="rounded-xl">
              {metrics.blockedTools ? text(language, `${metrics.blockedTools} 个受限`, `${metrics.blockedTools} blocked`) : text(language, '全部满足', 'all allowed')}
            </Badge>
          </SectionHeader>
          <div className="mb-4 rounded-2xl border border-white/10 bg-black/15 p-4">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="text-muted-foreground">{text(language, 'Console 当前配置：', 'Current console setting:')}</span>
              <Badge variant="secondary" className="rounded-xl font-mono">{settings.operationLevel}</Badge>
              <span className="text-muted-foreground">{text(language, '最高工具需求：', 'Highest required:')}</span>
              <Badge variant="outline" className="rounded-xl font-mono">{metrics.highestRequiredLevel}</Badge>
            </div>
            <p className="mt-3 text-xs leading-5 text-muted-foreground">
              {text(
                language,
                '这是前端基于 required_level 的可视化预检；实际执行仍以 daemon 的权限检查、scope 策略和审批结果为准。',
                'This is a frontend preflight view based on required_level; actual execution is still enforced by daemon permission checks, scope policy and approval decisions.',
              )}
            </p>
          </div>
          {!gateGroups.length ? (
            <EmptyState>{text(language, '暂无 gate 数据', 'No gate data')}</EmptyState>
          ) : (
            <div className="grid gap-3">
              {gateGroups.map(({ level, rows }) => {
                const allowed = agentOperationLevelAtLeast(settings.operationLevel, level)
                return (
                  <div key={level} className={cn('rounded-2xl border p-4', allowed ? 'border-emerald-400/20 bg-emerald-400/[0.035]' : 'border-amber-400/25 bg-amber-400/[0.045]')}>
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="font-mono text-xs text-primary">{level}</div>
                      <div className="flex items-center gap-2">
                        <Badge variant={allowed ? 'low' : 'medium'} className="rounded-xl">{allowed ? text(language, '可执行', 'allowed') : text(language, '需要提升', 'raise level')}</Badge>
                        <Badge variant="outline" className="rounded-xl">{rows.length}</Badge>
                      </div>
                    </div>
                    <div className="mt-3 flex flex-wrap gap-1.5">
                      {rows.slice(0, 8).map((tool) => (
                        <Badge key={`${tool.kind}:${tool.source}:${tool.name}`} variant="outline" className="rounded-xl font-mono text-[10px]">
                          {tool.name}
                        </Badge>
                      ))}
                      {rows.length > 8 && <Badge variant="secondary" className="rounded-xl text-[10px]">+{rows.length - 8}</Badge>}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </Section>
      </div>

      <div className="grid gap-4 xl:grid-cols-[0.95fr_1.05fr]">
        <Section>
          <SectionHeader>
            <SectionTitle icon={Network}>{text(language, 'MCP observability', 'MCP observability')}</SectionTitle>
            <Badge variant={mcp?.enabled_count ? 'low' : 'outline'} className="rounded-xl">{mcp?.enabled_count || 0} / {mcp?.count || 0}</Badge>
          </SectionHeader>
          {!mcp?.servers.length && !loading ? (
            <EmptyState>{text(language, '暂无 MCP servers', 'No MCP servers')}</EmptyState>
          ) : (
            <div className="grid gap-3">
              {(mcp?.servers || []).map((server) => (
                <div key={server.id} className={cn('rounded-2xl border p-4', server.enabled ? 'border-emerald-400/20 bg-emerald-400/[0.035]' : 'border-white/10 bg-white/[0.03]')}>
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="truncate font-semibold">{server.name}</h3>
                        {server.preset && <Badge variant="secondary" className="rounded-xl">preset</Badge>}
                        <Badge variant={server.enabled ? 'low' : 'outline'} className="rounded-xl">{server.enabled ? 'enabled' : 'disabled'}</Badge>
                      </div>
                      <div className="mt-1 font-mono text-[10px] text-muted-foreground">{server.id}</div>
                    </div>
                    <Power className={cn('h-4 w-4', server.enabled ? 'text-emerald-300' : 'text-muted-foreground')} />
                  </div>
                  <div className="mt-3 grid gap-2 text-xs text-muted-foreground sm:grid-cols-3">
                    <MiniInfo label="transport" value={server.transport || '—'} />
                    <MiniInfo label="status" value={server.status || '—'} />
                    <MiniInfo label="tools" value={String(server.tools?.length || 0)} />
                  </div>
                  {server.last_error && <div className="mt-3 truncate rounded-xl border border-red-400/20 bg-red-400/10 px-3 py-2 text-xs text-red-100">{server.last_error}</div>}
                  {server.last_sync_at && <div className="mt-3 font-mono text-[10px] text-muted-foreground">last_sync_at · {formatDate(server.last_sync_at)}</div>}
                </div>
              ))}
            </div>
          )}
          {!!mcp?.tool_capabilities.length && (
            <div className="mt-4 rounded-2xl border border-white/10 bg-black/15 p-4">
              <div className="mb-3 text-xs uppercase tracking-[0.22em] text-muted-foreground">{text(language, 'Injected capabilities', 'Injected capabilities')}</div>
              <div className="flex flex-wrap gap-1.5">
                {mcp.tool_capabilities.map((tool) => (
                  <Badge key={`${tool.source}:${tool.name}`} variant="secondary" className="rounded-xl font-mono text-[10px]">
                    {tool.name}
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </Section>

        <Section>
          <SectionHeader>
            <SectionTitle icon={Clock}>{text(language, 'Scheduler job state', 'Scheduler job state')}</SectionTitle>
            <Badge variant={metrics.inFlightSchedules ? 'low' : 'outline'} className="rounded-xl">{metrics.inFlightSchedules} in_flight</Badge>
          </SectionHeader>
          {!schedules.length && !loading ? (
            <EmptyState>{text(language, '暂无调度计划', 'No schedules')}</EmptyState>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[860px] text-sm">
                <thead className="text-xs uppercase text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 text-left">{text(language, '名称', 'Name')}</th>
                    <th className="px-3 py-2 text-left">ops_level</th>
                    <th className="px-3 py-2 text-left">Active</th>
                    <th className="px-3 py-2 text-left">last_status</th>
                    <th className="px-3 py-2 text-left">last_job</th>
                    <th className="px-3 py-2 text-left">next_run_at</th>
                  </tr>
                </thead>
                <tbody>
                  {schedules.map((schedule) => {
                    const active = schedule.active ?? schedule.enabled
                    const statusTone = schedule.last_error || lower(schedule.last_status).includes('fail') ? 'text-red-200' : schedule.in_flight ? 'text-cyan-200' : 'text-muted-foreground'
                    return (
                      <tr key={schedule.id} className="border-t border-white/5">
                        <td className="px-3 py-3">
                          <div className="font-medium">{schedule.name || schedule.id}</div>
                          <div className="font-mono text-[10px] text-muted-foreground">{schedule.id}</div>
                        </td>
                        <td className="px-3 py-3 font-mono text-xs">{schedule.ops_level || 'read_only'}</td>
                        <td className="px-3 py-3">
                          <div className="flex flex-wrap gap-1.5">
                            <Badge variant={active ? 'low' : 'outline'} className="rounded-xl">{active ? 'enabled' : 'disabled'}</Badge>
                            {schedule.in_flight && <Badge variant="secondary" className="rounded-xl">{text(language, '运行中', 'in flight')}</Badge>}
                          </div>
                        </td>
                        <td className={cn('max-w-[260px] px-3 py-3 text-xs', statusTone)}>
                          <div>{schedule.last_status || '—'}{schedule.last_run_at ? ` · ${formatDate(schedule.last_run_at)}` : ''}</div>
                          {schedule.last_error && <div className="mt-1 truncate text-red-200">{schedule.last_error}</div>}
                        </td>
                        <td className="px-3 py-3 font-mono text-xs text-muted-foreground">{schedule.last_job_id || schedule.last_message_id || '—'}</td>
                        <td className="px-3 py-3 font-mono text-xs text-muted-foreground">{formatDate(schedule.next_run_at)}</td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Section>
      </div>
    </div>
  )
}

function collectRegistryTools(skills: AgentSkillRegistryResponse | null, mcp: AgentMcpRegistryView | null): RegistryToolRow[] {
  const rows: RegistryToolRow[] = []
  const seen = new Set<string>()
  const push = (row: RegistryToolRow) => {
    const key = `${row.kind}:${row.source}:${row.name}`
    if (seen.has(key)) return
    seen.add(key)
    rows.push(row)
  }

  for (const tool of skills?.tool_capabilities || []) {
    push({
      name: tool.name,
      kind: 'tool_capability',
      source: tool.source || 'registry',
      description: tool.description,
      risk: tool.risk,
      requiredLevel: tool.required_level,
      autoRun: tool.auto_run,
      tags: tool.intent_tags,
      path: tool.catalog_path,
    })
  }
  for (const tool of skills?.security_tools || []) {
    push({
      name: tool.name,
      kind: 'security_tool',
      source: tool.source || 'security_tools',
      risk: tool.risk,
      requiredLevel: tool.required_level,
      autoRun: tool.auto_run,
      tags: tool.intent_tags,
      path: tool.path || tool.plugin_path,
    })
  }
  for (const tool of mcp?.tool_capabilities || []) {
    push({
      name: tool.name,
      kind: 'mcp_capability',
      source: tool.source || 'mcp',
      description: tool.description,
      risk: tool.risk,
      requiredLevel: tool.required_level,
      autoRun: tool.auto_run,
      tags: tool.intent_tags,
      path: tool.catalog_path,
    })
  }

  return rows.sort((a, b) => `${a.source}:${a.name}`.localeCompare(`${b.source}:${b.name}`))
}

function groupBySource(rows: RegistryToolRow[]): RegistrySourceGroup[] {
  const groups = new Map<string, RegistryToolRow[]>()
  for (const row of rows) {
    const source = row.source || 'unknown'
    groups.set(source, [...(groups.get(source) || []), row])
  }
  return Array.from(groups.entries())
    .map(([source, groupRows]) => ({ source, rows: groupRows }))
    .sort((a, b) => b.rows.length - a.rows.length || a.source.localeCompare(b.source))
}

function groupByRequiredLevel(rows: RegistryToolRow[]) {
  const groups = new Map<string, RegistryToolRow[]>()
  for (const row of rows) {
    const level = row.requiredLevel || 'read_only'
    groups.set(level, [...(groups.get(level) || []), row])
  }
  const known = OPERATION_LEVELS.filter((level) => groups.has(level)).map((level) => ({ level, rows: groups.get(level) || [] }))
  const unknown = Array.from(groups.entries())
    .filter(([level]) => !OPERATION_LEVELS.includes(level as AgentOperationLevel))
    .map(([level, groupRows]) => ({ level, rows: groupRows }))
    .sort((a, b) => a.level.localeCompare(b.level))
  return [...known, ...unknown]
}

function RegistrySourceBlock({ group, language }: { group: RegistrySourceGroup; language: 'zh' | 'en' }) {
  const levels = unique(group.rows.map((row) => row.requiredLevel || 'read_only')).sort(sortLevels)
  const risks = unique(group.rows.map((row) => row.risk || 'unspecified'))
  return (
    <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-mono text-xs text-primary">{group.source}</div>
          <div className="mt-1 text-sm text-muted-foreground">{text(language, `${group.rows.length} 个 registry 条目`, `${group.rows.length} registry entries`)}</div>
        </div>
        <div className="flex flex-wrap justify-end gap-1.5">
          {levels.map((level) => <Badge key={level} variant="secondary" className="rounded-xl font-mono text-[10px]">{level}</Badge>)}
          {risks.slice(0, 3).map((risk) => <Badge key={risk} variant={riskVariant(risk)} className="rounded-xl text-[10px]">{risk}</Badge>)}
        </div>
      </div>
      <div className="mt-3 grid gap-2">
        {group.rows.slice(0, 6).map((row) => (
          <div key={`${row.kind}:${row.source}:${row.name}`} className="rounded-xl border border-white/5 bg-black/15 px-3 py-2">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0 truncate font-mono text-xs">{row.name}</div>
              <Badge variant="outline" className="rounded-xl text-[10px]">{kindLabel(row.kind, language)}</Badge>
            </div>
            <div className="mt-1 flex flex-wrap gap-1.5 text-[10px] text-muted-foreground">
              <span>{row.requiredLevel || 'read_only'}</span>
              <span>·</span>
              <span>{row.autoRun ? 'auto_run' : 'manual/approval'}</span>
              {row.path && (
                <>
                  <span>·</span>
                  <span className="max-w-[260px] truncate">{row.path}</span>
                </>
              )}
            </div>
          </div>
        ))}
        {group.rows.length > 6 && <div className="text-xs text-muted-foreground">+{group.rows.length - 6} more</div>}
      </div>
    </div>
  )
}

function InfoTile({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div className="rounded-2xl border border-white/10 bg-black/15 p-4">
      <div className="text-xs uppercase tracking-[0.22em] text-muted-foreground">{label}</div>
      <div className="mt-2 truncate font-mono text-sm text-foreground">{value}</div>
      <div className="mt-2 text-xs text-muted-foreground">{detail}</div>
    </div>
  )
}

function MiniInfo({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-white/10 bg-black/10 px-3 py-2">
      <div className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-foreground/85">{value}</div>
    </div>
  )
}

function highestRequiredLevel(rows: RegistryToolRow[]) {
  if (!rows.length) return 'read_only'
  return rows.reduce((highest, row) => {
    const level = normalizeAgentOperationLevel(row.requiredLevel)
    return operationLevelRank(level) > operationLevelRank(highest) ? level : highest
  }, 'read_only' as AgentOperationLevel)
}

function agentOperationLevelAtLeast(current: unknown, required: unknown) {
  return operationLevelRank(normalizeAgentOperationLevel(current)) >= operationLevelRank(normalizeAgentOperationLevel(required))
}

function operationLevelRank(level: AgentOperationLevel) {
  return OPERATION_LEVELS.indexOf(level)
}

function sortLevels(a: string, b: string) {
  return operationLevelRank(normalizeAgentOperationLevel(a)) - operationLevelRank(normalizeAgentOperationLevel(b)) || a.localeCompare(b)
}

function riskVariant(risk: string) {
  const normalized = lower(risk)
  if (normalized === 'critical') return 'critical'
  if (normalized === 'high') return 'high'
  if (normalized === 'medium') return 'medium'
  if (normalized === 'low') return 'low'
  if (normalized === 'info') return 'info'
  return 'outline'
}

function kindLabel(kind: RegistryRowKind, language: 'zh' | 'en') {
  if (kind === 'mcp_capability') return text(language, 'MCP 注入', 'MCP')
  if (kind === 'security_tool') return text(language, '安全工具', 'security')
  return text(language, '工具能力', 'tool')
}

function unique(values: string[]) {
  return Array.from(new Set(values.filter((value) => value.trim())))
}

function lower(value: unknown) {
  return String(value || '').toLowerCase()
}

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function errorMessage(value: unknown) {
  return value instanceof Error ? value.message : String(value || 'request failed')
}
