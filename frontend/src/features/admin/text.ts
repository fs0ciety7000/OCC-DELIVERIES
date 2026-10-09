/** Minuscules sans accents, pour des recherches tolérantes. */
export function fold(s: string): string {
  return s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
}
