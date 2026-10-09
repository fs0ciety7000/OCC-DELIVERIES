/** Normalisation des identifiants Wero / Bancontact Pay (miroir du hook serveur). */

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** « 0470 12 34 56 » → « +32470123456 » ; « 0032… » → « +32… ». */
export function normalizeMobile(input: string): string {
  let v = input.replace(/[\s().\-/]/g, '')
  if (v.startsWith('00')) v = `+${v.slice(2)}`
  else if (v.startsWith('0')) v = `+32${v.slice(1)}`
  return v
}

export function isE164(v: string): boolean {
  return /^\+[1-9]\d{7,14}$/.test(v)
}

export function normalizeWeroId(input: string): string {
  const v = input.trim()
  if (v === '') return ''
  return v.includes('@') ? v.toLowerCase() : normalizeMobile(v)
}

export function isValidWeroId(input: string): boolean {
  const v = normalizeWeroId(input)
  return v === '' || (v.includes('@') ? EMAIL.test(v) : isE164(v))
}

export function isValidMobile(input: string): boolean {
  const v = input.trim()
  return v === '' || isE164(normalizeMobile(v))
}

export const QR_MAX_BYTES = 1024 * 1024
export const QR_TYPES = ['image/png', 'image/jpeg', 'image/webp']

export function checkQrFile(file: File): string | null {
  if (!QR_TYPES.includes(file.type)) return 'Format accepté : PNG, JPG ou WebP.'
  if (file.size > QR_MAX_BYTES) return 'Image trop lourde (1 Mo maximum).'
  return null
}
