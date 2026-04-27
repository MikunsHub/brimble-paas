import { createRoute } from '@tanstack/react-router'
import { pipelinesRoute } from './pipelines'
import { Terminal } from 'lucide-react'
import { DeployForm } from '../components/sidebar/DeployForm'

export const pipelinesIndexRoute = createRoute({
  getParentRoute: () => pipelinesRoute,
  path: '/',
  component: EmptyState,
})

function EmptyState() {
  return (
    <div className="flex-1 flex flex-col h-full">

      <div className="md:hidden flex flex-col flex-1 p-5 gap-6">
        <div>
          <p className="text-sm font-semibold text-zinc-200 mb-1">Deploy a service</p>
          <p className="text-xs text-zinc-500">Paste a GitHub URL or upload a zip to get started.</p>
        </div>
        <DeployForm />
      </div>

      <div className="hidden md:flex flex-1 flex-col items-center justify-center gap-3 text-center p-8 h-full">
        <div className="w-12 h-12 rounded-xl bg-zinc-900 border border-zinc-800 flex items-center justify-center">
          <Terminal className="w-5 h-5 text-zinc-600" />
        </div>
        <div>
          <p className="text-sm font-medium text-zinc-400">No deployment selected</p>
          <p className="text-xs text-zinc-600 mt-1">Deploy a service or select one from the sidebar</p>
        </div>
      </div>
    </div>
  )
}
