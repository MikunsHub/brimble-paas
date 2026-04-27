export type DeploymentStatus =
  | 'pending'
  | 'building'
  | 'deploying'
  | 'running'
  | 'failed'
  | 'stopped'

export interface Deployment {
  id: string
  status: DeploymentStatus
  subdomain: string
  git_url?: string
  s3_key?: string
  live_url?: string
  image_tag?: string
  error_message?: string
  created_at: string
}

export interface LogEntry {
  id: string
  deployment_id: string
  timestamp: string
  stream: string
  phase: string
  content: string
}

export interface UploadUrlResponse {
  file_path: string
  url: string
  method: string
}
