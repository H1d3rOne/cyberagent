import { useCallback, useEffect, useState } from 'react'
import { api, type ScanListItem } from '@/lib/api'

export function useScans(pollInterval = 3000) {
  const [scans, setScans] = useState<ScanListItem[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    try {
      const data = await api.listScans()
      setScans(data || [])
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, pollInterval)
    return () => clearInterval(id)
  }, [refresh, pollInterval])

  return { scans, loading, refresh }
}
