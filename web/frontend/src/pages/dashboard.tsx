import { Link } from 'react-router-dom'
import { Activity, AlertTriangle, CheckCircle, Plus, Target } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useScans } from '@/hooks/use-scans'
import type { ScanListItem } from '@/lib/api'

export function DashboardPage() {
  const { scans, loading } = useScans()

  const running = scans.filter((s) => s.status === 'running')
  const completed = scans.filter((s) => s.status === 'completed')
  const totalFindings = scans.reduce((sum, s) => sum + s.findings, 0)

  if (loading) {
    return <div className="text-muted-foreground">Loading...</div>
  }

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <h2 className="text-3xl font-bold tracking-tight">Dashboard</h2>
        <Link to="/scan/new">
          <Button>
            <Plus className="mr-2 h-4 w-4" /> New Scan
          </Button>
        </Link>
      </div>

      <div className="grid gap-4 md:grid-cols-4">
        <StatCard icon={Activity} label="Active Scans" value={running.length} color="text-green-500" />
        <StatCard icon={CheckCircle} label="Completed" value={completed.length} color="text-blue-500" />
        <StatCard icon={AlertTriangle} label="Total Findings" value={totalFindings} color="text-yellow-500" />
        <StatCard icon={Target} label="Total Scans" value={scans.length} color="text-purple-500" />
      </div>

      {running.length > 0 && (
        <section>
          <h3 className="text-lg font-semibold mb-4">Active Scans</h3>
          <div className="grid gap-4 md:grid-cols-2">
            {running.map((scan) => (
              <ScanCard key={scan.id} scan={scan} />
            ))}
          </div>
        </section>
      )}

      {completed.length > 0 && (
        <section>
          <h3 className="text-lg font-semibold mb-4">Recent Scans</h3>
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {completed.slice(0, 6).map((scan) => (
              <ScanCard key={scan.id} scan={scan} />
            ))}
          </div>
        </section>
      )}

      {scans.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <Target className="h-12 w-12 text-muted-foreground mb-4" />
            <p className="text-muted-foreground mb-4">No scans yet. Start your first security assessment.</p>
            <Link to="/scan/new">
              <Button>
                <Plus className="mr-2 h-4 w-4" /> Start Scan
              </Button>
            </Link>
          </CardContent>
        </Card>
      )}
    </div>
  )
}

function StatCard({ icon: Icon, label, value, color }: { icon: React.ElementType; label: string; value: number; color: string }) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-sm font-medium">{label}</CardTitle>
        <Icon className={`h-4 w-4 ${color}`} />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{value}</div>
      </CardContent>
    </Card>
  )
}

function ScanCard({ scan }: { scan: ScanListItem }) {
  const statusColor = {
    running: 'bg-green-500',
    completed: 'bg-blue-500',
    failed: 'bg-red-500',
    stopped: 'bg-yellow-500',
  }[scan.status]

  return (
    <Link to={`/scan/${scan.id}`}>
      <Card className="hover:border-primary/50 transition-colors cursor-pointer">
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm font-medium truncate">{scan.target}</CardTitle>
            <div className="flex items-center gap-2">
              <div className={`h-2 w-2 rounded-full ${statusColor}`} />
              <span className="text-xs text-muted-foreground capitalize">{scan.status}</span>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between text-sm text-muted-foreground">
            <span>Turn {scan.turns}/{scan.max_turns}</span>
            <span>{scan.duration}</span>
            {scan.findings > 0 && (
              <Badge variant="destructive">{scan.findings} findings</Badge>
            )}
          </div>
          {scan.status === 'running' && (
            <div className="mt-3 h-1.5 bg-secondary rounded-full overflow-hidden">
              <div
                className="h-full bg-primary rounded-full transition-all"
                style={{ width: `${(scan.turns / scan.max_turns) * 100}%` }}
              />
            </div>
          )}
        </CardContent>
      </Card>
    </Link>
  )
}
