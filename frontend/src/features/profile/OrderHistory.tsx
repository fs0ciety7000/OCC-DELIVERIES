import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { motion } from 'motion/react'
import { useState, type ReactNode } from 'react'
import { Button, Card, EmptyState, Money, Skeleton } from '@/components/ui'
import { CreatePartySheet } from '@/features/party/CreatePartySheet'
import { occ } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { plural } from '@/lib/format'
import { itemVariants, listVariants } from '@/lib/motion'
import { qk } from '@/lib/queryKeys'
import type { HistoryRestaurant } from '@/lib/types'
import { HistoryCard } from './HistoryCard'

/** Onglet « Mes commandes » du profil : statistiques + historique paginé (10 par page). */
export function OrderHistory({ userId }: { userId: string }) {
  const [relaunch, setRelaunch] = useState<HistoryRestaurant | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const history = useInfiniteQuery({
    queryKey: qk.history(userId),
    queryFn: ({ pageParam }) => occ.history(pageParam),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page < last.totalPages ? last.page + 1 : undefined),
  })
  const entries = history.data?.pages.flatMap((p) => p.items) ?? []
  const total = history.data?.pages[0]?.totalItems ?? 0

  return (
    <div className="space-y-5">
      <StatsHeader userId={userId} />

      {history.isPending ? (
        <div className="space-y-3" aria-busy="true" aria-label="Chargement de mes commandes">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-32 rounded-lg" />
          ))}
        </div>
      ) : history.isError ? (
        <Card>
          <EmptyState tone="danger" emoji="📡" title="Historique indisponible" description={errorMessage(history.error)} action={<Button onClick={() => history.refetch()}>Réessayer</Button>} />
        </Card>
      ) : entries.length === 0 ? (
        <Card>
          <EmptyState
            title="Pas encore de commande"
            description="Tes commandes groupées apparaîtront ici, avec tes plats et ce que tu as payé."
            action={
              <Button variant="secondary" leftIcon={<Plus className="size-4" />} onClick={() => setCreateOpen(true)}>
                Lancer une commande
              </Button>
            }
          />
        </Card>
      ) : (
        <section aria-labelledby="h-history" className="space-y-3">
          <h2 id="h-history" className="sr-only">
            Mes commandes
          </h2>
          <p className="text-sm text-muted tabular" aria-live="polite">
            {plural(total, 'commande')} · plus récentes d'abord
          </p>
          <motion.ul variants={listVariants} initial="hidden" animate="show" className="space-y-3">
            {entries.map((e) => (
              <motion.li key={e.id} variants={itemVariants}>
                <HistoryCard entry={e} onRelaunch={setRelaunch} />
              </motion.li>
            ))}
          </motion.ul>
          {history.hasNextPage && (
            <Button variant="secondary" block loading={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>
              Voir plus de commandes
            </Button>
          )}
        </section>
      )}

      <CreatePartySheet open={createOpen} onClose={() => setCreateOpen(false)} />
      <CreatePartySheet open={!!relaunch} onClose={() => setRelaunch(null)} restaurant={relaunch ?? undefined} />
    </div>
  )
}

function StatsHeader({ userId }: { userId: string }) {
  const stats = useQuery({ queryKey: qk.myStats(userId), queryFn: occ.myStats })
  if (stats.isPending) {
    return (
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4" aria-busy="true" aria-label="Chargement des statistiques">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-20 rounded-lg" />
        ))}
      </div>
    )
  }
  if (stats.isError) return <p className="text-sm text-danger">Statistiques indisponibles : {errorMessage(stats.error)}</p>
  const s = stats.data
  // Rien à résumer tant qu'aucune commande n'est terminée : la liste (ou l'état vide) suffit.
  if (s.orders === 0) return null
  const fr = s.favoriteRestaurant
  const fd = s.favoriteDish
  return (
    <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4" aria-label="Mes statistiques">
      <Stat label="Commandes passées" value={<span className="tabular">{s.orders}</span>} />
      <Stat label="Dépensé" value={<Money cents={s.totalSpent} />} />
      <Stat
        label="Resto chouchou"
        value={fr ? <span className="line-clamp-2 text-base leading-5">{`${fr.emoji ? `${fr.emoji} ` : ''}${fr.name}`}</span> : '—'}
        meta={fr ? `${plural(fr.orders, 'commande')}` : undefined}
      />
      <Stat label="Plat préféré" value={fd ? <span className="line-clamp-2 text-base leading-5">{fd.name}</span> : '—'} meta={fd ? `${fd.quantity}× commandé` : undefined} />
    </dl>
  )
}

function Stat({ label, value, meta }: { label: string; value: ReactNode; meta?: string }) {
  return (
    <div className="rounded-lg border border-border bg-surface p-3 shadow-card">
      <dt className="text-xs font-medium text-subtle">{label}</dt>
      <dd className="mt-1 font-display text-2xl leading-7 font-bold">{value}</dd>
      {meta && <dd className="mt-0.5 text-xs text-muted tabular">{meta}</dd>}
    </div>
  )
}
