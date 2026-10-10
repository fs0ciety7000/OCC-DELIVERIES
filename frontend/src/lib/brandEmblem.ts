import { CustomEase } from 'gsap/CustomEase'
import type { EmblemName } from '@/components/brand/emblemLoader'
import { gsap, prefersReducedMotion } from './gsap'

/**
 * Emblèmes animés de la charte CARDOR Media (repris de cardormedia.com, `emblem-motion.ts`,
 * docs/brand/emblem-motion.md du dépôt mons-corp). Chargé à la demande par `BrandEmblem` :
 * GSAP n'entre pas dans le bundle initial du shell.
 *
 * Les parties animées sont les `<g class="part">` posés par `emblemLoader` : GSAP n'écrase
 * jamais la position d'origine (portée par le `<g transform>` parent).
 */
// Courbes de la charte (motion tokens CARDOR).
gsap.registerPlugin(CustomEase)
CustomEase.create('cardor.out', 'M0,0 C0.16,1 0.3,1 1,1')
CustomEase.create('cardor.inOut', 'M0,0 C0.77,0 0.175,1 1,1')
CustomEase.create('cardor.snap', 'M0,0 C0.87,0 0.13,1 1,1')

/** Signature de l'emblème : `build()` crée une timeline neuve à chaque lecture. */
function signature(name: EmblemName, root: SVGGElement, vb: number[]): () => gsap.core.Timeline {
  const vw = vb[2] ?? 100
  if (name === 'cardor-monogram') {
    // La roue fait un tour complet (long amorti : les 14 rayons ne « reculent » jamais), le noyau bat.
    const core = root.querySelectorAll('.t3 .part')
    return () =>
      gsap
        .timeline()
        .to(root, { rotation: '+=360', duration: 1.6, ease: 'cardor.out' }, 0)
        .to(core, { scale: 1.6, duration: 0.2, ease: 'cardor.out' }, 0.15)
        .to(core, { scale: 1, duration: 0.9, ease: 'elastic.out(1, 0.45)' }, 0.35)
  }
  // Dragon : glitch — les facettes sautent au pixel par paliers, un fantôme RVB clignote, puis tout se recale.
  const t2 = root.querySelector<SVGGElement>('.t2')
  const facets = [...root.querySelectorAll<SVGGElement>('.t2 .part')]
  const ghost = t2?.cloneNode(true) as SVGGElement | undefined
  if (ghost && t2) {
    ghost.removeAttribute('class')
    // Magenta du glitch : constante de la charte (fantôme RVB), comme les couleurs portées par le SVG.
    ghost.setAttribute('fill', '#ff2bd6')
    ghost.setAttribute('style', 'mix-blend-mode:screen;opacity:0')
    ghost.setAttribute('aria-hidden', 'true')
    t2.after(ghost)
  }
  const unit = () => vw / (root.ownerSVGElement?.getBoundingClientRect().width || vw)
  return () => {
    const u = unit()
    const jump = () => gsap.utils.snap(u, gsap.utils.random(-1, 1) * Math.max(0.05 * vw, 6 * u))
    return gsap
      .timeline()
      .to(root, { skewX: -10, duration: 0.24, ease: 'steps(3)', yoyo: true, repeat: 1 }, 0)
      .to(facets, { x: jump, y: jump, duration: 0.06, ease: 'none', stagger: { each: 0.01, from: 'random' }, repeat: 4, repeatRefresh: true }, 0)
      .to(
        ghost ?? [],
        {
          keyframes: [
            { opacity: 0.7, x: 4 * u },
            { opacity: 0, x: -3 * u },
            { opacity: 0.55, x: 2 * u },
            { opacity: 0, x: 0 },
          ],
          duration: 0.36,
          ease: 'steps(1)',
        },
        0.04,
      )
      .to(facets, { x: 0, y: 0, duration: 0.18, ease: 'cardor.snap' })
  }
}

/** Révélation au premier affichage : « boot » glitch pour le Dragon, rotation d'entrée pour la roue. */
function reveal(name: EmblemName, root: SVGGElement): gsap.core.Timeline {
  const tl = gsap.timeline({ paused: true })
  if (name === 'interactive') {
    const parts = [...root.querySelectorAll<SVGGElement>('.part')]
    gsap.set(parts, { autoAlpha: 0 })
    return tl
      .to(parts, { autoAlpha: 1, duration: 0.01, ease: 'none', stagger: { each: 0.025, from: 'random' } })
      .fromTo(root, { skewX: -12 }, { skewX: 0, duration: 0.5, ease: 'steps(5)' }, 0)
  }
  return tl.fromTo(root, { rotation: -180, autoAlpha: 0 }, { rotation: 0, autoAlpha: 1, duration: 1.2, ease: 'cardor.out' })
}

/**
 * Branche le mouvement : révélation la première fois que l'emblème est visible (repli à 4 s),
 * puis signature au survol / focus de `host`. Rien si les animations sont réduites.
 */
export function attachEmblemMotion(name: EmblemName, svg: SVGSVGElement, host: HTMLElement | null): () => void {
  const root = svg.querySelector<SVGGElement>('g[data-root]')
  if (!root || prefersReducedMotion()) return () => undefined
  // Origine fixée une seule fois (centre du signe, en unités du viewBox) et sans « smoothOrigin » :
  // deux tweens aux origines calculées différemment faisaient glisser l'emblème hors de sa boîte.
  const [vx = 0, vy = 0, vw = 100, vh = 100] = (svg.getAttribute('viewBox') ?? '0 0 100 100').split(' ').map(Number)
  gsap.set(root, { svgOrigin: `${vx + vw / 2} ${vy + vh / 2}`, smoothOrigin: false })
  root.querySelectorAll<SVGGElement>('.t3 .part').forEach((core) => gsap.set(core, { svgOrigin: `${vx + vw / 2} ${vy + vh / 2}`, smoothOrigin: false }))
  const build = signature(name, root, (svg.getAttribute('viewBox') ?? '0 0 100 100').split(' ').map(Number))
  let current: gsap.core.Timeline | null = null
  let revealing = true
  const intro = reveal(name, root)
  const start = () => {
    io.disconnect()
    clearTimeout(fallback)
    void intro.play().then(() => {
      revealing = false
    })
  }
  const io = new IntersectionObserver(([e]) => e?.isIntersecting && start(), { threshold: 0.01 })
  io.observe(svg)
  const fallback = setTimeout(() => {
    if (svg.getBoundingClientRect().top < innerHeight) start()
  }, 4000)
  const play = () => {
    if (revealing || current?.isActive() || document.hidden) return
    current?.kill()
    current = build()
  }
  host?.addEventListener('pointerenter', play)
  host?.addEventListener('focusin', play)
  return () => {
    io.disconnect()
    clearTimeout(fallback)
    intro.kill()
    current?.kill()
    host?.removeEventListener('pointerenter', play)
    host?.removeEventListener('focusin', play)
  }
}
