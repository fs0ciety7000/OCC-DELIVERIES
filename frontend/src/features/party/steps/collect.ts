import type { CollectItem, CollectQR } from '@/lib/types'

/**
 * Ce que le QR « Encaisser » encode :
 * - `epc` : QR virement SEPA (EPC069-12), lu par les apps bancaires ;
 * - `revolut` / `paypal` : le lien du payeur **avec le montant**, lu par l'appareil photo.
 */
export type CollectKind = 'epc' | 'revolut' | 'paypal'

export const COLLECT_KIND_LABELS: Record<CollectKind, string> = {
  epc: 'Virement',
  revolut: 'Revolut',
  paypal: 'PayPal',
}

const ORDER: CollectKind[] = ['epc', 'revolut', 'paypal']

/** Types de QR réellement disponibles (seuls les liens à montant pré-rempli comptent). */
export function collectKinds(data: CollectQR | undefined): CollectKind[] {
  if (!data) return []
  return ORDER.filter((k) => data.items.some((it) => qrValueFor(it, k) !== null))
}

/** Contenu du QR d'une part, `null` si ce type n'est pas disponible pour elle. */
export function qrValueFor(item: CollectItem | undefined, kind: CollectKind): string | null {
  if (!item) return null
  if (kind === 'epc') return item.epc
  return item.links.find((l) => l.kind === kind && l.amountPrefilled)?.url ?? null
}

/** Index suivant / précédent, borné (pas de boucle : on sait quand on a fini). */
export function stepIndex(index: number, delta: number, length: number): number {
  if (length <= 0) return 0
  return Math.min(length - 1, Math.max(0, index + delta))
}
