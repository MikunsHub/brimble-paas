import { DeployForm } from './DeployForm'
import { ActiveJobs } from './ActiveJobs'

interface Props {
  onClose: () => void
}

export function Sidebar({ onClose }: Props) {
  return (
    <aside className="w-64 h-full border-r border-zinc-800 flex flex-col shrink-0 overflow-hidden bg-[#0d0d0d]">
      <div className="p-4 border-b border-zinc-800">
        <h2 className="text-[11px] font-semibold text-zinc-500 mb-3">
          Deploy Service
        </h2>
        <DeployForm />
      </div>

      <div className="flex-1 overflow-y-auto">
        <p className="px-4 pt-4 pb-2 text-[10px] font-semibold text-zinc-700 uppercase tracking-widest">
          Active Jobs
        </p>
        <ActiveJobs onSelect={onClose} />
      </div>
    </aside>
  )
}
