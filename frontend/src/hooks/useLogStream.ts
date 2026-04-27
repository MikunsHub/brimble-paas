import { useState, useEffect } from 'react'
import type { LogEntry } from '../types'

export function useLogStream(id: string | undefined, enabled: boolean) {
  const [streamLogs, setStreamLogs] = useState<LogEntry[]>([])
  const [isConnected, setIsConnected] = useState(false)

  useEffect(() => {
    if (!id || !enabled) {
      setIsConnected(false)
      return
    }

    const es = new EventSource(`/api/deployments/${id}/logs/stream`)

    es.onopen = () => setIsConnected(true)

    es.addEventListener('log', (e: MessageEvent) => {
      try {
        const log = JSON.parse(e.data as string) as LogEntry
        setStreamLogs((prev) => [...prev, log])
      } catch {
      }
    })

    es.onerror = () => {
      setIsConnected(false)
      es.close()
    }

    return () => {
      es.close()
      setIsConnected(false)
    }
  }, [id, enabled])

  return { streamLogs, isConnected }
}
