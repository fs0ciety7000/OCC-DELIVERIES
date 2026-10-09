import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { DeliveryBag } from './illustrations'

/** Panier vide : sac de livraison qui se balance, une miette qui tombe (boucle lente). */
export function EmptyBag({ className, size = 96 }: { className?: string; size?: number }) {
  const scope = useRef<HTMLDivElement>(null)
  useGSAP(
    () =>
      withMotion(() => {
        gsap.fromTo('[data-bag]', { rotate: -4 }, { rotate: 4, duration: 1.6, ease: 'sine.inOut', repeat: -1, yoyo: true, transformOrigin: '50% 100%' })
        gsap.fromTo('[data-crumb]', { y: -6, opacity: 0 }, { y: 18, opacity: 1, duration: 1.1, ease: 'power1.in', repeat: -1, repeatDelay: 1.4 })
      }),
    { scope },
  )
  return (
    <div ref={scope} aria-hidden className={cn('pointer-events-none relative mx-auto', className)} style={{ width: size, height: size }}>
      <div data-bag className="size-full">
        <DeliveryBag width={size} height={size} />
      </div>
      <span data-crumb className="absolute top-[10%] right-[18%] size-1.5 rounded-full bg-food-crust opacity-0" />
    </div>
  )
}

export default EmptyBag
