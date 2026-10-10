import { useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { gsap, useGSAP, withMotion } from '@/lib/gsap'

const INGREDIENTS = ['🍅', '🌿', '🧀', '🌶️', '🍋', '🥬']

/**
 * Révélation du resto gagnant (fin du vote) : la carte se retourne et grandit, l'emoji
 * rebondit, une gerbe d'ingrédients éclate (Physics2D, comme VoteBurst), puis tout s'efface.
 * Calque `pointer-events: none` (≈ 2 s, n'empêche rien), décoratif : le toast de statut
 * annonce déjà le choix. Mouvement réduit → rien (onDone immédiat).
 */
/** `detail` : score du vote par classement (« 9 pts · 2 premiers choix »), facultatif. */
export function WinnerReveal({ emoji, name, detail, onDone }: { emoji: string; name: string; detail?: string; onDone?: () => void }) {
  const scope = useRef<HTMLDivElement>(null)
  const [bits] = useState(() => Array.from({ length: 14 }, (_, i) => INGREDIENTS[i % INGREDIENTS.length]!))
  useGSAP(
    () =>
      withMotion(
        () => {
          const parts = gsap.utils.toArray<HTMLElement>('[data-bit]')
          const tl = gsap.timeline({ onComplete: onDone })
          tl.fromTo('[data-veil]', { opacity: 0 }, { opacity: 1, duration: 0.2 })
            .fromTo('[data-card]', { rotateY: -100, scale: 0.6, opacity: 0 }, { rotateY: 0, scale: 1, opacity: 1, duration: 0.6, ease: 'back.out(1.6)' }, 0.05)
            .fromTo('[data-emoji]', { y: 0 }, { y: -16, duration: 0.18, yoyo: true, repeat: 3, ease: 'power2.out' }, 0.45)
            .set(parts, { opacity: 1, x: 0, y: 0, scale: 0.6 }, 0.4)
            .to(
              parts,
              {
                duration: 1,
                physics2D: { velocity: 'random(180, 320)', angle: (i: number) => (360 / parts.length) * i + gsap.utils.random(-12, 12), gravity: 420 },
                rotate: 'random(-200, 200)',
                scale: 1,
                opacity: 0,
                ease: 'none',
              },
              0.4,
            )
            .to('[data-card]', { opacity: 0, scale: 0.96, duration: 0.3, ease: 'power2.in' }, 1.75)
            .to('[data-veil]', { opacity: 0, duration: 0.3 }, '<')
        },
        () => onDone?.(),
      ),
    { scope },
  )
  if (typeof document === 'undefined') return null
  return createPortal(
    <div ref={scope} aria-hidden className="pointer-events-none fixed inset-0 z-[65] grid place-items-center px-6" style={{ perspective: 900 }}>
      <div data-veil className="absolute inset-0 bg-bg/40 opacity-0" />
      <div className="relative grid place-items-center">
        {bits.map((b, i) => (
          <span key={i} data-bit className="absolute text-2xl opacity-0">
            {b}
          </span>
        ))}
        <div data-card className="w-[min(300px,80vw)] rounded-xl border border-brand/40 bg-elevated px-6 py-7 text-center opacity-0 shadow-glow">
          <p className="text-xs font-semibold tracking-[0.14em] text-brand uppercase">Le resto gagnant</p>
          <div data-emoji className="my-3 text-6xl leading-none">
            {emoji}
          </div>
          <p className="font-display text-2xl leading-8 font-bold text-pretty">{name}</p>
          {detail && <p className="mt-1 text-sm text-muted tabular">{detail}</p>}
        </div>
      </div>
    </div>,
    document.body,
  )
}

export default WinnerReveal
