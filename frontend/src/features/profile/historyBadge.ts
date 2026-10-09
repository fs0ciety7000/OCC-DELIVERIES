import type { BadgeVariant } from '@/components/ui'
import { METHOD_LABELS } from '@/features/party/labels'
import { STATUS_LABELS } from '@/features/party/hooks'
import type { HistoryEntry } from '@/lib/types'

const ACTIVE = new Set(['lobby', 'voting', 'ordering', 'review', 'paying'])

/** Badge principal d'une commande passée : où en est **mon** remboursement. */
export function historyBadge(e: HistoryEntry): { label: string; variant: BadgeVariant } {
  if (e.status === 'cancelled') return { label: 'Annulée', variant: 'danger' }
  if (e.items.length === 0) return { label: ACTIVE.has(e.status) ? STATUS_LABELS[e.status] : 'Sans article', variant: ACTIVE.has(e.status) ? 'brand' : 'neutral' }
  const pay = e.payment
  if (pay) {
    if (pay.method === 'self') return { label: "Tu as avancé l'argent", variant: 'brand' }
    if (pay.status === 'confirmed') return { label: 'Remboursé', variant: 'success' }
    if (pay.status === 'declared') return { label: `Déclaré${pay.method ? ` · ${METHOD_LABELS[pay.method]}` : ''}`, variant: 'info' }
    return { label: 'À rembourser', variant: 'warning' }
  }
  if (e.status === 'closed') return { label: 'Terminée', variant: 'success' }
  return { label: STATUS_LABELS[e.status], variant: 'brand' }
}

