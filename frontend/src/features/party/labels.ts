import type { BadgeVariant } from '@/components/ui'
import type { DeclareMethod, Dispatch, PaymentMethod, PaymentQR, Summary } from '@/lib/types'

export const METHOD_LABELS: Record<PaymentMethod, string> = {
  qr: 'Virement QR',
  wero: 'Wero',
  bancontact: 'Bancontact Pay',
  link: 'Lien de paiement',
  cash: 'Espèces',
  later: 'Plus tard',
  self: 'Payeur',
}

export const METHOD_BADGE: Record<PaymentMethod, BadgeVariant> = {
  qr: 'info',
  wero: 'wero',
  bancontact: 'bancontact',
  link: 'info',
  cash: 'neutral',
  later: 'warning',
  self: 'brand',
}

/** Ordre recommandé par le serveur ; repli raisonnable si `methods` est absent. */
export function availableMethods(qr: PaymentQR | undefined): DeclareMethod[] {
  if (qr?.methods?.length) return qr.methods
  const out: DeclareMethod[] = []
  if (qr?.wero) out.push('wero')
  if (qr?.bancontact) out.push('bancontact')
  if (qr?.epc) out.push('qr')
  if (qr?.link) out.push('link')
  out.push('cash', 'later')
  return out
}

export function declareLabel(method: DeclareMethod): string {
  switch (method) {
    case 'qr':
      return "J'ai payé par virement"
    case 'wero':
      return "J'ai payé avec Wero"
    case 'bancontact':
      return "J'ai payé avec Bancontact Pay"
    case 'link':
      return "J'ai payé via le lien"
    case 'cash':
      return 'Je paie en espèces'
    case 'later':
      return 'Je paierai plus tard'
  }
}

export const DISPATCH_LABELS: Record<Dispatch['method'], string> = {
  ubereats: 'Uber Eats',
  takeaway: 'Takeaway',
  deliveroo: 'Deliveroo',
  weloveat: 'weloveat',
  export: 'Export',
  phone: 'Téléphone',
}

export function consolidatedText(summary: Summary): string {
  const lines = summary.consolidated.map(
    (l) => `${l.quantity}× ${l.name}${l.optionsLabel ? ` (${l.optionsLabel})` : ''}${l.notes.length ? ` — ${l.notes.join(' / ')}` : ''}`,
  )
  return [`Commande ${summary.restaurant?.name ?? ''}`.trim(), ...lines].join('\n')
}
