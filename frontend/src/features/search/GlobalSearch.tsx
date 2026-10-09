import { Search } from 'lucide-react'
import { buttonClass } from '@/components/ui'
import { cn } from '@/lib/cn'
import { CommandPalette } from './CommandPalette'
import { shortcutLabel } from './constants'
import { Kbd } from './Kbd'
import { useSearchShortcuts } from './shortcuts'
import { openSearch, useSearchPalette } from './store'

/** Bouton d'en-tête : icône en mobile, pastille « Rechercher… ⌘K » dès 1280 px. */
export function SearchButton({ className }: { className?: string }) {
  const { open } = useSearchPalette()
  const hint = shortcutLabel()
  return (
    <button
      type="button"
      onClick={() => openSearch()}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-keyshortcuts="Control+K Meta+K /"
      aria-label={`Rechercher (${hint})`}
      className={cn(
        buttonClass('ghost', 'icon'),
        'xl:w-auto xl:justify-start xl:gap-2 xl:rounded-full xl:border xl:border-border xl:bg-surface xl:px-3 xl:text-sm xl:font-medium xl:text-subtle xl:hover:border-border-strong xl:hover:text-fg',
        className,
      )}
    >
      <Search className="size-5 xl:size-4" aria-hidden />
      <span className="hidden xl:inline">Rechercher…</span>
      <span className="ml-3 hidden xl:inline-flex" aria-hidden>
        <Kbd>{hint}</Kbd>
      </span>
    </button>
  )
}

/** Recherche globale : bouton d'en-tête + palette + raccourcis clavier (monté une fois, dans le shell). */
export function GlobalSearch({ onLaunch }: { onLaunch: () => void }) {
  useSearchShortcuts()
  return (
    <>
      <SearchButton />
      <CommandPalette onLaunch={onLaunch} />
    </>
  )
}
