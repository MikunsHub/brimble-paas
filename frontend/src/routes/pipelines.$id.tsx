import { createRoute } from '@tanstack/react-router'
import { useSuspenseQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { pipelinesRoute } from './pipelines'
import { deploymentQueryOptions } from '../queryOptions/deployments'
import { deploymentApi } from '../api/deployments'
import { useLogStream } from '../hooks/useLogStream'
import { useLogs } from '../hooks/useLogs'
import { StatusBadge } from '../components/ui/StatusBadge'
import { LogLine } from '../components/deployment/LogLine'
import { ExternalLink, Square, RotateCcw, Loader2 } from 'lucide-react'
import { useEffect, useRef } from 'react'
import type { DeploymentStatus } from '../types'

const STOPPABLE: DeploymentStatus[] = ['pending', 'building', 'deploying', 'running']
const RESTARTABLE: DeploymentStatus[] = ['stopped', 'failed']

export const deploymentRoute = createRoute({
  getParentRoute: () => pipelinesRoute,
  path: '$id',

  loader: async ({ params, context: { queryClient } }) => {
    await Promise.all([
      queryClient.ensureQueryData(deploymentQueryOptions.detail(params.id)),
      queryClient.ensureQueryData(deploymentQueryOptions.logs(params.id)),
    ])
  },

  component: DeploymentPage,
})

function DeploymentPage() {
  const { id } = deploymentRoute.useParams()
  return <DeploymentView key={id} />
}


function getDisplayName(subdomain: string) {
  const parts = subdomain.split('-')
  return parts.length > 1 ? parts.slice(0, -1).join('-') : subdomain
}

function getShortHash(imageTag: string | undefined) {
  if (!imageTag) return null
  const parts = imageTag.split('-')
  const tail = parts[parts.length - 1] ?? imageTag
  return tail.slice(0, 8)
}


function DeploymentView() {
  const { id } = deploymentRoute.useParams()
  const qc = useQueryClient()

  const { data: deployment } = useSuspenseQuery(deploymentQueryOptions.detail(id))

  const stopMutation = useMutation({
    mutationFn: () => deploymentApi.delete(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['deployments'] })
      void qc.invalidateQueries({ queryKey: ['deployments', id] })
      void qc.invalidateQueries({ queryKey: ['logs', id] })
    },
  })

  const restartMutation = useMutation({
    mutationFn: () => deploymentApi.restart(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['deployments'] })
      void qc.invalidateQueries({ queryKey: ['deployments', id] })
      void qc.invalidateQueries({ queryKey: ['logs', id] })
    },
  })

  const { data: polledLogs = [] } = useLogs(id)

  const isActive = ['pending', 'building', 'deploying', 'running'].includes(deployment.status)
  const { streamLogs } = useLogStream(id, isActive)

  const polledIds = new Set(polledLogs.map((l) => l.id))
  const allLogs = [...polledLogs, ...streamLogs.filter((l) => !polledIds.has(l.id))]

  const lastRestartIdx = allLogs.reduce((idx, log, i) => (log.phase === 'restart' ? i : idx), -1)

  const containerRef = useRef<HTMLDivElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const separatorRef = useRef<HTMLDivElement>(null)
  const atBottomRef = useRef(true)

  const onScroll = () => {
    const el = containerRef.current
    if (!el) return
    atBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 60
  }

  useEffect(() => {
    if (atBottomRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [allLogs.length])

  useEffect(() => {
    if (separatorRef.current) {
      separatorRef.current.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [])

  const displayName = getDisplayName(deployment.subdomain)
  const shortHash = getShortHash(deployment.image_tag)

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-3 px-6 py-3 border-b border-zinc-800 shrink-0">
        <h2 className="text-sm font-semibold text-white">{displayName}</h2>
        <StatusBadge status={deployment.status} />
        {shortHash && (
          <span className="text-[11px] font-mono text-zinc-500 bg-zinc-900 px-2 py-0.5 rounded border border-zinc-800">
            {shortHash}
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          {deployment.live_url && (
            <a
              href={deployment.live_url}
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-1 text-xs text-zinc-400 hover:text-white transition-colors"
            >
              <ExternalLink className="w-3.5 h-3.5" />
              Open
            </a>
          )}
          {STOPPABLE.includes(deployment.status) && (
            <button
              onClick={() => stopMutation.mutate()}
              disabled={stopMutation.isPending}
              className="flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium text-red-400 border border-red-900/60 hover:bg-red-950/50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {stopMutation.isPending ? (
                <Loader2 className="w-3 h-3 animate-spin" />
              ) : (
                <Square className="w-3 h-3 fill-current" />
              )}
              Stop
            </button>
          )}
          {RESTARTABLE.includes(deployment.status) && (
            <button
              onClick={() => restartMutation.mutate()}
              disabled={restartMutation.isPending}
              className="flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium text-zinc-300 border border-zinc-700 hover:bg-zinc-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {restartMutation.isPending ? (
                <Loader2 className="w-3 h-3 animate-spin" />
              ) : (
                <RotateCcw className="w-3 h-3" />
              )}
              Restart
            </button>
          )}
        </div>
      </div>

      <div
        ref={containerRef}
        onScroll={onScroll}
        className="flex-1 overflow-y-auto py-5 px-6 space-y-px font-mono text-xs"
      >
        {allLogs.length === 0 && (
          <p className="text-zinc-600 py-6 text-center">Waiting for logs…</p>
        )}
        {allLogs.map((log, i) => {
          if (log.phase === 'restart') {
            return (
              <div
                key={log.id}
                ref={i === lastRestartIdx ? separatorRef : undefined}
                className="flex items-center gap-3 py-3 my-1"
              >
                <div className="flex-1 h-px bg-zinc-800" />
                <div className="flex items-center gap-1.5 text-[10px] text-zinc-600 select-none">
                  <RotateCcw className="w-3 h-3" />
                  <span>Restarted</span>
                </div>
                <div className="flex-1 h-px bg-zinc-800" />
              </div>
            )
          }
          return (
            <div key={log.id} className={lastRestartIdx !== -1 && i < lastRestartIdx ? 'opacity-50' : undefined}>
              <LogLine log={log} />
            </div>
          )
        })}

        {isActive && (
          <div className="flex items-center gap-1 pt-1 text-zinc-600">
            <span>{'>'}</span>
            <span className="inline-block w-2 h-3.5 bg-zinc-500 animate-[cursor-blink_1s_step-end_infinite]" />
          </div>
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
