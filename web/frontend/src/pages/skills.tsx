import { useCallback, useEffect, useMemo, useState } from 'react'
import { BrainCircuit, Boxes, CheckCircle2, RefreshCcw, ShieldAlert, Sparkles, Wrench } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState, MetricCard, PageDescription, PageHeader, PageTitle, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import { type AgentSkillRegistryResponse } from '@/lib/api'
import { text, useConsole } from '@/lib/console'

export function SkillsPage() {
  const { client, language } = useConsole()
  const [data, setData] = useState<AgentSkillRegistryResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setData(await client.getAgentSkills(language))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    } finally {
      setLoading(false)
    }
  }, [client, language])

  useEffect(() => {
    refresh()
  }, [refresh])

  const metrics = useMemo(() => {
    const skills = data?.skills.length || 0
    const tools = data?.tool_capabilities.length || 0
    const securityTools = data?.security_tools?.length || 0
    const missingRequired = (data?.skills || []).filter((skill) =>
      (skill.required_tools || []).some((tool) => !(data?.tool_capabilities || []).some((cap) => cap.name === tool)),
    ).length
    return { skills, tools, securityTools, missingRequired }
  }, [data])

  return (
    <div className="space-y-5">
      <PageHeader>
        <div>
          <PageTitle className="flex items-center gap-3">
            <BrainCircuit className="h-8 w-8 text-primary" />
            Skills
          </PageTitle>
          <PageDescription>
            {text(language, 'Skills 描述 Agent “怎么做任务”，Tool Capability 描述当前“能调用什么”。两者共同驱动 Planner，而不是为每个任务硬编码。', 'Skills describe how the Agent should work; Tool Capabilities describe what can actually be called. Together they drive the planner without hard-coding every task.')}
          </PageDescription>
        </div>
        <Button variant="outline" onClick={refresh} className="rounded-2xl bg-transparent">
          <RefreshCcw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          {text(language, '刷新', 'Refresh')}
        </Button>
      </PageHeader>

      <div className="grid gap-3 md:grid-cols-4">
        <MetricCard icon={BrainCircuit} label="Skills" value={metrics.skills} tone="blue" />
        <MetricCard icon={Boxes} label={text(language, '工具能力', 'Tool capabilities')} value={metrics.tools} tone="cyan" />
        <MetricCard icon={Wrench} label={text(language, '安全工具', 'Security tools')} value={metrics.securityTools} tone="blue" />
        <MetricCard icon={ShieldAlert} label={text(language, '缺失工具的 Skill', 'Skills with missing tools')} value={metrics.missingRequired} tone={metrics.missingRequired ? 'amber' : 'emerald'} />
      </div>

      {error && <Section className="border-red-400/25 text-sm text-red-200">{error}</Section>}

      <div className="grid gap-4 xl:grid-cols-[1fr_0.9fr]">
        <Section>
          <SectionHeader>
            <SectionTitle icon={Sparkles}>{text(language, 'Skill Registry', 'Skill Registry')}</SectionTitle>
          </SectionHeader>
          {!data?.skills.length && !loading ? (
            <EmptyState>{text(language, '暂无 Skills', 'No skills')}</EmptyState>
          ) : (
            <div className="grid gap-3">
              {(data?.skills || []).map((skill) => (
                <div key={skill.id} className="rounded-2xl border border-white/10 bg-white/[0.035] p-4">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <div className="font-mono text-xs text-primary">{skill.id}</div>
                      <h3 className="mt-1 font-semibold">{skill.label}</h3>
                    </div>
                    <Badge variant="outline" className="rounded-xl">{skill.risk}</Badge>
                  </div>
                  <p className="mt-3 text-sm leading-6 text-muted-foreground">{skill.description}</p>
                  <PillRow title={text(language, '意图', 'Intents')} values={skill.intent_tags} />
                  <PillRow title={text(language, '必需工具', 'Required tools')} values={skill.required_tools || []} />
                  {!!skill.fallback_prompts?.length && <PillRow title={text(language, 'Fallback', 'Fallback')} values={skill.fallback_prompts} />}
                </div>
              ))}
            </div>
          )}
        </Section>

        <Section>
          <SectionHeader>
            <SectionTitle icon={CheckCircle2}>{text(language, 'Tool Capabilities', 'Tool Capabilities')}</SectionTitle>
          </SectionHeader>
          <div className="grid gap-3">
            {(data?.tool_capabilities || []).map((tool) => (
              <div key={tool.name} className="rounded-2xl border border-white/10 bg-black/15 p-4">
                <div className="flex items-center justify-between gap-3">
                  <div className="truncate font-mono text-xs text-primary">{tool.name}</div>
                  <Badge variant="secondary" className="rounded-xl">{tool.source}</Badge>
                </div>
                <p className="mt-2 line-clamp-2 text-xs leading-5 text-muted-foreground">{tool.description}</p>
                <div className="mt-3 flex flex-wrap gap-1.5">
                  {tool.intent_tags?.slice(0, 5).map((tag) => <Badge key={tag} variant="outline" className="rounded-xl text-[10px]">{tag}</Badge>)}
                </div>
              </div>
            ))}
          </div>
        </Section>
      </div>

      <Section>
        <SectionHeader>
          <SectionTitle icon={Wrench}>{text(language, 'Security Tools Catalog', 'Security Tools Catalog')}</SectionTitle>
        </SectionHeader>
        {!data?.security_tools?.length && !loading ? (
          <EmptyState>{text(language, '暂无安全工具目录', 'No security tools catalog')}</EmptyState>
        ) : (
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {(data?.security_tools || []).map((tool) => (
              <div key={`${tool.source}:${tool.name}`} className="rounded-2xl border border-white/10 bg-black/15 p-4">
                <div className="flex items-center justify-between gap-3">
                  <div className="truncate font-mono text-xs text-primary">{tool.name}</div>
                  <Badge variant="secondary" className="rounded-xl">{tool.source}</Badge>
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {tool.risk && <Badge variant="outline" className="rounded-xl text-[10px]">{tool.risk}</Badge>}
                  {tool.required_level && <Badge variant="outline" className="rounded-xl text-[10px]">{tool.required_level}</Badge>}
                </div>
                <PillRow title={text(language, '意图', 'Intents')} values={tool.intent_tags || []} />
                <div className="mt-3 truncate font-mono text-[10px] text-muted-foreground">{tool.path || tool.plugin_path || '—'}</div>
              </div>
            ))}
          </div>
        )}
      </Section>
    </div>
  )
}

function PillRow({ title, values }: { title: string; values: string[] }) {
  if (!values.length) return null
  return (
    <div className="mt-3">
      <div className="mb-1.5 text-[11px] uppercase tracking-[0.22em] text-muted-foreground">{title}</div>
      <div className="flex flex-wrap gap-1.5">
        {values.map((value) => (
          <Badge key={value} variant="secondary" className="rounded-xl text-[10px]">
            {value}
          </Badge>
        ))}
      </div>
    </div>
  )
}
