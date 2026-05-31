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
  Plus,
  RefreshCcw,
  RotateCcw,
  Save,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  UserRound,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageDescription, PageHeader, PageTitle, Section, SectionHeader, SectionTitle } from '@/components/layout-primitives'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { AgentProviderProfile, AgentProviderType, ApiAuthProfile, ApiAuthProfileType, Provider } from '@/lib/api'
import { normalizeAgentOperationLevel, type AgentOperationLevel } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

const PROVIDER_TYPES: { id: AgentProviderType; name: string; defaultModel: string; detail: string }[] = [
  { id: 'openai', name: 'OpenAI', defaultModel: 'gpt-4o', detail: 'Chat Completions / compatible' },
  { id: 'anthropic', name: 'Anthropic', defaultModel: 'claude-sonnet-4-20250514', detail: 'Claude Messages / compatible' },
]

const OPERATION_LEVELS: AgentOperationLevel[] = ['read_only', 'workspace', 'safe_scan', 'approval_ops', 'advanced', 'bypass']
const AUTH_PROFILE_TYPES: ApiAuthProfileType[] = ['bearer', 'basic', 'api_key', 'cookie', 'custom']

function operationLevelLabel(level: string, language: 'zh' | 'en') {
  const labels: Record<AgentOperationLevel, { zh: string; en: string }> = {
    read_only: { zh: '只读', en: 'Read only' },
    workspace: { zh: '工作区', en: 'Workspace' },
    workspace_write: { zh: '工作区', en: 'Workspace' },
    safe_scan: { zh: '安全扫描', en: 'Safe scan' },
    approval_ops: { zh: '审批操作', en: 'Approval ops' },
    advanced: { zh: '高级', en: 'Advanced' },
    bypass: { zh: 'Bypass 权限', en: 'Bypass permissions' },
  }
  const normalized = normalizeAgentOperationLevel(level)
  return labels[normalized][language]
}

function operationLevelDetail(level: string, language: 'zh' | 'en') {
  const details: Record<AgentOperationLevel, { zh: string; en: string }> = {
    read_only: { zh: '仅分析、规划、读取运行上下文，不执行工具写入或扫描。', en: 'Analyze, plan and read runtime context only.' },
    workspace: { zh: '允许低风险工作区命令与文件读写/变更预览。', en: 'Allow low-risk workspace commands and file operations.' },
    workspace_write: { zh: '允许低风险工作区命令与文件读写/变更预览。', en: 'Allow low-risk workspace commands and file operations.' },
    safe_scan: { zh: '允许在明确授权目标与 scope 后发起 safe Web/API 任务。', en: 'Allow safe Web/API tasks after explicit target and scope.' },
    approval_ops: { zh: '允许按用户显式指令处理审批与恢复运行。', en: 'Allow approval handling and resume on explicit instruction.' },
    advanced: { zh: '开放当前已实现的高级内置动作，仍受策略和授权约束。', en: 'Enable implemented advanced actions under policy and scope.' },
    bypass: { zh: '本地沙箱模式：跳过权限确认弹窗，直接执行已规划的内置工具动作。', en: 'Local sandbox mode: skip permission prompts and directly execute planned built-in tool actions.' },
  }
  const normalized = normalizeAgentOperationLevel(level)
  return details[normalized][language]
}

function authProfileTypeLabel(type: ApiAuthProfileType, language: 'zh' | 'en') {
  const labels: Record<ApiAuthProfileType, { zh: string; en: string }> = {
    bearer: { zh: 'Bearer', en: 'Bearer' },
    basic: { zh: 'Basic', en: 'Basic' },
    api_key: { zh: 'API Key', en: 'API Key' },
    cookie: { zh: 'Cookie', en: 'Cookie' },
    custom: { zh: '自定义 Header', en: 'Custom Header' },
  }
  return labels[type][language]
}

function shouldHydrateProviderProfiles(profiles: AgentProviderProfile[]) {
  if (!profiles.length) return true
  if (profiles.length > 1) return false
  const only = profiles[0]
  const emptySecret = !only.apiKey?.trim() && !only.baseUrl?.trim()
  return emptySecret && (only.id === 'openai-default' || !only.model?.trim())
}

export function SettingsPage() {
  const { client, language, setLanguage, settings, updateSettings, resetSettings, t } = useConsole()
  const [saved, setSaved] = useState(false)
  const [checking, setChecking] = useState(false)
  const [error, setError] = useState('')
  const [connected, setConnected] = useState(false)
  const [providers, setProviders] = useState<Provider[]>([])
  const [savingProviders, setSavingProviders] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ ok: boolean; message: string; requestUrl?: string } | null>(null)

  const aiProviders = settings.aiProviders || []
  const activeProvider = aiProviders.find((p) => p.id === settings.activeAiProviderId) || aiProviders[0]
  const providerType = PROVIDER_TYPES.find((tp) => tp.id === activeProvider?.type)
  const providerReady = Boolean(activeProvider?.apiKey.trim() || activeProvider?.baseUrl.trim())

  const check = useCallback(async () => {
    setChecking(true)
    setError('')
    try {
      const providerList = await client.getProviders()
      setProviders(providerList || [])
      setConnected(true)
      try {
        const remote = await client.getProviderProfiles()
        if (remote.providers.length && shouldHydrateProviderProfiles(settings.aiProviders || [])) {
          updateSettings({ aiProviders: remote.providers, activeAiProviderId: remote.activeId || remote.providers[0].id })
        }
      } catch {
        // provider profiles unavailable; fall back to local settings
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'connection failed')
      setConnected(false)
      setProviders([])
    } finally {
      setChecking(false)
    }
  }, [client, settings.aiProviders, updateSettings])

  useEffect(() => {
    check()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client])

  const save = () => {
    setSaved(true)
    window.setTimeout(() => setSaved(false), 1600)
  }

  const updateActiveProvider = (patch: Partial<AgentProviderProfile>) => {
    if (!activeProvider) return
    setTestResult(null)
    updateSettings({
      aiProviders: aiProviders.map((p) => (p.id === activeProvider.id ? { ...p, ...patch } : p)),
    })
  }

  const addProvider = () => {
    const id = `ai_provider_${Date.now().toString(36)}`
    updateSettings({
      aiProviders: [
        ...aiProviders,
        {
          id,
          name: text(language, '新 Provider', 'New provider'),
          type: 'openai',
          model: 'gpt-4o',
          apiKey: '',
          baseUrl: '',
          instructions: '',
          maxTurns: 50,
        },
      ],
      activeAiProviderId: id,
    })
  }

  const deleteProvider = (id: string) => {
    if (aiProviders.length <= 1) return
    const next = aiProviders.filter((p) => p.id !== id)
    updateSettings({
      aiProviders: next,
      activeAiProviderId: settings.activeAiProviderId === id ? next[0]?.id || '' : settings.activeAiProviderId,
    })
  }

  const saveProviders = async () => {
    if (!connected) return
    setSavingProviders(true)
    try {
      await client.saveProviderProfiles(aiProviders, settings.activeAiProviderId)
      save()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'save failed')
    } finally {
      setSavingProviders(false)
    }
  }

  const testProvider = async () => {
    if (!activeProvider) return
    setTesting(true)
    setTestResult(null)
    try {
      const res = await client.testProvider({
        type: activeProvider.type,
        model: activeProvider.model,
        api_key: activeProvider.apiKey,
        base_url: activeProvider.baseUrl || undefined,
      })
      setTestResult(
        res.ok
          ? { ok: true, message: `${res.provider} · ${res.latency_ms}ms`, requestUrl: res.request_url }
          : { ok: false, message: res.error || text(language, '连接失败', 'connection failed'), requestUrl: res.request_url },
      )
    } catch (err) {
      setTestResult({ ok: false, message: err instanceof Error ? err.message : 'test failed' })
    } finally {
      setTesting(false)
    }
  }

  const authProfiles = settings.apiAuthProfiles || []

  const addAuthProfile = () => {
    const id = `api_auth_${Date.now().toString(36)}`
    updateSettings({
      apiAuthProfiles: [
        ...authProfiles,
        {
          id,
          name: text(language, '新 API Profile', 'New API profile'),
          type: 'bearer',
          baseUrl: '',
          token: '',
          username: '',
          password: '',
          headerName: '',
          headerValue: '',
          cookie: '',
          enabled: true,
        },
      ],
      defaultApiAuthProfileId: settings.defaultApiAuthProfileId || id,
    })
  }

  const updateAuthProfile = (id: string, patch: Partial<ApiAuthProfile>) => {
    const nextId = patch.id || id
    updateSettings({
      apiAuthProfiles: authProfiles.map((profile) => (profile.id === id ? { ...profile, ...patch } : profile)),
      defaultApiAuthProfileId: settings.defaultApiAuthProfileId === id ? nextId : settings.defaultApiAuthProfileId,
    })
  }

  const deleteAuthProfile = (id: string) => {
    const nextProfiles = authProfiles.filter((profile) => profile.id !== id)
    updateSettings({
      apiAuthProfiles: nextProfiles,
      defaultApiAuthProfileId: settings.defaultApiAuthProfileId === id ? nextProfiles[0]?.id || '' : settings.defaultApiAuthProfileId,
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

            <Field icon={ShieldCheck} label={text(language, 'Agent 操作权限', 'Agent operation level')} htmlFor="operation-level">
              <select
                id="operation-level"
                value={settings.operationLevel}
                onChange={(event) => updateSettings({ operationLevel: normalizeAgentOperationLevel(event.target.value) })}
                className="h-10 w-full rounded-xl border border-input bg-background px-3 py-2 font-mono text-sm text-foreground ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              >
                {OPERATION_LEVELS.map((level) => (
                  <option key={level} value={level}>{operationLevelLabel(level, language)}</option>
                ))}
              </select>
              <div className="mt-1 text-[11px] leading-5 text-muted-foreground">
                {operationLevelDetail(settings.operationLevel, language)}
              </div>
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
            <Button type="button" variant="outline" size="sm" onClick={addProvider} className="h-8 rounded-xl bg-transparent px-2">
              <Plus className="mr-1 h-3.5 w-3.5" />
              {text(language, '新增', 'Add')}
            </Button>
          </SectionHeader>

          <div className="grid gap-4">
            <div className="grid gap-4 lg:grid-cols-2">
              <Field icon={PlugZap} label={text(language, '选择 Provider', 'Select provider')} htmlFor="provider-select">
                <select
                  id="provider-select"
                  value={settings.activeAiProviderId}
                  onChange={(event) => updateSettings({ activeAiProviderId: event.target.value })}
                  className="h-10 w-full rounded-xl border border-input bg-background px-3 py-2 font-mono text-sm text-foreground ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  {aiProviders.map((provider) => (
                    <option key={provider.id} value={provider.id}>{provider.name || provider.id}</option>
                  ))}
                </select>
              </Field>

              <Field icon={Bot} label={text(language, '类型', 'Type')} htmlFor="provider-type">
                <select
                  id="provider-type"
                  value={activeProvider?.type || 'openai'}
                  onChange={(event) => updateActiveProvider({ type: event.target.value as AgentProviderType })}
                  className="h-10 w-full rounded-xl border border-input bg-background px-3 py-2 font-mono text-sm text-foreground ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  {PROVIDER_TYPES.map((tp) => (
                    <option key={tp.id} value={tp.id}>{tp.name}</option>
                  ))}
                </select>
                <div className="mt-1 font-mono text-[11px] text-muted-foreground">{providerType?.detail}</div>
              </Field>
            </div>

            <div className="grid gap-4 lg:grid-cols-2">
              <Field icon={MessageSquare} label={text(language, '名称', 'Name')} htmlFor="provider-name">
                <Input
                  id="provider-name"
                  value={activeProvider?.name || ''}
                  onChange={(event) => updateActiveProvider({ name: event.target.value })}
                  placeholder={text(language, 'Provider 名称', 'Provider name')}
                  className="h-10 rounded-xl font-mono"
                />
              </Field>

              <Field icon={SlidersHorizontal} label={t('model')} htmlFor="provider-model">
                <Input
                  id="provider-model"
                  value={activeProvider?.model || ''}
                  onChange={(event) => updateActiveProvider({ model: event.target.value })}
                  placeholder={providerType?.defaultModel || 'model name'}
                  className="h-10 rounded-xl font-mono"
                />
              </Field>

              <Field icon={KeyRound} label="API Key" htmlFor="provider-api-key">
                <Input
                  id="provider-api-key"
                  value={activeProvider?.apiKey || ''}
                  onChange={(event) => updateActiveProvider({ apiKey: event.target.value })}
                  placeholder="sk-..."
                  type="password"
                  className="h-10 rounded-xl font-mono"
                />
              </Field>

              <Field icon={Globe2} label="Provider Base URL" htmlFor="provider-base-url">
                <Input
                  id="provider-base-url"
                  value={activeProvider?.baseUrl || ''}
                  onChange={(event) => updateActiveProvider({ baseUrl: event.target.value })}
                  placeholder={activeProvider?.type === 'anthropic' ? 'https://api.example.com/v1/messages' : 'https://api.example.com/v1/chat/completions'}
                  className="h-10 rounded-xl font-mono"
                />
                <div className="mt-1 font-mono text-[11px] text-muted-foreground">
                  {activeProvider?.type === 'anthropic'
                    ? text(language, 'Anthropic 实际请求应为 /v1/messages；可填 /v1 或完整 endpoint，测试会显示实际请求 URL。', 'Anthropic requests should use /v1/messages; /v1 or the full endpoint are supported, and test shows the actual request URL.')
                    : text(language, 'OpenAI 实际请求应为 /v1/chat/completions；可填 /v1、/api/v1 或完整 endpoint。', 'OpenAI requests should use /v1/chat/completions; /v1, /api/v1 or the full endpoint are supported.')}
                </div>
              </Field>

              <Field icon={SlidersHorizontal} label={t('turns')} htmlFor="provider-max-turns">
                <Input
                  id="provider-max-turns"
                  type="number"
                  value={activeProvider?.maxTurns || 50}
                  onChange={(event) => updateActiveProvider({ maxTurns: Number(event.target.value) })}
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

            <Field icon={MessageSquare} label={text(language, 'Agent 指令', 'Agent instructions')} htmlFor="provider-instructions">
              <textarea
                id="provider-instructions"
                value={activeProvider?.instructions || ''}
                onChange={(event) => updateActiveProvider({ instructions: event.target.value })}
                placeholder={text(language, '可选的全局偏好、限制或回答风格...', 'Optional global preferences, limits or response style...')}
                rows={2}
                className="w-full rounded-xl subtle-surface px-3 py-2 font-mono text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
              />
            </Field>

            <div className="flex flex-wrap items-center gap-2">
              <Button type="button" onClick={saveProviders} disabled={!connected || savingProviders} className="rounded-2xl">
                {saved ? <Check className="mr-2 h-4 w-4" /> : <Save className={cn('mr-2 h-4 w-4', savingProviders && 'animate-spin')} />}
                {saved ? text(language, '已保存', 'Saved') : t('save')}
              </Button>
              <Button type="button" variant="outline" onClick={testProvider} disabled={!connected || testing} className="rounded-2xl bg-transparent">
                <Activity className={cn('mr-2 h-4 w-4', testing && 'animate-spin')} />
                {text(language, '测试连通性', 'Test connection')}
              </Button>
              {aiProviders.length > 1 && activeProvider && (
                <Button type="button" variant="ghost" onClick={() => deleteProvider(activeProvider.id)} className="rounded-2xl text-muted-foreground">
                  <Trash2 className="mr-2 h-4 w-4" />
                  {text(language, '删除', 'Delete')}
                </Button>
              )}
              {testResult && (
                <span className={cn('max-w-full break-all font-mono text-xs', testResult.ok ? 'text-emerald-300' : 'text-red-300')}>
                  {testResult.ok ? '✓ ' : '✗ '}{testResult.message}
                  {testResult.requestUrl ? (
                    <span className="ml-2 text-muted-foreground">
                      {text(language, '实际请求', 'request')}: {testResult.requestUrl}
                    </span>
                  ) : null}
                </span>
              )}
            </div>
          </div>
        </div>

        <div className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
          <SectionHeader>
            <SectionTitle icon={KeyRound}>{text(language, 'API Auth Profiles', 'API Auth Profiles')}</SectionTitle>
            <div className="flex items-center gap-2">
              {authProfiles.length > 0 && (
                <select
                  value={settings.defaultApiAuthProfileId}
                  onChange={(event) => updateSettings({ defaultApiAuthProfileId: event.target.value })}
                  className="h-8 rounded-xl border border-input bg-background px-2 font-mono text-xs text-foreground"
                  title={text(language, '默认 Profile', 'Default profile')}
                >
                  <option value="">{text(language, '无默认', 'No default')}</option>
                  {authProfiles.map((profile) => (
                    <option key={profile.id} value={profile.id}>{profile.name || profile.id}</option>
                  ))}
                </select>
              )}
              <Button type="button" variant="outline" size="sm" onClick={addAuthProfile} className="h-8 rounded-xl bg-transparent px-2">
                <Plus className="mr-1 h-3.5 w-3.5" />
                {text(language, '新增', 'Add')}
              </Button>
            </div>
          </SectionHeader>

          <p className="mb-3 text-xs leading-5 text-muted-foreground">
            {text(
              language,
              '用于 http.request 工具的授权上下文。密钥只发给 daemon，LLM 只看到 profile ID；默认 profile 仅在 Base URL host 匹配时自动附加。',
              'Authorization context for the http.request tool. Secrets are sent only to the daemon; the LLM sees profile IDs only. The default profile is attached only when the Base URL host matches.',
            )}
          </p>

          <div className="space-y-3">
            {authProfiles.map((profile) => (
              <div key={profile.id} className="rounded-2xl border border-white/10 bg-white/[0.035] p-3">
                <div className="mb-3 flex flex-wrap items-center gap-2">
                  <input
                    type="checkbox"
                    checked={profile.enabled}
                    onChange={(event) => updateAuthProfile(profile.id, { enabled: event.target.checked })}
                    className="h-4 w-4 accent-primary"
                    aria-label={text(language, '启用 Profile', 'Enable profile')}
                  />
                  <Input
                    value={profile.name}
                    onChange={(event) => updateAuthProfile(profile.id, { name: event.target.value })}
                    placeholder={text(language, '名称', 'Name')}
                    className="h-8 min-w-[150px] flex-1 rounded-xl font-mono text-xs"
                  />
                  <select
                    value={profile.type}
                    onChange={(event) => updateAuthProfile(profile.id, { type: event.target.value as ApiAuthProfileType })}
                    className="h-8 rounded-xl border border-input bg-background px-2 font-mono text-xs text-foreground"
                  >
                    {AUTH_PROFILE_TYPES.map((type) => (
                      <option key={type} value={type}>{authProfileTypeLabel(type, language)}</option>
                    ))}
                  </select>
                  <Button type="button" variant="ghost" size="sm" onClick={() => deleteAuthProfile(profile.id)} className="h-8 rounded-xl px-2 text-muted-foreground">
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>

                <div className="grid gap-3 lg:grid-cols-2">
                  <Field icon={Globe2} label="Base URL / Host Scope" htmlFor={`${profile.id}-base-url`}>
                    <Input
                      id={`${profile.id}-base-url`}
                      value={profile.baseUrl}
                      onChange={(event) => updateAuthProfile(profile.id, { baseUrl: event.target.value })}
                      placeholder="https://api.example.com"
                      className="h-9 rounded-xl font-mono text-xs"
                    />
                  </Field>

                  <Field icon={KeyRound} label="Profile ID" htmlFor={`${profile.id}-id`}>
                    <Input
                      id={`${profile.id}-id`}
                      value={profile.id}
                      onChange={(event) => updateAuthProfile(profile.id, { id: event.target.value.trim() || profile.id })}
                      className="h-9 rounded-xl font-mono text-xs"
                    />
                  </Field>

                  {profile.type === 'basic' ? (
                    <>
                      <Field icon={UserRound} label="Username" htmlFor={`${profile.id}-username`}>
                        <Input
                          id={`${profile.id}-username`}
                          value={profile.username}
                          onChange={(event) => updateAuthProfile(profile.id, { username: event.target.value })}
                          className="h-9 rounded-xl font-mono text-xs"
                        />
                      </Field>
                      <Field icon={Lock} label="Password" htmlFor={`${profile.id}-password`}>
                        <Input
                          id={`${profile.id}-password`}
                          value={profile.password}
                          onChange={(event) => updateAuthProfile(profile.id, { password: event.target.value })}
                          type="password"
                          className="h-9 rounded-xl font-mono text-xs"
                        />
                      </Field>
                    </>
                  ) : profile.type === 'api_key' || profile.type === 'custom' ? (
                    <>
                      <Field icon={KeyRound} label="Header Name" htmlFor={`${profile.id}-header-name`}>
                        <Input
                          id={`${profile.id}-header-name`}
                          value={profile.headerName}
                          onChange={(event) => updateAuthProfile(profile.id, { headerName: event.target.value })}
                          placeholder={profile.type === 'api_key' ? 'X-API-Key' : 'X-Custom-Auth'}
                          className="h-9 rounded-xl font-mono text-xs"
                        />
                      </Field>
                      <Field icon={Lock} label="Header Value" htmlFor={`${profile.id}-header-value`}>
                        <Input
                          id={`${profile.id}-header-value`}
                          value={profile.headerValue}
                          onChange={(event) => updateAuthProfile(profile.id, { headerValue: event.target.value })}
                          type="password"
                          className="h-9 rounded-xl font-mono text-xs"
                        />
                      </Field>
                    </>
                  ) : profile.type === 'cookie' ? (
                    <Field icon={Lock} label="Cookie" htmlFor={`${profile.id}-cookie`}>
                      <Input
                        id={`${profile.id}-cookie`}
                        value={profile.cookie}
                        onChange={(event) => updateAuthProfile(profile.id, { cookie: event.target.value })}
                        placeholder="session=..."
                        type="password"
                        className="h-9 rounded-xl font-mono text-xs"
                      />
                    </Field>
                  ) : (
                    <Field icon={Lock} label="Bearer Token" htmlFor={`${profile.id}-token`}>
                      <Input
                        id={`${profile.id}-token`}
                        value={profile.token}
                        onChange={(event) => updateAuthProfile(profile.id, { token: event.target.value })}
                        placeholder="token"
                        type="password"
                        className="h-9 rounded-xl font-mono text-xs"
                      />
                    </Field>
                  )}
                </div>
              </div>
            ))}

            {!authProfiles.length && (
              <button
                type="button"
                onClick={addAuthProfile}
                className="w-full rounded-2xl border border-dashed border-white/15 bg-white/[0.025] p-4 text-center text-sm text-muted-foreground transition hover:border-primary/30 hover:text-foreground"
              >
                <Plus className="mx-auto mb-2 h-4 w-4" />
                {text(language, '添加一个 API 授权 Profile', 'Add an API auth profile')}
              </button>
            )}
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
          value={activeProvider?.type || '—'}
          detail={activeProvider?.model || providerType?.defaultModel || text(language, '默认模型', 'default model')}
        />
        <StatusCard
          icon={KeyRound}
          label={text(language, 'API 授权', 'API Auth')}
          ok={authProfiles.some((profile) => profile.enabled)}
          value={`${authProfiles.filter((profile) => profile.enabled).length} profiles`}
          detail={settings.defaultApiAuthProfileId || text(language, '无默认 Profile', 'No default profile')}
        />
        <StatusCard
          icon={ShieldCheck}
          label={text(language, '操作权限', 'Operation level')}
          ok
          value={operationLevelLabel(settings.operationLevel, language)}
          detail={operationLevelDetail(settings.operationLevel, language)}
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
