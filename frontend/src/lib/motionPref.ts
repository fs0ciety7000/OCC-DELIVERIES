import { useCallback, useEffect, useState, useSyncExternalStore } from 'react'

/**
 * Réglage « Animations réduites » (Profil → Apparence), même schéma que le thème :
 * localStorage + `data-motion="reduced"` sur <html> (posé aussi par le script d'index.html
 * pour éviter un flash). Il s'ajoute à `prefers-reduced-motion` : l'un OU l'autre suffit
 * à couper les animations non essentielles (GSAP via `withMotion`, `motion` via MotionConfig,
 * CSS via `motion-reduce:` / `motion-safe:` redéfinis dans styles/index.css).
 */
const KEY = 'occ-motion'
const QUERY = '(prefers-reduced-motion: reduce)'
const listeners = new Set<() => void>()

export function readUserReducedMotion(): boolean {
  try {
    return localStorage.getItem(KEY) === 'reduced'
  } catch {
    return false
  }
}

export function systemReducedMotion(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia(QUERY).matches
}

/** Vrai si les animations non essentielles doivent être coupées (système ou réglage). */
export function motionReduced(): boolean {
  return readUserReducedMotion() || systemReducedMotion()
}

export function applyMotionPref(reduced: boolean = readUserReducedMotion()) {
  if (typeof document === 'undefined') return
  if (reduced) document.documentElement.setAttribute('data-motion', 'reduced')
  else document.documentElement.removeAttribute('data-motion')
}

export function setUserReducedMotion(reduced: boolean) {
  try {
    if (reduced) localStorage.setItem(KEY, 'reduced')
    else localStorage.removeItem(KEY)
  } catch {
    /* stockage indisponible : le réglage vaut pour la session (attribut posé) */
  }
  applyMotionPref(reduced)
  listeners.forEach((l) => l())
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

function useSystemReduced(): boolean {
  const [reduced, setReduced] = useState(systemReducedMotion)
  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia(QUERY)
    const onChange = () => setReduced(mq.matches)
    mq.addEventListener?.('change', onChange)
    return () => mq.removeEventListener?.('change', onChange)
  }, [])
  return reduced
}

export interface MotionPref {
  /** Effectif : système OU réglage utilisateur. */
  reduced: boolean
  /** Réglage « Animations réduites » de l'app. */
  userReduced: boolean
  /** `prefers-reduced-motion: reduce` du système. */
  systemReduced: boolean
  setUserReduced: (reduced: boolean) => void
}

export function useMotionPref(): MotionPref {
  const userReduced = useSyncExternalStore(subscribe, readUserReducedMotion, () => false)
  const systemReduced = useSystemReduced()
  const set = useCallback((r: boolean) => setUserReducedMotion(r), [])
  return { reduced: userReduced || systemReduced, userReduced, systemReduced, setUserReduced: set }
}
