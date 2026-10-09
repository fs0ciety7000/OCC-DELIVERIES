import type { TeamRole, Weekday } from '@/lib/types'

export const WEEKDAYS: { id: Weekday; short: string; long: string }[] = [
  { id: 'mon', short: 'Lun', long: 'lundi' },
  { id: 'tue', short: 'Mar', long: 'mardi' },
  { id: 'wed', short: 'Mer', long: 'mercredi' },
  { id: 'thu', short: 'Jeu', long: 'jeudi' },
  { id: 'fri', short: 'Ven', long: 'vendredi' },
  { id: 'sat', short: 'Sam', long: 'samedi' },
  { id: 'sun', short: 'Dim', long: 'dimanche' },
]

const WORKWEEK: Weekday[] = ['mon', 'tue', 'wed', 'thu', 'fri']

/** « Du lundi au vendredi à 12:15 », « Mardi, jeudi à 12:00 », « À 12:15 », ''. */
export function usualSummary(days: Weekday[] | null | undefined, time: string | null | undefined): string {
  const d = WEEKDAYS.filter((w) => days?.includes(w.id))
  let when = ''
  if (d.length === 7) when = 'Tous les jours'
  else if (d.length === 5 && WORKWEEK.every((w) => days?.includes(w))) when = 'Du lundi au vendredi'
  else if (d.length > 0) {
    const names = d.map((w) => w.long).join(', ')
    when = names.charAt(0).toUpperCase() + names.slice(1)
  }
  if (time) return when ? `${when} à ${time}` : `À ${time}`
  return when
}

export const ROLE_LABELS: Record<Exclude<TeamRole, ''>, string> = {
  owner: 'Propriétaire',
  admin: 'Admin',
  member: 'Membre',
}

export function canManage(role: TeamRole | undefined): boolean {
  return role === 'owner' || role === 'admin'
}

/** Lien d'invitation permanent d'une équipe. */
export function teamInviteUrl(code: string): string {
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  return `${origin}/e/${code}`
}

export const TEAM_CODE_LENGTH = 8

/** « k7m2-qxab » → « K7M2QXAB ». */
export function normalizeTeamCode(raw: string): string {
  return raw.toUpperCase().replace(/[\s-]/g, '')
}
