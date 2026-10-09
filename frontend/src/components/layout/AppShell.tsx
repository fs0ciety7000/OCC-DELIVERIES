import { Home, Plus, User, UtensilsCrossed } from 'lucide-react'
import { Suspense, useState, type CSSProperties } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { FoodLoader } from '@/components/food'
import { Avatar, buttonClass, Logo, ThemeToggle } from '@/components/ui'
import { CreatePartySheet } from '@/features/party/CreatePartySheet'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/cn'

const NAV = [
  { to: '/', label: 'Accueil', icon: Home, end: true },
  { to: '/restaurants', label: 'Restos', icon: UtensilsCrossed, end: false },
] as const

export function AppShell() {
  const { user } = useAuth()
  const location = useLocation()
  const navigate = useNavigate()
  const [createOpen, setCreateOpen] = useState(false)
  const inParty = location.pathname.startsWith('/party/')
  const launch = () => (user ? setCreateOpen(true) : navigate(`/login?next=${encodeURIComponent(location.pathname)}`))
  const shellStyle = inParty ? ({ '--tabbar-h': '0px' } as CSSProperties) : undefined

  return (
    <div className="relative min-h-dvh" style={shellStyle}>
      <div className="app-aurora" aria-hidden />
      <a href="#main" className="sr-only z-[80] rounded-md bg-elevated px-4 py-2 focus:not-sr-only focus:fixed focus:top-3 focus:left-3">
        Aller au contenu
      </a>

      {/* En-tête : barre complète desktop, compacte mobile */}
      <header className="pt-safe sticky top-0 z-40 border-b border-border bg-bg/75 backdrop-blur-xl md:h-16">
        <div className="mx-auto flex h-14 max-w-[1200px] items-center gap-4 px-4 md:h-16 md:px-8">
          <Link to="/" className="rounded-md" aria-label="OCC Deliveries — accueil">
            <Logo />
          </Link>
          <nav aria-label="Navigation principale" className="ml-4 hidden items-center gap-1 md:flex">
            {NAV.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.end}
                className={({ isActive }) => cn('rounded-full px-3.5 py-2 text-sm font-semibold transition-colors', isActive ? 'bg-fg/[0.08] text-fg' : 'text-muted hover:text-fg')}
              >
                {n.label}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-1">
            <ThemeToggle />
            <button type="button" onClick={launch} className={cn(buttonClass('primary', 'sm'), 'hidden md:inline-flex')}>
              <Plus className="size-4" /> Lancer une commande
            </button>
            {user ? (
              <Link to="/profile" className="hidden rounded-full p-1.5 md:block" aria-label="Mon profil">
                <Avatar user={user} size={32} decorative />
              </Link>
            ) : (
              <Link to="/login" className={cn(buttonClass('ghost', 'sm'), 'hidden md:inline-flex')}>
                Connexion
              </Link>
            )}
          </div>
        </div>
      </header>

      <main id="main" className="relative z-10 mx-auto w-full max-w-[1200px] px-4 pt-4 pb-[calc(var(--tabbar-h)+env(safe-area-inset-bottom)+32px)] md:px-8 md:pt-8">
        <Suspense fallback={<FoodLoader className="py-24" />}>
          <Outlet />
        </Suspense>
      </main>

      {/* Barre d'onglets mobile */}
      {!inParty && (
        <nav aria-label="Navigation" className="pb-safe fixed inset-x-0 bottom-0 z-40 border-t border-border bg-bg/85 backdrop-blur-xl md:hidden">
          <div className="mx-auto grid h-16 max-w-md grid-cols-4 items-center">
            {NAV.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }) => cn('flex h-full flex-col items-center justify-center gap-0.5 text-[11px] font-semibold', isActive ? 'text-fg' : 'text-subtle')}>
                {({ isActive }) => (
                  <>
                    <n.icon className={cn('size-5', isActive && 'text-brand')} aria-hidden />
                    {n.label}
                  </>
                )}
              </NavLink>
            ))}
            <button type="button" onClick={launch} className="flex h-full flex-col items-center justify-center gap-0.5 text-[11px] font-semibold text-subtle" aria-label="Lancer une commande">
              <span className="grid size-9 place-items-center rounded-full bg-ember text-brand-fg shadow-glow">
                <Plus className="size-5" aria-hidden />
              </span>
            </button>
            <NavLink to={user ? '/profile' : '/login'} className={({ isActive }) => cn('flex h-full flex-col items-center justify-center gap-0.5 text-[11px] font-semibold', isActive ? 'text-fg' : 'text-subtle')}>
              {user ? <Avatar user={user} size={24} decorative /> : <User className="size-5" aria-hidden />}
              {user ? 'Profil' : 'Connexion'}
            </NavLink>
          </div>
        </nav>
      )}
      <CreatePartySheet open={createOpen} onClose={() => setCreateOpen(false)} />
    </div>
  )
}
