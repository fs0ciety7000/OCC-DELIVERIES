import { useSyncExternalStore } from 'react'

/**
 * Petit store « évènement live » : quand un collègue agit (vote, prêt, paiement),
 * on incrémente un compteur par utilisateur ; l'Avatar s'en sert pour un pulse de 600 ms.
 */
const counters = new Map<string, number>()
const listeners = new Set<() => void>()

export function pulse(userId: string | undefined | null) {
  if (!userId) return
  counters.set(userId, (counters.get(userId) ?? 0) + 1)
  listeners.forEach((l) => l())
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

export function usePulse(userId: string | undefined): number {
  return useSyncExternalStore(
    subscribe,
    () => (userId ? (counters.get(userId) ?? 0) : 0),
    () => 0,
  )
}

/* Cloche « prêt » : un compteur distinct, déclenché par le realtime quand un membre devient prêt. */
const bells = new Map<string, number>()

export function ringBell(userId: string | undefined | null) {
  if (!userId) return
  bells.set(userId, (bells.get(userId) ?? 0) + 1)
  listeners.forEach((l) => l())
}

export function useBell(userId: string | undefined): number {
  return useSyncExternalStore(
    subscribe,
    () => (userId ? (bells.get(userId) ?? 0) : 0),
    () => 0,
  )
}
