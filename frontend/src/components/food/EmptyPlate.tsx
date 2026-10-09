import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { Plate } from './illustrations'

/** Assiette vide, fourchette qui tapote doucement (boucle lente). */
export function EmptyPlate({ className, width = 120 }: { className?: string; width?: number }) {
  const scope = useRef<HTMLDivElement>(null)
  useGSAP(
    () =>
      withMotion(() => {
        const tl = gsap.timeline({ repeat: -1, repeatDelay: 1.6 })
        tl.to('[data-fork]', { rotate: -12, duration: 0.18, ease: 'power1.out' })
          .to('[data-fork]', { rotate: 0, duration: 0.22, ease: 'bounce.out' })
          .to('[data-fork]', { rotate: -10, duration: 0.16 })
          .to('[data-fork]', { rotate: 0, duration: 0.24, ease: 'bounce.out' })
        gsap.to('svg', { y: -3, duration: 2.4, ease: 'sine.inOut', repeat: -1, yoyo: true })
      }),
    { scope },
  )
  return (
    <div ref={scope} className={cn('pointer-events-none', className)} aria-hidden>
      <Plate width={width} height={(width * 64) / 96} />
    </div>
  )
}

export default EmptyPlate
