import { Link } from 'react-router-dom'
import { Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useScans } from '@/hooks/use-scans'
import { api } from '@/lib/api'

export function HistoryPage() {
  const { scans, loading, refresh } = useScans()

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this scan?')) return
    await api.deleteScan(id)
    refresh()
  }

  if (loading) {
    return <div className="text-muted-foreground">Loading...</div>
  }

  return (
    <div className="space-y-6">
      <h2 className="text-3xl font-bold tracking-tight">Scan History</h2>

      {scans.length === 0 ? (
        <p className="text-muted-foreground">No scans recorded yet.</p>
      ) : (
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="text-left p-3 font-medium">Target</th>
                <th className="text-left p-3 font-medium">Status</th>
                <th className="text-left p-3 font-medium">Provider</th>
                <th className="text-left p-3 font-medium">Turns</th>
                <th className="text-left p-3 font-medium">Findings</th>
                <th className="text-left p-3 font-medium">Duration</th>
                <th className="text-left p-3 font-medium">Started</th>
                <th className="p-3"></th>
              </tr>
            </thead>
            <tbody>
              {scans.map((scan) => (
                <tr key={scan.id} className="border-t hover:bg-muted/30 transition-colors">
                  <td className="p-3">
                    <Link to={`/scan/${scan.id}`} className="text-primary hover:underline font-medium">
                      {scan.target}
                    </Link>
                  </td>
                  <td className="p-3">
                    <StatusBadge status={scan.status} />
                  </td>
                  <td className="p-3 text-muted-foreground">{scan.provider}</td>
                  <td className="p-3 text-muted-foreground">{scan.turns}/{scan.max_turns}</td>
                  <td className="p-3">
                    {scan.findings > 0 ? (
                      <Badge variant="destructive">{scan.findings}</Badge>
                    ) : (
                      <span className="text-muted-foreground">0</span>
                    )}
                  </td>
                  <td className="p-3 text-muted-foreground">{scan.duration}</td>
                  <td className="p-3 text-muted-foreground">
                    {new Date(scan.started_at).toLocaleString()}
                  </td>
                  <td className="p-3">
                    <Button variant="ghost" size="icon" onClick={() => handleDelete(scan.id)}>
                      <Trash2 className="h-4 w-4 text-muted-foreground" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  const variant = {
    running: 'default' as const,
    completed: 'secondary' as const,
    failed: 'destructive' as const,
    stopped: 'outline' as const,
  }[status] || 'outline' as const

  return <Badge variant={variant}>{status}</Badge>
}
