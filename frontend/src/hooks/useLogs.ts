import { useSuspenseQuery } from '@tanstack/react-query';
import { deploymentQueryOptions } from '../queryOptions/deployments';

export function useLogs(id: string) {
  return useSuspenseQuery({
    ...deploymentQueryOptions.logs(id),
    refetchInterval: false,
    refetchOnWindowFocus: false,
  });
}
