import { useEffect, useRef, useState } from 'react'
import type { UIEvent } from '@/lib/api'

export function useScanEvents(scanId: string | undefined) {
  const [events, setEvents] = useState<UIEvent[]>([])
  const [connected, setConnected] = useState(false)
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!scanId) return

    const es = new EventSource(`/api/scans/${scanId}/events`)
    esRef.current = es

    es.onopen = () => setConnected(true)
    es.onmessage = (ev) => {
      const event: UIEvent = JSON.parse(ev.data)
      setEvents((prev) => [...prev.slice(-499), event])
    }
    es.onerror = () => setConnected(false)

    return () => {
      es.close()
      esRef.current = null
    }
  }, [scanId])

  return { events, connected }
}
