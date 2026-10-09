import { useRef } from 'react'
import { cn } from '@/lib/cn'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'
import { Basil, Chili, Tomato } from './illustrations'

/** « En attente des collègues… » : trois ingrédients qui sautillent doucement (boucle lente). */
export function WaitingDots({ label, className }: { label: string; className?: string }) {
  const scope = useRef<HTMLParagraphElement>(null)
  useGSAP(
    () =>
      withMotion(() => {
        gsap.to('[data-hop]', { y: -5, duration: 0.42, ease: 'sine.inOut', repeat: -1, yoyo: true, stagger: { each: 0.16, repeat: -1, yoyo: true } })
      }),
    { scope },
  )
  return (
    <p ref={scope} className={cn('flex items-center gap-2 text-sm text-muted', className)}>
      <span aria-hidden className="inline-flex items-end gap-1">
        <Tomato data-hop className="size-4" />
        <Basil data-hop className="size-4" />
        <Chili data-hop className="size-4" />
      </span>
      <span>{label}</span>
    </p>
  )
}

export default WaitingDots
