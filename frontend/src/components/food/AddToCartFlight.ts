import { gsap, prefersReducedMotion } from '@/lib/gsap'

/** Cible : le bouton « Mon panier » porte `data-cart-target`. */
const TARGET = '[data-cart-target]'

/**
 * La vignette du plat « vole » en arc jusqu'au sac de commande, qui se gonfle.
 * Calque `pointer-events: none`, interruptible (une nouvelle animation n'attend pas la précédente).
 */
export function flyToCart(from: Element | null | undefined, emoji = '🍽️') {
  const target = document.querySelector<HTMLElement>(TARGET)
  if (!target) return
  const puff = () => gsap.fromTo(target, { scale: 1 }, { scale: 1.08, duration: 0.14, yoyo: true, repeat: 1, ease: 'power2.out', overwrite: 'auto', clearProps: 'scale' })
  if (!from || prefersReducedMotion()) {
    if (!prefersReducedMotion()) puff()
    return
  }
  const a = from.getBoundingClientRect()
  const b = target.getBoundingClientRect()
  const startX = a.left + a.width / 2 - 20
  const startY = a.top + a.height / 2 - 20
  const endX = b.left + Math.min(b.width, 56) / 2 - 20
  const endY = b.top + b.height / 2 - 20

  const el = document.createElement('div')
  el.setAttribute('aria-hidden', 'true')
  el.textContent = emoji
  el.className = 'pointer-events-none fixed top-0 left-0 z-[60] grid size-10 place-items-center rounded-full bg-elevated text-2xl shadow-glow'
  document.body.appendChild(el)

  const peakY = Math.min(startY, endY) - 120
  gsap
    .timeline({ onComplete: () => el.remove(), onInterrupt: () => el.remove() })
    .set(el, { x: startX, y: startY, scale: 0.6, opacity: 0 })
    .to(el, { opacity: 1, scale: 1.1, duration: 0.12 })
    .to(el, {
      duration: 0.7,
      ease: 'power1.inOut',
      motionPath: { path: [{ x: startX, y: startY }, { x: (startX + endX) / 2, y: peakY }, { x: endX, y: endY }], curviness: 1.4 },
      scale: 0.45,
      rotate: 200,
    })
    .to(el, { opacity: 0, duration: 0.1 }, '-=0.08')
    .add(puff, '-=0.05')
}
