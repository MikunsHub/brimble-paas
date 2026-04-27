import { useSuspenseQuery } from '@tanstack/react-query'
import { deploymentQueryOptions } from '../queryOptions/deployments'

export function useDeployment(id: string) {
  return useSuspenseQuery(deploymentQueryOptions.detail(id))
}
