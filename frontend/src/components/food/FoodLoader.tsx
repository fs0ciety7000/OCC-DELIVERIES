import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { Pizza, RamenBowl } from './illustrations'

export interface FoodLoaderProps {
  label?: string
  variant?: 'pizza' | 'ramen'
  className?: string
  size?: number
}

/** Indicateur de chargement principal : parts de pizza qui se découpent puis se recomposent (ou bol de ramen fumant). */
export function FoodLoader({ label = 'Chargement…', variant = 'pizza', className, size = 72 }: FoodLoaderProps) {
  const scope = useRef<HTMLDivElement>(null)
  useGSAP(
    () =>
      withMotion(() => {
        if (variant === 'pizza') {
          const slices = gsap.utils.toArray<SVGGElement>('[data-slice]')
          const tl = gsap.timeline({ repeat: -1, repeatDelay: 0.15 })
          tl.to(slices, {
            x: (_i, el: Element) => 6 * Math.cos(((Number(el.getAttribute('data-slice')) - 60) * Math.PI) / 180),
            y: (_i, el: Element) => 6 * Math.sin(((Number(el.getAttribute('data-slice')) - 60) * Math.PI) / 180),
            duration: 0.45,
            ease: 'back.out(2)',
            stagger: 0.05,
          })
            .to(slices, { x: 0, y: 0, duration: 0.4, ease: 'power3.inOut', stagger: 0.04 }, '+=0.2')
            .to('svg', { rotate: '+=60', duration: 0.5, ease: 'power2.inOut' }, '<')
        } else {
          gsap.to('[data-steam]', {
            y: -4,
            opacity: 0.2,
            duration: 0.9,
            ease: 'sine.inOut',
            stagger: { each: 0.25, repeat: -1, yoyo: true },
          })
        }
      }),
    { scope, dependencies: [variant] },
  )
  return (
    <div ref={scope} role="status" aria-live="polite" className={cn('flex flex-col items-center gap-3 text-muted', className)}>
      {variant === 'pizza' ? <Pizza width={size} height={size} /> : <RamenBowl width={size} height={size} />}
      <span className="text-sm font-medium">{label}</span>
    </div>
  )
}

export default FoodLoader
