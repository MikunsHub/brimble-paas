import { useSuspenseQuery } from '@tanstack/react-query'
import { deploymentQueryOptions } from '../queryOptions/deployments'

export function useDeployments() {
  return useSuspenseQuery(deploymentQueryOptions.list())
}
