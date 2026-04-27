import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Github, Upload, ArrowRight, Loader2 } from 'lucide-react'
import { deploymentApi } from '../../api/deployments'
import { cn } from '../../utils/cn'

type Tab = 'git' | 'upload'

export function DeployForm() {
  const [tab, setTab] = useState<Tab>('git')
  const [gitUrl, setGitUrl] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const navigate = useNavigate()
  const qc = useQueryClient()

  const gitMutation = useMutation({
    mutationFn: (url: string) => deploymentApi.create({ git_url: url }),
    onSuccess: (d) => {
      void qc.invalidateQueries({ queryKey: ['deployments'] })
      void navigate({ to: '/pipelines/$id', params: { id: d.id } })
    },
  })

  const uploadMutation = useMutation({
    mutationFn: async (f: File) => {
      const { file_path, url, method } = await deploymentApi.createUploadUrl({
        file_name: f.name,
        content_type: f.type || 'application/zip',
      })

      const uploadUrl = url
        .replace(/^https?:\/\/localstack:\d+/, '/s3-upload')
        .replace(/^http:\/\/localhost:4566/, '/s3-upload')

      const res = await fetch(uploadUrl, {
        method,
        body: f,
        headers: { 'Content-Type': f.type || 'application/zip' },
      })
      if (!res.ok) {
        throw new Error(`S3 upload failed: ${res.status} ${res.statusText}`)
      }

      return deploymentApi.create({ file_path })
    },
    onSuccess: (d) => {
      void qc.invalidateQueries({ queryKey: ['deployments'] })
      void navigate({ to: '/pipelines/$id', params: { id: d.id } })
    },
  })

  const isPending = gitMutation.isPending || uploadMutation.isPending
  const error = gitMutation.error ?? uploadMutation.error

  const canDeploy = tab === 'git' ? gitUrl.trim().length > 0 : file !== null

  function handleDeploy() {
    if (tab === 'git' && gitUrl.trim()) {
      gitMutation.mutate(gitUrl.trim())
    } else if (tab === 'upload' && file) {
      uploadMutation.mutate(file)
    }
  }

  return (
    <div className="space-y-2.5">
      <div className="flex rounded-md bg-zinc-900 p-[3px] border border-zinc-800">
        {(['git', 'upload'] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={cn(
              'flex-1 py-[5px] text-[11px] rounded transition-colors',
              tab === t
                ? 'bg-zinc-700 text-white'
                : 'text-zinc-500 hover:text-zinc-300',
            )}
          >
            {t === 'git' ? 'Git URL' : 'CLI/Upload'}
          </button>
        ))}
      </div>

      {tab === 'git' ? (
        <div className="flex items-center gap-2 bg-zinc-900 border border-zinc-800 rounded px-2.5 py-[7px]">
          <Github className="w-3.5 h-3.5 text-zinc-600 shrink-0" />
          <input
            type="text"
            value={gitUrl}
            onChange={(e) => setGitUrl(e.target.value)}
            placeholder="github.com/user/repo"
            className="flex-1 bg-transparent text-xs text-zinc-300 placeholder:text-zinc-700 outline-none"
            onKeyDown={(e) => e.key === 'Enter' && handleDeploy()}
          />
        </div>
      ) : (
        <label className="flex items-center gap-2 bg-zinc-900 border border-dashed border-zinc-700 rounded px-2.5 py-3 cursor-pointer hover:border-zinc-600 transition-colors">
          <Upload className="w-3.5 h-3.5 text-zinc-600 shrink-0" />
          <span className="text-xs text-zinc-600 truncate">
            {file ? file.name : 'Click to upload .zip'}
          </span>
          <input
            type="file"
            accept=".zip"
            className="hidden"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
          />
        </label>
      )}

      <button
        onClick={handleDeploy}
        disabled={isPending || !canDeploy}
        className="w-full flex items-center justify-center gap-1.5 py-[7px] bg-white text-black text-xs font-semibold rounded hover:bg-zinc-100 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
      >
        {isPending ? (
          <Loader2 className="w-3.5 h-3.5 animate-spin" />
        ) : (
          <>
            Deploy
            <ArrowRight className="w-3.5 h-3.5" />
          </>
        )}
      </button>

      {error && (
        <p className="text-[11px] text-red-400 leading-snug">
          {error.message ?? 'Deploy failed. Please try again.'}
        </p>
      )}
    </div>
  )
}
