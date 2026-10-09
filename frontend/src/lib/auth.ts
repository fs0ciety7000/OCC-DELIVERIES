import { useSyncExternalStore } from 'react'
import { pb } from './pb'
import type { User } from './types'

/*
 * Attention : `LocalAuthStore.record` relit (JSON.parse) le stockage à chaque accès et
 * renvoie donc un NOUVEL objet à chaque fois. On calcule l'instantané uniquement
 * quand l'authStore change, sinon useSyncExternalStore boucle à l'infini.
 */
function compute(): User | null {
  return pb.authStore.isValid && pb.authStore.record ? (pb.authStore.record as unknown as User) : null
}

let snapshot: User | null = compute()
const listeners = new Set<() => void>()
pb.authStore.onChange(() => {
  snapshot = compute()
  listeners.forEach((l) => l())
})

function subscribe(cb: () => void) {
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}

function getSnapshot(): User | null {
  return snapshot
}

/** Utilisateur connecté (réactif via `pb.authStore.onChange`). */
export function useAuth(): { user: User | null; isAuthenticated: boolean } {
  const user = useSyncExternalStore(subscribe, getSnapshot, () => null)
  return { user, isAuthenticated: user !== null }
}

export function logout() {
  pb.authStore.clear()
}

/** Rafraîchit silencieusement la session au démarrage ; la vide si le jeton est invalide. */
export async function refreshAuth() {
  if (!pb.authStore.isValid) return
  try {
    await pb.collection('users').authRefresh()
  } catch (err) {
    const status = (err as { status?: number }).status
    if (status === 401 || status === 403 || status === 404) pb.authStore.clear()
  }
}
