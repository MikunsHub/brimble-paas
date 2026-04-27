import { queryOptions } from '@tanstack/react-query'
import { deploymentApi } from '../api/deployments'

export const deploymentQueryOptions = {
  list: () =>
    queryOptions({
      queryKey: ['deployments'] as const,
      queryFn: deploymentApi.list,
      refetchInterval: 5_000,
      staleTime: 0,
    }),

  detail: (id: string) =>
    queryOptions({
      queryKey: ['deployments', id] as const,
      queryFn: () => deploymentApi.get(id),
      refetchInterval: (query) => {
        const status = query.state.data?.status
        if (!status || status === 'failed' || status === 'stopped') return false
        return status === 'running' ? 5_000 : 2_000
      },
      staleTime: 0,
    }),

  logs: (id: string) =>
    queryOptions({
      queryKey: ['logs', id] as const,
      queryFn: () => deploymentApi.getLogs(id),
      staleTime: 0,
    }),
}
