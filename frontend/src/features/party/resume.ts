import { useMemo } from 'react'
import { PARTY_STEPS, stepIndex } from '@/components/ui/steps'
import { isActiveStatus, readLastParty } from '@/lib/lastParty'
import type { PartyStatus, Restaurant } from '@/lib/types'
import { useMyParties } from './hooks'

/** Commande en cours, forme minimale commune (liste serveur ou souvenir local). */
export interface ActiveParty {
  id: string
  title: string
  status: PartyStatus
  restaurant?: Pick<Restaurant, 'id' | 'name' | 'emoji' | 'cover' | 'cover_url'>
}

/**
 * Mes commandes en cours (statut ≠ closed / cancelled), mises à jour par le realtime global.
 * Si la liste serveur échoue, repli sur la dernière commande ouverte (localStorage).
 */
export function useActiveParties(userId: string | undefined) {
  const q = useMyParties(userId)
  const parties = useMemo<ActiveParty[]>(() => {
    if (q.data) return q.data.map((p) => ({ id: p.id, title: p.title || 'Commande groupée', status: p.status, restaurant: p.expand?.restaurant }))
    if (q.isError) {
      const last = readLastParty(userId)
      return last && isActiveStatus(last.status) ? [{ id: last.id, title: last.title || 'Commande groupée', status: last.status }] : []
    }
    return []
  }, [q.data, q.isError, userId])
  return { parties, isPending: q.isPending && !!userId }
}

/** « Étape 3/5 · Commande » (libellés courts du stepper). */
export function stepText(status: PartyStatus): string {
  const i = Math.min(stepIndex(status), PARTY_STEPS.length - 1)
  return `Étape ${i + 1}/${PARTY_STEPS.length} · ${PARTY_STEPS[i]!.label}`
}

