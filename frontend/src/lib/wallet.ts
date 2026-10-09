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

/* -------- liens de paiement (miroir de backend/internal/domain/payout.go) -------- */

const REVTAG = /^[a-z0-9][a-z0-9._-]{1,31}$/
const PAYPAL_ME = /^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$/

function looksLikeUrl(v: string): boolean {
  return v.includes('/') || v.includes('.me') || v.includes('.com')
}

function parseLoose(v: string): URL | null {
  if (!v || /\s/.test(v)) return null
  try {
    const u = new URL(v.includes('://') ? v : `https://${v}`)
    return u.hostname.includes('.') && !u.username ? u : null
  } catch {
    return null
  }
}

const host = (u: URL) => u.hostname.toLowerCase().replace(/^www\./, '')
const segments = (u: URL) => u.pathname.split('/').filter(Boolean)

/** « @Bob », « revolut.me/bob » → « bob » ; '' si vide ; `null` si invalide. */
export function normalizeRevolutTag(input: string): string | null {
  let v = input.trim()
  if (!v) return ''
  if (looksLikeUrl(v)) {
    const u = parseLoose(v)
    if (!u || host(u) !== 'revolut.me') return null
    v = segments(u)[0] ?? ''
  }
  v = v.replace(/^@/, '').toLowerCase()
  return REVTAG.test(v) ? v : null
}

/** « paypal.me/jdoe », « paypal.com/paypalme/jdoe » → « jdoe » ; '' si vide ; `null` si invalide. */
export function normalizePayPalMe(input: string): string | null {
  let v = input.trim()
  if (!v) return ''
  if (looksLikeUrl(v)) {
    const u = parseLoose(v)
    if (!u) return null
    const segs = segments(u)
    if (host(u) === 'paypal.me') v = segs[0] ?? ''
    else if (host(u) === 'paypal.com' && segs[0]?.toLowerCase() === 'paypalme') v = segs[1] ?? ''
    else return null
  }
  v = v.replace(/^@/, '')
  return PAYPAL_ME.test(v) ? v : null
}

/** Lien libre : https:// ajouté si absent ; '' si vide ; `null` si invalide. */
export function normalizePaymentLink(input: string): string | null {
  const v = input.trim()
  if (!v) return ''
  const u = parseLoose(v)
  if (!u || (u.protocol !== 'https:' && u.protocol !== 'http:')) return null
  u.protocol = 'https:'
  return u.toString()
}
