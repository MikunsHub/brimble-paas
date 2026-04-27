import { createRouter } from '@tanstack/react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { rootRoute } from './routes/__root'
import { indexRoute } from './routes/index'
import { pipelinesRoute } from './routes/pipelines'
import { pipelinesIndexRoute } from './routes/pipelines.index'
import { deploymentRoute } from './routes/pipelines.$id'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 10_000,
      retry: 1,
    },
  },
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  pipelinesRoute.addChildren([pipelinesIndexRoute, deploymentRoute]),
])

export const router = createRouter({
  routeTree,

  context: { queryClient },

  defaultPreload: 'intent',

  defaultPreloadStaleTime: 0,

  scrollRestoration: true,

  Wrap: ({ children }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  ),
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
