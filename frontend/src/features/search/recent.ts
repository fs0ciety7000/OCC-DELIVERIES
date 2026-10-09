import { fold } from './fold'

/** Recherches récentes, mémorisées sur l'appareil (jamais envoyées au serveur). */
const KEY = 'occ:search:recent'
export const MAX_RECENT = 6

export function readRecent(): string[] {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return []
    const v: unknown = JSON.parse(raw)
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string' && x.trim() !== '').slice(0, MAX_RECENT) : []
  } catch {
    return []
  }
}

function write(list: string[]) {
  try {
    localStorage.setItem(KEY, JSON.stringify(list))
  } catch {
    /* stockage plein ou bloqué (navigation privée) : on ne mémorise pas */
  }
}

/** Ajoute une recherche en tête (doublons insensibles à la casse et aux accents retirés). */
export function pushRecent(q: string): string[] {
  const v = q.trim().replace(/\s+/g, ' ')
  if (v.length < 2) return readRecent()
  const key = fold(v)
  const list = [v, ...readRecent().filter((x) => fold(x) !== key)].slice(0, MAX_RECENT)
  write(list)
  return list
}

export function removeRecent(q: string): string[] {
  const list = readRecent().filter((x) => x !== q)
  write(list)
  return list
}

export function clearRecent(): string[] {
  try {
    localStorage.removeItem(KEY)
  } catch {
    /* ignoré */
  }
  return []
}
