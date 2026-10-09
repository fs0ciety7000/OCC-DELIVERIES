/** Heures limites des commandes : toujours exprimées à l'heure de Bruxelles. */
export const BRUSSELS = 'Europe/Brussels'

const clockFmt = new Intl.DateTimeFormat('fr-BE', { timeZone: BRUSSELS, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })

function toDate(v: string | Date): Date | null {
  const d = typeof v === 'string' ? new Date(v.includes('T') ? v : v.replace(' ', 'T')) : v
  return Number.isNaN(d.getTime()) ? null : d
}

/** « 11:45 » (Bruxelles), `''` si invalide. */
export function formatClock(v: string | Date | null | undefined): string {
  if (!v) return ''
  const d = toDate(v)
  return d ? clockFmt.format(d).replace(/\s/g, '').replace('h', ':') : ''
}

/** Décalage (ms) de Bruxelles par rapport à UTC à l'instant `d`. */
function offsetMs(d: Date): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: BRUSSELS,
    hourCycle: 'h23',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).formatToParts(d)
  const get = (t: string) => Number(parts.find((p) => p.type === t)?.value)
  const asUTC = Date.UTC(get('year'), get('month') - 1, get('day'), get('hour'), get('minute'), get('second'))
  return asUTC - Math.floor(d.getTime() / 1000) * 1000
}

/**
 * « HH:MM » (Bruxelles) → ISO UTC de la prochaine occurrence après `now` : aujourd'hui,
 * ou demain si l'heure est déjà passée de plus de 12 h (sinon `null` : heure passée).
 */
export function brusselsTimeToISO(hhmm: string, now: Date = new Date()): string | null {
  const m = /^([01]?\d|2[0-3]):([0-5]\d)$/.exec(hhmm.trim())
  if (!m) return null
  const h = Number(m[1])
  const min = Number(m[2])
  // date du jour à Bruxelles
  const local = new Date(now.getTime() + offsetMs(now))
  let guess = Date.UTC(local.getUTCFullYear(), local.getUTCMonth(), local.getUTCDate(), h, min) - offsetMs(now)
  // corrige un changement d'heure entre maintenant et l'échéance
  guess = Date.UTC(local.getUTCFullYear(), local.getUTCMonth(), local.getUTCDate(), h, min) - offsetMs(new Date(guess))
  if (guess <= now.getTime()) {
    if (now.getTime() - guess < 12 * 3600_000) return null
    guess += 24 * 3600_000
  }
  return new Date(guess).toISOString()
}

/** Durées proposées pour les heures limites (raccourcis de l'hôte, durée du vote au salon). */
export const DEADLINE_MINUTES = [5, 10, 15, 20] as const

/** ISO dans `minutes` minutes, arrondi à la minute suivante (le planificateur tourne à chaque minute). */
export function isoInMinutesRounded(minutes: number, now: Date = new Date()): string {
  const t = now.getTime() + minutes * 60_000
  return new Date(Math.ceil(t / 60_000) * 60_000).toISOString()
}
