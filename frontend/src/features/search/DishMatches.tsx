import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router'
import { Button, Money, Skeleton } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { Highlight } from './Highlight'
import { useSearch } from './useSearch'

/**
 * « Plats correspondants » (page Restos) : mêmes résultats que la recherche globale,
 * chaque plat ouvre son restaurant, défilé jusqu'à lui. Rien n'est affiché sans saisie
 * (≥ 2 caractères) ni quand aucun plat ne correspond (la liste des restos gère son état vide).
 */
export function DishMatches({ q }: { q: string }) {
  const query = q.trim()
  const enabled = query.length >= 2
  const search = useSearch(query, { limit: 6, enabled })
  if (!enabled) return null
  const fresh = search.data?.query === query ? search.data : undefined

  if (search.isError && !fresh) {
    return (
      <section aria-labelledby="dish-matches" className="space-y-2">
        <h2 id="dish-matches" className="font-display text-lg font-semibold">
          Plats correspondants
        </h2>
        <p role="alert" className="flex flex-wrap items-center gap-3 text-sm text-muted">
          {errorMessage(search.error, 'Impossible de chercher dans les menus.')}
          <Button variant="ghost" size="sm" onClick={() => search.refetch()}>
            Réessayer
          </Button>
        </p>
      </section>
    )
  }
  if (!fresh) {
    return (
      <section aria-label="Plats correspondants" aria-busy="true" className="space-y-2">
        <Skeleton className="h-6 w-48" />
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-16 rounded-lg" />
          ))}
        </div>
      </section>
    )
  }
  if (fresh.dishes.length === 0) return null

  return (
    <section aria-labelledby="dish-matches" className="space-y-2">
      <h2 id="dish-matches" className="flex items-baseline gap-2 font-display text-lg font-semibold">
        Plats correspondants
        {fresh.fuzzy && <span className="font-sans text-xs font-normal text-subtle">résultats approchants</span>}
      </h2>
      <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {fresh.dishes.map((d) => (
          <li key={d.id}>
            <Link
              to={`/restaurants/${d.restaurant.id}?plat=${encodeURIComponent(d.id)}`}
              className="flex min-h-16 items-center gap-3 rounded-lg border border-border bg-surface p-3 shadow-card transition-colors duration-[120ms] hover:border-border-strong"
            >
              <span aria-hidden className="grid size-10 shrink-0 place-items-center rounded-md bg-elevated text-xl">
                {d.emoji || d.restaurant.emoji || '🍽️'}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate font-semibold">
                  <Highlight text={d.name} terms={fresh.terms} />
                </span>
                <span className="block truncate text-xs text-muted">
                  Chez {d.restaurant.name} · <Money cents={d.price} />
                </span>
              </span>
              <ChevronRight className="size-4 shrink-0 text-subtle" aria-hidden />
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
