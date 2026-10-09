import { PARTY_STEPS, stepIndex } from '@/components/ui/steps'
import type { DispatchMethod, PartyStatus } from '@/lib/types'
import { DISPATCH_LABELS } from '../labels'

/** Texte annoncé (aria-live) quand l'étape change : « Étape 3 sur 5 : Commande ». */
export function stepAnnouncement(status: PartyStatus): string {
  if (status === 'closed') return 'Commande terminée'
  if (status === 'cancelled') return 'Commande annulée'
  const i = stepIndex(status)
  return `Étape ${i + 1} sur ${PARTY_STEPS.length} : ${PARTY_STEPS[i]?.label ?? ''}`
}

/** Ordre des étapes pour le sens de la transition (annulée = après tout). */
export function stepOrder(status: PartyStatus): number {
  return status === 'cancelled' ? PARTY_STEPS.length + 1 : stepIndex(status)
}

/** Titre de la scène « Commande envoyée ». */
export function dispatchHeadline(method: DispatchMethod): string {
  if (method === 'phone') return 'Commande passée par téléphone'
  if (method === 'export') return 'Commande exportée'
  return `Commande envoyée via ${DISPATCH_LABELS[method]}`
}

/**
 * « arrivée estimée ~35 min » d'après le délai du resto (eta_min / eta_max, minutes) :
 * milieu de la fourchette arrondi à 5 min ; `null` si aucun délai connu.
 */
export function etaEstimate(etaMin?: number, etaMax?: number): string | null {
  const lo = etaMin && etaMin > 0 ? etaMin : 0
  const hi = etaMax && etaMax > 0 ? etaMax : 0
  if (!lo && !hi) return null
  const mid = lo && hi ? (lo + hi) / 2 : lo || hi
  const rounded = Math.max(5, Math.round(mid / 5) * 5)
  return `arrivée estimée ~${rounded} min`
}
