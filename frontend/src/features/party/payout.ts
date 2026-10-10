import { ClientResponseError } from 'pocketbase'
import type { CollectMode, PaymentLinkKind, PayoutStatus, Summary } from '@/lib/types'

/**
 * Garde du payeur (ADR 0003, mise à jour 4) : en mode `transfer`, le payeur doit
 * avoir un moyen de remboursement utilisable (IBAN de préférence, Revolut,
 * PayPal.me, autre lien). Le mode `cash` ne bloque jamais. Le serveur fait foi :
 * ces helpers ne servent qu'à guider l'interface.
 */

/** Code machine de l'erreur 409 (`data.payer.code`). */
export const PAYER_NO_PAYOUT = 'payer_no_payout'

const LINK_LABELS: Record<PaymentLinkKind, string> = { revolut: 'Revolut', paypal: 'PayPal', link: 'Lien' }

/** Indices affichés à côté d'un membre : « IBAN ✓ », « Revolut ✓ »… ; vide = aucun moyen. */
export function payoutHints(status: PayoutStatus | undefined): string[] {
  if (!status) return []
  const out: string[] = []
  if (status.iban) out.push('IBAN ✓')
  for (const k of status.links) out.push(`${LINK_LABELS[k]} ✓`)
  return out
}

/** Le payeur doit-il pouvoir recevoir un virement ? Seulement si un·e autre membre a commandé. */
export function payoutRequired(payer: string, summary: Pick<Summary, 'participants'> | undefined): boolean {
  if (!summary) return true
  return summary.participants.some((p) => p.items.length > 0 && p.user.id !== payer)
}

/** Raison pour laquelle la validation est impossible (`null` = possible). */
export function payerBlockReason(opts: { payer: string; meId: string; name: string; mode: CollectMode; status: PayoutStatus | undefined; required: boolean }): string | null {
  if (opts.mode === 'cash' || !opts.required || opts.status?.ready) return null
  if (!opts.status) return 'Vérification des moyens de remboursement…'
  return opts.payer === opts.meId
    ? 'Ajoute ton IBAN (ou Revolut / PayPal) avant de valider, ou choisis « Espèces ».'
    : `${opts.name || 'Ce membre'} n'a encore renseigné aucun moyen de remboursement : demande-lui son IBAN ou choisis « Espèces ».`
}

/** Le serveur a refusé le payeur faute de moyen de remboursement (course, ancien écran…). */
export function isPayerNoPayout(err: unknown): boolean {
  if (!(err instanceof ClientResponseError)) return false
  const data = err.response?.data as { payer?: { code?: string } } | undefined
  return data?.payer?.code === PAYER_NO_PAYOUT
}

/* -------- carte « Ajoute ton IBAN » masquée par party (préférence locale) -------- */

const NUDGE_KEY = 'occ-payout-nudge-dismissed'

function readDismissed(): string[] {
  try {
    const raw = localStorage.getItem(NUDGE_KEY)
    const list = raw ? (JSON.parse(raw) as unknown) : []
    return Array.isArray(list) ? list.filter((x): x is string => typeof x === 'string') : []
  } catch {
    return []
  }
}

export function isNudgeDismissed(partyId: string): boolean {
  return readDismissed().includes(partyId)
}

export function dismissNudge(partyId: string): void {
  try {
    const list = readDismissed().filter((id) => id !== partyId)
    list.push(partyId)
    localStorage.setItem(NUDGE_KEY, JSON.stringify(list.slice(-50)))
  } catch {
    /* stockage indisponible : la carte réapparaîtra */
  }
}
