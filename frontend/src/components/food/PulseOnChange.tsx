import { useRef, type ReactNode } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'

/**
 * Petit « pouls » (scale 1 → 1,14 → 1, 320 ms) quand `value` change — compteurs live
 * « 3/5 prêts », « 2/4 ont voté ». Rien au premier rendu ; mouvement réduit → rien.
 */
export function PulseOnChange({ value, children, className }: { value: string | number; children: ReactNode; className?: string }) {
  const ref = useRef<HTMLSpanElement>(null)
  const prev = useRef(value)
  useGSAP(
    () => {
      if (prev.current === value) return
      prev.current = value
      return withMotion(() => {
        gsap.fromTo(ref.current, { scale: 1 }, { scale: 1.14, duration: 0.16, yoyo: true, repeat: 1, ease: 'power2.out', clearProps: 'scale' })
      })
    },
    { dependencies: [value], scope: ref, revertOnUpdate: true },
  )
  return (
    <span ref={ref} className={cn('inline-flex will-change-transform', className)}>
      {children}
    </span>
  )
}

export default PulseOnChange
