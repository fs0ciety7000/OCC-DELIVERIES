import type { Restaurant } from '@/lib/types'

/** Seuil proposé quand on active le filtre « cartes incomplètes ». */
export const DEFAULT_MIN_MENU_ITEMS = 10
/** Seuil maximum accepté par le serveur. */
export const MAX_MIN_MENU_ITEMS = 100

/** Même règle que le serveur (`domain.HiddenIncomplete`) : 0 = filtre désactivé. */
export function isIncomplete(r: Pick<Restaurant, 'items_count'>, minItems: number): boolean {
  return minItems > 0 && (r.items_count ?? 0) < minItems
}

/** Restaurants actifs absents des listes publiques pour ce seuil. */
export function countIncomplete(list: readonly Pick<Restaurant, 'items_count' | 'active'>[], minItems: number): number {
  return list.filter((r) => r.active && isIncomplete(r, minItems)).length
}

/** Valide la saisie du seuil (entier 1–100), message en français sinon. */
export function parseThreshold(raw: string): { value: number } | { error: string } {
  const n = Number(raw.trim())
  if (!raw.trim() || !Number.isInteger(n) || n < 1 || n > MAX_MIN_MENU_ITEMS) {
    return { error: `Entre un nombre entier de 1 à ${MAX_MIN_MENU_ITEMS}.` }
  }
  return { value: n }
}
