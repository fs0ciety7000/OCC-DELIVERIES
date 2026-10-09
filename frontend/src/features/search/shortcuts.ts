import { useEffect } from 'react'
import { openSearch, toggleSearch } from './store'

/** Vrai si la touche part d'un champ de saisie (on n'y vole jamais « / »). */
function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
}

/** Une autre fenêtre modale (feuille, confirmation) est ouverte. */
function otherModalOpen(): boolean {
  return Array.from(document.querySelectorAll('[aria-modal="true"]')).some((el) => el.getAttribute('aria-label') !== 'Recherche')
}

/** Raccourcis globaux : Ctrl K / ⌘K (ouvre ou ferme), « / » (ouvre, hors champ de saisie). */
export function useSearchShortcuts() {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.isComposing) return
      if ((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === 'k') {
        if (otherModalOpen()) return
        e.preventDefault()
        toggleSearch()
        return
      }
      if (e.key === '/' && !e.metaKey && !e.ctrlKey && !e.altKey && !isTyping(e.target) && !otherModalOpen()) {
        e.preventDefault()
        openSearch()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
}
