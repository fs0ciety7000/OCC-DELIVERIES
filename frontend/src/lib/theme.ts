import { useCallback, useEffect, useSyncExternalStore } from 'react'

export type ThemePref = 'dark' | 'light' | 'system'
const KEY = 'occ-theme'
const listeners = new Set<() => void>()

function readPref(): ThemePref {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'light' || v === 'system' ? v : 'dark'
  } catch {
    return 'dark'
  }
}

export function resolveTheme(pref: ThemePref): 'dark' | 'light' {
  if (pref !== 'system') return pref
  return typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

export function applyTheme(pref: ThemePref) {
  document.documentElement.setAttribute('data-theme', resolveTheme(pref))
}

export function setThemePref(pref: ThemePref) {
  try {
    localStorage.setItem(KEY, pref)
  } catch {
    /* stockage indisponible */
  }
  applyTheme(pref)
  listeners.forEach((l) => l())
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  return () => listeners.delete(cb)
}

export function useTheme() {
  const pref = useSyncExternalStore(subscribe, readPref, () => 'dark' as ThemePref)
  useEffect(() => {
    if (pref !== 'system') return
    const mq = matchMedia('(prefers-color-scheme: light)')
    const onChange = () => applyTheme('system')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [pref])
  const set = useCallback((p: ThemePref) => setThemePref(p), [])
  return { pref, resolved: resolveTheme(pref), setPref: set }
}
