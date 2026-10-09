import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'

/** Coche « prêt·e » qui se dessine (stroke-dashoffset) dans un cercle qui « pop ». */
export function DrawCheck({ className }: { className?: string }) {
  const scope = useRef<SVGSVGElement>(null)
  useGSAP(
    () =>
      withMotion(() => {
        gsap
          .timeline()
          .fromTo('[data-ring]', { scale: 0.4, opacity: 0, transformOrigin: '50% 50%' }, { scale: 1, opacity: 1, duration: 0.22, ease: 'back.out(2.5)' })
          .fromTo('[data-tick]', { strokeDashoffset: 16 }, { strokeDashoffset: 0, duration: 0.28, ease: 'power2.out' }, '-=0.05')
      }),
    { scope },
  )
  return (
    <svg ref={scope} viewBox="0 0 24 24" aria-hidden focusable="false" className={cn('size-5', className)} fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <circle data-ring cx="12" cy="12" r="10" />
      <path data-tick d="m8 12.5 2.6 2.6L16 9.6" strokeDasharray="16" strokeDashoffset="0" />
    </svg>
  )
}

export default DrawCheck
