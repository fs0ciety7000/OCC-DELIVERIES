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
