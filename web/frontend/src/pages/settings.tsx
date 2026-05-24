import { useCallback, useEffect, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  Check,
  Globe2,
  KeyRound,
  Languages,
  PlugZap,
  RefreshCcw,
  RotateCcw,
  Save,
  ShieldCheck,
  UserRound,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { DoctorReport, PluginDoctorReport, VersionInfo } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

export function SettingsPage() {
  const { client, language, setLanguage, settings, updateSettings, resetSettings, t } = useConsole()
  const [saved, setSaved] = useState(false)
  const [checking, setChecking] = useState(false)
  const [error, setError] = useState('')
  const [version, setVersion] = useState<VersionInfo | null>(null)
  const [doctor, setDoctor] = useState<DoctorReport | null>(null)
  const [pluginDoctor, setPluginDoctor] = useState<PluginDoctorReport | null>(null)

  const check = useCallback(async () => {
    setChecking(true)
    setError('')
    try {
      const [versionInfo, doctorReport, pluginReport] = await Promise.all([
        client.version(),
        client.doctor(),
        client.pluginsDoctor(),
      ])
      setVersion(versionInfo)
      setDoctor(doctorReport)
      setPluginDoctor(pluginReport)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
      setVersion(null)
      setDoctor(null)
      setPluginDoctor(null)
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

  return (
    <div className="grid gap-6 xl:grid-cols-[1fr_420px]">
      <section className="space-y-6">
        <header className="rounded-[2rem] border border-white/10 bg-white/[0.035] p-5 sm:p-7">
          <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
            <PlugZap className="h-3.5 w-3.5" />
            {t('config')}
          </div>
          <h1 className="text-3xl font-semibold tracking-tight sm:text-5xl">{t('settings')}</h1>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
            {text(language, '本页只保存本地浏览器控制台配置：daemon 地址、Bearer token、操作员与语言。', 'This page stores only local console preferences: daemon URL, bearer token, operator and language.')}
          </p>
        </header>

        <div className="rounded-[2rem] border border-white/10 bg-white/[0.03] p-5 sm:p-7">
          <div className="mb-6 flex items-center gap-3">
            <span className="grid h-11 w-11 place-items-center rounded-2xl border border-blue-400/20 bg-blue-400/10 text-blue-300">
              <Globe2 className="h-5 w-5" />
            </span>
            <div>
              <h2 className="text-sm font-semibold">{t('daemon')}</h2>
              <p className="font-mono text-xs text-muted-foreground">/api/v1</p>
            </div>
          </div>

          <div className="grid gap-5">
            <Field icon={Globe2} label={t('baseUrl')} htmlFor="base-url">
              <Input
                id="base-url"
                value={settings.baseUrl}
                onChange={(event) => updateSettings({ baseUrl: event.target.value })}
                placeholder="http://127.0.0.1:8787"
                className="h-12 rounded-2xl font-mono"
              />
            </Field>

            <Field icon={KeyRound} label={t('token')} htmlFor="bearer-token">
              <Input
                id="bearer-token"
                value={settings.token}
                onChange={(event) => updateSettings({ token: event.target.value })}
                placeholder="Bearer token"
                type="password"
                className="h-12 rounded-2xl font-mono"
              />
            </Field>

            <Field icon={UserRound} label={t('operator')} htmlFor="operator">
              <Input
                id="operator"
                value={settings.operator}
                onChange={(event) => updateSettings({ operator: event.target.value })}
                placeholder="local-user"
                className="h-12 rounded-2xl font-mono"
              />
            </Field>
          </div>

          <div className="mt-6 flex flex-wrap gap-3">
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

        <div className="rounded-[2rem] border border-white/10 bg-white/[0.03] p-5 sm:p-7">
          <div className="mb-5 flex items-center gap-3">
            <span className="grid h-11 w-11 place-items-center rounded-2xl border border-violet-400/20 bg-violet-400/10 text-violet-300">
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
                'rounded-3xl border p-4 text-left transition',
                language === 'zh' ? 'border-primary/35 bg-primary/10 text-foreground' : 'border-white/10 bg-black/20 text-muted-foreground hover:bg-white/[0.04]',
              )}
            >
              <span className="block text-lg font-semibold">中文</span>
              <span className="mt-1 block text-xs">简体中文界面</span>
            </button>
            <button
              type="button"
              onClick={() => setLanguage('en')}
              className={cn(
                'rounded-3xl border p-4 text-left transition',
                language === 'en' ? 'border-primary/35 bg-primary/10 text-foreground' : 'border-white/10 bg-black/20 text-muted-foreground hover:bg-white/[0.04]',
              )}
            >
              <span className="block text-lg font-semibold">English</span>
              <span className="mt-1 block text-xs">English interface</span>
            </button>
          </div>
        </div>
      </section>

      <aside className="space-y-4">
        {error && (
          <div className="rounded-[2rem] border border-red-400/20 bg-red-500/10 p-5 text-sm text-red-100">
            <div className="mb-2 flex items-center gap-2 font-semibold">
              <AlertTriangle className="h-4 w-4" />
              {t('offline')}
            </div>
            <p className="leading-6">{error}</p>
          </div>
        )}

        <StatusCard
          icon={Activity}
          label={t('version')}
          ok={Boolean(version)}
          value={version?.version || '—'}
          detail={version?.module || 'github.com/H1d3rOne/cyberagent'}
        />
        <StatusCard
          icon={ShieldCheck}
          label={t('doctor')}
          ok={doctor?.ok !== false && Boolean(doctor)}
          value={doctor ? (doctor.ok ? 'ok' : 'warn') : '—'}
          detail={`${doctor?.checks?.length || 0} checks`}
        />
        <StatusCard
          icon={PlugZap}
          label={t('plugins')}
          ok={pluginDoctor?.ok !== false && Boolean(pluginDoctor)}
          value={pluginDoctor ? (pluginDoctor.ok ? 'ok' : 'warn') : '—'}
          detail={`${pluginDoctor?.count || 0} plugins`}
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
    <div className="rounded-[2rem] border border-white/10 bg-white/[0.03] p-5">
      <div className="mb-4 flex items-center justify-between">
        <span className="grid h-11 w-11 place-items-center rounded-2xl border border-white/10 bg-black/20">
          <Icon className={cn('h-5 w-5', ok ? 'text-emerald-300' : 'text-amber-300')} />
        </span>
        <span className={cn('h-2.5 w-2.5 rounded-full', ok ? 'bg-emerald-300' : 'bg-amber-300')} />
      </div>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-2 truncate font-mono text-lg font-semibold">{value}</div>
      <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{detail}</div>
    </div>
  )
}
