import { createRoute, Outlet } from '@tanstack/react-router'
import { rootRoute } from './__root'
import { deploymentQueryOptions } from '../queryOptions/deployments'
import { Header } from '../components/layout/Header'
import { Sidebar } from '../components/sidebar/Sidebar'
import { Suspense, useState } from 'react'
import { cn } from '../utils/cn'

export const pipelinesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/pipelines',

  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(deploymentQueryOptions.list()),

  component: PipelinesLayout,
})

function PipelinesLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)

  return (
    <div className="h-screen flex flex-col bg-[#0a0a0a] text-zinc-100 overflow-hidden">
      <Header onMenuClick={() => setSidebarOpen((v) => !v)} />

      <div className="flex flex-1 overflow-hidden relative">
        {sidebarOpen && (
          <div
            className="absolute inset-0 bg-black/60 z-10 md:hidden"
            onClick={() => setSidebarOpen(false)}
          />
        )}

        <div
          className={cn(
            'absolute inset-y-0 left-0 z-20 transition-transform duration-200 ease-in-out',
            'md:relative md:translate-x-0',
            sidebarOpen ? 'translate-x-0' : '-translate-x-full',
          )}
        >
          <Suspense fallback={<aside className="w-64 h-full border-r border-zinc-800 shrink-0" />}>
            <Sidebar onClose={() => setSidebarOpen(false)} />
          </Suspense>
        </div>

        <main className="flex-1 flex flex-col overflow-hidden">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
