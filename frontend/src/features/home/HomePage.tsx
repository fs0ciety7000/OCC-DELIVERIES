import { ArrowRight, ChevronRight, Plus, Users } from 'lucide-react'
import { motion } from 'motion/react'
import { Suspense, useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { FoodHero } from '@/components/food'
import { AvatarStack, Badge, Button, Card, EmptyState, Skeleton } from '@/components/ui'
import { ResumeHero } from '@/features/party/ActiveParties'
import { CreatePartySheet } from '@/features/party/CreatePartySheet'
import { STATUS_LABELS, useMyParties } from '@/features/party/hooks'
import { JoinByCode } from '@/features/party/JoinByCode'
import { useNearby } from '@/features/restaurants/hooks'
import { LocationBar } from '@/features/restaurants/LocationBar'
import { RestaurantCard, RestaurantCardSkeleton } from '@/features/restaurants/RestaurantCard'
import { RestaurantCover } from '@/features/restaurants/RestaurantCover'
import { TeamsSection } from '@/features/teams/TeamsSection'
import { useAuth } from '@/lib/auth'
import { formatRelativeTime } from '@/lib/format'
import { itemVariants, listVariants } from '@/lib/motion'

const TITLE = "Qu'est-ce qu'on mange ?"

export function HomePage() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const [createOpen, setCreateOpen] = useState(false)
  const nearby = useNearby({ radiusKm: 5 })
  const parties = useMyParties(user?.id)

  const launch = () => (user ? setCreateOpen(true) : navigate('/login?next=/'))
  // Une seule commande en cours (ex. retour après connexion) : on la met en avant tout en haut.
  const only = parties.data?.length === 1 ? parties.data[0]! : null

  return (
    <div className="space-y-10">
      {user && only && (
        <section aria-label="Commande en cours" className="pt-2">
          <ResumeHero party={{ id: only.id, title: only.title || 'Commande groupée', status: only.status, restaurant: only.expand?.restaurant }} />
        </section>
      )}
      <section className="relative space-y-6 pt-4 sm:pt-10">
        <p className="text-sm font-semibold tracking-wide text-brand uppercase">{user ? `Salut ${user.name?.split(' ')[0] || ''} 👋` : 'Commandes groupées entre collègues'}</p>
        <Suspense fallback={<h1 className="font-display text-[40px] leading-[44px] font-bold sm:text-[56px] sm:leading-[60px]">{TITLE}</h1>}>
          <FoodHero title={TITLE} className="max-w-2xl" />
        </Suspense>
        <p className="max-w-xl text-lg text-muted">On vote pour le resto, chacun compose son panier, et on se rembourse en un scan. Fini les tableurs du midi.</p>
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
          <Button size="lg" onClick={launch} leftIcon={<Plus className="size-5" />} className="sm:min-w-60">
            Lancer une commande
          </Button>
          <div className="w-full sm:max-w-sm">
            <JoinByCode />
          </div>
        </div>
      </section>

      {user && !only && (
        <section className="space-y-3" aria-labelledby="h-active">
          <h2 id="h-active" className="font-display text-2xl font-semibold">
            Mes commandes en cours
          </h2>
          {parties.isPending ? (
            <div className="grid gap-3 sm:grid-cols-2">
              <Skeleton className="h-24 rounded-lg" />
              <Skeleton className="h-24 rounded-lg" />
            </div>
          ) : (parties.data?.length ?? 0) === 0 ? (
            <Card>
              <EmptyState
                title="Rien sur le feu"
                description="Lance une commande ou demande le code à un·e collègue."
                action={
                  <Button variant="secondary" onClick={launch} leftIcon={<Users className="size-4" />}>
                    Créer un salon
                  </Button>
                }
                className="py-8"
              />
            </Card>
          ) : (
            <motion.ul variants={listVariants} initial="hidden" animate="show" className="grid gap-3 sm:grid-cols-2">
              {parties.data!.map((p) => (
                <motion.li key={p.id} variants={itemVariants}>
                  <Link to={`/party/${p.id}`} className="block rounded-lg">
                    <Card variant="interactive" className="flex items-center gap-3 p-3.5">
                      {p.expand?.restaurant ? (
                        <RestaurantCover restaurant={p.expand.restaurant} thumb="120x120" className="size-14 shrink-0 rounded-md" emojiClassName="text-2xl" />
                      ) : (
                        <div aria-hidden className="grid size-14 shrink-0 place-items-center rounded-md bg-elevated text-2xl">
                          🗳️
                        </div>
                      )}
                      <div className="min-w-0 flex-1 space-y-1">
                        <p className="truncate font-semibold">{p.title || 'Commande groupée'}</p>
                        <div className="flex items-center gap-2">
                          <Badge variant="brand" dot>
                            {STATUS_LABELS[p.status]}
                          </Badge>
                          <span className="truncate text-xs text-subtle">{formatRelativeTime(p.updated)}</span>
                        </div>
                      </div>
                      <AvatarStack users={p.expand?.members ?? []} size={24} max={3} />
                      <ChevronRight aria-hidden className="size-5 text-subtle" />
                    </Card>
                  </Link>
                </motion.li>
              ))}
            </motion.ul>
          )}
        </section>
      )}

      {user && <TeamsSection user={user} />}

      <section className="space-y-3" aria-labelledby="h-nearby">
        <div className="flex flex-wrap items-end justify-between gap-2">
          <div>
            <h2 id="h-nearby" className="font-display text-2xl font-semibold">
              À deux pas
            </h2>
            <LocationBar />
          </div>
          <Link to="/restaurants" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-brand hover:underline">
            Tous les restos <ArrowRight className="size-4" />
          </Link>
        </div>
        <div className="scrollbar-none relative -mx-4 flex snap-x snap-mandatory gap-3 overflow-x-auto px-4 pb-2 sm:-mx-8 sm:px-8" role="list" aria-label="Restaurants à proximité">
          {nearby.isPending
            ? Array.from({ length: 4 }, (_, i) => (
                <div key={i} className="w-[260px] shrink-0 snap-start" role="listitem">
                  <RestaurantCardSkeleton />
                </div>
              ))
            : (nearby.data ?? []).slice(0, 10).map((r) => (
                <div key={r.id} className="w-[260px] shrink-0 snap-start" role="listitem">
                  <RestaurantCard restaurant={r} to={`/restaurants/${r.id}`} compact />
                </div>
              ))}
          {nearby.isSuccess && nearby.data.length === 0 && <p className="text-sm text-muted">Aucun resto dans le coin pour l'instant.</p>}
          {nearby.isError && <p className="text-sm text-danger">Impossible de charger les restos à proximité.</p>}
        </div>
      </section>

      <CreatePartySheet open={createOpen} onClose={() => setCreateOpen(false)} />
    </div>
  )
}
