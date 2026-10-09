import type { PartyStatus } from './types'

/** Dernière commande ouverte, mémorisée localement (repli si la liste serveur est indisponible). */
export interface LastParty {
  id: string
  title: string
  status: PartyStatus
  code: string
  at: number
}

const KEY = 'occ:lastParty'
const ACTIVE: PartyStatus[] = ['lobby', 'voting', 'ordering', 'review', 'paying']

export function isActiveStatus(status: PartyStatus | undefined): boolean {
  return !!status && ACTIVE.includes(status)
}

/** Lit la dernière commande mémorisée (null si absente, illisible ou stockage bloqué). */
export function readLastParty(userId: string | undefined): LastParty | null {
  if (!userId) return null
  try {
    const raw = localStorage.getItem(`${KEY}:${userId}`)
    if (!raw) return null
    const v = JSON.parse(raw) as Partial<LastParty>
    if (typeof v.id !== 'string' || typeof v.status !== 'string') return null
    return { id: v.id, title: v.title ?? '', status: v.status, code: v.code ?? '', at: v.at ?? 0 }
  } catch {
    return null
  }
}

/** Mémorise la commande ouverte ; une commande terminée efface le souvenir. */
export function rememberParty(userId: string | undefined, p: { id: string; title: string; status: PartyStatus; code: string }) {
  if (!userId) return
  try {
    if (!isActiveStatus(p.status)) {
      const cur = readLastParty(userId)
      if (cur?.id === p.id) localStorage.removeItem(`${KEY}:${userId}`)
      return
    }
    localStorage.setItem(`${KEY}:${userId}`, JSON.stringify({ id: p.id, title: p.title, status: p.status, code: p.code, at: Date.now() }))
  } catch {
    /* stockage indisponible (navigation privée…) : sans conséquence */
  }
}

export function forgetParty(userId: string | undefined) {
  if (!userId) return
  try {
    localStorage.removeItem(`${KEY}:${userId}`)
  } catch {
    /* ignoré */
  }
}
