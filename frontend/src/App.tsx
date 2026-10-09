import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MotionConfig } from 'motion/react'
import { RouterProvider } from 'react-router'
import { Toaster } from 'sonner'
import { LocationProvider } from '@/lib/geo'
import { useMotionPref } from '@/lib/motionPref'
import { useTheme } from '@/lib/theme'
import { router } from './router'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      // hors ligne : une tentative (réponse du cache du service worker), puis pause jusqu'au retour du réseau
      networkMode: 'offlineFirst',
      retry: (count, err) => {
        const status = (err as { status?: number }).status
        if (status && status >= 400 && status < 500) return false
        return count < 2
      },
      refetchOnWindowFocus: true,
    },
    // les actions rejouables passent par la file hors ligne (lib/offlineActions) : jamais mises en pause
    mutations: { networkMode: 'always' },
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
  // « Animations réduites » (profil) force le mode réduit de `motion`, en plus du système.
  const { userReduced } = useMotionPref()
  return (
    <QueryClientProvider client={queryClient}>
      <MotionConfig reducedMotion={userReduced ? 'always' : 'user'}>
        <LocationProvider>
          <RouterProvider router={router} />
          <ThemedToaster />
        </LocationProvider>
      </MotionConfig>
    </QueryClientProvider>
  )
}
