import type { ReactNode } from 'react'
import { highlightRanges } from './fold'

/** Texte avec les mots trouvés surlignés (`<mark>`, annoncé normalement par les lecteurs d'écran). */
export function Highlight({ text, terms }: { text: string; terms: string[] }) {
  const ranges = highlightRanges(text, terms)
  if (ranges.length === 0) return <>{text}</>
  const parts: ReactNode[] = []
  let at = 0
  ranges.forEach((r, i) => {
    if (r.start > at) parts.push(text.slice(at, r.start))
    parts.push(
      <mark key={i} className="rounded-[3px] bg-brand/20 px-px text-inherit">
        {text.slice(r.start, r.end)}
      </mark>,
    )
    at = r.end
  })
  if (at < text.length) parts.push(text.slice(at))
  return <>{parts}</>
}
