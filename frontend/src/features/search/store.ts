import { useSyncExternalStore } from 'react'

/** Ouverture de la palette de recherche, pilotable de partout (en-tête, raccourcis, pages). */
let state = { open: false, initial: '' }
const listeners = new Set<() => void>()
const emit = () => listeners.forEach((l) => l())

export function openSearch(initial = '') {
  state = { open: true, initial }
  emit()
}

export function closeSearch() {
  if (!state.open) return
  state = { open: false, initial: '' }
  emit()
}

export function toggleSearch() {
  if (state.open) closeSearch()
  else openSearch()
}

function subscribe(cb: () => void) {
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}

export function useSearchPalette(): { open: boolean; initial: string } {
  return useSyncExternalStore(
    subscribe,
    () => state,
    () => state,
  )
}
