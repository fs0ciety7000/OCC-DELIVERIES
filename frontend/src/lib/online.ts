import { ClientResponseError } from 'pocketbase'
import { useSyncExternalStore } from 'react'

/** Le navigateur se croit en ligne (toujours vrai hors navigateur). */
export function isOnline(): boolean {
  return typeof navigator === 'undefined' || navigator.onLine !== false
}

function subscribe(cb: () => void) {
  if (typeof window === 'undefined') return () => undefined
  window.addEventListener('online', cb)
  window.addEventListener('offline', cb)
  return () => {
    window.removeEventListener('online', cb)
    window.removeEventListener('offline', cb)
  }
}

/** État de connexion réactif (`online` / `offline`). */
export function useOnline(): boolean {
  return useSyncExternalStore(subscribe, isOnline, () => true)
}

/** Échec réseau (serveur injoignable), par opposition à un refus du serveur (4xx/5xx). */
export function isNetworkError(err: unknown): boolean {
  if (err instanceof ClientResponseError) return !err.isAbort && err.status === 0
  return err instanceof TypeError
}

/** Message des actions indisponibles hors ligne (infobulles, boutons désactivés). */
export const OFFLINE_HINT = 'Indisponible hors ligne'
