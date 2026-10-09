import { useRef, useState } from 'react'
import { cn } from '@/lib/cn'
import { gsap, prefersReducedMotion, useGSAP, withMotion } from '@/lib/gsap'
import { Scooter } from './illustrations'

export interface DispatchedSceneProps {
  /** « Commande envoyée via Uber Eats ». */
  headline: string
  /** « arrivée estimée ~35 min » — omis si le resto n'a pas de délai connu. */
  eta?: string | null
  /** Jouer la traversée (première fois) ; sinon carte statique. */
  play: boolean
  /** Retarde le départ (ex. pendant qu'une sheet est ouverte). */
  paused?: boolean
  className?: string
}

/**
 * « Commande envoyée » : un scooter traverse l'écran sur une route (lignes de vitesse,
 * marquage qui défile), repasse et se gare ; puis le texte apparaît. ≈ 2 s, passable.
 * `play=false` ou mouvement réduit → carte statique (scooter garé, texte visible).
 */
export function DispatchedScene({ headline, eta, play, paused = false, className }: DispatchedSceneProps) {
  const scope = useRef<HTMLDivElement>(null)
  const tl = useRef<gsap.core.Timeline | null>(null)
  const [playing, setPlaying] = useState(() => play && !prefersReducedMotion())

  useGSAP(
    () => {
      const road = scope.current?.querySelector<HTMLElement>('[data-road]')
      const width = road?.clientWidth ?? 320
      const park = Math.max(0, width - 104)
      const still = () => {
        gsap.set('[data-rider]', { x: park })
        gsap.set('[data-speed]', { opacity: 0 })
        gsap.set('[data-copy]', { opacity: 1, y: 0 })
      }
      if (!play) {
        still()
        return
      }
      return withMotion(
        () => {
          const t = gsap.timeline({ paused: true, onComplete: () => setPlaying(false) })
          t.set('[data-copy]', { opacity: 0, y: 6 })
            .set('[data-speed]', { opacity: 1 })
            .fromTo('[data-rider]', { x: -96 }, { x: width + 24, duration: 1.05, ease: 'power1.in' })
            .fromTo('[data-lane]', { x: 0 }, { x: -48, duration: 1.05, ease: 'none' }, 0)
            .to('[data-bob]', { y: -2, duration: 0.09, yoyo: true, repeat: 11, ease: 'sine.inOut' }, 0)
            .fromTo('[data-speed] line', { scaleX: 0.4, transformOrigin: '100% 50%' }, { scaleX: 1, duration: 0.18, yoyo: true, repeat: 5, stagger: 0.05 }, 0)
            .set('[data-rider]', { x: -96 })
            .to('[data-rider]', { x: park, duration: 0.6, ease: 'back.out(1.3)' })
            .to('[data-speed]', { opacity: 0, duration: 0.2 }, '-=0.25')
            .to('[data-copy]', { opacity: 1, y: 0, duration: 0.3, ease: 'power2.out' }, '-=0.2')
          tl.current = t
          return () => {
            tl.current = null
          }
        },
        still,
      )
    },
    { scope, dependencies: [play] },
  )

  // Départ différé tant que `paused` (sheet d'envoi encore ouverte côté hôte).
  useGSAP(
    () => {
      if (!paused) tl.current?.play()
    },
    { dependencies: [paused, play] },
  )

  return (
    <div ref={scope} className={cn('relative overflow-hidden rounded-lg border border-success/30 bg-success/[0.06]', className)} role="status" tabIndex={-1}>
      <div data-road aria-hidden className="relative h-20 overflow-hidden">
        {/* chaussée + marquage central qui défile */}
        <div className="absolute inset-x-0 bottom-3 h-7 bg-fg/[0.06]" />
        <div data-lane className="absolute -right-12 bottom-[25px] left-0 h-[3px] bg-[repeating-linear-gradient(90deg,var(--color-border-strong)_0_16px,transparent_16px_32px)]" />
        <div data-rider className="absolute bottom-3 left-0 w-24 will-change-transform">
          <svg data-speed viewBox="0 0 40 24" className="absolute top-5 -left-6 h-6 w-10 opacity-0" strokeLinecap="round">
            <line x1="4" y1="5" x2="36" y2="5" className="stroke-muted" strokeWidth={2} />
            <line x1="12" y1="12" x2="38" y2="12" className="stroke-brand" strokeWidth={2} />
            <line x1="8" y1="19" x2="34" y2="19" className="stroke-muted" strokeWidth={2} />
          </svg>
          <div data-bob className="ml-6 size-16">
            <Scooter width={64} height={64} />
          </div>
        </div>
      </div>
      <div data-copy className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-4 pb-3.5">
        <p className="font-semibold text-fg">
          {headline}
          {eta && (
            <>
              <span aria-hidden className="hidden font-normal text-muted sm:inline"> · </span>
              <span className="sr-only sm:hidden">, </span>
              <span className="block text-sm font-normal text-muted sm:inline sm:text-base">{eta}</span>
            </>
          )}
        </p>
      </div>
      {playing && (
        <button
          type="button"
          onClick={() => {
            tl.current?.progress(1)
            // Le bouton disparaît : on garde le focus dans la carte.
            scope.current?.focus({ preventScroll: true })
          }}
          className="absolute top-1 right-1 min-h-9 rounded-sm px-2.5 text-sm font-medium text-muted underline-offset-4 hover:text-fg hover:underline"
        >
          Passer l'animation
        </button>
      )}
    </div>
  )
}

export default DispatchedScene
