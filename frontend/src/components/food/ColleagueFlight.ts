import { useEffect, useRef } from 'react'
import { gsap, prefersReducedMotion } from '@/lib/gsap'

/**
 * « Panier vivant » : quand un·e collègue ajoute un plat (évènement realtime), sa vignette
 * vole depuis le menu (ou le bord de l'écran) jusqu'à son avatar dans l'en-tête de la party
 * (`[data-party-avatars] [data-avatar-user]`), qui fait un petit « bump ». Si l'avatar est hors
 * écran, la vignette rejoint le total du groupe de la barre collante (`[data-group-total]`).
 * Rafales lissées : départs espacés de ≥ 220 ms, 3 vols en l'air au plus (au-delà : bump seul).
 */
const MIN_GAP_MS = 220
const MAX_IN_FLIGHT = 3
let inFlight = 0
let nextSlot = 0

function visible(el: Element | null | undefined): el is HTMLElement {
  if (!el) return false
  const r = el.getBoundingClientRect()
  return r.width > 0 && r.bottom > 0 && r.right > 0 && r.top < window.innerHeight && r.left < window.innerWidth
}

function findTarget(userId: string): HTMLElement | null {
  const stack = document.querySelector('[data-party-avatars]')
  const esc = typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(userId) : userId.replace(/"/g, '')
  const avatar = stack?.querySelector(`[data-avatar-user="${esc}"]`) ?? stack?.querySelector('[data-avatar-more]')
  if (visible(avatar)) return avatar
  const total = document.querySelector('[data-group-total]')
  return visible(total) ? total : null
}

export function bump(el: Element) {
  gsap.fromTo(el, { scale: 1 }, { scale: 1.25, duration: 0.14, yoyo: true, repeat: 1, ease: 'power2.out', overwrite: 'auto', clearProps: 'scale,zIndex', zIndex: 2 })
}

/** Lance un vol (ou un simple bump) vers l'avatar de `userId`. Sans effet en mouvement réduit. */
export function flyToAvatar(userId: string, emoji = '🍽️', from?: Element | null): void {
  if (typeof document === 'undefined' || prefersReducedMotion()) return
  const target = findTarget(userId)
  if (!target) return
  if (inFlight >= MAX_IN_FLIGHT) {
    bump(target)
    return
  }
  const now = performance.now()
  const delay = Math.max(0, nextSlot - now) / 1000
  nextSlot = Math.max(now, nextSlot) + MIN_GAP_MS

  const b = target.getBoundingClientRect()
  const src = visible(from) ? from.getBoundingClientRect() : null
  // Depuis le plat s'il est à l'écran, sinon depuis le bord droit (40 % de la hauteur).
  const startX = src ? src.left + src.width / 2 - 16 : window.innerWidth + 8
  const startY = src ? src.top + src.height / 2 - 16 : window.innerHeight * 0.4
  const endX = b.left + b.width / 2 - 16
  const endY = b.top + b.height / 2 - 16

  const el = document.createElement('div')
  el.setAttribute('aria-hidden', 'true')
  el.textContent = emoji
  el.className = 'pointer-events-none fixed top-0 left-0 z-[60] grid size-8 place-items-center rounded-full bg-elevated text-lg shadow-card'
  el.style.opacity = '0'
  document.body.appendChild(el)
  inFlight++
  const done = () => {
    inFlight = Math.max(0, inFlight - 1)
    el.remove()
  }
  const peakY = Math.min(startY, endY) - 80
  gsap
    .timeline({ delay, onComplete: done, onInterrupt: done })
    .set(el, { x: startX, y: startY, scale: 0.5, opacity: 0 })
    .to(el, { opacity: 1, scale: 1, duration: 0.12 })
    .to(el, {
      duration: 0.75,
      ease: 'power1.inOut',
      motionPath: { path: [{ x: startX, y: startY }, { x: (startX + endX) / 2, y: peakY }, { x: endX, y: endY }], curviness: 1.3 },
      scale: 0.55,
      rotate: -160,
    })
    .to(el, { opacity: 0, duration: 0.1 }, '-=0.08')
    .add(() => bump(target), '-=0.05')
}

export interface CartLine {
  id: string
  user: string
  quantity: number
  menu_item: string
}

/**
 * Observe les lignes de panier (TanStack Query, invalidées par le realtime) et fait voler
 * les nouveaux plats des collègues. Le premier chargement ne déclenche rien ; une hausse de
 * quantité compte comme un ajout. Mes propres ajouts gardent `flyToCart`.
 */
export function useColleagueFlights<T extends CartLine>(lines: T[] | undefined, meId: string, emojiFor: (line: T) => string) {
  const known = useRef<Map<string, number> | null>(null)
  const emojiRef = useRef(emojiFor)
  useEffect(() => {
    emojiRef.current = emojiFor
  }, [emojiFor])
  useEffect(() => {
    if (!lines) return
    const prev = known.current
    known.current = new Map(lines.map((l) => [l.id, l.quantity]))
    if (!prev) return
    const added = lines.filter((l) => l.user !== meId && (prev.get(l.id) ?? 0) < l.quantity)
    for (const l of added.slice(0, MAX_IN_FLIGHT + 1)) {
      const esc = typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(l.menu_item) : l.menu_item
      flyToAvatar(l.user, emojiRef.current(l), document.querySelector(`[data-menu-item="${esc}"]`))
    }
  }, [lines, meId])
}

/** Tests : remet les compteurs de rafale à zéro. */
export function resetFlightsForTesting() {
  inFlight = 0
  nextSlot = 0
}
