import { useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'

const EMOJIS = ['🍕', '🍣', '🍔', '🥟', '🌮', '🍜', '🍟', '🥗']

/** Pluie d'emojis avec gravité (~1,5 s), une seule fois, sur un calque `pointer-events: none`. */
export function FoodRain({ count = 28, onDone }: { count?: number; onDone?: () => void }) {
  const scope = useRef<HTMLDivElement>(null)
  const [items] = useState(() => Array.from({ length: count }, (_, i) => ({ id: i, emoji: EMOJIS[i % EMOJIS.length], left: Math.random() * 100 })))
  useGSAP(
    () =>
      withMotion(
        () => {
          const h = window.innerHeight
          gsap.utils.toArray<HTMLElement>('[data-drop]').forEach((el, i) => {
            gsap.fromTo(
              el,
              { y: -40, opacity: 1, rotate: 0 },
              {
                duration: 1.5 + Math.random() * 0.4,
                delay: i * 0.025,
                physics2D: { velocity: gsap.utils.random(80, 260), angle: gsap.utils.random(60, 120), gravity: h * 0.9 },
                rotate: gsap.utils.random(-240, 240),
                ease: 'none',
                onComplete: i === items.length - 1 ? onDone : undefined,
              },
            )
            gsap.to(el, { opacity: 0, duration: 0.3, delay: 1.4 + i * 0.025 })
          })
        },
        () => onDone?.(),
      ),
    { scope },
  )
  if (typeof document === 'undefined') return null
  return createPortal(
    <div ref={scope} aria-hidden className="pointer-events-none fixed inset-0 z-[70] overflow-hidden">
      {items.map((it) => (
        <span key={it.id} data-drop className="absolute top-0 text-3xl opacity-0" style={{ left: `${it.left}%` }}>
          {it.emoji}
        </span>
      ))}
    </div>,
    document.body,
  )
}

export default FoodRain
