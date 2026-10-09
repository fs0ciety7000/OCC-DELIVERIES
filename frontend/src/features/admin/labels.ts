import type { BadgeVariant } from '@/components/ui'
import type { PartyStatus } from '@/lib/types'

export const STATUS_LABEL: Record<PartyStatus, string> = {
  lobby: 'Salon',
  voting: 'Vote',
  ordering: 'Commande',
  review: 'Récap',
  paying: 'Paiement',
  closed: 'Terminée',
  cancelled: 'Annulée',
}

export const STATUS_VARIANT: Record<PartyStatus, BadgeVariant> = {
  lobby: 'info',
  voting: 'brand',
  ordering: 'brand',
  review: 'warning',
  paying: 'warning',
  closed: 'success',
  cancelled: 'neutral',
}

export const STATUSES: PartyStatus[] = ['lobby', 'voting', 'ordering', 'review', 'paying', 'closed', 'cancelled']

const dayFmt = new Intl.DateTimeFormat('fr-BE', { weekday: 'short', day: 'numeric', month: 'short', timeZone: 'UTC' })
const shortFmt = new Intl.DateTimeFormat('fr-BE', { day: 'numeric', month: 'short', timeZone: 'UTC' })

/** « 2026-10-08 » → { long: « mer. 8 oct. », short: « 8 oct. » } */
export function dayLabels(isoDay: string): { long: string; short: string } {
  const d = new Date(`${isoDay}T00:00:00Z`)
  return { long: dayFmt.format(d), short: shortFmt.format(d) }
}

/** Étiquettes proposées dans l'éditeur de menu (les autres restent libres). */
export const PRESET_TAGS: { id: string; label: string }[] = [
  { id: 'veggie', label: 'Végé' },
  { id: 'vegan', label: 'Vegan' },
  { id: 'spicy', label: 'Épicé' },
  { id: 'gluten_free', label: 'Sans gluten' },
  { id: 'new', label: 'Nouveau' },
]
