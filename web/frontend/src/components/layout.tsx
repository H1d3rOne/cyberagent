import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet } from 'react-router-dom'
import {
  Activity,
  Bot,
  BrainCircuit,
  Crosshair,
  History,
  KeyRound,
  Languages,
  Network,
  Radar,
  Settings,
  ShieldCheck,
  WifiOff,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useConsole } from '@/lib/console'
import type { DaemonConnectionStatus } from '@/lib/api'
import { cn } from '@/lib/utils'

const navItems = [
  { to: '/', icon: Radar, labelKey: 'overview' as const },
  { to: '/agent', icon: Bot, labelKey: 'agent' as const },
  { to: '/skills', icon: BrainCircuit, labelKey: 'skills' as const },
  { to: '/mcp', icon: Network, labelKey: 'mcp' as const },
  { to: '/tasks', icon: Crosshair, labelKey: 'tasks' as const },
  { to: '/history', icon: History, labelKey: 'history' as const },
  { to: '/settings', icon: Settings, labelKey: 'settings' as const },
]

export function Layout() {
  const { client, language, setLanguage, t } = useConsole()
  const [connection, setConnection] = useState<DaemonConnectionStatus>('offline')
  const online = connection === 'online'
  const unauthorized = connection === 'unauthorized'
  const connectionLabel = online
    ? t('connected')
    : unauthorized
      ? language === 'zh'
        ? '需要令牌'
        : 'Token required'
      : t('offline')

  useEffect(() => {
    let cancelled = false
    const ping = async () => {
      const status = await client.pingStatus()
      if (!cancelled) setConnection(status)
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

      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[248px] flex-col border-r border-white/10 bg-[#050816]/88 shadow-2xl shadow-black/30 backdrop-blur-2xl lg:flex">
        <Link to="/" className="group flex h-20 items-center gap-3 px-5" aria-label="CyberAgent">
          <span className="relative grid h-12 w-12 place-items-center rounded-2xl border border-primary/35 bg-primary/10 shadow-[0_0_40px_rgba(59,130,246,0.28)]">
            <ShieldCheck className="h-6 w-6 text-primary transition-transform group-hover:scale-110" />
            <span className="absolute -right-1 -top-1 h-3 w-3 rounded-full border border-[#050816] bg-emerald-300 shadow-[0_0_18px_rgba(110,231,183,0.8)]" />
          </span>
          <span className="min-w-0">
            <span className="block text-sm font-semibold tracking-tight">CyberAgent</span>
            <span className="block font-mono text-[10px] uppercase tracking-[0.28em] text-muted-foreground">Agent Cockpit</span>
          </span>
        </Link>

        <nav className="flex flex-1 flex-col gap-2 px-3 py-4">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              title={t(item.labelKey)}
              aria-label={t(item.labelKey)}
              className={({ isActive }) =>
                cn(
                  'group flex h-12 items-center gap-3 rounded-2xl px-3 text-sm text-muted-foreground transition-all duration-200 hover:bg-white/[0.07] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  isActive && 'bg-white/[0.09] text-foreground shadow-[inset_0_0_0_1px_rgba(96,165,250,0.22),0_16px_40px_rgba(0,0,0,0.24)]',
                )
              }
            >
              {({ isActive }) => (
                <>
                  <span
                    className={cn(
                      'grid h-9 w-9 shrink-0 place-items-center rounded-xl border border-white/10 bg-white/[0.04] transition',
                      isActive && 'border-primary/35 bg-primary/15 text-primary',
                    )}
                  >
                    <item.icon className="h-4 w-4" />
                  </span>
                  <span className="truncate">{t(item.labelKey)}</span>
                </>
              )}
            </NavLink>
          ))}
        </nav>

        <div className="border-t border-white/10 p-3">
          <span
            title={connectionLabel}
            className={cn(
              'flex h-12 items-center gap-3 rounded-2xl border px-3 font-mono text-xs',
              online
                ? 'border-emerald-400/30 bg-emerald-400/10 text-emerald-300'
                : unauthorized
                  ? 'border-amber-400/30 bg-amber-400/10 text-amber-300'
                  : 'border-red-400/30 bg-red-400/10 text-red-300',
            )}
          >
            {online ? <Activity className="h-4 w-4" /> : unauthorized ? <KeyRound className="h-4 w-4" /> : <WifiOff className="h-4 w-4" />}
            <span>{connectionLabel}</span>
          </span>
        </div>
      </aside>

      <div className="relative z-10 flex min-h-dvh flex-col lg:pl-[248px]">
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
              <div>
                <div className="font-mono text-[10px] uppercase tracking-[0.35em] text-muted-foreground">authorized web/api security agent</div>
                <div className="text-sm font-medium text-foreground/90">{language === 'zh' ? '通过自然语言驱动安全测试、运维与分析' : 'Drive testing, ops and analysis with natural language'}</div>
              </div>
            </div>

            <div className="ml-auto flex items-center gap-2">
              <span
                className={cn(
                  'inline-flex h-10 items-center gap-2 rounded-2xl border px-3 font-mono text-xs',
                  online
                    ? 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300'
                    : unauthorized
                      ? 'border-amber-400/25 bg-amber-400/10 text-amber-300'
                      : 'border-red-400/25 bg-red-400/10 text-red-300',
                )}
              >
                <span className={cn('h-2 w-2 rounded-full', online ? 'bg-emerald-300' : unauthorized ? 'bg-amber-300' : 'bg-red-300')} />
                {connectionLabel}
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
                <Link to="/agent">
                  <Bot className="mr-2 h-4 w-4" />
                  {t('agent')}
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
