import { Search } from 'lucide-react'
import { buttonClass } from '@/components/ui'
import { cn } from '@/lib/cn'
import { CommandPalette } from './CommandPalette'
import { shortcutLabel } from './constants'
import { Kbd } from './Kbd'
import { useSearchShortcuts } from './shortcuts'
import { openSearch, useSearchPalette } from './store'

/** Bouton d'en-tête : icône en mobile, pastille « Rechercher… ⌘K » dès 1024 px. */
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
        'lg:w-auto lg:justify-start lg:gap-2 lg:rounded-full lg:border lg:border-border lg:bg-surface lg:px-3 lg:text-sm lg:font-medium lg:text-subtle lg:hover:border-border-strong lg:hover:text-fg',
        className,
      )}
    >
      <Search className="size-5 lg:size-4" aria-hidden />
      <span className="hidden lg:inline">Rechercher…</span>
      <span className="ml-3 hidden lg:inline-flex" aria-hidden>
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
