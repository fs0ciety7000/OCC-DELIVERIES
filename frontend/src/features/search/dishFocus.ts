import { useEffect, useRef } from 'react'
import { useSearchParams } from 'react-router'

/** Attribut posé sur chaque plat du menu (MenuView) : cible de `?plat=<id>`. */
export const DISH_ATTR = 'data-dish'

/**
 * Arrivée depuis la recherche (`/restaurants/:id?plat=<id>`) : une fois le menu affiché,
 * fait défiler jusqu'au plat (sa catégorie, pas la rangée « Populaires »), le met en
 * évidence (pulse braise 2 × 600 ms ; contour fixe si `prefers-reduced-motion`) et y place le focus.
 */
export function useDishFocus(ready: boolean) {
  const [params] = useSearchParams()
  const dishId = params.get('plat')
  const done = useRef<string | null>(null)

  useEffect(() => {
    if (!ready || !dishId || done.current === dishId) return
    let tries = 0
    let timer = 0
    const attempt = () => {
      const all = Array.from(document.querySelectorAll<HTMLElement>(`[${DISH_ATTR}="${CSS.escape(dishId)}"]`))
      const el = all.find((x) => !x.closest('[data-section="__popular"]')) ?? all[0]
      if (!el) {
        // le menu s'affiche en plusieurs passes (sections animées) : on réessaie un peu
        if (tries++ < 20) timer = window.setTimeout(attempt, 100)
        return
      }
      done.current = dishId
      const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
      el.scrollIntoView?.({ behavior: reduce ? 'auto' : 'smooth', block: 'center' })
      const target = el.querySelector<HTMLElement>('button') ?? el
      target.focus({ preventScroll: true })
      highlight(target, reduce)
    }
    attempt()
    return () => window.clearTimeout(timer)
  }, [ready, dishId])
}

function highlight(el: HTMLElement, reduce: boolean) {
  const ring = 'color-mix(in srgb, var(--color-brand) 55%, transparent)'
  const clear = 'color-mix(in srgb, var(--color-brand) 0%, transparent)'
  if (reduce || typeof el.animate !== 'function') {
    const prev = el.style.boxShadow
    el.style.boxShadow = `0 0 0 3px ${ring}`
    window.setTimeout(() => {
      el.style.boxShadow = prev
    }, 2000)
    return
  }
  // pulse 600 ms (design system), deux fois, après le défilement
  el.animate([{ boxShadow: `0 0 0 0 ${ring}` }, { boxShadow: `0 0 0 8px ${clear}` }], {
    duration: 600,
    iterations: 2,
    easing: 'cubic-bezier(.2,.8,.2,1)',
    delay: 300,
  })
}
