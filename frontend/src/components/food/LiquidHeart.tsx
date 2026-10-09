import { useId, useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'

const HEART = 'M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z'
// Surface du liquide : une vague plus large que le cœur, qui glisse pendant le remplissage.
const WAVE = 'M-24 3 q3 -3 6 0 t6 0 t6 0 t6 0 t6 0 t6 0 t6 0 t6 0 t6 0 t6 0 V30 H-24 Z'

/**
 * Cœur de vote qui se remplit « comme un liquide » (vague qui monte) avec un petit pop.
 * Mouvement réduit : rempli / vide immédiatement. Couleur = `currentColor`.
 */
export function LiquidHeart({ filled, className }: { filled: boolean; className?: string }) {
  const scope = useRef<SVGSVGElement>(null)
  const clip = useId().replace(/:/g, '')
  const prev = useRef(filled)
  useGSAP(
    () => {
      const changed = prev.current !== filled
      prev.current = filled
      const level = filled ? 0 : 24
      const still = () => {
        gsap.set('[data-liquid]', { y: level })
      }
      if (!changed) {
        still()
        return
      }
      return withMotion(() => {
        const tl = gsap.timeline()
        if (filled) {
          tl.fromTo('[data-liquid]', { y: 24 }, { y: 0, duration: 0.55, ease: 'power2.out' })
            .fromTo('[data-wave]', { x: 0 }, { x: 12, duration: 0.55, ease: 'none' }, 0)
            .fromTo(scope.current, { scale: 1 }, { scale: 1.22, duration: 0.14, yoyo: true, repeat: 1, ease: 'power2.out', transformOrigin: '50% 60%' }, 0.08)
        } else {
          tl.to('[data-liquid]', { y: 24, duration: 0.25, ease: 'power2.in' })
        }
      }, still)
    },
    { dependencies: [filled], scope, revertOnUpdate: false },
  )
  return (
    <svg ref={scope} viewBox="0 0 24 24" aria-hidden focusable="false" className={cn('size-6 overflow-visible', className)} fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <defs>
        <clipPath id={clip}>
          <path d={HEART} />
        </clipPath>
      </defs>
      <g clipPath={`url(#${clip})`} stroke="none">
        <g data-liquid transform={`translate(0 ${filled ? 0 : 24})`}>
          <path data-wave d={WAVE} fill="currentColor" />
        </g>
      </g>
      <path d={HEART} />
    </svg>
  )
}

export default LiquidHeart
