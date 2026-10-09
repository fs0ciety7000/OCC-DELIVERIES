/** Valeur intermédiaire du compteur, toujours en centimes entiers (jamais de fraction de centime). */
export function counterValue(from: number, to: number, progress: number): number {
  const p = Math.min(1, Math.max(0, progress))
  return Math.round(from + (to - from) * p)
}
