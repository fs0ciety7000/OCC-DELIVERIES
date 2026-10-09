import { UserPlus, X } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { buttonClass } from '@/components/ui'
import { cn } from '@/lib/cn'
import type { User } from '@/lib/types'

const DISMISS_KEY = 'occ.guestBanner.dismissed'

function readDismissed(): boolean {
  try {
    return localStorage.getItem(DISMISS_KEY) === '1'
  } catch {
    return false
  }
}

/** Bandeau « Tu es invité·e — crée un compte pour garder ton historique » (masquable). */
export function GuestBanner({ user, className }: { user: User | null; className?: string }) {
  const [dismissed, setDismissed] = useState(readDismissed)
  if (!user?.is_guest || dismissed) return null
  return (
    <aside aria-label="Compte invité" className={cn('flex items-start gap-3 rounded-md border border-info/30 bg-info/10 py-1.5 pr-1.5 pl-3 text-sm', className)}>
      <UserPlus aria-hidden className="mt-2.5 size-5 shrink-0 text-info" />
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 py-1">
        <p className="min-w-[min(100%,14rem)] flex-1 py-1">
          <span className="font-semibold">Tu es invité·e</span> <span className="text-muted">— crée un compte pour garder ton historique.</span>
        </p>
        <Link to="/profile?onglet=infos" className={buttonClass('ghost', 'sm', false, 'min-h-11')}>
          Créer mon compte
        </Link>
      </div>
      <button
        type="button"
        aria-label="Masquer ce rappel"
        className="grid size-11 shrink-0 place-items-center rounded-full text-muted hover:bg-fg/[0.06] hover:text-fg"
        onClick={() => {
          setDismissed(true)
          try {
            localStorage.setItem(DISMISS_KEY, '1')
          } catch {
            /* stockage indisponible */
          }
        }}
      >
        <X aria-hidden className="size-4" />
      </button>
    </aside>
  )
}
