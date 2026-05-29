import { useCallback, useEffect, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  Bot,
  Check,
  Globe2,
  KeyRound,
  Languages,
  Lock,
  MessageSquare,
  PlugZap,
  RefreshCcw,
  RotateCcw,
  Save,
  ShieldCheck,
  SlidersHorizontal,
  UserRound,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageDescription, PageHeader, PageTitle, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { AgentProviderSettings, Provider } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

const AI_PROVIDER_OPTIONS = [
  {
    id: 'openai',
    name: 'OpenAI',
    defaultModel: 'gpt-4o',
    detail: 'Chat Completions',
  },
  {
    id: 'anthropic',
    name: 'Anthropic',
    defaultModel: 'claude-sonnet-4-20250514',
    detail: 'Claude Messages',
  },
  {
    id: 'ollama',
    name: 'Ollama',
    defaultModel: 'llama3.1',
    detail: 'Local /v1 compatible',
  },
  {
    id: 'openai-compatible',
    name: 'OpenAI Compatible',
    defaultModel: '',
    detail: 'Custom /v1 endpoint',
  },
  {
    id: 'anthropic-compatible',
    name: 'Anthropic Compatible',
    defaultModel: '',
    detail: 'Custom messages endpoint',
  },
]

export function SettingsPage() {
  const { client, language, setLanguage, settings, updateSettings, resetSettings, t } = useConsole()
  const agentProvider = settings.agentProvider
  const [saved, setSaved] = useState(false)
  const [checking, setChecking] = useState(false)
  const [error, setError] = useState('')
  const [connected, setConnected] = useState(false)
  const [providers, setProviders] = useState<Provider[]>([])

  const check = useCallback(async () => {
    setChecking(true)
    setError('')
    try {
      const providerList = await client.getProviders()
      setProviders(providerList || [])
      setConnected(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'connection failed')
      setConnected(false)
      setProviders([])
    } finally {
      setChecking(false)
    }
  }, [client])

  useEffect(() => {
    check()
  }, [check])

  const save = () => {
    setSaved(true)
    window.setTimeout(() => setSaved(false), 1600)
  }

  const updateAgentProvider = <K extends keyof AgentProviderSettings>(key: K, value: AgentProviderSettings[K]) => {
    updateSettings({ agentProvider: { [key]: value } as Partial<AgentProviderSettings> })
  }
  const selectedProvider = AI_PROVIDER_OPTIONS.find((provider) => provider.id === agentProvider.provider)
  const providerReady =
    agentProvider.provider === 'ollama' || Boolean(agentProvider.apiKey.trim() || agentProvider.providerBaseUrl.trim())

  const chooseProvider = (providerId: string) => {
    const provider = AI_PROVIDER_OPTIONS.find((item) => item.id === providerId)
    updateSettings({
      agentProvider: {
        provider: providerId,
        model: provider?.defaultModel || agentProvider.model,
      },
    })
  }

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_340px]">
      <section className="space-y-4">
        <Section>
          <PageHeader>
            <div>
              <div className="mb-2 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
                <PlugZap className="h-3.5 w-3.5" />
                {t('config')}
              </div>
              <PageTitle>{t('settings')}</PageTitle>
              <PageDescription className="mt-2">
                {text(
                  language,
                  '所有控制台设置都会自动保存到本地浏览器：daemon 连接、Agent 操作员、AI Provider 和语言。',
                  'All console settings are auto-saved in this browser: daemon connection, Agent operator, AI provider and language.',
                )}
              </PageDescription>
            </div>
          </PageHeader>
        </Section>

        <div className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
          <SectionHeader>
            <SectionTitle icon={Globe2}>{text(language, '后端连接', 'Backend Connection')}</SectionTitle>
            <span className="font-mono text-xs text-muted-foreground">/api</span>
          </SectionHeader>

          <div className="grid gap-4">
            <Field icon={Globe2} label={t('baseUrl')} htmlFor="base-url">
              <Input
                id="base-url"
                value={settings.baseUrl}
                onChange={(event) => updateSettings({ baseUrl: event.target.value })}
                placeholder="http://127.0.0.1:8787"
                className="h-10 rounded-xl font-mono"
              />
            </Field>

            <Field icon={KeyRound} label={t('token')} htmlFor="bearer-token">
              <Input
                id="bearer-token"
                value={settings.token}
                onChange={(event) => updateSettings({ token: event.target.value })}
                placeholder="Bearer token"
                type="password"
                className="h-10 rounded-xl font-mono"
              />
            </Field>

            <Field icon={UserRound} label={t('operator')} htmlFor="operator">
              <Input
                id="operator"
                value={settings.operator}
                onChange={(event) => updateSettings({ operator: event.target.value })}
                placeholder="local-user"
                className="h-10 rounded-xl font-mono"
              />
            </Field>
          </div>

          <div className="mt-4 flex flex-wrap gap-2">
            <Button type="button" onClick={save} className="rounded-2xl">
              {saved ? <Check className="mr-2 h-4 w-4" /> : <Save className="mr-2 h-4 w-4" />}
              {saved ? text(language, '已保存', 'Saved') : t('save')}
            </Button>
            <Button type="button" variant="outline" onClick={check} className="rounded-2xl bg-transparent">
              <RefreshCcw className={cn('mr-2 h-4 w-4', checking && 'animate-spin')} />
              {t('connect')}
            </Button>
            <Button type="button" variant="ghost" onClick={resetSettings} className="rounded-2xl">
              <RotateCcw className="mr-2 h-4 w-4" />
              {text(language, '重置', 'Reset')}
            </Button>
          </div>
        </div>

        <div className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
          <SectionHeader>
            <SectionTitle icon={Bot}>{text(language, 'AI Provider', 'AI Provider')}</SectionTitle>
            <span className="font-mono text-xs text-muted-foreground">chat / planner / cockpit</span>
          </SectionHeader>

          <div className="grid gap-4">
            <Field icon={PlugZap} label={t('provider')} htmlFor="default-provider">
              <select
                id="default-provider"
                value={agentProvider.provider}
                onChange={(event) => chooseProvider(event.target.value)}
                className="h-10 w-full rounded-xl border border-input bg-background px-3 py-2 font-mono text-sm text-foreground ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              >
                {AI_PROVIDER_OPTIONS.map((provider) => (
                  <option key={provider.id} value={provider.id}>
                    {provider.name}
                  </option>
                ))}
              </select>
              <div className="mt-1 font-mono text-[11px] text-muted-foreground">
                {selectedProvider?.defaultModel || selectedProvider?.detail || text(language, '选择提供商', 'Choose provider')}
              </div>
            </Field>

            <div className="grid gap-4 lg:grid-cols-2">
              <Field icon={SlidersHorizontal} label={t('model')} htmlFor="default-model">
                <Input
                  id="default-model"
                  value={agentProvider.model}
                  onChange={(event) => updateAgentProvider('model', event.target.value)}
                  placeholder={selectedProvider?.defaultModel || 'model name'}
                  className="h-10 rounded-xl font-mono"
                />
              </Field>

              <Field icon={KeyRound} label="API Key" htmlFor="default-api-key">
                <Input
                  id="default-api-key"
                  value={agentProvider.apiKey}
                  onChange={(event) => updateAgentProvider('apiKey', event.target.value)}
                  placeholder="sk-..."
                  type="password"
                  className="h-10 rounded-xl font-mono"
                />
              </Field>
            </div>

            <div className="grid gap-4 lg:grid-cols-2">
              <Field icon={Globe2} label="Provider Base URL" htmlFor="default-provider-base-url">
                <Input
                  id="default-provider-base-url"
                  value={agentProvider.providerBaseUrl}
                  onChange={(event) => updateAgentProvider('providerBaseUrl', event.target.value)}
                  placeholder={text(language, '可选', 'Optional')}
                  className="h-10 rounded-xl font-mono"
                />
              </Field>

              <Field icon={SlidersHorizontal} label={t('turns')} htmlFor="default-max-turns">
                <Input
                  id="default-max-turns"
                  type="number"
                  value={agentProvider.maxTurns}
                  onChange={(event) => updateAgentProvider('maxTurns', Number(event.target.value))}
                  min={1}
                  max={200}
                  className="h-10 rounded-xl font-mono"
                />
              </Field>

              <Field icon={SlidersHorizontal} label={text(language, '自动续行上限', 'Auto-continue limit')} htmlFor="auto-continue-limit">
                <Input
                  id="auto-continue-limit"
                  type="number"
                  value={settings.autoContinueLimit || 3}
                  onChange={(event) => updateSettings({ autoContinueLimit: Number(event.target.value) })}
                  min={0}
                  max={10}
                  className="h-10 rounded-xl font-mono"
                />
              </Field>
            </div>

            <Field icon={MessageSquare} label={text(language, 'Agent 指令', 'Agent instructions')} htmlFor="default-instructions">
              <textarea
                id="default-instructions"
                value={agentProvider.instructions}
                onChange={(event) => updateAgentProvider('instructions', event.target.value)}
                placeholder={text(language, '可选的全局偏好、限制或回答风格...', 'Optional global preferences, limits or response style...')}
                rows={2}
                className="w-full rounded-xl subtle-surface px-3 py-2 font-mono text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </Field>
          </div>
        </div>

        <div className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
          <div className="mb-4 flex items-center gap-3">
            <span className="grid h-9 w-9 place-items-center rounded-xl border border-violet-400/20 bg-violet-400/10 text-violet-300">
              <Languages className="h-5 w-5" />
            </span>
            <div>
              <h2 className="text-sm font-semibold">{text(language, '语言', 'Language')}</h2>
              <p className="font-mono text-xs text-muted-foreground">中文 / English</p>
            </div>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <button
              type="button"
              onClick={() => setLanguage('zh')}
              className={cn(
                'rounded-2xl border p-3 text-left transition',
                language === 'zh' ? 'border-primary/35 bg-primary/10 text-foreground' : 'list-item rounded-2xl p-3 text-muted-foreground hover:text-foreground',
              )}
            >
              <span className="block text-base font-semibold">中文</span>
              <span className="mt-1 block text-xs">简体中文界面</span>
            </button>
            <button
              type="button"
              onClick={() => setLanguage('en')}
              className={cn(
                'rounded-2xl border p-3 text-left transition',
                language === 'en' ? 'border-primary/35 bg-primary/10 text-foreground' : 'list-item rounded-2xl p-3 text-muted-foreground hover:text-foreground',
              )}
            >
              <span className="block text-base font-semibold">English</span>
              <span className="mt-1 block text-xs">English interface</span>
            </button>
          </div>
        </div>
      </section>

      <aside className="space-y-4">
        {error && (
          <div className="rounded-[1.5rem] border border-red-400/20 bg-red-500/10 p-4 text-sm text-red-100">
            <div className="mb-2 flex items-center gap-2 font-semibold">
              <AlertTriangle className="h-4 w-4" />
              {t('offline')}
            </div>
            <p className="leading-6">{error}</p>
          </div>
        )}

        <StatusCard
          icon={Activity}
          label={t('connected')}
          ok={connected}
          value={connected ? 'online' : 'offline'}
          detail={settings.baseUrl}
        />
        <StatusCard
          icon={ShieldCheck}
          label={t('provider')}
          ok={connected && providers.length > 0}
          value={`${providers.length} providers`}
          detail={providers.map((p) => p.name).join(', ') || '—'}
        />
        <StatusCard
          icon={PlugZap}
          label={text(language, 'AI 模型', 'AI Model')}
          ok={providerReady}
          value={agentProvider.provider || '—'}
          detail={agentProvider.model || selectedProvider?.defaultModel || text(language, '默认模型', 'default model')}
        />
        <StatusCard
          icon={Lock}
          label={text(language, '持久化', 'Persistence')}
          ok
          value="localStorage"
          detail="cyberagent_console_settings"
        />
      </aside>
    </div>
  )
}

function Field({
  icon: Icon,
  label,
  htmlFor,
  children,
}: {
  icon: React.ElementType
  label: string
  htmlFor: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-2">
      <Label htmlFor={htmlFor} className="flex items-center gap-2 text-xs text-muted-foreground">
        <Icon className="h-4 w-4" />
        {label}
      </Label>
      {children}
    </div>
  )
}

function StatusCard({
  icon: Icon,
  label,
  ok,
  value,
  detail,
}: {
  icon: React.ElementType
  label: string
  ok: boolean
  value: string
  detail: string
}) {
  return (
    <div className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
      <div className="mb-3 flex items-center justify-between">
        <span className="grid h-9 w-9 place-items-center rounded-xl subtle-surface">
          <Icon className={cn('h-4 w-4', ok ? 'text-emerald-300' : 'text-amber-300')} />
        </span>
        <span className={cn('h-2.5 w-2.5 rounded-full', ok ? 'bg-emerald-300' : 'bg-amber-300')} />
      </div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 truncate font-mono text-base font-semibold">{value}</div>
      <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{detail}</div>
    </div>
  )
}
