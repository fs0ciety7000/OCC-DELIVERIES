import { Search } from 'lucide-react'
import { motion } from 'motion/react'
import { useMemo, useState } from 'react'
import { Button, Chip, EmptyState, Input } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { useDebounced } from '@/lib/hooks'
import { itemVariants, listVariants } from '@/lib/motion'
import { plural } from '@/lib/format'
import { useNearby } from './hooks'
import { LocationBar } from './LocationBar'
import { RestaurantCard, RestaurantCardSkeleton } from './RestaurantCard'
import { cuisineLabel } from './visual'

const RADII = [2, 5, 10] as const

export function RestaurantsPage() {
  const [q, setQ] = useState('')
  const [cuisine, setCuisine] = useState<string | null>(null)
  const [radiusKm, setRadiusKm] = useState<number>(5)
  const debounced = useDebounced(q, 300)
  const nearby = useNearby({ q: debounced, radiusKm })

  const all = useMemo(() => nearby.data ?? [], [nearby.data])
  const cuisines = useMemo(() => {
    const counts = new Map<string, number>()
    for (const r of all) for (const c of r.cuisines ?? []) counts.set(c.toLowerCase(), (counts.get(c.toLowerCase()) ?? 0) + 1)
    return [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c)
  }, [all])
  const list = cuisine ? all.filter((r) => (r.cuisines ?? []).some((c) => c.toLowerCase() === cuisine)) : all

  return (
    <div className="space-y-5">
      <header className="space-y-2">
        <h1 className="font-display text-[32px] leading-9 font-bold">Restos à proximité</h1>
        <LocationBar />
      </header>

      <div className="space-y-3">
        <div role="search">
          <label htmlFor="resto-search" className="sr-only">
            Rechercher un restaurant
          </label>
          <Input
            id="resto-search"
            type="search"
            placeholder="Pizza, sushi, nom du resto…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            leftIcon={<Search className="size-4" />}
            autoComplete="off"
          />
        </div>
        <div className="scrollbar-none -mx-4 flex gap-2 overflow-x-auto px-4 sm:mx-0 sm:flex-wrap sm:px-0" role="group" aria-label="Filtrer par cuisine">
          <Chip selected={cuisine === null} onClick={() => setCuisine(null)}>
            Tout
          </Chip>
          {cuisines.map((c) => (
            <Chip key={c} selected={cuisine === c} onClick={() => setCuisine(cuisine === c ? null : c)}>
              {cuisineLabel(c)}
            </Chip>
          ))}
        </div>
        <div className="flex items-center gap-2 text-sm text-muted" role="group" aria-label="Rayon de recherche">
          <span>Rayon</span>
          {RADII.map((r) => (
            <Chip key={r} selected={radiusKm === r} onClick={() => setRadiusKm(r)} className="min-h-8 px-3">
              {r} km
            </Chip>
          ))}
          {nearby.data && (
            <span className="ml-auto text-xs" aria-live="polite">
              {plural(list.length, 'resto')}
            </span>
          )}
        </div>
      </div>

      {nearby.isPending ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }, (_, i) => (
            <RestaurantCardSkeleton key={i} />
          ))}
        </div>
      ) : nearby.isError ? (
        <EmptyState
          tone="danger"
          emoji="📡"
          title="Impossible de charger les restos"
          description={errorMessage(nearby.error)}
          action={<Button onClick={() => nearby.refetch()}>Réessayer</Button>}
        />
      ) : list.length === 0 ? (
        <EmptyState
          emoji="🔍"
          title="Aucun resto trouvé"
          description={q || cuisine ? 'Essaie une autre recherche ou élargis le rayon.' : 'Pas de restaurant dans cette zone pour le moment.'}
          action={
            (q || cuisine || radiusKm < 10) && (
              <Button
                variant="secondary"
                onClick={() => {
                  setQ('')
                  setCuisine(null)
                  setRadiusKm(10)
                }}
              >
                Élargir la recherche
              </Button>
            )
          }
        />
      ) : (
        <motion.ul variants={listVariants} initial="hidden" animate="show" className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {list.map((r) => (
            <motion.li key={r.id} variants={itemVariants}>
              <RestaurantCard restaurant={r} to={`/restaurants/${r.id}`} />
            </motion.li>
          ))}
        </motion.ul>
      )}
    </div>
  )
}
