import type { BadgeVariant } from '@/components/ui'
import type { DeclareMethod, Dispatch, PaymentLink, PaymentLinkKind, PaymentMethod, PaymentQR, PayoutProfile, Summary } from '@/lib/types'

export const METHOD_LABELS: Record<PaymentMethod, string> = {
  qr: 'Virement QR',
  revolut: 'Revolut',
  paypal: 'PayPal',
  link: 'Lien de paiement',
  wero: 'Wero',
  bancontact: 'Bancontact Pay',
  cash: 'Espèces',
  later: 'Plus tard',
  self: 'Payeur',
}

export const METHOD_BADGE: Record<PaymentMethod, BadgeVariant> = {
  qr: 'info',
  revolut: 'info',
  paypal: 'info',
  link: 'info',
  wero: 'wero',
  bancontact: 'bancontact',
  cash: 'neutral',
  later: 'warning',
  self: 'brand',
}

/** Méthodes « lien » : une tuile par wallet, avec son lien dans `PaymentQR.links`. */
export const LINK_METHODS = ['revolut', 'paypal', 'link'] as const satisfies readonly DeclareMethod[]

export function isLinkMethod(m: DeclareMethod): m is PaymentLinkKind {
  return (LINK_METHODS as readonly string[]).includes(m)
}

export function linkFor(qr: PaymentQR | undefined, kind: PaymentLinkKind): PaymentLink | undefined {
  return qr?.links?.find((l) => l.kind === kind)
}

/**
 * Ordre recommandé par le serveur (QR virement avec montant, wallets à lien
 * pré-rempli, lien libre, Wero / Bancontact Pay, espèces, plus tard) ; repli
 * raisonnable si `methods` est absent.
 */
export function availableMethods(qr: PaymentQR | undefined): DeclareMethod[] {
  if (qr?.methods?.length) return qr.methods
  const out: DeclareMethod[] = []
  if (qr?.epc) out.push('qr')
  for (const k of LINK_METHODS) if (linkFor(qr, k)) out.push(k)
  if (qr?.wero) out.push('wero')
  if (qr?.bancontact) out.push('bancontact')
  out.push('cash', 'later')
  return out
}

/**
 * Sur mobile, le téléphone qui affiche le QR ne peut pas le scanner : les
 * liens à montant pré-rempli passent devant le virement QR.
 */
export function orderForDevice(methods: DeclareMethod[], qr: PaymentQR | undefined, mobile: boolean): DeclareMethod[] {
  if (!mobile) return methods
  const prefilled = methods.filter((m) => isLinkMethod(m) && linkFor(qr, m)?.amountPrefilled)
  return [...prefilled, ...methods.filter((m) => !prefilled.includes(m))]
}

/** Sous-titre de tuile : le montant est-il déjà rempli pour le débiteur ? */
export function methodHint(method: DeclareMethod, qr: PaymentQR | undefined): string | undefined {
  if (method === 'qr') return 'Montant inclus'
  if (isLinkMethod(method)) return linkFor(qr, method)?.amountPrefilled ? 'Montant pré-rempli' : 'Montant à saisir'
  if (method === 'wero' || method === 'bancontact') return 'Montant à saisir'
  return undefined
}

/** Moyens que le profil du payeur rend visibles aux autres (aide au diagnostic « mon lien ne s'affiche pas »). */
export function payoutMethods(p: PayoutProfile | null | undefined): PaymentMethod[] {
  if (!p) return []
  const out: PaymentMethod[] = []
  if (p.iban) out.push('qr')
  if (p.revolut_tag) out.push('revolut')
  if (p.paypal_me) out.push('paypal')
  if (p.payment_link) out.push('link')
  if (p.wero_id || p.wero_qr) out.push('wero')
  if (p.bancontact_phone || p.bancontact_qr) out.push('bancontact')
  return out
}

export function declareLabel(method: DeclareMethod): string {
  switch (method) {
    case 'qr':
      return "J'ai payé par virement"
    case 'revolut':
      return "J'ai payé avec Revolut"
    case 'paypal':
      return "J'ai payé avec PayPal"
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
