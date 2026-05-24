import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  AlertTriangle,
  Check,
  Crosshair,
  Fingerprint,
  Lock,
  PlugZap,
  Radar,
  ShieldCheck,
  Sparkles,
  Target,
  TerminalSquare,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { PluginManifest, StartWebScanResponse } from '@/lib/api'
import { rememberJob } from '@/lib/api'
import { text, useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

export function NewScanPage() {
  const navigate = useNavigate()
  const { client, language, t } = useConsole()
  const [target, setTarget] = useState('')
  const [scope, setScope] = useState('')
  const [allowExternalTools, setAllowExternalTools] = useState(false)
  const [enabledPlugins, setEnabledPlugins] = useState<string[]>([])
  const [plugins, setPlugins] = useState<PluginManifest[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [job, setJob] = useState<StartWebScanResponse | null>(null)

  useEffect(() => {
    client
      .plugins()
      .then((response) => setPlugins(response.plugins || []))
      .catch(() => setPlugins([]))
  }, [client])

  const selectablePlugins = useMemo(
    () => plugins.filter((plugin) => plugin.type === 'tool' || plugin.runtime || plugin.id),
    [plugins],
  )

  const togglePlugin = (id: string) => {
    setEnabledPlugins((current) => (current.includes(id) ? current.filter((value) => value !== id) : [...current, id]))
  }

  const start = async (event: React.FormEvent) => {
    event.preventDefault()
    setLoading(true)
    setError('')
    setJob(null)
    try {
      const response = await client.startWebScan({
        target,
        scope,
        profile: 'safe',
        no_external_tools: !allowExternalTools,
        enabled_plugins: allowExternalTools ? enabledPlugins : [],
      })
      rememberJob({ job_id: response.job_id, run_id: response.run_id, created_at: new Date().toISOString() })
      setJob(response)
      navigate(`/scan/${response.run_id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="grid gap-6 xl:grid-cols-[1fr_420px]">
      <form onSubmit={start} className="space-y-6">
        <section className="rounded-[2rem] border border-white/10 bg-white/[0.035] p-5 sm:p-7">
          <div className="mb-8 flex items-start justify-between gap-4">
            <div>
              <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-primary/25 bg-primary/10 px-3 py-1 font-mono text-xs text-primary">
                <Crosshair className="h-3.5 w-3.5" />
                {t('scan')} · {t('safe')}
              </div>
              <h1 className="text-3xl font-semibold tracking-tight sm:text-5xl">{t('launch')}</h1>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">
                {text(language, '默认 safe profile 与 no external tools。scope 必填。', 'Safe profile and no external tools by default. Scope is required.')}
              </p>
            </div>
            <div className="hidden h-20 w-20 place-items-center rounded-[1.5rem] border border-primary/25 bg-primary/10 sm:grid">
              <Radar className="h-9 w-9 text-primary" />
            </div>
          </div>

          <div className="grid gap-5">
            <Field icon={Target} label={t('target')} htmlFor="target">
              <Input
                id="target"
                value={target}
                onChange={(event) => setTarget(event.target.value)}
                placeholder="http://127.0.0.1:18080"
                required
                className="h-12 rounded-2xl font-mono"
              />
            </Field>
            <Field icon={Fingerprint} label={t('scope')} htmlFor="scope">
              <Input
                id="scope"
                value={scope}
                onChange={(event) => setScope(event.target.value)}
                placeholder="127.0.0.1"
                required
                className="h-12 rounded-2xl font-mono"
              />
            </Field>
          </div>
        </section>

        <section className="rounded-[2rem] border border-white/10 bg-white/[0.03] p-5 sm:p-7">
          <div className="mb-5 flex items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <span className="grid h-11 w-11 place-items-center rounded-2xl border border-emerald-400/20 bg-emerald-400/10 text-emerald-300">
                <ShieldCheck className="h-5 w-5" />
              </span>
              <div>
                <h2 className="text-sm font-semibold">{t('profile')}</h2>
                <p className="font-mono text-xs text-muted-foreground">safe / P0</p>
              </div>
            </div>
            <span className="rounded-full border border-emerald-400/25 bg-emerald-400/10 px-3 py-1 text-xs text-emerald-200">
              {t('safe')}
            </span>
          </div>

          <div className="grid gap-3 sm:grid-cols-2">
            <button
              type="button"
              onClick={() => {
                setAllowExternalTools(false)
                setEnabledPlugins([])
              }}
              className={cn(
                'flex min-h-24 items-center gap-3 rounded-3xl border p-4 text-left transition',
                !allowExternalTools
                  ? 'border-primary/35 bg-primary/10 text-foreground'
                  : 'border-white/10 bg-black/20 text-muted-foreground hover:bg-white/[0.04]',
              )}
            >
              <Lock className="h-5 w-5 text-primary" />
              <span>
                <span className="block text-sm font-medium">{text(language, '内置工具', 'Built-ins')}</span>
                <span className="mt-1 block text-xs text-muted-foreground">{t('externalOff')}</span>
              </span>
            </button>
            <button
              type="button"
              onClick={() => setAllowExternalTools(true)}
              className={cn(
                'flex min-h-24 items-center gap-3 rounded-3xl border p-4 text-left transition',
                allowExternalTools
                  ? 'border-amber-400/35 bg-amber-400/10 text-foreground'
                  : 'border-white/10 bg-black/20 text-muted-foreground hover:bg-white/[0.04]',
              )}
            >
              <PlugZap className="h-5 w-5 text-amber-300" />
              <span>
                <span className="block text-sm font-medium">{t('allowTools')}</span>
                <span className="mt-1 block text-xs text-muted-foreground">{t('externalOn')}</span>
              </span>
            </button>
          </div>

          {allowExternalTools && (
            <div className="mt-5 grid gap-2">
              {selectablePlugins.map((plugin) => {
                const selected = enabledPlugins.includes(plugin.id)
                return (
                  <button
                    key={plugin.id}
                    type="button"
                    onClick={() => togglePlugin(plugin.id)}
                    className={cn(
                      'flex items-center justify-between gap-3 rounded-2xl border px-4 py-3 text-left transition',
                      selected ? 'border-primary/35 bg-primary/10' : 'border-white/10 bg-black/20 hover:bg-white/[0.04]',
                    )}
                  >
                    <span className="min-w-0">
                      <span className="block truncate font-mono text-sm">{plugin.id}</span>
                      <span className="mt-1 block truncate text-xs text-muted-foreground">
                        {plugin.risk || 'risk'} · {plugin.runtime || plugin.type || 'plugin'}
                      </span>
                    </span>
                    {selected && <Check className="h-4 w-4 text-primary" />}
                  </button>
                )
              })}
              {!selectablePlugins.length && <p className="rounded-2xl border border-dashed border-white/10 p-4 text-sm text-muted-foreground">{t('noData')}</p>}
            </div>
          )}
        </section>

        {error && (
          <div className="flex items-center gap-3 rounded-2xl border border-red-400/20 bg-red-500/10 px-4 py-3 text-sm text-red-100">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        <Button type="submit" disabled={loading || !target.trim() || !scope.trim()} className="h-14 w-full rounded-2xl text-base">
          <Sparkles className={cn('mr-2 h-5 w-5', loading && 'animate-pulse')} />
          {loading ? text(language, '启动中', 'Starting') : t('start')}
        </Button>
      </form>

      <aside className="space-y-4">
        <div className="rounded-[2rem] border border-white/10 bg-black/25 p-5">
          <div className="mb-4 flex items-center gap-2">
            <TerminalSquare className="h-4 w-4 text-primary" />
            <h2 className="text-sm font-semibold">{t('job')}</h2>
          </div>
          <div className="space-y-3 font-mono text-xs text-muted-foreground">
            <PreviewLine label="POST" value="/api/v1/scans/web" />
            <PreviewLine label={t('target')} value={target || '—'} />
            <PreviewLine label={t('scope')} value={scope || '—'} />
            <PreviewLine label={t('tools')} value={allowExternalTools ? enabledPlugins.join(', ') || '[]' : 'no_external_tools'} />
          </div>
        </div>

        {job && (
          <div className="rounded-[2rem] border border-emerald-400/20 bg-emerald-400/10 p-5 text-emerald-100">
            <div className="mb-3 flex items-center gap-2 text-sm font-semibold">
              <Check className="h-4 w-4" />
              {job.status}
            </div>
            <div className="space-y-2 font-mono text-xs">
              <div>{job.job_id}</div>
              <div>{job.run_id}</div>
            </div>
          </div>
        )}
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

function PreviewLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-2xl border border-white/10 bg-white/[0.03] px-3 py-2">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate text-right text-foreground">{value}</span>
    </div>
  )
}
