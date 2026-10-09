import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { formatMoney } from '@/lib/format'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { counterValue } from './counter'

/**
 * Montant qui « compte » jusqu'à sa nouvelle valeur (GSAP, `snap` au centime, `tabular-nums`).
 * Le texte animé est décoratif (`aria-hidden`) ; la valeur finale est annoncée en `sr-only`.
 * Interruptible : une nouvelle valeur repart du chiffre affiché. Mouvement réduit → saut direct.
 */
export function AnimatedMoney({ cents, className, duration = 0.6 }: { cents: number; className?: string; duration?: number }) {
  const ref = useRef<HTMLSpanElement>(null)
  const shown = useRef(cents)
  useGSAP(
    () => {
      const el = ref.current
      if (!el) return
      const from = shown.current
      const still = () => {
        shown.current = cents
        el.textContent = formatMoney(cents)
      }
      if (from === cents) {
        still()
        return
      }
      return withMotion(() => {
        // Progression 0 → 1, valeur arrondie au centime à chaque image (équivalent d'un `snap` 1).
        const o = { p: 0 }
        gsap.to(o, {
          p: 1,
          duration,
          ease: 'power2.out',
          onUpdate: () => {
            const v = counterValue(from, cents, o.p)
            if (v === shown.current && el.textContent) return
            shown.current = v
            el.textContent = formatMoney(v)
          },
          onComplete: still,
        })
      }, still)
    },
    { dependencies: [cents], scope: ref },
  )
  return (
    <span className={cn('tabular whitespace-nowrap', className)}>
      <span ref={ref} aria-hidden data-counter />
      <span className="sr-only">{formatMoney(cents)}</span>
    </span>
  )
}

export default AnimatedMoney
