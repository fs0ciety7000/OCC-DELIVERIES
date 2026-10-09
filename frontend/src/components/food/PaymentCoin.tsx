import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { Coin, PiggyBurger } from './illustrations'

/** Pièce qui tombe dans la tirelire-burger (paiement confirmé). Rejoue quand `trigger` change. */
export function PaymentCoin({ trigger = 1, className, size = 88 }: { trigger?: number; className?: string; size?: number }) {
  const scope = useRef<HTMLDivElement>(null)
  useGSAP(
    () =>
      withMotion(() => {
        const tl = gsap.timeline()
        tl.fromTo('[data-coin]', { y: -size * 0.9, opacity: 0, rotateY: 0 }, { opacity: 1, duration: 0.1 })
          .to('[data-coin]', { y: size * 0.12, rotateY: 540, duration: 0.6, ease: 'power2.in' })
          .set('[data-coin]', { opacity: 0 })
          .fromTo('[data-piggy]', { scaleY: 1, scaleX: 1 }, { scaleY: 0.9, scaleX: 1.06, duration: 0.1, yoyo: true, repeat: 1, transformOrigin: '50% 100%', ease: 'power1.out' })
          .fromTo('[data-sparkle]', { scale: 0, opacity: 1 }, { scale: 1.4, opacity: 0, duration: 0.5, stagger: 0.05, ease: 'power2.out' }, '<')
      }),
    { scope, dependencies: [trigger], revertOnUpdate: true },
  )
  return (
    <div ref={scope} aria-hidden className={cn('pointer-events-none relative mx-auto', className)} style={{ width: size, height: size }}>
      <div data-coin className="absolute top-0 left-1/2 -ml-3 size-6 opacity-0" style={{ perspective: 200 }}>
        <Coin className="size-6" />
      </div>
      <div data-piggy className="absolute inset-x-0 bottom-0">
        <PiggyBurger width={size} height={size} />
      </div>
      {[-1, 0, 1].map((d) => (
        <span key={d} data-sparkle className="absolute top-[18%] size-2 rounded-full bg-food-gold opacity-0" style={{ left: `calc(50% + ${d * 16}px)` }} />
      ))}
    </div>
  )
}

export default PaymentCoin
