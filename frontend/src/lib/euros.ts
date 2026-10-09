/**
 * Saisie de prix en euros (admin) ↔ centimes entiers.
 * Accepte les décimales françaises (« 12,50 »), anglaises (« 12.50 »), « 12€50 »,
 * les espaces / séparateurs de milliers (« 1 234,50 », « 1.234,50 ») et le symbole €.
 * Miroir de `domain.ParseEuros` côté Go.
 */
export function parseEuros(input: string): number | null {
  let s = input.trim().replace(/(\d)\s*€\s*(\d)/, '$1,$2')
  s = s.replace(/€|eur/gi, '').replace(/[\s\u00a0\u202f']/g, '')
  if (s === '') return null
  const lastComma = s.lastIndexOf(',')
  const lastDot = s.lastIndexOf('.')
  if (lastComma >= 0 && lastDot >= 0) {
    s = lastComma > lastDot ? s.replace(/\./g, '').replace(',', '.') : s.replace(/,/g, '')
  } else if (lastComma >= 0) {
    s = s.replace(',', '.')
  }
  const m = /^(\d+)(?:\.(\d{1,2}))?$/.exec(s)
  if (!m) return null
  const euros = Number(m[1])
  if (euros > 100000) return null
  return euros * 100 + Number((m[2] ?? '').padEnd(2, '0'))
}

/** 1250 → « 12,50 » (valeur de champ, sans symbole). */
export function centsToEuros(cents: number | null | undefined): string {
  const c = Number.isFinite(cents) ? Math.max(0, Math.round(cents as number)) : 0
  return `${Math.floor(c / 100)},${String(c % 100).padStart(2, '0')}`
}

/** « 50,4542 » / « 50.4542 » → 50.4542 ; `null` si invalide. */
export function parseDecimal(input: string): number | null {
  const s = input.trim().replace(/[\s\u00a0]/g, '').replace(',', '.')
  if (!/^-?\d+(\.\d+)?$/.test(s)) return null
  return Number(s)
}
