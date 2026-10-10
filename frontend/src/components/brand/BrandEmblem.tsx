import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/cn'
import { loadEmblem, type EmblemName } from './emblemLoader'

const RATIO: Record<EmblemName, number> = { interactive: 770 / 625, 'cardor-monogram': 1 }

/**
 * Emblème animé de la charte CARDOR Media : révélation au premier affichage, puis signature
 * au survol / focus du parent (`host`, par défaut le parent direct). Couleurs de marque en
 * thème sombre, version « mono » (currentColor) en thème clair (index.css, `.brand-emblem`).
 * Animations réduites : emblème statique.
 */
export function BrandEmblem({ name, host = 'a', className }: { name: EmblemName; host?: string; className?: string }) {
  const ref = useRef<SVGSVGElement>(null)
  const [emblem, setEmblem] = useState<{ viewBox: string; markup: string } | null>(null)

  useEffect(() => {
    let alive = true
    loadEmblem(name).then(
      (e) => alive && setEmblem(e),
      () => undefined, // emblème absent : la boîte reste vide, le texte suffit
    )
    return () => {
      alive = false
    }
  }, [name])

  useEffect(() => {
    const svg = ref.current
    if (!emblem || !svg) return
    let cleanup: (() => void) | undefined
    let alive = true
    void import('@/lib/brandEmblem').then(({ attachEmblemMotion }) => {
      if (!alive) return
      const hostEl = (svg.closest<HTMLElement>(host) ?? svg.parentElement) as HTMLElement | null
      cleanup = attachEmblemMotion(name, svg, hostEl)
    })
    return () => {
      alive = false
      cleanup?.()
    }
  }, [emblem, name, host])

  return (
    <svg
      ref={ref}
      viewBox={emblem?.viewBox ?? (name === 'interactive' ? '127 199 770 625' : '0 0 100 100')}
      data-emblem={name}
      aria-hidden
      focusable="false"
      className={cn('brand-emblem block overflow-visible', className)}
      style={{ aspectRatio: RATIO[name] }}
    >
      {emblem && <g data-root="" dangerouslySetInnerHTML={{ __html: emblem.markup }} />}
    </svg>
  )
}
