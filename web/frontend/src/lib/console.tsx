import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { createApiClient, defaultSettings, loadSettings, saveSettings, type ConsoleSettings } from '@/lib/api'

const messages = {
  zh: {
    active: '活动',
    allowTools: '外部工具',
    approvals: '审批',
    approve: '通过',
    artifacts: '证据',
    baseUrl: '地址',
    cancel: '取消',
    cancelled: '已取消',
    comment: '备注',
    config: '配置',
    connect: '连接',
    connected: '在线',
    copy: '复制',
    daemon: 'Daemon',
    deny: '拒绝',
    doctor: '体检',
    empty: '空',
    events: '事件',
    externalOff: '关闭',
    externalOn: '启用',
    failed: '失败',
    findings: '发现',
    health: '健康',
    job: '作业',
    launch: '发起',
    loading: '加载',
    noData: '无数据',
    offline: '离线',
    operator: '操作员',
    overview: '总览',
    pending: '待审',
    plugins: '插件',
    profile: '策略',
    refresh: '刷新',
    report: '报告',
    resume: '恢复',
    running: '运行',
    runs: '运行',
    safe: '安全',
    save: '保存',
    scan: '扫描',
    scope: '范围',
    settings: '设置',
    start: '启动',
    succeeded: '完成',
    target: '目标',
    token: '令牌',
    tools: '工具',
    version: '版本',
  },
  en: {
    active: 'Active',
    allowTools: 'Tools',
    approvals: 'Approvals',
    approve: 'Approve',
    artifacts: 'Artifacts',
    baseUrl: 'Base',
    cancel: 'Cancel',
    cancelled: 'Cancelled',
    comment: 'Note',
    config: 'Config',
    connect: 'Connect',
    connected: 'Online',
    copy: 'Copy',
    daemon: 'Daemon',
    deny: 'Deny',
    doctor: 'Doctor',
    empty: 'Empty',
    events: 'Events',
    externalOff: 'Off',
    externalOn: 'On',
    failed: 'Failed',
    findings: 'Findings',
    health: 'Health',
    job: 'Job',
    launch: 'Launch',
    loading: 'Loading',
    noData: 'No data',
    offline: 'Offline',
    operator: 'Operator',
    overview: 'Overview',
    pending: 'Pending',
    plugins: 'Plugins',
    profile: 'Profile',
    refresh: 'Refresh',
    report: 'Report',
    resume: 'Resume',
    running: 'Running',
    runs: 'Runs',
    safe: 'Safe',
    save: 'Save',
    scan: 'Scan',
    scope: 'Scope',
    settings: 'Settings',
    start: 'Start',
    succeeded: 'Done',
    target: 'Target',
    token: 'Token',
    tools: 'Tools',
    version: 'Version',
  },
} as const

export type Language = keyof typeof messages
export type MessageKey = keyof typeof messages.zh

interface ConsoleContextValue {
  settings: ConsoleSettings
  updateSettings: (patch: Partial<ConsoleSettings>) => void
  resetSettings: () => void
  client: ReturnType<typeof createApiClient>
  language: Language
  setLanguage: (language: Language) => void
  t: (key: MessageKey) => string
}

const ConsoleContext = createContext<ConsoleContextValue | null>(null)

export function ConsoleProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<ConsoleSettings>(() => loadSettings())

  useEffect(() => {
    saveSettings(settings)
    document.documentElement.lang = settings.language === 'zh' ? 'zh-CN' : 'en'
  }, [settings])

  const updateSettings = useCallback((patch: Partial<ConsoleSettings>) => {
    setSettings((current) => ({ ...current, ...patch }))
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
    () => ({
      settings,
      updateSettings,
      resetSettings,
      client,
      language: settings.language,
      setLanguage,
      t,
    }),
    [client, resetSettings, setLanguage, settings, t, updateSettings],
  )

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
