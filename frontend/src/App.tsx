import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MotionConfig } from 'motion/react'
import { RouterProvider } from 'react-router'
import { Toaster } from 'sonner'
import { LocationProvider } from '@/lib/geo'
import { useTheme } from '@/lib/theme'
import { router } from './router'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: (count, err) => {
        const status = (err as { status?: number }).status
        if (status && status >= 400 && status < 500) return false
        return count < 2
      },
      refetchOnWindowFocus: true,
    },
  },
})

function ThemedToaster() {
  const { resolved } = useTheme()
  return (
    <Toaster
      theme={resolved}
      position="top-center"
      richColors
      closeButton
      toastOptions={{ className: 'font-sans', style: { borderRadius: 'var(--radius-md)' } }}
      offset={{ top: 'calc(env(safe-area-inset-top) + 12px)' }}
    />
  )
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion="user">
        <LocationProvider>
          <RouterProvider router={router} />
          <ThemedToaster />
        </LocationProvider>
      </MotionConfig>
    </QueryClientProvider>
  )
}
