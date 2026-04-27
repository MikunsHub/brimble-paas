import { api } from './client'
import type { Deployment, LogEntry, UploadUrlResponse } from '../types'

export const deploymentApi = {
  list: () =>
    api.get<Deployment[]>('/deployments'),

  get: (id: string) =>
    api.get<Deployment>(`/deployments/${id}`),

  create: (body: { git_url?: string; file_path?: string }) =>
    api.post<Deployment>('/deployments', body),

  restart: (id: string) =>
    api.post<Deployment>(`/deployments/${id}/restart`, {}),

  createUploadUrl: (body: { file_name: string; content_type: string }) =>
    api.post<UploadUrlResponse>('/deployments/upload-url', body),

  delete: (id: string) =>
    api.delete(`/deployments/${id}`),

  getLogs: (id: string, offset = 0) =>
    api.get<LogEntry[]>(`/deployments/${id}/logs?offset=${offset}`),
}
