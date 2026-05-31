import { useCallback, useEffect, useMemo, useState } from 'react'
import { Boxes, CheckCircle2, Network, Power, RefreshCcw, ShieldAlert, Wrench } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState, MetricCard, PageDescription, PageHeader, PageTitle, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import { type AgentMcpRegistryView, type AgentMcpServerConfig } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

export function McpPage() {
  const { client, language } = useConsole()
  const [data, setData] = useState<AgentMcpRegistryView | null>(null)
  const [loading, setLoading] = useState(true)
  const [savingId, setSavingId] = useState('')
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setData(await client.getAgentMcpRegistry())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    } finally {
      setLoading(false)
    }
  }, [client])

  useEffect(() => {
    refresh()
  }, [refresh])

  const metrics = useMemo(() => ({
    total: data?.count || 0,
    enabled: data?.enabled_count || 0,
    capabilities: data?.tool_capabilities.length || 0,
  }), [data])

  const toggle = async (server: AgentMcpServerConfig) => {
    setSavingId(server.id)
    setError('')
    try {
      await client.setAgentMcpServerEnabled(server.id, !server.enabled)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    } finally {
      setSavingId('')
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader>
        <div>
          <PageTitle className="flex items-center gap-3">
            <Network className="h-8 w-8 text-primary" />
            MCP
          </PageTitle>
          <PageDescription>
            {text(language, 'MCP 用来接入外部工具，但不会绕过 CyberAgent 的权限、策略和审批。预置项默认禁用，启用后才会作为 ToolCapability 注入 Agent。', 'MCP connects external tools, but never bypasses CyberAgent permissions, policy, or approvals. Presets are disabled by default and only become ToolCapabilities after enabling.')}
          </PageDescription>
        </div>
        <Button variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
          <RefreshCcw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          {text(language, '刷新', 'Refresh')}
        </Button>
      </PageHeader>

      <div className="grid gap-3 md:grid-cols-3">
        <MetricCard icon={Network} label="MCP Servers" value={metrics.total} tone="blue" />
        <MetricCard icon={Power} label={text(language, '已启用', 'Enabled')} value={metrics.enabled} tone={metrics.enabled ? 'emerald' : 'amber'} />
        <MetricCard icon={Boxes} label={text(language, '注入能力', 'Injected capabilities')} value={metrics.capabilities} tone="cyan" />
      </div>

      {error && <Section className="border-red-400/25 text-sm text-red-200">{error}</Section>}

      <div className="grid gap-4 xl:grid-cols-[1fr_0.8fr]">
        <Section>
          <SectionHeader>
            <SectionTitle icon={Wrench}>{text(language, 'MCP Presets / Servers', 'MCP Presets / Servers')}</SectionTitle>
          </SectionHeader>
          {!data?.servers.length && !loading ? (
            <EmptyState>{text(language, '暂无 MCP Server', 'No MCP servers')}</EmptyState>
          ) : (
            <div className="grid gap-3">
              {(data?.servers || []).map((server) => (
                <div key={server.id} className={cn('rounded-2xl border p-4 transition', server.enabled ? 'border-emerald-400/25 bg-emerald-400/[0.045]' : 'border-white/10 bg-white/[0.035]')}>
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="font-semibold">{server.name}</h3>
                        {server.preset && <Badge variant="secondary" className="rounded-xl">preset</Badge>}
                        <Badge variant={server.enabled ? 'low' : 'outline'} className="rounded-xl">{server.enabled ? 'enabled' : 'disabled'}</Badge>
                      </div>
                      <div className="mt-1 font-mono text-xs text-muted-foreground">{server.id}</div>
                    </div>
                    <Button
                      type="button"
                      variant={server.enabled ? 'outline' : 'default'}
                      className="rounded-2xl"
                      disabled={savingId === server.id}
                      onClick={() => toggle(server)}
                    >
                      <Power className="mr-2 h-4 w-4" />
                      {server.enabled ? text(language, '禁用', 'Disable') : text(language, '启用', 'Enable')}
                    </Button>
                  </div>

                  <p className="mt-3 text-sm leading-6 text-muted-foreground">{server.description || '-'}</p>
                  <div className="mt-3 grid gap-2 text-xs text-muted-foreground sm:grid-cols-3">
                    <Info label="transport" value={server.transport} />
                    <Info label="command/url" value={server.command || server.url || '-'} />
                    <Info label="status" value={server.status || '-'} />
                  </div>
                  {!!server.tools?.length && (
                    <div className="mt-3 flex flex-wrap gap-1.5">
                      {server.tools.map((tool) => (
                        <Badge key={tool.name} variant="outline" className="rounded-xl text-[10px]">
                          {tool.name}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </Section>

        <Section>
          <SectionHeader>
            <SectionTitle icon={CheckCircle2}>{text(language, 'Active ToolCapabilities', 'Active ToolCapabilities')}</SectionTitle>
          </SectionHeader>
          {!data?.tool_capabilities.length ? (
            <EmptyState className="min-h-[12rem]">
              <div className="px-4 text-center">
                <ShieldAlert className="mx-auto mb-3 h-6 w-6 text-amber-300" />
                {text(language, '当前没有启用的 MCP 能力。启用 preset 后，能力会进入 Agent planner，但仍需要权限和审批。', 'No enabled MCP capability yet. Enable a preset to inject capabilities into the Agent planner; permissions and approvals still apply.')}
              </div>
            </EmptyState>
          ) : (
            <div className="grid gap-3">
              {data.tool_capabilities.map((tool) => (
                <div key={tool.name} className="rounded-2xl border border-white/10 bg-black/15 p-4">
                  <div className="font-mono text-xs text-primary">{tool.name}</div>
                  <p className="mt-2 text-xs leading-5 text-muted-foreground">{tool.description}</p>
                  <div className="mt-3 flex flex-wrap gap-1.5">
                    <Badge variant="secondary" className="rounded-xl text-[10px]">{tool.required_level}</Badge>
                    <Badge variant="outline" className="rounded-xl text-[10px]">{tool.risk}</Badge>
                    {tool.intent_tags?.slice(0, 4).map((tag) => <Badge key={tag} variant="outline" className="rounded-xl text-[10px]">{tag}</Badge>)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </Section>
      </div>
    </div>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-white/10 bg-black/10 px-3 py-2">
      <div className="font-mono text-[10px] uppercase tracking-[0.18em] text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-foreground/85">{value}</div>
    </div>
  )
}
