import type { BadgeVariant } from '@/components/ui'
import { BRUSSELS } from '@/lib/brussels'
import type { BackupHealth } from '@/lib/types'

export const HEALTH_LABEL: Record<BackupHealth, string> = {
  ok: 'À jour',
  stale: 'En retard',
  pending: 'Première sauvegarde attendue',
  disabled: 'Non configurées',
}

export const HEALTH_VARIANT: Record<BackupHealth, BadgeVariant> = {
  ok: 'success',
  stale: 'danger',
  pending: 'neutral',
  disabled: 'warning',
}

export const CHECK_TRIGGER_LABEL = {
  cron: 'automatique',
  manual: 'manuelle',
  backup: 'après la sauvegarde',
} as const

/** 101 564 → « 99,2 Ko » (base 1024, décimale française). */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '—'
  if (n < 1024) return `${n} o`
  const units = ['Ko', 'Mo', 'Go', 'To']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  const digits = v < 10 ? 1 : 0
  return `${v.toLocaleString('fr-BE', { minimumFractionDigits: digits, maximumFractionDigits: digits })} ${units[i]}`
}

const dateTimeFmt = new Intl.DateTimeFormat('fr-BE', {
  timeZone: BRUSSELS,
  day: 'numeric',
  month: 'short',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
})

/** ISO UTC → « 11 oct. 2026, 02:15 » (heure de Bruxelles), `''` si vide / invalide. */
export function formatBackupDate(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : dateTimeFmt.format(d)
}

/** Âge lisible d'une sauvegarde : « il y a 3 h », « il y a 2 j ». */
export function formatAge(iso: string | null | undefined, now: Date = new Date()): string {
  if (!iso) return ''
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return ''
  const min = Math.max(0, Math.round((now.getTime() - t) / 60_000))
  if (min < 1) return "à l'instant"
  if (min < 60) return `il y a ${min} min`
  const h = Math.round(min / 60)
  if (h < 48) return `il y a ${h} h`
  return `il y a ${Math.round(h / 24)} j`
}

/** Lignes d'erreur du journal (une par ligne, vides retirées). */
export function errorLines(error: string): string[] {
  return error
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
}
