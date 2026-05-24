import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet } from 'react-router-dom'
import {
  Activity,
  Bot,
  Crosshair,
  History,
  Languages,
  PlugZap,
  Radar,
  Settings,
  ShieldCheck,
  WifiOff,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useConsole } from '@/lib/console'
import { cn } from '@/lib/utils'

const navItems = [
  { to: '/', icon: Radar, labelKey: 'overview' as const },
  { to: '/scan/new', icon: Crosshair, labelKey: 'scan' as const },
  { to: '/history', icon: History, labelKey: 'runs' as const },
  { to: '/settings', icon: Settings, labelKey: 'settings' as const },
]

export function Layout() {
  const { client, settings, updateSettings, language, setLanguage, t } = useConsole()
  const [online, setOnline] = useState(false)

  useEffect(() => {
    let cancelled = false
    const ping = async () => {
      try {
        await client.healthz()
        if (!cancelled) setOnline(true)
      } catch {
        if (!cancelled) setOnline(false)
      }
    }
    ping()
    const id = window.setInterval(ping, 15_000)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [client])

  return (
    <div className="min-h-dvh overflow-hidden bg-background text-foreground">
      <div className="pointer-events-none fixed inset-0 cyber-grid opacity-70" />
      <div className="pointer-events-none fixed -left-32 top-10 h-72 w-72 rounded-full bg-primary/20 blur-3xl" />
      <div className="pointer-events-none fixed -right-36 bottom-0 h-96 w-96 rounded-full bg-cyan-500/10 blur-3xl" />

      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[76px] flex-col border-r border-white/10 bg-black/45 backdrop-blur-xl lg:flex">
        <Link to="/" className="group flex h-20 items-center justify-center" aria-label="CyberAgent">
          <span className="relative grid h-12 w-12 place-items-center rounded-2xl border border-primary/30 bg-primary/10 shadow-[0_0_36px_rgba(37,99,235,0.25)]">
            <ShieldCheck className="h-6 w-6 text-primary transition-transform group-hover:scale-110" />
          </span>
        </Link>

        <nav className="flex flex-1 flex-col items-center gap-2 px-3 py-4">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              title={t(item.labelKey)}
              aria-label={t(item.labelKey)}
              className={({ isActive }) =>
                cn(
                  'group relative grid h-12 w-12 place-items-center rounded-2xl text-muted-foreground transition-all duration-200 hover:bg-white/[0.08] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  isActive && 'bg-primary text-primary-foreground shadow-[0_0_28px_rgba(37,99,235,0.35)]',
                )
              }
            >
              <item.icon className="h-5 w-5" />
            </NavLink>
          ))}
        </nav>

        <div className="flex flex-col items-center gap-3 border-t border-white/10 p-3">
          <span
            title={online ? t('connected') : t('offline')}
            className={cn(
              'grid h-10 w-10 place-items-center rounded-2xl border',
              online
                ? 'border-emerald-400/30 bg-emerald-400/10 text-emerald-300'
                : 'border-red-400/30 bg-red-400/10 text-red-300',
            )}
          >
            {online ? <Activity className="h-4 w-4" /> : <WifiOff className="h-4 w-4" />}
          </span>
        </div>
      </aside>

      <div className="relative z-10 flex min-h-dvh flex-col lg:pl-[76px]">
        <header className="sticky top-0 z-20 border-b border-white/10 bg-background/78 backdrop-blur-xl">
          <div className="flex min-h-16 items-center gap-3 px-4 sm:px-6">
            <Link to="/" className="flex items-center gap-3 lg:hidden" aria-label="CyberAgent">
              <span className="grid h-10 w-10 place-items-center rounded-2xl border border-primary/30 bg-primary/10">
                <ShieldCheck className="h-5 w-5 text-primary" />
              </span>
              <span className="font-semibold tracking-tight">CyberAgent</span>
            </Link>

            <div className="hidden items-center gap-3 lg:flex">
              <Bot className="h-4 w-4 text-primary" />
              <div className="font-mono text-xs uppercase tracking-[0.35em] text-muted-foreground">CyberAgent</div>
            </div>

            <div className="ml-auto flex items-center gap-2">
              <label className="sr-only" htmlFor="daemon-base-url">
                {t('baseUrl')}
              </label>
              <div className="hidden items-center gap-2 rounded-2xl border border-white/10 bg-white/[0.03] px-2 py-1 md:flex">
                <PlugZap className="h-4 w-4 text-muted-foreground" />
                <Input
                  id="daemon-base-url"
                  value={settings.baseUrl}
                  onChange={(event) => updateSettings({ baseUrl: event.target.value })}
                  placeholder="http://127.0.0.1:8787"
                  className="h-8 w-48 border-0 bg-transparent px-1 font-mono text-xs ring-offset-0 focus-visible:ring-0 xl:w-72"
                />
              </div>

              <span
                className={cn(
                  'inline-flex h-10 items-center gap-2 rounded-2xl border px-3 font-mono text-xs',
                  online
                    ? 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300'
                    : 'border-red-400/25 bg-red-400/10 text-red-300',
                )}
              >
                <span className={cn('h-2 w-2 rounded-full', online ? 'bg-emerald-300' : 'bg-red-300')} />
                {online ? t('connected') : t('offline')}
              </span>

              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-10 w-10 rounded-2xl"
                title={language === 'zh' ? 'English' : '中文'}
                aria-label={language === 'zh' ? 'Switch to English' : '切换到中文'}
                onClick={() => setLanguage(language === 'zh' ? 'en' : 'zh')}
              >
                <Languages className="h-4 w-4" />
              </Button>

              <Button asChild className="hidden rounded-2xl px-4 sm:inline-flex">
                <Link to="/scan/new">
                  <Crosshair className="mr-2 h-4 w-4" />
                  {t('start')}
                </Link>
              </Button>
            </div>
          </div>
        </header>

        <main className="mx-auto w-full max-w-[1600px] flex-1 px-4 pb-24 pt-5 sm:px-6 lg:pb-8">
          <Outlet />
        </main>

        <nav className="fixed inset-x-0 bottom-0 z-30 flex border-t border-white/10 bg-background/90 backdrop-blur-xl lg:hidden">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex flex-1 flex-col items-center gap-1 px-2 py-3 text-[11px] text-muted-foreground',
                  isActive && 'text-primary',
                )
              }
            >
              <item.icon className="h-5 w-5" />
              <span>{t(item.labelKey)}</span>
            </NavLink>
          ))}
        </nav>
      </div>
    </div>
  )
}
