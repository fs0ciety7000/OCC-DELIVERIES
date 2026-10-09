import { useRef } from 'react'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'

const DEFAULT = ['🍅', '🌿', '🧀', '🌶️', '🍕']

/** Le cœur « explose » en mini-ingrédients (Physics2D). Rejoue à chaque changement de `trigger` (> 0). */
export function VoteBurst({ trigger, emojis = DEFAULT }: { trigger: number; emojis?: string[] }) {
  const scope = useRef<HTMLSpanElement>(null)
  useGSAP(
    () => {
      if (trigger <= 0) return
      return withMotion(() => {
        const parts = gsap.utils.toArray<HTMLElement>('[data-particle]')
        gsap.set(parts, { x: 0, y: 0, opacity: 1, scale: 0.6, rotate: 0 })
        gsap.to(parts, {
          duration: 0.9,
          physics2D: { velocity: 'random(120, 220)', angle: (i: number) => -90 + (i - (parts.length - 1) / 2) * 28 + gsap.utils.random(-10, 10), gravity: 520 },
          rotate: 'random(-180, 180)',
          scale: 1,
          opacity: 0,
          ease: 'none',
        })
      })
    },
    { scope, dependencies: [trigger], revertOnUpdate: true },
  )
  return (
    <span ref={scope} aria-hidden className="pointer-events-none absolute inset-0 grid place-items-center overflow-visible">
      {trigger > 0 &&
        Array.from({ length: 7 }, (_, i) => (
          <span key={i} data-particle className="absolute text-base opacity-0">
            {emojis[i % emojis.length]}
          </span>
        ))}
    </span>
  )
}

export default VoteBurst
