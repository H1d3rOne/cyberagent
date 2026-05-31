import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  createApiClient,
  defaultSettings,
  loadLocalDaemonBootstrap,
  loadSettings,
  saveSettings,
  type ConsoleSettings,
  type AgentProviderSettings,
  type AgentProviderProfile,
  type ScanDraft,
} from '@/lib/api'

const messages = {
  zh: {
    agent: 'Agent',
    approvals: '审批',
    artifacts: '证据',
    baseUrl: '后端地址',
    cancel: '取消',
    chat: '对话',
    completed: '完成',
    config: '配置',
    connect: '连接',
    connected: '在线',
    cost: '费用',
    daemon: 'Daemon',
    delete: '删除',
    doctor: '体检',
    duration: '耗时',
    events: '事件',
    failed: '失败',
    findings: '发现',
    health: '健康',
    history: '历史',
    interactive: '交互',
    job: '作业',
    language: '语言',
    launch: '发起',
    loading: '加载中',
    model: '模型',
    multiAgent: '多Agent',
    noData: '无数据',
    offline: '离线',
    operator: '操作员',
    overview: '总览',
    plugins: '插件',
    profile: '策略',
    prompt: '提示词',
    provider: '提供商',
    refresh: '刷新',
    report: '报告',
    running: '运行中',
    runs: '运行',
    save: '保存',
    scan: '扫描',
    scope: '范围',
    send: '发送',
    settings: '设置',
    skills: 'Skills',
    mcp: 'MCP',
    start: '启动',
    stop: '停止',
    stopped: '已停止',
    succeeded: '完成',
    target: '目标',
    tasks: '任务',
    token: '令牌',
    tokens: 'Token',
    tools: '工具',
    turns: '轮次',
    version: '版本',
  },
  en: {
    agent: 'Agent',
    approvals: 'Approvals',
    artifacts: 'Artifacts',
    baseUrl: 'Backend URL',
    cancel: 'Cancel',
    chat: 'Chat',
    completed: 'Completed',
    config: 'Config',
    connect: 'Connect',
    connected: 'Online',
    cost: 'Cost',
    daemon: 'Daemon',
    delete: 'Delete',
    doctor: 'Doctor',
    duration: 'Duration',
    events: 'Events',
    failed: 'Failed',
    findings: 'Findings',
    health: 'Health',
    history: 'History',
    interactive: 'Interactive',
    job: 'Job',
    language: 'Language',
    launch: 'Launch',
    loading: 'Loading',
    model: 'Model',
    multiAgent: 'Multi-Agent',
    noData: 'No data',
    offline: 'Offline',
    operator: 'Operator',
    overview: 'Overview',
    plugins: 'Plugins',
    profile: 'Profile',
    prompt: 'Prompt',
    provider: 'Provider',
    refresh: 'Refresh',
    report: 'Report',
    running: 'Running',
    runs: 'Runs',
    save: 'Save',
    scan: 'Scan',
    scope: 'Scope',
    send: 'Send',
    settings: 'Settings',
    skills: 'Skills',
    mcp: 'MCP',
    start: 'Start',
    stop: 'Stop',
    stopped: 'Stopped',
    succeeded: 'Done',
    target: 'Target',
    tasks: 'Tasks',
    token: 'Token',
    tokens: 'Tokens',
    tools: 'Tools',
    turns: 'Turns',
    version: 'Version',
  },
} as const

export type Language = keyof typeof messages
export type MessageKey = keyof typeof messages.zh
type ConsoleSettingsPatch = Omit<Partial<ConsoleSettings>, 'agentProvider' | 'scanDraft'> & {
  agentProvider?: Partial<AgentProviderSettings>
  scanDraft?: Partial<ScanDraft>
}

interface ConsoleContextValue {
  settings: ConsoleSettings
  updateSettings: (patch: ConsoleSettingsPatch) => void
  resetSettings: () => void
  client: ReturnType<typeof createApiClient>
  language: Language
  setLanguage: (language: Language) => void
  t: (key: MessageKey) => string
}

const ConsoleContext = createContext<ConsoleContextValue | null>(null)

export function ConsoleProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<ConsoleSettings>(() => loadSettings())
  const [bootstrapped, setBootstrapped] = useState(false)

  useEffect(() => {
    let cancelled = false
    loadLocalDaemonBootstrap()
      .then((bootstrap) => {
        if (cancelled || !bootstrap?.available) return
        const baseUrl = (bootstrap.base_url || '').trim()
        const token = (bootstrap.token || '').trim()
        if (!baseUrl || !token) return
        setSettings((current) => {
          const currentBaseUrl = current.baseUrl.trim()
          const shouldApply =
            !current.token.trim() ||
            currentBaseUrl === defaultSettings.baseUrl ||
            isLocalDaemonBaseUrl(currentBaseUrl)
          if (!shouldApply) return current
          if (current.baseUrl === baseUrl && current.token === token) return current
          return { ...current, baseUrl, token }
        })
      })
      .catch(() => {
        // The bootstrap endpoint only exists on the bundled local dashboard.
      })
      .finally(() => {
        if (!cancelled) setBootstrapped(true)
      })
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    saveSettings(settings)
    document.documentElement.lang = settings.language === 'zh' ? 'zh-CN' : 'en'
  }, [settings])

  const updateSettings = useCallback((patch: ConsoleSettingsPatch) => {
    setSettings((current) => reconcileActiveProvider({
      ...current,
      ...patch,
      agentProvider: {
        ...current.agentProvider,
        ...(patch.agentProvider || {}),
      },
      scanDraft: {
        ...current.scanDraft,
        ...(patch.scanDraft || {}),
      },
    }))
  }, [])

  const resetSettings = useCallback(() => {
    setSettings(defaultSettings)
  }, [])

  const setLanguage = useCallback((language: Language) => {
    setSettings((current) => ({ ...current, language }))
  }, [])

  const client = useMemo(
    () => createApiClient({ baseUrl: settings.baseUrl, token: settings.token }),
    [settings.baseUrl, settings.token],
  )

  const t = useCallback(
    (key: MessageKey) => messages[settings.language][key] || messages.en[key] || key,
    [settings.language],
  )

  const value = useMemo(
    () => ({ settings, updateSettings, resetSettings, client, language: settings.language, setLanguage, t }),
    [client, resetSettings, setLanguage, settings, t, updateSettings],
  )

  if (!bootstrapped) {
    return <ConsoleBootstrapScreen language={settings.language} />
  }

  return <ConsoleContext.Provider value={value}>{children}</ConsoleContext.Provider>
}

export function useConsole() {
  const context = useContext(ConsoleContext)
  if (!context) throw new Error('useConsole must be used inside ConsoleProvider')
  return context
}

export function text(language: Language, zh: string, en: string) {
  return language === 'zh' ? zh : en
}

function isLocalDaemonBaseUrl(value: string) {
  try {
    const url = new URL(value)
    return ['127.0.0.1', 'localhost', '::1'].includes(url.hostname)
  } catch {
    return false
  }
}

function reconcileActiveProvider(settings: ConsoleSettings): ConsoleSettings {
  const profiles = settings.aiProviders || []
  const active = profiles.find((profile) => profile.id === settings.activeAiProviderId) || profiles[0]
  if (!active) return settings
  return {
    ...settings,
    activeAiProviderId: active.id,
    agentProvider: agentProviderSettingsFromProfile(active),
  }
}

function agentProviderSettingsFromProfile(profile: AgentProviderProfile): AgentProviderSettings {
  return {
    provider: profile.type,
    model: profile.model,
    apiKey: profile.apiKey,
    providerBaseUrl: profile.baseUrl,
    instructions: profile.instructions,
    maxTurns: profile.maxTurns,
  }
}

function ConsoleBootstrapScreen({ language }: { language: Language }) {
  return (
    <div className="grid min-h-dvh place-items-center bg-slate-950 px-6 text-slate-100">
      <div className="w-full max-w-sm rounded-3xl border border-blue-400/15 bg-slate-900/70 p-6 shadow-2xl shadow-blue-950/40">
        <div className="mb-5 flex items-center gap-3">
          <div className="grid size-11 place-items-center rounded-2xl border border-blue-300/20 bg-blue-500/10">
            <span className="size-2.5 animate-pulse rounded-full bg-cyan-300 shadow-[0_0_24px_rgba(34,211,238,0.75)]" />
          </div>
          <div>
            <p className="text-sm font-semibold tracking-[0.22em] text-blue-100">CyberAgent</p>
            <p className="text-xs text-slate-400">
              {text(language, '正在连接本地 Agent Runtime', 'Connecting local Agent Runtime')}
            </p>
          </div>
        </div>
        <div className="h-1.5 overflow-hidden rounded-full bg-slate-800">
          <div className="h-full w-2/3 animate-pulse rounded-full bg-gradient-to-r from-blue-500 via-cyan-300 to-emerald-300" />
        </div>
      </div>
    </div>
  )
}
