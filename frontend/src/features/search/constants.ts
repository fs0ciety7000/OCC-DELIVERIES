/** Envies proposées quand le champ est vide (et quand rien n'est trouvé). */
export const SUGGESTIONS = ['Pizza', 'Sushi', 'Burger', 'Poké', 'Ramen', 'Thaï', 'Indien', 'Libanais', 'Frites'] as const

/** « ⌘K » sur Apple, « Ctrl K » ailleurs. */
export function shortcutLabel(): string {
  const platform = typeof navigator !== 'undefined' ? navigator.platform || navigator.userAgent : ''
  return /Mac|iPhone|iPad/i.test(platform) ? '⌘K' : 'Ctrl K'
}
