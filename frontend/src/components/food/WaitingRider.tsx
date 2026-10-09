import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { Scooter } from './illustrations'

const ink = 'stroke-food-ink'

/**
 * Livreur qui attend à côté de son scooter : regarde sa montre, tapote du pied,
 * petite bulle « … ». Boucle lente (≈ 3 s), transform/opacité uniquement.
 */
export function WaitingRider({ className }: { className?: string }) {
  const scope = useRef<HTMLDivElement>(null)
  useGSAP(
    () =>
      withMotion(() => {
        const tl = gsap.timeline({ repeat: -1, repeatDelay: 0.9, defaults: { ease: 'power2.inOut' } })
        tl.to('[data-arm]', { rotate: -150, svgOrigin: '89 39', duration: 0.38 })
          .to('[data-head]', { rotate: 12, svgOrigin: '81 31', duration: 0.3 }, '<0.08')
          .fromTo('[data-dot]', { opacity: 0, scale: 0.4, transformOrigin: '50% 50%' }, { opacity: 1, scale: 1, duration: 0.18, stagger: 0.16, ease: 'back.out(3)' }, '+=0.1')
          .to('[data-arm]', { rotate: 0, svgOrigin: '89 39', duration: 0.36 }, '+=0.45')
          .to('[data-head]', { rotate: 0, svgOrigin: '81 31', duration: 0.3 }, '<')
          .to('[data-dot]', { opacity: 0, duration: 0.2 }, '<')
          .to('[data-foot]', { rotate: -16, svgOrigin: '85 58', duration: 0.12, yoyo: true, repeat: 3, ease: 'sine.inOut' })
        gsap.to('[data-idle]', { y: -0.8, duration: 1.2, ease: 'sine.inOut', repeat: -1, yoyo: true })
      }),
    { scope },
  )
  return (
    <div ref={scope} aria-hidden className={cn('pointer-events-none', className)}>
      <svg viewBox="0 0 120 80" className="size-full overflow-visible" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
        <path d="M2 74h116" className="stroke-border-strong" strokeDasharray="2 6" />
        <Scooter x={2} y={22} width={54} height={54} />
        <g data-idle>
          {/* jambes */}
          <path d="M78 58 76 73" className={ink} strokeWidth={3} />
          <g data-foot>
            <path d="M85 58 87 73h4" fill="none" className={ink} strokeWidth={3} />
          </g>
          {/* corps (veste de livreur) */}
          <rect x="72" y="35" width="18" height="25" rx="6" className={`fill-food-crust ${ink}`} />
          <path d="M74 44h14" className="stroke-food-cheese" />
          {/* bras gauche */}
          <path d="M73 40 69 55" className={ink} strokeWidth={3} />
          {/* bras droit + montre */}
          <g data-arm>
            <path d="M89 39 95 53" className={ink} strokeWidth={3} />
            <circle cx="95.5" cy="53.5" r="2.4" className={`fill-food-gold ${ink}`} strokeWidth={1.2} />
          </g>
          {/* tête + casque */}
          <g data-head>
            <circle cx="81" cy="26" r="7" className={`fill-food-bun ${ink}`} />
            <path d="M73.6 25a7.4 7.4 0 0 1 14.8 0Z" className={`fill-food-tomato ${ink}`} />
            <circle cx="83.5" cy="27.5" r="0.9" className="fill-food-ink" stroke="none" />
          </g>
        </g>
        {/* bulle d'attente */}
        <g>
          <circle data-dot cx="99" cy="14" r="2" className="fill-muted" stroke="none" opacity="0" />
          <circle data-dot cx="106" cy="14" r="2" className="fill-muted" stroke="none" opacity="0" />
          <circle data-dot cx="113" cy="14" r="2" className="fill-muted" stroke="none" opacity="0" />
        </g>
      </svg>
    </div>
  )
}

export default WaitingRider
