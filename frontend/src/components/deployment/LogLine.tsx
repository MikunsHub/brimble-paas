import type { LogEntry } from '../../types'

// Strip ANSI escape codes the Go build tools emit
function stripAnsi(s: string) {
  // eslint-disable-next-line no-control-regex
  return s.replace(/\x1b\[[0-9;]*[mGKHFJABCDsu]/g, '')
}

function formatTime(iso: string) {
  const d = new Date(iso)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  const ss = String(d.getSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss}`
}

function contentClass(content: string, _stream: string): string {
  const lower = content.toLowerCase()
  // Green — explicit success signals (including teardown completion)
  if (/successfully|✓|checked out commit|done\.|complete\.|built image|route removed|deployment stopped/.test(lower))
    return 'text-green-400'
  // Bright white — Dockerfile step headers
  if (/^step \d+\/\d+/.test(lower)) return 'text-zinc-300'
  // Red — only when content actually signals a problem
  if (/\berror\b|\bfailed\b|\bfailure\b/.test(lower)) return 'text-red-400'
  // Yellow — in-progress / informational (git clone, remote:, teardown progress, etc.)
  if (/cloning into|remote:|resolving|receiving|unpacking|enumerating|counting|compressing|writing objects|total \d|warn|stopping deployment|sending sigterm/.test(lower))
    return 'text-yellow-400'
  return 'text-zinc-500'
}

interface Props {
  log: LogEntry
}

export function LogLine({ log }: Props) {
  const content = stripAnsi(log.content)
  return (
    <div className="flex gap-4 leading-5 hover:bg-white/[0.02] rounded px-1 -mx-1">
      <span className="text-zinc-700 shrink-0 select-none w-16 text-right tabular-nums">
        {formatTime(log.timestamp)}
      </span>
      <span className={contentClass(content, log.stream)}>{content}</span>
    </div>
  )
}
