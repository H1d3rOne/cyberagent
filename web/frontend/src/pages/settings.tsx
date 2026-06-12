import { useCallback, useEffect, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  Bot,
  BriefcaseBusiness,
  Check,
  Globe2,
  KeyRound,
  Languages,
  LogIn,
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
import type { AgentProviderProfile, AgentProviderType, ApiAuthProfile, ApiAuthProfileType, Provider, WebSearchConfig } from '@/lib/api'
import { defaultWebSearchConfig, normalizeAgentOperationLevel, type AgentOperationLevel } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import {
  consoleSessionAuthLabel,
  consoleSessionIdentityLabel,
  consoleSessionModeLabel,
  consoleSessionScopeLabel,
  consoleSessionWorkspaceLabel,
  normalizeConsoleSession,
} from '@/lib/session'
import { cn } from '@/lib/utils'

const PROVIDER_TYPES: { id: AgentProviderType; name: string; defaultModel: string; detail: string }[] = [
  { id: 'openai', name: 'OpenAI', defaultModel: 'gpt-4o', detail: 'Chat Completions / compatible' },
  { id: 'anthropic', name: 'Anthropic', defaultModel: 'claude-sonnet-4-20250514', detail: 'Claude Messages / compatible' },
]

const OPERATION_LEVELS: AgentOperationLevel[] = ['read_only', 'workspace', 'safe_scan', 'approval_ops', 'advanced', 'bypass']
const AUTH_PROFILE_TYPES: ApiAuthProfileType[] = ['bearer', 'basic', 'api_key', 'cookie', 'custom']
const WEB_SEARCH_PROVIDERS = [
  { id: 'disabled', name: 'Disabled' },
  { id: 'tavily', name: 'Tavily' },
  { id: 'brave', name: 'Brave' },
  { id: 'serpapi', name: 'SerpAPI' },
]

type ApiAuthSecretField = 'token' | 'password' | 'headerValue' | 'cookie'
type ApiAuthSecretDrafts = Record<string, Partial<Record<ApiAuthSecretField, string>>>

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
	const emptySecret = !only.apiKey?.trim() && !only.apiKeyConfigured && !only.baseUrl?.trim()
	return emptySecret && (only.id === 'openai-default' || !only.model?.trim())
}

function shouldHydrateAuthProfiles(profiles: ApiAuthProfile[]) {
  return !profiles.length
}

function apiAuthProfileSecretConfigured(profile: ApiAuthProfile) {
  switch (profile.type) {
    case 'basic':
      return Boolean(profile.passwordConfigured || profile.password.trim())
    case 'api_key':
      return Boolean(profile.headerValueConfigured || profile.tokenConfigured || profile.headerValue.trim() || profile.token.trim())
    case 'cookie':
      return Boolean(profile.cookieConfigured || profile.headerValueConfigured || profile.cookie.trim() || profile.headerValue.trim())
    case 'custom':
      return Boolean(profile.headerValueConfigured || profile.headerValue.trim())
    default:
      return Boolean(profile.tokenConfigured || profile.headerValueConfigured || profile.token.trim() || profile.headerValue.trim())
  }
}

function authProfileDraftValue(drafts: ApiAuthSecretDrafts, id: string, field: ApiAuthSecretField) {
  return drafts[id]?.[field] || ''
}

function authProfileDraftHasSecret(drafts: ApiAuthSecretDrafts, id: string) {
  const draft = drafts[id]
  return Boolean(draft && Object.values(draft).some((value) => (value || '').trim()))
}

function apiAuthProfileSecretFieldConfigured(profile: ApiAuthProfile, field: ApiAuthSecretField) {
  switch (field) {
    case 'token':
      return Boolean(profile.tokenConfigured || profile.token.trim())
    case 'password':
      return Boolean(profile.passwordConfigured || profile.password.trim())
    case 'headerValue':
      return Boolean(profile.headerValueConfigured || profile.headerValue.trim())
    case 'cookie':
      return Boolean(profile.cookieConfigured || profile.cookie.trim())
  }
}

export function SettingsPage() {
  const { client, language, serverScopeStatus, setLanguage, settings, updateSettings, resetSettings, t } = useConsole()
  const [saved, setSaved] = useState(false)
  const [checking, setChecking] = useState(false)
  const [error, setError] = useState('')
  const [connected, setConnected] = useState(false)
  const [providers, setProviders] = useState<Provider[]>([])
	const [savingProviders, setSavingProviders] = useState(false)
	const [providerSaveResult, setProviderSaveResult] = useState<{ ok: boolean; message: string } | null>(null)
	const [webSearch, setWebSearch] = useState<WebSearchConfig>(defaultWebSearchConfig)
	const [providerSecretDrafts, setProviderSecretDrafts] = useState<Record<string, string>>({})
	const [savingWebSearch, setSavingWebSearch] = useState(false)
	const [webSearchSaveResult, setWebSearchSaveResult] = useState<{ ok: boolean; message: string } | null>(null)
	const [authProfileSecretDrafts, setAuthProfileSecretDrafts] = useState<ApiAuthSecretDrafts>({})
	const [savingAuthProfiles, setSavingAuthProfiles] = useState(false)
	const [authProfileSaveResult, setAuthProfileSaveResult] = useState<{ ok: boolean; message: string } | null>(null)
	const [testing, setTesting] = useState(false)
	const [testResult, setTestResult] = useState<{ ok: boolean; message: string; requestUrl?: string } | null>(null)

	const aiProviders = settings.aiProviders || []
	const activeProvider = aiProviders.find((p) => p.id === settings.activeAiProviderId) || aiProviders[0]
	const providerType = PROVIDER_TYPES.find((tp) => tp.id === activeProvider?.type)
	const activeProviderSecretDraft = activeProvider ? providerSecretDrafts[activeProvider.id] || '' : ''
	const activeProviderSecretConfigured = Boolean(activeProvider?.apiKeyConfigured || activeProvider?.apiKey.trim() || activeProviderSecretDraft.trim())
	const providerReady = Boolean(activeProviderSecretConfigured || activeProvider?.baseUrl.trim())
	const webSearchEnabled = webSearch.provider !== 'disabled'
	const webSearchSecretConfigured = Boolean(webSearch.apiKeyConfigured || webSearch.apiKey.trim())
	const webSearchReady = webSearchEnabled && Boolean(webSearchSecretConfigured || webSearch.baseUrl.trim())

  const check = useCallback(async () => {
    setChecking(true)
    setError('')
    try {
      const providerList = await client.getProviders()
      setProviders(providerList || [])
      setConnected(true)
      try {
        const remote = await client.getProviderProfiles()
        setWebSearch(remote.webSearch || defaultWebSearchConfig)
        if (remote.providers.length && shouldHydrateProviderProfiles(settings.aiProviders || [])) {
          updateSettings({ aiProviders: remote.providers, activeAiProviderId: remote.activeId || remote.providers[0].id })
        }
        if (remote.authProfiles.length && shouldHydrateAuthProfiles(settings.apiAuthProfiles || [])) {
          updateSettings({
            apiAuthProfiles: remote.authProfiles,
            defaultApiAuthProfileId: remote.defaultAuthProfileId,
          })
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
  }, [client, settings.aiProviders, settings.apiAuthProfiles, updateSettings])

  useEffect(() => {
    check()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [client])

  const save = () => {
    setSaved(true)
    window.setTimeout(() => setSaved(false), 1600)
  }

  const clearLocalSettings = () => {
    resetSettings()
    setError('')
    setProviderSaveResult(null)
    setWebSearchSaveResult(null)
    setTestResult(null)
    setProviderSecretDrafts({})
    setAuthProfileSecretDrafts({})
    setAuthProfileSaveResult(null)
    setWebSearch(defaultWebSearchConfig)
    save()
  }

  const updateActiveProvider = (patch: Partial<AgentProviderProfile>) => {
    if (!activeProvider) return
    setTestResult(null)
    setProviderSaveResult(null)
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
          apiKeyConfigured: false,
          baseUrl: '',
          instructions: '',
          maxTurns: 50,
        },
      ],
      activeAiProviderId: id,
    })
  }

  const updateActiveProviderSecretDraft = (value: string) => {
    if (!activeProvider) return
    setTestResult(null)
    setProviderSaveResult(null)
    setProviderSecretDrafts((current) => ({ ...current, [activeProvider.id]: value }))
  }

  const deleteProvider = (id: string) => {
    if (aiProviders.length <= 1) return
    const next = aiProviders.filter((p) => p.id !== id)
    setProviderSecretDrafts((current) => {
      const nextDrafts = { ...current }
      delete nextDrafts[id]
      return nextDrafts
    })
    updateSettings({
      aiProviders: next,
      activeAiProviderId: settings.activeAiProviderId === id ? next[0]?.id || '' : settings.activeAiProviderId,
    })
  }

  const providersWithSecretDrafts = () => aiProviders.map((provider) => ({
    ...provider,
    apiKey: providerSecretDrafts[provider.id] || '',
  }))

  const markProviderSecretDraftsSaved = () => {
    const draftedProviderIds = new Set(
      Object.entries(providerSecretDrafts)
        .filter(([, value]) => value.trim())
        .map(([id]) => id),
    )
    const hasLegacyProviderSecrets = aiProviders.some((provider) => provider.apiKey.trim())
    if (draftedProviderIds.size || hasLegacyProviderSecrets) {
      updateSettings({
        aiProviders: aiProviders.map((provider) => ({
          ...provider,
          apiKey: '',
          apiKeyConfigured: provider.apiKeyConfigured || draftedProviderIds.has(provider.id) || Boolean(provider.apiKey.trim()),
        })),
      })
    }
    setProviderSecretDrafts({})
  }

  const markWebSearchSecretSaved = () => {
    setWebSearch((current) => ({
      ...current,
      apiKey: '',
      apiKeyConfigured: current.provider !== 'disabled' && (current.apiKeyConfigured || Boolean(current.apiKey.trim())),
    }))
  }

  const saveProviders = async () => {
    setSavingProviders(true)
    setProviderSaveResult(null)
    setError('')
    if (!connected) {
      setSavingProviders(false)
      setProviderSaveResult({
        ok: false,
        message: text(language, '后端未连接，先点连接', 'Backend offline — click Connect first'),
      })
      return
    }
    try {
      await client.saveProviderProfiles(providersWithSecretDrafts(), settings.activeAiProviderId, webSearch)
      markProviderSecretDraftsSaved()
      markWebSearchSecretSaved()
      setProviderSaveResult({
        ok: true,
        message: text(language, '已写入后端', 'Persisted to backend'),
      })
      save()
    } catch (err) {
      const message = err instanceof Error ? err.message : 'save failed'
      setProviderSaveResult({ ok: false, message })
      setError(message)
    } finally {
      setSavingProviders(false)
    }
  }

  const saveWebSearch = async () => {
    setSavingWebSearch(true)
    setWebSearchSaveResult(null)
    setError('')
    if (!connected) {
      setSavingWebSearch(false)
      setWebSearchSaveResult({
        ok: false,
        message: text(language, '后端未连接，先点连接', 'Backend offline — click Connect first'),
      })
      return
    }
    try {
      await client.saveProviderProfiles(providersWithSecretDrafts(), settings.activeAiProviderId, webSearch)
      markProviderSecretDraftsSaved()
      markWebSearchSecretSaved()
      setWebSearchSaveResult({
        ok: true,
        message: text(language, '已写入后端', 'Persisted to backend'),
      })
      save()
    } catch (err) {
      const message = err instanceof Error ? err.message : 'save failed'
      setWebSearchSaveResult({ ok: false, message })
      setError(message)
    } finally {
      setSavingWebSearch(false)
    }
  }

  const testProvider = async () => {
    if (!activeProvider) return
    if (!activeProviderSecretDraft.trim() && activeProvider.apiKeyConfigured && !activeProvider.apiKey.trim()) {
      setTestResult({
        ok: true,
        message: text(language, 'API Key 已在 daemon 配置；如需重新测试，请输入新 key。', 'API key is configured on the daemon; enter a new key to re-test.'),
      })
      return
    }
    setTesting(true)
    setTestResult(null)
    setProviderSaveResult(null)
    try {
      const res = await client.testProvider({
        type: activeProvider.type,
        model: activeProvider.model,
        api_key: activeProviderSecretDraft || activeProvider.apiKey,
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
  const session = normalizeConsoleSession(settings.session, settings.operator)
  const sessionMode = consoleSessionModeLabel(session, language)
  const sessionIdentity = consoleSessionIdentityLabel(session, language)
  const sessionScope = consoleSessionScopeLabel(session)
  const sessionWorkspace = consoleSessionWorkspaceLabel(session)
  const sessionAuth = consoleSessionAuthLabel(session, language)
  const serverScopeStatusLabel = serverScopeStatus === 'synced'
    ? text(language, '后端 /api/v1/me 已同步', 'Server /api/v1/me synced')
    : serverScopeStatus === 'loading'
      ? text(language, '正在同步后端作用域', 'Syncing server scope')
      : text(language, '使用本地默认作用域回退', 'Using local default scope fallback')

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
          secretConfigured: false,
          tokenConfigured: false,
          passwordConfigured: false,
          headerValueConfigured: false,
          cookieConfigured: false,
        },
      ],
      defaultApiAuthProfileId: settings.defaultApiAuthProfileId || id,
    })
  }

  const updateAuthProfile = (id: string, patch: Partial<ApiAuthProfile>) => {
    const nextId = patch.id || id
    setAuthProfileSaveResult(null)
    if (nextId !== id) {
      setAuthProfileSecretDrafts((current) => {
        if (!current[id]) return current
        const next = { ...current, [nextId]: current[id] }
        delete next[id]
        return next
      })
    }
    updateSettings({
      apiAuthProfiles: authProfiles.map((profile) => (profile.id === id ? { ...profile, ...patch } : profile)),
      defaultApiAuthProfileId: settings.defaultApiAuthProfileId === id ? nextId : settings.defaultApiAuthProfileId,
    })
  }

  const updateAuthProfileId = (profile: ApiAuthProfile, value: string) => {
    const nextId = value.trim() || profile.id
    if (nextId === profile.id) {
      updateAuthProfile(profile.id, { id: nextId })
      return
    }
    updateAuthProfile(profile.id, {
      id: nextId,
      token: '',
      password: '',
      headerValue: '',
      cookie: '',
      secretConfigured: false,
      tokenConfigured: false,
      passwordConfigured: false,
      headerValueConfigured: false,
      cookieConfigured: false,
    })
  }

  const deleteAuthProfile = (id: string) => {
    const nextProfiles = authProfiles.filter((profile) => profile.id !== id)
    setAuthProfileSecretDrafts((current) => {
      const next = { ...current }
      delete next[id]
      return next
    })
    updateSettings({
      apiAuthProfiles: nextProfiles,
      defaultApiAuthProfileId: settings.defaultApiAuthProfileId === id ? nextProfiles[0]?.id || '' : settings.defaultApiAuthProfileId,
    })
  }

  const updateAuthProfileSecretDraft = (id: string, field: ApiAuthSecretField, value: string) => {
    setAuthProfileSaveResult(null)
    setAuthProfileSecretDrafts((current) => ({
      ...current,
      [id]: {
        ...(current[id] || {}),
        [field]: value,
      },
    }))
  }

  const clearAuthProfileSecretDraft = (id: string) => {
    setAuthProfileSecretDrafts((current) => {
      const next = { ...current }
      delete next[id]
      return next
    })
  }

  const updateAuthProfileType = (profile: ApiAuthProfile, type: ApiAuthProfileType) => {
    clearAuthProfileSecretDraft(profile.id)
    updateAuthProfile(profile.id, {
      type,
      token: '',
      password: '',
      headerValue: '',
      cookie: '',
      secretConfigured: false,
      tokenConfigured: false,
      passwordConfigured: false,
      headerValueConfigured: false,
      cookieConfigured: false,
    })
  }

  const authProfilesWithSecretDrafts = () => authProfiles.map((profile) => ({
    ...profile,
    token: authProfileDraftValue(authProfileSecretDrafts, profile.id, 'token') || profile.token,
    password: authProfileDraftValue(authProfileSecretDrafts, profile.id, 'password') || profile.password,
    headerValue: authProfileDraftValue(authProfileSecretDrafts, profile.id, 'headerValue') || profile.headerValue,
    cookie: authProfileDraftValue(authProfileSecretDrafts, profile.id, 'cookie') || profile.cookie,
  }))

  const markAuthProfileSecretDraftsSaved = () => {
    const draftedProfileIds = new Set(
      Object.entries(authProfileSecretDrafts)
        .filter(([, draft]) => Object.values(draft).some((value) => (value || '').trim()))
        .map(([id]) => id),
    )
    const hasLegacySecrets = authProfiles.some(apiAuthProfileSecretConfigured)
    if (draftedProfileIds.size || hasLegacySecrets) {
      updateSettings({
        apiAuthProfiles: authProfiles.map((profile) => {
          const saved = draftedProfileIds.has(profile.id) || apiAuthProfileSecretConfigured(profile)
          return {
            ...profile,
            token: '',
            password: '',
            headerValue: '',
            cookie: '',
            secretConfigured: profile.secretConfigured || saved,
            tokenConfigured: profile.tokenConfigured || Boolean(profile.token.trim()) || Boolean(authProfileDraftValue(authProfileSecretDrafts, profile.id, 'token').trim()),
            passwordConfigured: profile.passwordConfigured || Boolean(profile.password.trim()) || Boolean(authProfileDraftValue(authProfileSecretDrafts, profile.id, 'password').trim()),
            headerValueConfigured: profile.headerValueConfigured || Boolean(profile.headerValue.trim()) || Boolean(authProfileDraftValue(authProfileSecretDrafts, profile.id, 'headerValue').trim()),
            cookieConfigured: profile.cookieConfigured || Boolean(profile.cookie.trim()) || Boolean(authProfileDraftValue(authProfileSecretDrafts, profile.id, 'cookie').trim()),
          }
        }),
      })
    }
    setAuthProfileSecretDrafts({})
  }

  const saveAuthProfiles = async () => {
    setSavingAuthProfiles(true)
    setAuthProfileSaveResult(null)
    setError('')
    if (!connected) {
      setSavingAuthProfiles(false)
      setAuthProfileSaveResult({
        ok: false,
        message: text(language, '后端未连接，先点连接', 'Backend offline — click Connect first'),
      })
      return
    }
    try {
      await client.saveApiAuthProfiles(authProfilesWithSecretDrafts(), settings.defaultApiAuthProfileId)
      markAuthProfileSecretDraftsSaved()
      setAuthProfileSaveResult({
        ok: true,
        message: text(language, '已写入后端', 'Persisted to backend'),
      })
      save()
    } catch (err) {
      const message = err instanceof Error ? err.message : 'save failed'
      setAuthProfileSaveResult({ ok: false, message })
      setError(message)
    } finally {
      setSavingAuthProfiles(false)
    }
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
                  '控制台设置会自动保存到当前浏览器 localStorage；daemon token 仍保存在本地。AI Provider、Web Search 与 API Auth Profile secret 只作为本次输入草稿，点击保存后写入 daemon，前端只显示配置状态。',
                  'Console settings are auto-saved to this browser localStorage; the daemon token is still stored locally. AI Provider, Web Search and API Auth Profile secrets are in-memory drafts until saved to the daemon; the frontend shows configuration status only.',
                )}
              </PageDescription>
            </div>
          </PageHeader>
          <div className="mt-4 rounded-2xl border border-amber-300/20 bg-amber-400/10 p-3 text-xs leading-5 text-amber-50">
            <div className="mb-1 flex items-center gap-2 font-semibold">
              <AlertTriangle className="h-3.5 w-3.5 text-amber-200" />
              {text(language, '密钥存储说明', 'Secret storage note')}
            </div>
            <p className="text-amber-100/80">
              {text(
                language,
                '本地持久化键为 cyberagent_console_settings。点击“清除本地配置”会恢复本地控制台设置并清空本地 daemon token、未保存的 provider/web-search/API auth secret 草稿；不会删除已保存到 daemon 的配置。',
                'The local persistence key is cyberagent_console_settings. “Clear local settings” restores local console settings and clears the local daemon token plus unsaved provider/web-search/API auth secret drafts; it does not delete config already saved to the daemon.',
              )}
            </p>
          </div>
        </Section>

        <div id="session-workspace" className="scroll-mt-24 rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
          <SectionHeader className="items-start">
            <div>
              <SectionTitle icon={UserRound}>{text(language, 'Session 与 Workspace', 'Session & Workspace')}</SectionTitle>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {text(
                  language,
                  'L3-A9 会从后端 GET /api/v1/me 同步真实 local/default 作用域展示；当前仍是本地 daemon bearer 模式，不改变 API 请求认证、不迁移 runs，也不会把前端 workspace 选择当作服务端隔离边界。',
                  'L3-A9 hydrates the displayed real local/default scope from backend GET /api/v1/me; the console still uses local daemon bearer auth, API requests are unchanged, runs are not migrated, and frontend workspace selection is not treated as a server-side isolation boundary.',
                )}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <span className="rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
                {sessionMode}
              </span>
              <span className={cn(
                'rounded-full border px-3 py-1 font-mono text-xs',
                serverScopeStatus === 'synced'
                  ? 'border-emerald-300/25 bg-emerald-400/10 text-emerald-200'
                  : serverScopeStatus === 'loading'
                    ? 'border-cyan-300/25 bg-cyan-400/10 text-cyan-200'
                    : 'border-amber-300/25 bg-amber-400/10 text-amber-100',
              )}>
                {serverScopeStatusLabel}
              </span>
            </div>
          </SectionHeader>

          <div className="grid gap-3 lg:grid-cols-3">
            <SessionInfoTile
              icon={UserRound}
              label={text(language, '当前身份', 'Current identity')}
              value={sessionIdentity}
              detail={`${session.principalId} · ${settings.operator}`}
            />
            <SessionInfoTile
              icon={BriefcaseBusiness}
              label={text(language, '工作区', 'Workspace')}
              value={sessionWorkspace}
              detail={sessionScope}
            />
            <SessionInfoTile
              icon={KeyRound}
              label={text(language, '认证方式', 'Auth method')}
              value={sessionAuth}
              detail={text(language, '继续使用 Settings 内的 daemon token', 'Still uses the daemon token in Settings')}
            />
          </div>

          <div className="mt-4 rounded-2xl border border-cyan-300/20 bg-cyan-400/10 p-3 text-xs leading-5 text-cyan-50">
            <div className="mb-1 flex items-center gap-2 font-semibold">
              <ShieldCheck className="h-3.5 w-3.5 text-cyan-200" />
              {text(language, '多用户登录占位', 'Multi-user login placeholder')}
            </div>
            <p className="text-cyan-100/80">
              {text(
                language,
                '真正的 Web 多用户登录需要后端 server-side session、tenant/workspace resolver、服务端 secret storage 与 audit/rate-limit 后才能开启；当前 UI 只展示 local/default scope，避免造成伪隔离。',
                'Real Web multi-user login must wait for backend server-side sessions, tenant/workspace resolver, server-side secret storage, audit, and rate limiting; this UI only exposes the local/default scope to avoid fake isolation.',
              )}
            </p>
          </div>

          <div className="mt-4 flex flex-wrap gap-2">
            <Button asChild type="button" variant="outline" className="min-h-11 rounded-2xl bg-transparent">
              <a href="#bearer-token">
                <KeyRound className="mr-2 h-4 w-4" />
                {text(language, '配置 daemon token', 'Configure daemon token')}
              </a>
            </Button>
            <Button type="button" variant="ghost" disabled className="min-h-11 rounded-2xl">
              <LogIn className="mr-2 h-4 w-4" />
              {text(language, '多用户登录待启用', 'Multi-user sign-in pending')}
            </Button>
          </div>
        </div>

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
              <div className="mt-1 text-[11px] leading-5 text-muted-foreground">
                {text(language, '该 daemon token 会自动保存到当前浏览器 localStorage。', 'This daemon token is auto-saved to this browser localStorage.')}
              </div>
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
            <Button
              type="button"
              variant="ghost"
              onClick={clearLocalSettings}
              className="rounded-2xl text-amber-100/85 hover:text-amber-50"
              title={text(language, '恢复默认并清空本地 token/未保存 secret 草稿', 'Restore defaults and clear local token/unsaved secret drafts')}
            >
              <RotateCcw className="mr-2 h-4 w-4" />
              {text(language, '清除本地配置', 'Clear local settings')}
            </Button>
          </div>
        </div>

        <div className="rounded-[1.5rem] border border-white/[0.08] bg-white/[0.03] p-4 sm:p-5">
          <SectionHeader>
            <SectionTitle icon={Globe2}>{text(language, 'Web Search', 'Web Search')}</SectionTitle>
            <span className="font-mono text-xs text-muted-foreground">web.search</span>
          </SectionHeader>

          <p className="mb-3 text-xs leading-5 text-muted-foreground">
            {text(
              language,
              '用于 Agent 的 web.search 工具。该 API Key 不写入控制台 settings localStorage；点击保存后写入 daemon provider 配置。未配置时 Agent 会返回 not_configured。',
              'Used by the Agent web.search tool. This API key is not written to console settings localStorage; clicking Save writes it to daemon provider config. When unset, the Agent returns not_configured.',
            )}
          </p>

          <div className="grid gap-4 lg:grid-cols-2">
            <Field icon={PlugZap} label={text(language, 'Search Provider', 'Search provider')} htmlFor="web-search-provider">
              <select
                id="web-search-provider"
                value={webSearch.provider}
                onChange={(event) => {
                  setWebSearch({
                    ...webSearch,
                    provider: event.target.value,
                    apiKey: '',
                    apiKeyConfigured: event.target.value === webSearch.provider ? webSearch.apiKeyConfigured : false,
                  })
                  setWebSearchSaveResult(null)
                }}
                className="h-10 w-full rounded-xl border border-input bg-background px-3 py-2 font-mono text-sm text-foreground ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              >
                {WEB_SEARCH_PROVIDERS.map((provider) => (
                  <option key={provider.id} value={provider.id}>{provider.name}</option>
                ))}
              </select>
            </Field>

            <Field icon={SlidersHorizontal} label={text(language, 'Max Results', 'Max results')} htmlFor="web-search-max-results">
              <Input
                id="web-search-max-results"
                type="number"
                value={webSearch.maxResults}
                onChange={(event) => {
                  const parsed = Number(event.target.value)
                  setWebSearch({ ...webSearch, maxResults: Number.isFinite(parsed) ? Math.min(10, Math.max(1, Math.round(parsed))) : 5 })
                  setWebSearchSaveResult(null)
                }}
                min={1}
                max={10}
                className="h-10 rounded-xl font-mono"
              />
            </Field>

            <Field icon={KeyRound} label="API Key" htmlFor="web-search-api-key">
              <Input
                id="web-search-api-key"
                value={webSearch.apiKey}
                onChange={(event) => {
                  setWebSearch({ ...webSearch, apiKey: event.target.value })
                  setWebSearchSaveResult(null)
                }}
                placeholder={webSearch.apiKeyConfigured ? text(language, '已在 daemon 配置，留空保留', 'Configured on daemon; leave blank to keep') : (webSearch.provider === 'tavily' ? 'tvly-...' : 'api key')}
                type="password"
                disabled={!webSearchEnabled}
                className="h-10 rounded-xl font-mono"
              />
              <div className="mt-1 text-[11px] leading-5 text-muted-foreground">
                {webSearch.apiKey.trim()
                  ? text(language, '已输入新的 secret 草稿；保存后写入 daemon 并清空输入框。', 'New secret draft entered; saving writes it to the daemon and clears this field.')
                  : webSearch.apiKeyConfigured
                    ? text(language, '已在 daemon 配置；留空保存会保留当前 secret。', 'Configured on the daemon; saving with this field blank keeps the current secret.')
                    : text(language, '未配置；输入后点击保存写入 daemon。', 'Not configured; enter a value and save to write it to the daemon.')}
              </div>
            </Field>

            <Field icon={Globe2} label="Base URL" htmlFor="web-search-base-url">
              <Input
                id="web-search-base-url"
                value={webSearch.baseUrl}
                onChange={(event) => {
                  setWebSearch({ ...webSearch, baseUrl: event.target.value })
                  setWebSearchSaveResult(null)
                }}
                placeholder="https://api.tavily.com/search"
                disabled={!webSearchEnabled}
                className="h-10 rounded-xl font-mono"
              />
              <div className="mt-1 font-mono text-[11px] text-muted-foreground">
                {text(language, '可留空使用 provider 默认 endpoint；自定义 URL 会经过 SSRF 防护。', 'Leave blank for the provider default endpoint; custom URLs pass SSRF checks.')}
              </div>
            </Field>
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-2">
            <Button type="button" onClick={saveWebSearch} disabled={savingWebSearch} className="rounded-2xl">
              {saved ? <Check className="mr-2 h-4 w-4" /> : <Save className={cn('mr-2 h-4 w-4', savingWebSearch && 'animate-spin')} />}
              {saved ? text(language, '已保存', 'Saved') : t('save')}
            </Button>
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setWebSearch(defaultWebSearchConfig)
                setWebSearchSaveResult(null)
              }}
              className="rounded-2xl"
            >
              <RotateCcw className="mr-2 h-4 w-4" />
              {text(language, '重置', 'Reset')}
            </Button>
            {webSearchSaveResult && (
              <span className={cn('max-w-full break-all font-mono text-xs', webSearchSaveResult.ok ? 'text-emerald-300' : 'text-red-300')}>
                {webSearchSaveResult.ok ? '✓ ' : '✗ '}{webSearchSaveResult.message}
              </span>
            )}
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

          <p className="mb-3 text-xs leading-5 text-muted-foreground">
            {text(
              language,
              'Provider 非密钥字段会随控制台设置自动保存到当前浏览器 localStorage；API Key 只作为本次输入草稿，点击保存后写入 daemon。已配置的 secret 只显示状态，不回显明文。',
              'Non-secret provider fields are auto-saved with console settings to this browser localStorage; API keys are in-memory drafts until saved to the daemon. Configured secrets show status only and are never echoed back.',
            )}
          </p>

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
                  onChange={(event) => {
                    updateActiveProvider({ type: event.target.value as AgentProviderType, apiKey: '', apiKeyConfigured: false })
                    if (activeProvider) {
                      setProviderSecretDrafts((current) => ({ ...current, [activeProvider.id]: '' }))
                    }
                  }}
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
                  value={activeProviderSecretDraft}
                  onChange={(event) => updateActiveProviderSecretDraft(event.target.value)}
                  placeholder={activeProvider?.apiKeyConfigured ? text(language, '已在 daemon 配置，留空保留', 'Configured on daemon; leave blank to keep') : 'sk-...'}
                  type="password"
                  className="h-10 rounded-xl font-mono"
                />
                <div className="mt-1 text-[11px] leading-5 text-muted-foreground">
                  {activeProviderSecretDraft.trim()
                    ? text(language, '已输入新的 secret 草稿；保存后写入 daemon 并清空输入框。', 'New secret draft entered; saving writes it to the daemon and clears this field.')
                    : activeProvider?.apiKeyConfigured
                      ? text(language, '已在 daemon 配置；留空保存会保留当前 secret。', 'Configured on the daemon; saving with this field blank keeps the current secret.')
                      : text(language, '未配置；输入后点击保存写入 daemon。', 'Not configured; enter a value and save to write it to the daemon.')}
                </div>
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
              <Button type="button" onClick={saveProviders} disabled={savingProviders} className="rounded-2xl">
                {saved ? <Check className="mr-2 h-4 w-4" /> : <Save className={cn('mr-2 h-4 w-4', savingProviders && 'animate-spin')} />}
                {saved ? text(language, '已保存', 'Saved') : t('save')}
              </Button>
              <Button type="button" variant="outline" onClick={testProvider} disabled={testing} className="rounded-2xl bg-transparent">
                <Activity className={cn('mr-2 h-4 w-4', testing && 'animate-spin')} />
                {text(language, '测试连通性', 'Test connection')}
              </Button>
              {aiProviders.length > 1 && activeProvider && (
                <Button type="button" variant="ghost" onClick={() => deleteProvider(activeProvider.id)} className="rounded-2xl text-muted-foreground">
                  <Trash2 className="mr-2 h-4 w-4" />
                  {text(language, '删除', 'Delete')}
                </Button>
              )}
              {providerSaveResult && (
                <span className={cn('max-w-full break-all font-mono text-xs', providerSaveResult.ok ? 'text-emerald-300' : 'text-red-300')}>
                  {providerSaveResult.ok ? '✓ ' : '✗ '}{providerSaveResult.message}
                </span>
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
              <Button type="button" onClick={saveAuthProfiles} disabled={savingAuthProfiles || !authProfiles.length} size="sm" className="h-8 rounded-xl px-2">
                <Save className={cn('mr-1 h-3.5 w-3.5', savingAuthProfiles && 'animate-spin')} />
                {text(language, '保存到 daemon', 'Save to daemon')}
              </Button>
              <Button type="button" variant="outline" size="sm" onClick={addAuthProfile} className="h-8 rounded-xl bg-transparent px-2">
                <Plus className="mr-1 h-3.5 w-3.5" />
                {text(language, '新增', 'Add')}
              </Button>
            </div>
          </SectionHeader>

          <p className="mb-3 text-xs leading-5 text-muted-foreground">
            {text(
              language,
              '用于 http.request 工具的授权上下文。Profile secret 只作为本次内存草稿；点击“保存到 daemon”后写入服务端 provider config。前端与 Agent 调用只携带 profile metadata/configured 状态，daemon 会按 id+type 合并服务端 secret，LLM 只看到 profile ID。',
              'Authorization context for the http.request tool. Profile secrets are in-memory drafts only; clicking “Save to daemon” writes them to the server-side provider config. The frontend and Agent calls carry profile metadata/configured status only, the daemon merges server-side secrets by id+type, and the LLM sees profile IDs only.',
            )}
          </p>
          {authProfileSaveResult && (
            <div className={cn('mb-3 max-w-full break-all font-mono text-xs', authProfileSaveResult.ok ? 'text-emerald-300' : 'text-red-300')}>
              {authProfileSaveResult.ok ? '✓ ' : '✗ '}{authProfileSaveResult.message}
            </div>
          )}

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
                    onChange={(event) => updateAuthProfileType(profile, event.target.value as ApiAuthProfileType)}
                    className="h-8 rounded-xl border border-input bg-background px-2 font-mono text-xs text-foreground"
                  >
                    {AUTH_PROFILE_TYPES.map((type) => (
                      <option key={type} value={type}>{authProfileTypeLabel(type, language)}</option>
                    ))}
                  </select>
                  {authProfileDraftHasSecret(authProfileSecretDrafts, profile.id) && (
                    <span className="rounded-full border border-cyan-300/20 bg-cyan-300/10 px-2 py-1 text-[11px] font-mono text-cyan-200">
                      {text(language, 'secret 草稿未保存', 'secret draft unsaved')}
                    </span>
                  )}
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
                      onChange={(event) => updateAuthProfileId(profile, event.target.value)}
                      className="h-9 rounded-xl font-mono text-xs"
                    />
                    <div className="mt-1 font-mono text-[11px] text-muted-foreground">
                      {text(language, '修改 ID 会切换 secret scope；需重新保存 secret 草稿。', 'Changing the ID changes the secret scope; save a new secret draft.')}
                    </div>
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
                      <AuthProfileSecretField
                        icon={Lock}
                        label="Password"
                        htmlFor={`${profile.id}-password`}
                        language={language}
                        profile={profile}
                        drafts={authProfileSecretDrafts}
                        field="password"
                        placeholder="password"
                        onDraftChange={updateAuthProfileSecretDraft}
                      />
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
                      <AuthProfileSecretField
                        icon={Lock}
                        label="Header Value"
                        htmlFor={`${profile.id}-header-value`}
                        language={language}
                        profile={profile}
                        drafts={authProfileSecretDrafts}
                        field="headerValue"
                        placeholder={profile.type === 'api_key' ? 'api key' : 'header value'}
                        onDraftChange={updateAuthProfileSecretDraft}
                      />
                    </>
                  ) : profile.type === 'cookie' ? (
                    <AuthProfileSecretField
                      icon={Lock}
                      label="Cookie"
                      htmlFor={`${profile.id}-cookie`}
                      language={language}
                      profile={profile}
                      drafts={authProfileSecretDrafts}
                      field="cookie"
                      placeholder="session=..."
                      onDraftChange={updateAuthProfileSecretDraft}
                    />
                  ) : (
                    <AuthProfileSecretField
                      icon={Lock}
                      label="Bearer Token"
                      htmlFor={`${profile.id}-token`}
                      language={language}
                      profile={profile}
                      drafts={authProfileSecretDrafts}
                      field="token"
                      placeholder="token"
                      onDraftChange={updateAuthProfileSecretDraft}
                    />
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
          icon={UserRound}
          label={text(language, 'Session', 'Session')}
          ok
          value={sessionMode}
          detail={`${sessionIdentity} · ${sessionScope}`}
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
          icon={Globe2}
          label={text(language, 'Web Search', 'Web Search')}
          ok={!webSearchEnabled || webSearchReady}
          value={webSearchEnabled ? webSearch.provider : text(language, '禁用', 'disabled')}
          detail={webSearchEnabled ? `${webSearch.maxResults} max results` : text(language, '未启用联网搜索', 'Web search disabled')}
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
          value={text(language, '状态本地保存', 'Status saved locally')}
          detail={text(language, 'localStorage 不保存 provider/web/API auth secret', 'localStorage excludes provider/web/API auth secrets')}
        />
      </aside>
    </div>
  )
}

function SessionInfoTile({
  icon: Icon,
  label,
  value,
  detail,
}: {
  icon: React.ElementType
  label: string
  value: string
  detail: string
}) {
  return (
    <div className="rounded-2xl border border-white/[0.08] bg-white/[0.035] p-3">
      <div className="mb-3 flex items-center gap-2 text-xs text-muted-foreground">
        <span className="grid h-8 w-8 place-items-center rounded-xl border border-white/10 bg-white/[0.04]">
          <Icon className="h-4 w-4 text-primary" />
        </span>
        {label}
      </div>
      <div className="truncate font-mono text-sm font-semibold text-foreground">{value}</div>
      <div className="mt-1 truncate font-mono text-[11px] text-muted-foreground">{detail}</div>
    </div>
  )
}

function AuthProfileSecretField({
  icon: Icon,
  label,
  htmlFor,
  language,
  profile,
  drafts,
  field,
  placeholder,
  onDraftChange,
}: {
  icon: React.ElementType
  label: string
  htmlFor: string
  language: 'zh' | 'en'
  profile: ApiAuthProfile
  drafts: ApiAuthSecretDrafts
  field: ApiAuthSecretField
  placeholder: string
  onDraftChange: (id: string, field: ApiAuthSecretField, value: string) => void
}) {
  const draft = authProfileDraftValue(drafts, profile.id, field)
  const configured = apiAuthProfileSecretFieldConfigured(profile, field)
  return (
    <Field icon={Icon} label={label} htmlFor={htmlFor}>
      <Input
        id={htmlFor}
        value={draft}
        onChange={(event) => onDraftChange(profile.id, field, event.target.value)}
        placeholder={configured ? text(language, '已在 daemon 配置，留空保留', 'Configured on daemon; leave blank to keep') : placeholder}
        type="password"
        className="h-9 rounded-xl font-mono text-xs"
      />
      <div className={cn('mt-1 text-[11px] leading-5', draft.trim() ? 'text-cyan-300' : configured ? 'text-emerald-300' : 'text-muted-foreground')}>
        {draft.trim()
          ? text(language, '本次输入草稿，保存后写入 daemon。', 'Secret draft entered; saving writes it to the daemon.')
          : configured
            ? text(language, '已在 daemon 配置；留空保存会保留当前 secret。', 'Configured on the daemon; saving with this field blank keeps the current secret.')
            : text(language, '未配置；输入后点击“保存到 daemon”。', 'Not configured; enter a value and click “Save to daemon”.')}
      </div>
    </Field>
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
