import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { Scooter } from './illustrations'

const ROUTE = 'M16 70 C 70 70, 70 24, 130 30 S 210 70, 284 34'

/** Scooter de livraison qui parcourt une route pointillée jusqu'au bureau. */
export function DispatchScooter({ className }: { className?: string }) {
  const scope = useRef<HTMLDivElement>(null)
  useGSAP(
    () =>
      withMotion(
        () => {
          const path = scope.current?.querySelector<SVGPathElement>('[data-route]')
          const len = path?.getTotalLength?.() ?? 320
          gsap.fromTo('[data-route]', { strokeDashoffset: len }, { strokeDashoffset: 0, duration: 1.4, ease: 'power1.inOut' })
          gsap.fromTo('[data-office]', { scale: 0.6, opacity: 0, transformOrigin: '50% 100%' }, { scale: 1, opacity: 1, duration: 0.4, delay: 1.1, ease: 'back.out(2)' })
          gsap.to('[data-scooter]', {
            duration: 1.4,
            ease: 'power1.inOut',
            motionPath: { path: '[data-route]', align: '[data-route]', alignOrigin: [0.5, 0.85], autoRotate: true },
          })
        },
        () => {
          gsap.set('[data-route]', { strokeDashoffset: 0 })
          gsap.set('[data-scooter]', { motionPath: { path: '[data-route]', align: '[data-route]', alignOrigin: [0.5, 0.85], start: 1, end: 1 } })
        },
      ),
    { scope },
  )
  return (
    <div ref={scope} aria-hidden className={cn('pointer-events-none relative h-24 w-full overflow-visible', className)}>
      <svg viewBox="0 0 300 90" className="absolute inset-0 size-full overflow-visible">
        <path d={ROUTE} fill="none" className="stroke-border-strong" strokeWidth={3} strokeLinecap="round" strokeDasharray="2 9" />
        <path data-route d={ROUTE} fill="none" className="stroke-brand" strokeWidth={3} strokeLinecap="round" strokeDasharray="1000" />
        <g data-office transform="translate(272 6)">
          <rect x="0" y="6" width="22" height="24" rx="2" className="fill-elevated stroke-fg/40" strokeWidth={1.5} />
          <path d="M5 12h4M13 12h4M5 18h4M13 18h4" className="stroke-brand-2" strokeWidth={2} />
          <rect x="8" y="23" width="6" height="7" className="fill-brand" />
        </g>
        <g data-scooter>
          <Scooter x={-18} y={-30} width={36} height={36} />
        </g>
      </svg>
    </div>
  )
}

export default DispatchScooter
