import type { ReactNode } from 'react'

/** Touche de clavier (aide de la palette, bouton d'en-tête). */
export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="inline-flex h-5 min-w-5 items-center justify-center rounded-[6px] border border-border bg-surface px-1 font-sans text-[11px] font-semibold text-muted">{children}</kbd>
}
