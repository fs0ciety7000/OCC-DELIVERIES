import { useEffect, useState } from 'react'

/** Clé « déjà vu » persistée (localStorage, try/catch : navigation privée, quota…). */
export function seenOnce(key: string): boolean {
  try {
    return localStorage.getItem(key) === '1'
  } catch {
    return false
  }
}

export function markSeen(key: string) {
  try {
    localStorage.setItem(key, '1')
  } catch {
    /* stockage indisponible : la scène pourra rejouer, sans gravité */
  }
}

/**
 * `true` la première fois que `key` est rencontrée (et `enabled`), puis `false` pour toujours.
 * Lecture dans l'initialiseur, écriture dans un effet : sûr en StrictMode (double rendu).
 */
export function useFirstTime(key: string, enabled = true): boolean {
  const [first] = useState(() => enabled && !seenOnce(key))
  useEffect(() => {
    if (first) markSeen(key)
  }, [first, key])
  return first
}
