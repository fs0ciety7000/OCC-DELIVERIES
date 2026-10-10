import { Home, Plus, ShieldCheck, User, UtensilsCrossed } from 'lucide-react'
import { Suspense, useCallback, useEffect, useRef, useState, type CSSProperties } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { toast } from 'sonner'
import { BrandEmblem } from '@/components/brand/BrandEmblem'
import { FoodLoader } from '@/components/food'
import { Avatar, buttonClass, Logo, ThemeToggle } from '@/components/ui'
import { ResumeBanner } from '@/features/party/ActiveParties'
import { useActiveParties } from '@/features/party/resume'
import { VerifyEmailBanner } from '@/features/profile/VerifyEmailBanner'
import { UserMenu } from './UserMenu'
import { PwaRuntime } from '@/pwa/PwaRuntime'
import { CreatePartySheet } from '@/features/party/CreatePartySheet'
import { GuestBanner } from '@/features/teams/GuestBanner'
import { TeamLaunchListener } from '@/features/teams/TeamLaunchListener'
import { GlobalSearch } from '@/features/search/GlobalSearch'
import { useAuth } from '@/lib/auth'
import { cn } from '@/lib/cn'
import { useMyPartiesRealtime } from '@/lib/realtime'
import type { Party, PartyStatus } from '@/lib/types'

/** Notifications globales : un collègue fait avancer une commande pendant que je suis ailleurs. */
const STATUS_NOTIFS: Partial<Record<PartyStatus, string>> = {
  voting: 'Le vote est ouvert',
  ordering: 'À vos paniers : le resto est choisi',
  review: 'Les paniers sont verrouillés (récap)',
  paying: 'Place aux remboursements',
  closed: 'Commande terminée',
  cancelled: 'Commande annulée',
}

const NAV = [
  { to: '/', label: 'Accueil', icon: Home, end: true },
  { to: '/restaurants', label: 'Restos', icon: UtensilsCrossed, end: false },
] as const

/** Adresse de contact publique (aussi dans les e-mails : `ContactEmail` côté Go). */
const CONTACT_EMAIL = 'contact@eat.fs0ciety.org'

export function AppShell() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const location = useLocation()
  const navigate = useNavigate()
  const [createOpen, setCreateOpen] = useState(false)
  const inParty = location.pathname.startsWith('/party/')
  // Une seule action principale par écran : le raccourci d'en-tête est secondaire et
  // masqué là où l'écran porte déjà son propre CTA (héros d'accueil, auth, party).
  const inAuth = ['/login', '/register'].includes(location.pathname) || location.pathname.startsWith('/auth/')
  const showHeaderCta = !inParty && !inAuth && location.pathname !== '/' && !user?.is_guest
  const launch = () => (user ? setCreateOpen(true) : navigate(`/login?next=${encodeURIComponent(location.pathname)}`))
  const shellStyle = inParty ? ({ '--tabbar-h': '0px' } as CSSProperties) : undefined

  // « Commande en cours » : bandeau persistant + toasts de statut où que l'on soit.
  const { parties: active } = useActiveParties(user?.id)
  const pathRef = useRef(location.pathname)
  useEffect(() => {
    pathRef.current = location.pathname
  }, [location.pathname])
  const onStatusChange = useCallback(
    (p: Party) => {
      // La page de la party affiche déjà ses propres toasts.
      if (pathRef.current === `/party/${p.id}`) return
      const msg = STATUS_NOTIFS[p.status]
      if (!msg) return
      toast(`${msg} — « ${p.title || 'Commande groupée'} »`, {
        id: `party-status-${p.id}`,
        action: { label: 'Voir', onClick: () => navigate(`/party/${p.id}`) },
      })
    },
    [navigate],
  )
  useMyPartiesRealtime(user?.id, onStatusChange)
  const path = location.pathname
  const inInvite = path.startsWith('/j/') || path.startsWith('/e/')
  // L'accueil liste déjà les commandes en cours (héros ou cartes) : pas de pastille ni de dock en double.
  const hideResume = inParty || inInvite || inAuth || (path === '/' && active.length > 0)
  const resume = user && !hideResume ? active : []
  // Le dock mobile masquerait les tableaux et barres d'action de l'administration.
  const showDock = resume.length > 0 && !path.startsWith('/admin')
  // « + » de la tab bar : seulement pour un compte qui peut lancer, hors pages d'invitation.
  const showTabLaunch = !!user && !user.is_guest && !inInvite

  return (
    <div className={cn('relative min-h-dvh', showDock && 'has-resume-dock')} style={shellStyle}>
      <div className="app-aurora" aria-hidden />
      <a href="#main" className="sr-only z-[80] rounded-md bg-elevated px-4 py-2 focus:not-sr-only focus:fixed focus:top-3 focus:left-3">
        Aller au contenu
      </a>

      {/* En-tête : barre complète desktop, compacte mobile */}
      <header className="pt-safe sticky top-0 z-40 border-b border-border bg-bg/75 backdrop-blur-xl md:h-16">
        <div className="mx-auto flex h-14 max-w-[1200px] items-center gap-4 px-4 md:h-16 md:px-8">
          <Link to="/" className="shrink-0 rounded-md" aria-label="OCC Deliveries — accueil">
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
            {isAdmin && (
              <NavLink
                to="/admin"
                className={({ isActive }) => cn('rounded-full px-3.5 py-2 text-sm font-semibold transition-colors', isActive ? 'bg-fg/[0.08] text-fg' : 'text-muted hover:text-fg')}
              >
                Admin
              </NavLink>
            )}
          </nav>
          <div className="ml-auto flex min-w-0 items-center gap-1">
            <ResumeBanner parties={resume} variant="header" />
            <GlobalSearch onLaunch={launch} />
            {isAdmin && (
              <Link to="/admin" className={cn(buttonClass('ghost', 'icon'), 'md:hidden')} aria-label="Administration">
                <ShieldCheck className="size-5" aria-hidden />
              </Link>
            )}
            <ThemeToggle />
            {showHeaderCta && (
              <button type="button" onClick={launch} aria-label="Lancer une commande" className={cn(buttonClass('secondary', 'sm'), 'hidden shrink-0 md:inline-flex')}>
                <Plus className="size-4" aria-hidden /> <span className="hidden xl:inline">Lancer une commande</span>
              </button>
            )}
            {user ? (
              <UserMenu user={user} />
            ) : (
              <Link to="/login" className={cn(buttonClass('ghost', 'sm'), 'hidden md:inline-flex')}>
                Connexion
              </Link>
            )}
          </div>
        </div>
      </header>

      <main id="main" className="relative z-10 mx-auto flex min-h-[calc(100dvh-4rem)] w-full flex-col max-w-[1200px] px-4 pt-4 pb-[calc(var(--tabbar-h)+env(safe-area-inset-bottom)+32px)] md:px-8 md:pt-8">
        {/* Bloc normal (pas un item flex à marges auto) : les pages en `mx-auto max-w-…` gardent toute la largeur. */}
        <div className="mb-16">
          {!inParty && !inAuth && location.pathname !== '/profile' && <VerifyEmailBanner user={user} dismissible className="mb-4" />}
          {!inParty && !inAuth && location.pathname !== '/profile' && <GuestBanner user={user} className="mb-4" />}
          <TeamLaunchListener user={user} />
          <PwaRuntime />
          <Suspense fallback={<FoodLoader className="py-24" />}>
            <Outlet />
          </Suspense>
        </div>
        {/* mt-auto : sur une page courte, le pied reste en bas (au-dessus du dock grâce au padding). */}
        <footer className="mt-auto border-t border-border pt-6 text-xs text-subtle">
          <div className="mx-auto flex w-fit flex-wrap items-center justify-center gap-x-1.5 gap-y-1 py-1 text-center">
            <a
              href="https://interactive.cardormedia.com/"
              target="_blank"
              rel="noopener noreferrer"
              className="group flex items-center gap-2.5 rounded-md underline-offset-4 focus-visible:text-fg"
            >
              {/* Emblème « Le Dragon » de la charte CARDOR : glitch au survol, « boot » au premier affichage. */}
              <BrandEmblem name="interactive" className="h-6 w-auto" />
              <span>
                Développé par{' '}
                <span className="font-semibold text-muted group-hover:text-fg group-hover:underline">
                  <span className="font-mono tracking-[0.15em]">OCC</span> Interactive
                </span>
              </span>
            </a>
            <span>
              <span className="hidden sm:inline">· </span>une division de{' '}
              <a
                href="https://cardormedia.com/"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 rounded-md align-middle font-semibold text-muted underline-offset-4 hover:text-fg hover:underline focus-visible:text-fg"
              >
                {/* Roue du Car d'Or (monogramme) : un tour au survol. */}
                <BrandEmblem name="cardor-monogram" className="size-4" />
                CARDOR Media
              </a>{' '}
              · © {new Date().getFullYear()}
            </span>
          </div>
          <p className="mt-1 text-center">
            Contact :{' '}
            <a href={`mailto:${CONTACT_EMAIL}`} className="rounded-sm font-semibold text-muted underline-offset-4 hover:text-fg hover:underline focus-visible:text-fg">
              {CONTACT_EMAIL}
            </a>
          </p>
        </footer>
      </main>

      {showDock && <ResumeBanner parties={resume} variant="dock" />}

      {/* Barre d'onglets mobile */}
      {!inParty && (
        <nav aria-label="Navigation" className="pb-safe fixed inset-x-0 bottom-0 z-40 border-t border-border bg-bg/85 backdrop-blur-xl md:hidden">
          <div className={cn('mx-auto grid h-16 max-w-md items-center', showTabLaunch ? 'grid-cols-4' : 'grid-cols-3')}>
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
            {showTabLaunch && (
              <button type="button" onClick={launch} className="flex h-full flex-col items-center justify-center gap-0.5 text-[11px] font-semibold text-subtle" aria-label="Lancer une commande">
                <span className="grid size-9 place-items-center rounded-full bg-ember text-brand-fg shadow-glow">
                  <Plus className="size-5" aria-hidden />
                </span>
              </button>
            )}
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
