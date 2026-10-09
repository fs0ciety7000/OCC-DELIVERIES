import { readUserReducedMotion } from './motionPref'

/**
 * Retours haptiques courts (`navigator.vibrate`), toujours facultatifs :
 * absents sur iOS / desktop, ignorés sans geste utilisateur récent, coupés par le
 * réglage « Animations réduites ». Ne jamais porter d'information uniquement par là.
 */
export const HAPTICS = {
  vote: 10,
  ready: [15, 40, 15],
  paid: [20, 60, 20],
} as const satisfies Record<string, number | readonly number[]>

export type HapticKind = keyof typeof HAPTICS

/** Renvoie `true` si une vibration a été demandée au navigateur. */
export function haptic(kind: HapticKind): boolean {
  if (typeof navigator === 'undefined' || typeof navigator.vibrate !== 'function') return false
  if (readUserReducedMotion()) return false
  try {
    const p = HAPTICS[kind]
    return navigator.vibrate(typeof p === 'number' ? p : [...p])
  } catch {
    return false
  }
}
