import { ArrowLeft, Info, MapPin, Phone, Users } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { Button, EmptyState, Money, Skeleton } from '@/components/ui'
import { CreatePartySheet } from '@/features/party/CreatePartySheet'
import { useAuth } from '@/lib/auth'
import { errorMessage, isNotFound } from '@/lib/errors'
import { formatPhone, mapHref, telHref } from '@/lib/format'
import type { MenuItem } from '@/lib/types'
import { useMenu, useRestaurant } from './hooks'
import { ItemSheet } from './ItemSheet'
import { MenuSkeleton, MenuView } from './MenuView'
import { OsmAttribution } from './OsmAttribution'
import { PartialMenuBadge, PartialMenuBanner } from './PartialMenu'
import { ProviderBadges, RestaurantMeta } from './RestaurantCard'
import { RestaurantCover } from './RestaurantCover'
import { cuisineLabel } from './visual'

export function RestaurantDetailPage() {
  const { id } = useParams()
  const restaurant = useRestaurant(id)
  const menu = useMenu(id)
  const [picked, setPicked] = useState<MenuItem | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const { isAuthenticated } = useAuth()
  const navigate = useNavigate()

  const startParty = () => {
    if (!isAuthenticated) {
      navigate(`/login?next=${encodeURIComponent(`/restaurants/${id}`)}`)
      return
    }
    setPicked(null)
    setCreateOpen(true)
  }

  if (restaurant.isError) {
    return (
      <EmptyState
        tone={isNotFound(restaurant.error) ? 'default' : 'danger'}
        emoji={isNotFound(restaurant.error) ? '🫥' : '📡'}
        title={isNotFound(restaurant.error) ? 'Restaurant introuvable' : 'Chargement impossible'}
        description={isNotFound(restaurant.error) ? "Il n'existe plus ou n'est pas actif." : errorMessage(restaurant.error)}
        action={
          <Link to="/restaurants" className="text-brand underline-offset-4 hover:underline">
            Voir les restos
          </Link>
        }
      />
    )
  }

  const r = restaurant.data
  return (
    <div className="space-y-5">
      <div className="relative -mx-4 -mt-4 sm:mx-0 sm:mt-0">
        {r ? (
          <RestaurantCover restaurant={r} thumb="1200x480" className="aspect-[16/8] w-full sm:aspect-[16/5] sm:rounded-xl" emojiClassName="text-7xl" />
        ) : (
          <Skeleton className="aspect-[16/8] w-full rounded-none sm:aspect-[16/5] sm:rounded-xl" />
        )}
        <div aria-hidden className="pointer-events-none absolute inset-0 bg-gradient-to-t from-bg via-bg/10 to-transparent sm:rounded-xl" />
        <Link
          to="/restaurants"
          className="absolute top-[max(0.75rem,env(safe-area-inset-top))] left-3 grid size-11 place-items-center rounded-full bg-bg/70 text-fg backdrop-blur-md hover:bg-bg"
          aria-label="Retour aux restaurants"
        >
          <ArrowLeft className="size-5" />
        </Link>
      </div>

      {r ? (
        <header className="space-y-3">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="space-y-1">
              <h1 className="font-display text-[32px] leading-9 font-bold">
                <span aria-hidden className="mr-2">{r.emoji}</span>
                {r.name}
              </h1>
              {(r.cuisines?.length ?? 0) > 0 && <p className="text-sm text-subtle">{(r.cuisines ?? []).map(cuisineLabel).join(' · ')}</p>}
            </div>
            <Button size="lg" onClick={startParty} leftIcon={<Users className="size-5" />} className="hidden sm:inline-flex">
              Lancer une commande ici
            </Button>
          </div>
          {r.description && <p className="max-w-[680px] text-muted">{r.description}</p>}
          <RestaurantMeta restaurant={r} />
          <PartialMenuBadge restaurant={r} />
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-muted">
            {r.address && (
              <a
                href={mapHref(r)}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex min-h-11 items-center gap-1.5 underline-offset-4 hover:text-fg hover:underline"
                aria-label={`Adresse : ${r.address} (ouvrir la carte dans un nouvel onglet)`}
              >
                <MapPin aria-hidden className="size-4 shrink-0" /> {r.address}
              </a>
            )}
            {r.phone && (
              <a
                href={telHref(r.phone)}
                className="inline-flex min-h-11 items-center gap-1.5 font-medium text-fg tabular underline-offset-4 hover:underline"
                aria-label={`Appeler ${r.name} au ${formatPhone(r.phone)}`}
              >
                <Phone aria-hidden className="size-4 shrink-0" /> {formatPhone(r.phone)}
              </a>
            )}
            {r.min_order > 0 && (
              <span className="inline-flex items-center gap-1.5">
                <Info aria-hidden className="size-4" /> Minimum de commande <Money cents={r.min_order} />
              </span>
            )}
          </div>
          <OsmAttribution restaurant={r} />
          <ProviderBadges restaurant={r} />
        </header>
      ) : (
        <div className="space-y-3">
          <Skeleton className="h-9 w-2/3" />
          <Skeleton className="h-4 w-1/2" />
        </div>
      )}

      <p className="flex items-center gap-1.5 text-xs text-subtle">
        <Info aria-hidden className="size-3.5" /> Prix indicatifs, susceptibles de varier sur la plateforme de livraison.
      </p>

      {r && <PartialMenuBanner restaurant={r} />}

      {menu.isPending ? (
        <MenuSkeleton />
      ) : menu.isError ? (
        <EmptyState tone="danger" emoji="📡" title="Menu indisponible" description={errorMessage(menu.error)} action={<Button onClick={() => menu.refetch()}>Réessayer</Button>} />
      ) : (
        <MenuView sections={menu.data.sections} onPick={setPicked} />
      )}

      {/* CTA mobile collant au-dessus de la barre d'onglets */}
      {r && (
        <div className="fixed inset-x-0 bottom-[calc(var(--tabbar-h)+env(safe-area-inset-bottom))] z-30 p-3 sm:hidden">
          <Button block size="lg" onClick={startParty} leftIcon={<Users className="size-5" />}>
            Lancer une commande ici
          </Button>
        </div>
      )}
      <div className="h-16 sm:hidden" aria-hidden />

      <ItemSheet
        item={picked}
        open={!!picked}
        onClose={() => setPicked(null)}
        browseFooter={
          <Button block size="lg" onClick={startParty} leftIcon={<Users className="size-5" />}>
            Commander en groupe ici
          </Button>
        }
      />
      {r && <CreatePartySheet open={createOpen} onClose={() => setCreateOpen(false)} restaurant={r} />}
    </div>
  )
}
