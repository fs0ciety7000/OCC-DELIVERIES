import { useRef, type KeyboardEvent } from 'react'
import { cn } from '@/lib/cn'
import { PROFILE_TABS, panelId, tabId, type ProfileTab } from './tabs'

/** Onglets du profil (motif ARIA tabs : flèches, Début / Fin), en pills comme `Segmented`. */
export function ProfileTabs({ value, onChange }: { value: ProfileTab; onChange: (t: ProfileTab) => void }) {
  const refs = useRef<Record<string, HTMLButtonElement | null>>({})
  const onKey = (e: KeyboardEvent) => {
    const i = PROFILE_TABS.findIndex((t) => t.value === value)
    let next = -1
    if (e.key === 'ArrowRight') next = (i + 1) % PROFILE_TABS.length
    else if (e.key === 'ArrowLeft') next = (i - 1 + PROFILE_TABS.length) % PROFILE_TABS.length
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = PROFILE_TABS.length - 1
    if (next < 0) return
    e.preventDefault()
    const t = PROFILE_TABS[next]!.value
    onChange(t)
    refs.current[t]?.focus()
  }
  return (
    <div role="tablist" aria-label="Profil" className="flex w-full rounded-full border border-border bg-elevated p-1" onKeyDown={onKey}>
      {PROFILE_TABS.map((t) => {
        const active = t.value === value
        return (
          <button
            key={t.value}
            ref={(el) => {
              refs.current[t.value] = el
            }}
            id={tabId(t.value)}
            type="button"
            role="tab"
            aria-selected={active}
            aria-controls={panelId(t.value)}
            tabIndex={active ? 0 : -1}
            onClick={() => onChange(t.value)}
            className={cn(
              'min-h-11 flex-1 rounded-full px-3.5 text-sm font-semibold whitespace-nowrap transition-colors duration-[120ms] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand',
              active ? 'bg-surface text-fg shadow-card' : 'text-muted hover:text-fg',
            )}
          >
            {t.label}
          </button>
        )
      })}
    </div>
  )
}
