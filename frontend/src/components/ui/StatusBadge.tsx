import type { DeploymentStatus } from '../../types'
import { cn } from '../../utils/cn'

interface StatusConfig {
  label: string
  shortLabel: string
  textColor: string
  dotColor: string
  bgColor: string
  pulse?: boolean
}

const CONFIG: Record<DeploymentStatus, StatusConfig> = {
  pending: {
    label: 'Pending',
    shortLabel: 'QUEUE',
    textColor: 'text-zinc-400',
    dotColor: 'bg-zinc-500',
    bgColor: 'bg-zinc-800/60',
  },
  building: {
    label: 'Building Image',
    shortLabel: 'BUILD',
    textColor: 'text-blue-400',
    dotColor: 'bg-blue-400',
    bgColor: 'bg-blue-950/40',
    pulse: true,
  },
  deploying: {
    label: 'Deploying',
    shortLabel: 'DEPLOY',
    textColor: 'text-yellow-400',
    dotColor: 'bg-yellow-400',
    bgColor: 'bg-yellow-950/40',
    pulse: true,
  },
  restarting: {
    label: 'Restarting',
    shortLabel: 'RESTART',
    textColor: 'text-amber-300',
    dotColor: 'bg-amber-300',
    bgColor: 'bg-amber-950/40',
    pulse: true,
  },
  running: {
    label: 'Live',
    shortLabel: 'READY',
    textColor: 'text-green-400',
    dotColor: 'bg-green-400',
    bgColor: 'bg-green-950/40',
  },
  failed: {
    label: 'Error',
    shortLabel: 'ERROR',
    textColor: 'text-red-400',
    dotColor: 'bg-red-500',
    bgColor: 'bg-red-950/40',
  },
  stopping: {
    label: 'Stopping',
    shortLabel: 'STOPPING',
    textColor: 'text-orange-300',
    dotColor: 'bg-orange-300',
    bgColor: 'bg-orange-950/40',
    pulse: true,
  },
  stopped: {
    label: 'Stopped',
    shortLabel: 'STOP',
    textColor: 'text-zinc-500',
    dotColor: 'bg-zinc-600',
    bgColor: 'bg-zinc-900',
  },
}

interface Props {
  status: DeploymentStatus
  compact?: boolean
}

export function StatusBadge({ status, compact = false }: Props) {
  const cfg = CONFIG[status] ?? CONFIG.stopped

  if (compact) {
    if (cfg.pulse || status === 'pending') {
      return (
        <span
          className={cn(
            'w-1.5 h-1.5 rounded-full inline-block',
            cfg.dotColor,
            cfg.pulse && 'animate-pulse',
          )}
        />
      )
    }
    return (
      <span className={cn('text-[10px] font-semibold tracking-wide', cfg.textColor)}>
        {cfg.shortLabel}
      </span>
    )
  }

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[11px] font-medium',
        cfg.textColor,
        cfg.bgColor,
      )}
    >
      <span
        className={cn(
          'w-1.5 h-1.5 rounded-full inline-block',
          cfg.dotColor,
          cfg.pulse && 'animate-pulse',
        )}
      />
      {cfg.label}
    </span>
  )
}
