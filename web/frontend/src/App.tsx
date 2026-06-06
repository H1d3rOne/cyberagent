import { lazy, Suspense, type ReactNode } from 'react'
import { Routes, Route } from 'react-router-dom'
import { Layout } from './components/layout'

const AgentCockpitPage = lazy(() =>
  import('./pages/agent-cockpit').then((module) => ({ default: module.AgentCockpitPage })),
)
const DashboardPage = lazy(() => import('./pages/dashboard').then((module) => ({ default: module.DashboardPage })))
const SkillsPage = lazy(() => import('./pages/skills').then((module) => ({ default: module.SkillsPage })))
const McpPage = lazy(() => import('./pages/mcp').then((module) => ({ default: module.McpPage })))
const NewScanPage = lazy(() => import('./pages/new-scan').then((module) => ({ default: module.NewScanPage })))
const ScanDetailPage = lazy(() =>
  import('./pages/scan-detail').then((module) => ({ default: module.ScanDetailPage })),
)
const HistoryPage = lazy(() => import('./pages/history').then((module) => ({ default: module.HistoryPage })))
const SettingsPage = lazy(() => import('./pages/settings').then((module) => ({ default: module.SettingsPage })))

function RouteFallback() {
  return (
    <section
      aria-busy="true"
      aria-live="polite"
      role="status"
      className="glass-surface glass-surface-panel grid min-h-[320px] place-items-center rounded-3xl border border-white/10 p-8 text-center"
    >
      <div className="space-y-4">
        <div className="mx-auto h-12 w-12 animate-pulse rounded-2xl border border-primary/30 bg-primary/10 shadow-[0_0_38px_rgba(59,130,246,0.22)]" />
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.32em] text-primary/80">loading route</p>
          <p className="mt-2 text-sm text-muted-foreground">Preparing the CyberAgent workspace…</p>
        </div>
      </div>
    </section>
  )
}

function routeElement(node: ReactNode) {
  return <Suspense fallback={<RouteFallback />}>{node}</Suspense>
}

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={routeElement(<DashboardPage />)} />
        <Route path="/agent" element={routeElement(<AgentCockpitPage />)} />
        <Route path="/skills" element={routeElement(<SkillsPage />)} />
        <Route path="/mcp" element={routeElement(<McpPage />)} />
        <Route path="/ops" element={routeElement(<DashboardPage />)} />
        <Route path="/tasks" element={routeElement(<NewScanPage />)} />
        <Route path="/scan/new" element={routeElement(<NewScanPage />)} />
        <Route path="/scan/:id" element={routeElement(<ScanDetailPage />)} />
        <Route path="/history" element={routeElement(<HistoryPage />)} />
        <Route path="/settings" element={routeElement(<SettingsPage />)} />
      </Route>
    </Routes>
  )
}
