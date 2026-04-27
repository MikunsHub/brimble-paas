import { Link, useRouterState } from '@tanstack/react-router'
import { Zap, User, Menu } from 'lucide-react'
import { cn } from '../../utils/cn'

interface Props {
  onMenuClick: () => void
}

export function Header({ onMenuClick }: Props) {
  const { location } = useRouterState()
  const isPipelinesActive = location.pathname.startsWith('/pipelines')

  return (
    <header className="h-11 flex items-center px-4 border-b border-zinc-800 bg-[#0a0a0a] shrink-0">
      {/* Hamburger — mobile only */}
      <button
        onClick={onMenuClick}
        className="md:hidden mr-3 p-1 -ml-1 text-zinc-400 hover:text-white transition-colors"
        aria-label="Toggle sidebar"
      >
        <Menu className="w-4 h-4" />
      </button>

      {/* Logo + nav */}
      <div className="flex items-center gap-5">
        <div className="flex items-center gap-2">
          <div className="w-5 h-5 bg-white rounded flex items-center justify-center">
            <Zap className="w-3 h-3 text-black fill-black" />
          </div>
          <span className="text-[11px] font-bold tracking-[0.2em] text-white">DPLY</span>
        </div>

        <nav>
          <Link
            to="/pipelines"
            className={cn(
              'px-3 py-1.5 text-xs rounded transition-colors',
              isPipelinesActive ? 'text-zinc-200 bg-zinc-800' : 'text-zinc-500 hover:text-zinc-200',
            )}
          >
            Pipelines
          </Link>
        </nav>
      </div>

      {/* Avatar */}
      <div className="ml-auto">
        <div className="w-6 h-6 rounded-full bg-zinc-800 border border-zinc-700 flex items-center justify-center">
          <User className="w-3.5 h-3.5 text-zinc-500" />
        </div>
      </div>
    </header>
  )
}
