import type { PartyStatus } from '@/lib/types'

export const PARTY_STEPS: { status: PartyStatus; label: string }[] = [
  { status: 'lobby', label: 'Salon' },
  { status: 'voting', label: 'Vote' },
  { status: 'ordering', label: 'Commande' },
  { status: 'review', label: 'Récap' },
  { status: 'paying', label: 'Paiement' },
]

export function stepIndex(status: PartyStatus): number {
  if (status === 'closed') return PARTY_STEPS.length
  const i = PARTY_STEPS.findIndex((s) => s.status === status)
  return i < 0 ? 0 : i
}
