import { LogOut, Receipt, Shield, UserRound } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Avatar } from '@/components/ui'
import { logout } from '@/lib/auth'
import type { User } from '@/lib/types'

/** Menu du compte (en-tête desktop) : profil, commandes, admin, déconnexion. */
export function UserMenu({ user }: { user: User }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const base = 'flex min-h-11 w-full items-center gap-3 rounded-md px-3 text-sm hover:bg-elevated focus-visible:bg-elevated focus-visible:outline-none'
  const item = `${base} text-fg`
  return (
    <div ref={ref} className="relative hidden md:block">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label="Mon compte"
        className="rounded-full p-1.5"
      >
        <Avatar user={user} size={32} decorative />
      </button>
      {open && (
        <div role="menu" className="absolute right-0 z-50 mt-2 w-60 rounded-lg border border-border bg-surface p-1.5 shadow-card">
          <div className="border-b border-border px-3 pt-1.5 pb-2.5">
            <p className="truncate text-sm font-semibold">{user.name || 'Mon compte'}</p>
            <p className="truncate text-xs text-muted">{user.email}</p>
          </div>
          <div className="pt-1.5">
            <Link role="menuitem" to="/profile" className={item} onClick={() => setOpen(false)}>
              <Receipt className="size-4 text-muted" aria-hidden /> Mes commandes
            </Link>
            <Link role="menuitem" to="/profile?onglet=infos" className={item} onClick={() => setOpen(false)}>
              <UserRound className="size-4 text-muted" aria-hidden /> Mes infos
            </Link>
            {user.role === 'admin' && (
              <Link role="menuitem" to="/admin" className={item} onClick={() => setOpen(false)}>
                <Shield className="size-4 text-muted" aria-hidden /> Administration
              </Link>
            )}
            <button
              type="button"
              role="menuitem"
              className={`${base} text-danger`}
              onClick={() => {
                setOpen(false)
                logout()
                toast('À bientôt !')
                navigate('/')
              }}
            >
              <LogOut className="size-4" aria-hidden /> Se déconnecter
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
