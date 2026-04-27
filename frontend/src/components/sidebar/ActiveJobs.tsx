import { Link, useRouterState } from '@tanstack/react-router'
import { useDeployments } from '../../hooks/useDeployments'
import { StatusBadge } from '../ui/StatusBadge'
import { cn } from '../../utils/cn'
import type { Deployment } from '../../types'

function displayName(d: Deployment) {
  // Subdomain format: word-word-hash → strip trailing hash segment
  const parts = d.subdomain.split('-')
  return parts.length > 1 ? parts.slice(0, -1).join('-') : d.subdomain
}

interface Props {
  onSelect: () => void
}

export function ActiveJobs({ onSelect }: Props) {
  const { data: deployments } = useDeployments()
  const { location } = useRouterState()

  // Extract current deployment id from pathname
  const currentId = location.pathname.match(/^\/pipelines\/([^/]+)/)?.[1]

  if (deployments.length === 0) {
    return (
      <p className="text-[11px] text-zinc-700 px-4 py-6 text-center">
        No deployments yet
      </p>
    )
  }

  return (
    <div className="space-y-px">
      {deployments.map((d) => {
        const active = d.id === currentId
        return (
          // skill: prefer <Link> over useNavigate for list items
          <Link
            key={d.id}
            to="/pipelines/$id"
            params={{ id: d.id }}
            preload="intent"
            onClick={onSelect}
            className={cn(
              'flex items-center justify-between px-3 py-2 transition-colors border-l-2',
              active
                ? 'bg-zinc-800/70 border-blue-500'
                : 'border-transparent hover:bg-zinc-800/40 hover:border-zinc-700',
            )}
          >
            <div className="min-w-0 flex-1">
              <p className="text-[12px] text-zinc-200 font-medium truncate leading-tight">
                {displayName(d)}
              </p>
              <p className="text-[10px] text-zinc-600 truncate mt-0.5">
                {d.subdomain}
              </p>
            </div>
            <div className="shrink-0 ml-2">
              <StatusBadge status={d.status} compact />
            </div>
          </Link>
        )
      })}
    </div>
  )
}
