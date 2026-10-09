import { BarChart3, FileUp, Store, ShoppingBag, Users } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link, Navigate, NavLink, Outlet, useLocation } from 'react-router'
import { buttonClass, EmptyState } from '@/components/ui'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/cn'

const ADMIN_NAV = [
  { to: '/admin', label: 'Tableau de bord', icon: BarChart3, end: true },
  { to: '/admin/restaurants', label: 'Restaurants', icon: Store, end: false },
  { to: '/admin/import', label: 'Import', icon: FileUp, end: false },
  { to: '/admin/commandes', label: 'Commandes', icon: ShoppingBag, end: false },
  { to: '/admin/utilisateurs', label: 'Utilisateurs', icon: Users, end: false },
] as const

/** Garde : connecté **et** rôle `admin` (le serveur vérifie de toute façon). */
export function RequireAdmin({ children }: { children: ReactNode }) {
  const { user } = useAuth()
  const location = useLocation()
  if (!user) return <Navigate to={`/login?next=${encodeURIComponent(location.pathname + location.search)}`} replace />
  if (user.role !== 'admin') {
    return (
      <EmptyState
        emoji="🔒"
        title="Accès réservé"
        description="Cet espace est réservé aux administrateurs d'OCC Deliveries."
        action={
          <Link to="/" className={buttonClass('secondary')}>
            Retour à l'accueil
          </Link>
        }
      />
    )
  }
  return children
}

/** Mise en page admin : barre latérale (desktop) / onglets défilants (mobile). */
export function AdminLayout() {
  return (
    <RequireAdmin>
      <div className="md:grid md:grid-cols-[220px_minmax(0,1fr)] md:gap-8">
        <aside className="mb-5 md:mb-0">
          <p className="mb-2 hidden px-3 text-xs font-semibold tracking-wide text-subtle uppercase md:block">Administration</p>
          <nav aria-label="Administration" className="relative -mx-4 overflow-x-auto px-4 md:mx-0 md:overflow-visible md:px-0">
            <ul className="flex gap-1 md:sticky md:top-24 md:flex-col">
              {ADMIN_NAV.map((n) => (
                <li key={n.to} className="shrink-0">
                  <NavLink
                    to={n.to}
                    end={n.end}
                    className={({ isActive }) =>
                      cn(
                        'flex min-h-11 items-center gap-2.5 rounded-full px-3.5 text-sm font-semibold whitespace-nowrap transition-colors md:rounded-md',
                        isActive ? 'bg-fg/[0.08] text-fg' : 'text-muted hover:bg-fg/[0.04] hover:text-fg',
                      )
                    }
                  >
                    {({ isActive }) => (
                      <>
                        <n.icon className={cn('size-4', isActive && 'text-brand')} aria-hidden />
                        {n.label}
                      </>
                    )}
                  </NavLink>
                </li>
              ))}
            </ul>
          </nav>
        </aside>
        <section className="min-w-0">
          <Outlet />
        </section>
      </div>
    </RequireAdmin>
  )
}

export function AdminHeader({ title, description, actions }: { title: string; description?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div className="min-w-0">
        <h1 className="font-display text-[28px] leading-8 font-bold md:text-[32px] md:leading-9">{title}</h1>
        {description && <p className="mt-1 text-sm text-muted">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
    </header>
  )
}
