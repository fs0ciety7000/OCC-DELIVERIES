import { useGSAP } from '@gsap/react'
import gsap from 'gsap'
import { Flip } from 'gsap/Flip'
import { MorphSVGPlugin } from 'gsap/MorphSVGPlugin'
import { MotionPathPlugin } from 'gsap/MotionPathPlugin'
import { Physics2DPlugin } from 'gsap/Physics2DPlugin'
import { SplitText } from 'gsap/SplitText'

/**
 * GSAP : enregistré UNE seule fois ici (ADR 0004).
 * `motion` reste le moteur de l'UI ; GSAP sert aux scènes culinaires (src/components/food).
 */
gsap.registerPlugin(useGSAP, Flip, MotionPathPlugin, MorphSVGPlugin, Physics2DPlugin, SplitText)
gsap.defaults({ ease: 'power2.out', duration: 0.4 })

export { gsap, useGSAP, Flip, MotionPathPlugin, MorphSVGPlugin, Physics2DPlugin, SplitText }

export const REDUCED_MOTION = '(prefers-reduced-motion: reduce)'
export const FULL_MOTION = '(prefers-reduced-motion: no-preference)'

export function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia(REDUCED_MOTION).matches
}

/**
 * Exécute `animate` seulement si l'utilisateur accepte les animations,
 * sinon `still` (version statique). Renvoie une fonction de nettoyage.
 */
export function withMotion(animate: () => void | (() => void), still?: () => void | (() => void)): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    still?.()
    return () => undefined
  }
  const mm = gsap.matchMedia()
  mm.add({ full: FULL_MOTION, reduce: REDUCED_MOTION }, (ctx) => {
    const { full } = ctx.conditions as { full: boolean; reduce: boolean }
    return full ? animate() : still?.()
  })
  return () => mm.revert()
}
