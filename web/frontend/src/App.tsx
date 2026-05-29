import { Routes, Route } from 'react-router-dom'
import { Layout } from './components/layout'
import { AgentCockpitPage } from './pages/agent-cockpit'
import { DashboardPage } from './pages/dashboard'
import { NewScanPage } from './pages/new-scan'
import { ScanDetailPage } from './pages/scan-detail'
import { HistoryPage } from './pages/history'
import { SettingsPage } from './pages/settings'

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/agent" element={<AgentCockpitPage />} />
        <Route path="/ops" element={<DashboardPage />} />
        <Route path="/tasks" element={<NewScanPage />} />
        <Route path="/scan/new" element={<NewScanPage />} />
        <Route path="/scan/:id" element={<ScanDetailPage />} />
        <Route path="/history" element={<HistoryPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  )
}
