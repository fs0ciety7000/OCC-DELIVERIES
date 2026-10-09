import { Plus } from 'lucide-react'
import { motion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { Badge, EmptyState, Money, Skeleton } from '@/components/ui'
import { cn } from '@/lib/cn'
import { itemVariants, listVariants } from '@/lib/motion'
import type { MenuItem } from '@/lib/types'
import type { MenuSection } from './hooks'
import { itemImageUrl, TAG_LABELS } from './visual'

export interface MenuViewProps {
  sections: MenuSection[]
  onPick: (item: MenuItem, el?: HTMLElement) => void
  /** Quantités déjà au panier par menu_item (affiche une pastille). */
  inCart?: Record<string, number>
  stickyTop?: string
}

/** Menu avec onglets de catégories collants + scrollspy. */
export function MenuView({ sections, onPick, inCart, stickyTop = 'top-[var(--header-h)]' }: MenuViewProps) {
  const [active, setActive] = useState(sections[0]?.category?.id ?? '')
  const tabsRef = useRef<HTMLDivElement>(null)
  const ids = sections.map((s) => s.category?.id ?? '').join('|')

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries.filter((e) => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        const id = visible[0]?.target.getAttribute('data-section')
        if (id) setActive(id)
      },
      { rootMargin: '-120px 0px -60% 0px' },
    )
    document.querySelectorAll('[data-section]').forEach((el) => observer.observe(el))
    return () => observer.disconnect()
  }, [ids])

  useEffect(() => {
    const tab = tabsRef.current?.querySelector<HTMLElement>(`[data-tab="${CSS.escape(active)}"]`)
    tab?.scrollIntoView({ block: 'nearest', inline: 'center', behavior: 'smooth' })
  }, [active])

  if (!sections.length) return <EmptyState emoji="📭" title="Menu vide" description="Ce restaurant n'a pas encore publié sa carte." />

  const jump = (id: string) => {
    setActive(id)
    const el = document.getElementById(`cat-${id}`)
    if (el) {
      const reduce = matchMedia('(prefers-reduced-motion: reduce)').matches
      el.scrollIntoView({ behavior: reduce ? 'auto' : 'smooth', block: 'start' })
    }
  }

  return (
    <div>
      <nav
        ref={tabsRef}
        aria-label="Catégories du menu"
        className={cn('sticky z-20 -mx-4 border-b border-border bg-bg/85 px-4 backdrop-blur-xl sm:-mx-8 sm:px-8', stickyTop)}
      >
        <div className="scrollbar-none flex gap-1 overflow-x-auto py-2">
          {sections.map((s) => {
            const id = s.category?.id ?? ''
            const on = id === active
            return (
              <button
                key={id}
                type="button"
                data-tab={id}
                onClick={() => jump(id)}
                aria-current={on ? 'true' : undefined}
                className={cn(
                  'min-h-10 shrink-0 rounded-full px-3.5 text-sm font-semibold whitespace-nowrap transition-colors',
                  on ? 'bg-fg text-bg' : 'text-muted hover:bg-fg/[0.06] hover:text-fg',
                )}
              >
                {s.category?.name}
              </button>
            )
          })}
        </div>
      </nav>
      <div className="space-y-8 pt-5">
        {sections.map((s) => (
          <section key={s.category?.id} id={`cat-${s.category?.id}`} data-section={s.category?.id} className="scroll-mt-[calc(var(--header-h)+64px)]" aria-labelledby={`h-${s.category?.id}`}>
            <h2 id={`h-${s.category?.id}`} className="mb-3 font-display text-xl font-semibold">
              {s.category?.name}
            </h2>
            <motion.ul variants={listVariants} initial="hidden" whileInView="show" viewport={{ once: true, margin: '-40px' }} className="grid gap-2.5 md:grid-cols-2">
              {s.items.map((item) => (
                <motion.li key={item.id} variants={itemVariants}>
                  <MenuItemRow item={item} onPick={(el) => onPick(item, el)} count={inCart?.[item.id]} />
                </motion.li>
              ))}
            </motion.ul>
          </section>
        ))}
      </div>
    </div>
  )
}

export function MenuItemRow({ item, onPick, count }: { item: MenuItem; onPick: (el: HTMLElement) => void; count?: number }) {
  const img = itemImageUrl(item)
  const unavailable = item.available === false
  return (
    <button
      type="button"
      onClick={(e) => onPick(e.currentTarget.querySelector<HTMLElement>('[data-thumb]') ?? e.currentTarget)}
      disabled={unavailable}
      className={cn(
        'group flex w-full items-start gap-3 rounded-lg border bg-surface p-3 text-left shadow-card transition-[border-color,transform] duration-[120ms] hover:border-border-strong disabled:opacity-50',
        count ? 'border-brand/40' : 'border-border',
      )}
    >
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex items-center gap-2">
          <h3 className="font-semibold leading-6">{item.name}</h3>
          {count ? <Badge variant="brand" className="tabular">×{count}</Badge> : null}
        </div>
        {item.description && <p className="line-clamp-2 text-[13px] leading-5 text-muted">{item.description}</p>}
        <div className="flex flex-wrap items-center gap-2 pt-0.5">
          <Money cents={item.price} className="text-sm font-semibold" />
          {unavailable && <Badge variant="danger">Indisponible</Badge>}
          {item.popular && <Badge variant="brand">Populaire</Badge>}
          {(item.tags ?? []).slice(0, 2).map((t) => (
            <span key={t} className="text-xs text-subtle" title={TAG_LABELS[t]?.label ?? t}>
              {TAG_LABELS[t]?.emoji ?? ''} {TAG_LABELS[t]?.label ?? t}
            </span>
          ))}
        </div>
      </div>
      <div className="relative shrink-0">
        <div aria-hidden data-thumb className="grid size-20 place-items-center overflow-hidden rounded-md bg-elevated text-3xl">
          {img ? <img src={img} alt="" loading="lazy" className="size-full object-cover" /> : item.emoji || '🍽️'}
        </div>
        {!unavailable && (
          <span aria-hidden className="absolute -right-1.5 -bottom-1.5 grid size-8 place-items-center rounded-full bg-ember text-brand-fg shadow-glow transition-transform group-hover:scale-105">
            <Plus className="size-4" strokeWidth={2.5} />
          </span>
        )}
      </div>
    </button>
  )
}

export function MenuSkeleton() {
  return (
    <div className="space-y-3 pt-4" aria-busy="true" aria-label="Chargement du menu">
      <div className="flex gap-2">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-9 w-24 rounded-full" />
        ))}
      </div>
      <div className="grid gap-2.5 md:grid-cols-2">
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <Skeleton key={i} className="h-26 rounded-lg" />
        ))}
      </div>
    </div>
  )
}
