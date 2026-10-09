const moneyFormatter = new Intl.NumberFormat('fr-BE', { style: 'currency', currency: 'EUR' })

/** 1250 → « 12,50 € » (fr-BE). Les montants sont toujours des centimes entiers. */
export function formatMoney(cents: number | null | undefined): string {
  const value = Number.isFinite(cents) ? Math.round(cents as number) : 0
  return moneyFormatter.format(value / 100)
}

/** « 2,50 » / « 2.5 » / « 2 € » → 250. Renvoie `null` si invalide. */
export function parseMoneyToCents(input: string): number | null {
  const cleaned = input.replace(/\s|€/g, '').replace(',', '.')
  if (cleaned === '') return 0
  if (!/^\d+(\.\d{0,2})?$/.test(cleaned)) return null
  return Math.round(parseFloat(cleaned) * 100)
}

/** Centimes → valeur pour un champ de saisie (« 2,50 »). */
export function centsToInput(cents: number): string {
  return (cents / 100).toFixed(2).replace('.', ',')
}

const kmFormatter = new Intl.NumberFormat('fr-BE', { maximumFractionDigits: 1 })

/** 0.35 → « 350 m », 1.24 → « 1,2 km ». */
export function formatDistance(km: number | null | undefined): string {
  if (km == null || !Number.isFinite(km)) return ''
  if (km < 1) return `${Math.max(10, Math.round((km * 1000) / 10) * 10)} m`
  return `${kmFormatter.format(km)} km`
}

/** Parse une date PocketBase (« 2026-10-09 09:10:00.000Z ») ou ISO. */
export function parseDate(value: string | null | undefined): Date | null {
  if (!value) return null
  const d = new Date(value.includes('T') ? value : value.replace(' ', 'T'))
  return Number.isNaN(d.getTime()) ? null : d
}

const rtf = new Intl.RelativeTimeFormat('fr', { numeric: 'auto' })

/** « à l'instant », « il y a 5 minutes », « dans 2 heures », « hier »… */
export function formatRelativeTime(value: string | Date | null | undefined, now: Date = new Date()): string {
  const date = typeof value === 'string' ? parseDate(value) : value
  if (!date) return ''
  const diffSec = Math.round((date.getTime() - now.getTime()) / 1000)
  const abs = Math.abs(diffSec)
  if (abs < 45) return "à l'instant"
  if (abs < 3600) return rtf.format(Math.round(diffSec / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(diffSec / 3600), 'hour')
  if (abs < 86400 * 30) return rtf.format(Math.round(diffSec / 86400), 'day')
  if (abs < 86400 * 365) return rtf.format(Math.round(diffSec / (86400 * 30)), 'month')
  return rtf.format(Math.round(diffSec / (86400 * 365)), 'year')
}

const timeFormatter = new Intl.DateTimeFormat('fr-BE', { hour: '2-digit', minute: '2-digit' })

export function formatTime(value: string | Date | null | undefined): string {
  const date = typeof value === 'string' ? parseDate(value) : value
  return date ? timeFormatter.format(date) : ''
}

/** Millisecondes restantes → « mm:ss » (ou « h:mm:ss » au-delà d'une heure). */
export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`
}

/** 25, 35 → « 25–35 min ». */
export function formatEta(min?: number, max?: number): string {
  if (!min && !max) return ''
  if (min && max && max !== min) return `${min}–${max} min`
  return `${min || max} min`
}

export function formatRating(rating?: number): string {
  if (!rating) return ''
  return rating.toLocaleString('fr-BE', { minimumFractionDigits: 1, maximumFractionDigits: 1 })
}

/** « Alice Martin » → « AM ». */
export function initials(name: string | null | undefined): string {
  const parts = (name || '?').trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  const first = parts[0]?.[0] ?? ''
  const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : ''
  return (first + last).toUpperCase()
}

/** Accord simple : plural(2, 'article') → « 2 articles ». */
export function plural(n: number, singular: string, pluralForm = `${singular}s`): string {
  return `${n} ${Math.abs(n) >= 2 ? pluralForm : singular}`
}

export function priceLevel(level?: number): string {
  return level ? '€'.repeat(Math.min(4, Math.max(1, level))) : ''
}

/** Date ISO « dans N minutes » (échéances indicatives). */
export function isoInMinutes(minutes: number, from: number = Date.now()): string {
  return new Date(from + minutes * 60_000).toISOString()
}

/**
 * Téléphone stocké en E.164 (« +3265352964 ») → « +32 65 35 29 64 ».
 * Jumeau de `domain.FormatPhone` (Go) : mobiles « +32 475 12 34 56 », zones à
 * un chiffre (Bruxelles, Anvers, Liège, Gand) « +32 2 123 45 67 », numéros
 * spéciaux « +32 800 12 345 ». Tout autre format est rendu tel quel.
 */
export function formatPhone(phone: string | null | undefined): string {
  const p = (phone ?? '').trim()
  const n = /^\+32(\d{8,9})$/.exec(p)?.[1]
  if (!n) return p
  const group = (prefix: string, rest: string, sizes: number[]) => {
    const parts = ['+32', prefix]
    for (const s of sizes) {
      if (rest.length < s) break
      parts.push(rest.slice(0, s))
      rest = rest.slice(s)
    }
    if (rest) parts.push(rest)
    return parts.join(' ')
  }
  if (n.length === 9 && n.startsWith('4')) return group(n.slice(0, 3), n.slice(3), [2, 2, 2])
  if (n.length === 9) return p
  if (/^(800|90|70|78)/.test(n)) return group(n.slice(0, 3), n.slice(3), [2, 3])
  if ('2349'.includes(n.charAt(0))) return group(n.slice(0, 1), n.slice(1), [3, 2, 2])
  return group(n.slice(0, 2), n.slice(2), [2, 2, 2])
}

/** Lien `tel:` d'un numéro (espaces, points, barres retirés). */
export function telHref(phone: string): string {
  return `tel:${phone.replace(/[\s./()-]/g, '')}`
}

/**
 * Lien carte d'un restaurant : la position exacte quand elle est connue,
 * sinon une recherche OpenStreetMap sur l'adresse.
 */
export function mapHref(r: { address?: string; lat?: number; lng?: number; geo_approx?: boolean; name?: string }): string {
  if (r.lat && r.lng && !r.geo_approx) {
    return `https://www.openstreetmap.org/?mlat=${r.lat}&mlon=${r.lng}#map=18/${r.lat}/${r.lng}`
  }
  const query = r.address?.trim() || r.name?.trim() || ''
  return `https://www.openstreetmap.org/search?query=${encodeURIComponent(query)}`
}

const dateFormatter = new Intl.DateTimeFormat('fr-BE', { weekday: 'short', day: 'numeric', month: 'long', year: 'numeric' })

/** « ven. 9 octobre 2026 » */
export function formatDate(value: string | Date | null | undefined): string {
  const date = typeof value === 'string' ? parseDate(value) : value
  return date ? dateFormatter.format(date) : ''
}
