import { useRef, type ReactNode } from 'react'
import { cn } from '@/lib/cn'
import { gsap, SplitText, useGSAP, withMotion } from '@/lib/gsap'
import { Basil, Chili, Fries, Sushi, Tomato } from './illustrations'

const FLOATERS = [
  { C: Tomato, cls: 'left-[2%] top-[8%] size-12 sm:size-16', depth: 1.2 },
  { C: Basil, cls: 'right-[4%] top-[2%] size-10 sm:size-14', depth: 0.8 },
  { C: Chili, cls: 'left-[10%] bottom-[4%] size-10 sm:size-12', depth: 1.5 },
  { C: Sushi, cls: 'right-[10%] bottom-[10%] size-12 sm:size-16', depth: 1 },
  { C: Fries, cls: 'right-[34%] -top-[6%] size-9 sm:size-12 hidden sm:block', depth: 0.6 },
  { C: Basil, cls: 'left-[38%] -bottom-[6%] size-8 sm:size-10 rotate-90 hidden sm:block', depth: 1.8 },
] as const

/** Héros d'accueil : ingrédients flottants en parallaxe + titre révélé lettre par lettre. */
export function FoodHero({ title, children, className }: { title: string; children?: ReactNode; className?: string }) {
  const scope = useRef<HTMLDivElement>(null)
  useGSAP(
    () =>
      withMotion(() => {
        const split = SplitText.create('[data-hero-title]', { type: 'chars,words', aria: 'auto' })
        gsap.from(split.chars, { yPercent: 60, opacity: 0, rotate: 6, duration: 0.5, ease: 'back.out(1.8)', stagger: 0.025 })
        const floaters = gsap.utils.toArray<HTMLElement>('[data-floater]')
        floaters.forEach((el, i) => {
          gsap.fromTo(el, { opacity: 0, scale: 0.6 }, { opacity: 1, scale: 1, duration: 0.6, delay: 0.2 + i * 0.08, ease: 'back.out(2)' })
          gsap.to(el, { y: i % 2 ? 10 : -12, rotate: i % 2 ? 8 : -8, duration: 3 + i * 0.4, ease: 'sine.inOut', repeat: -1, yoyo: true })
        })
        // Parallaxe au pointeur (desktop uniquement).
        const root = scope.current
        if (!root || !window.matchMedia('(pointer: fine)').matches) return () => split.revert()
        const movers = floaters.map((el) => ({
          x: gsap.quickTo(el, 'x', { duration: 0.8, ease: 'power3.out' }),
          depth: Number(el.dataset.depth) || 1,
        }))
        const onMove = (e: PointerEvent) => {
          const r = root.getBoundingClientRect()
          const dx = (e.clientX - r.left) / r.width - 0.5
          movers.forEach((m) => m.x(dx * 24 * m.depth))
        }
        root.addEventListener('pointermove', onMove)
        return () => {
          root.removeEventListener('pointermove', onMove)
          split.revert()
        }
      }),
    { scope },
  )
  return (
    <div ref={scope} className={cn('relative isolate', className)}>
      <div aria-hidden className="pointer-events-none absolute -inset-x-2 -inset-y-6 -z-10">
        {FLOATERS.map(({ C, cls, depth }, i) => (
          <div key={i} data-floater data-depth={depth} className={cn('absolute opacity-90 drop-shadow-lg', cls)}>
            <C className="size-full" />
          </div>
        ))}
      </div>
      <h1 data-hero-title className="font-display text-[40px] leading-[44px] font-bold tracking-tight sm:text-[56px] sm:leading-[60px]">
        {title}
      </h1>
      {children}
    </div>
  )
}

export default FoodHero
