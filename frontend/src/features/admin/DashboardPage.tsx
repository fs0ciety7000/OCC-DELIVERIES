import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { Badge, Button, Card, CardBody, EmptyState, Money, Skeleton } from '@/components/ui'
import { adminApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import { AdminHeader } from './AdminLayout'
import { BarChart } from './BarChart'
import { dayLabels, STATUS_LABEL, STATUS_VARIANT, STATUSES } from './labels'

const ONGOING = ['lobby', 'voting', 'ordering', 'review', 'paying'] as const

function Tile({ label, value, hint }: { label: string; value: ReactNode; hint?: ReactNode }) {
  return (
    <Card>
      <CardBody className="space-y-1">
        <p className="text-sm text-muted">{label}</p>
        <p className="font-display text-[32px] leading-9 font-bold tabular-nums">{value}</p>
        {hint && <p className="text-xs text-subtle">{hint}</p>}
      </CardBody>
    </Card>
  )
}

export function DashboardPage() {
  const stats = useQuery({ queryKey: qk.admin.stats, queryFn: adminApi.stats, staleTime: 30_000 })

  if (stats.isError) {
    return (
      <EmptyState
        tone="danger"
        emoji="⚠️"
        title="Statistiques indisponibles"
        description={errorMessage(stats.error)}
        action={<Button variant="secondary" onClick={() => stats.refetch()}>Réessayer</Button>}
      />
    )
  }
  const s = stats.data
  const ongoing = s ? ONGOING.reduce((n, k) => n + (s.parties.byStatus[k] ?? 0), 0) : 0
  const last30 = s ? s.partiesPerDay.reduce((n, d) => n + d.count, 0) : 0

  return (
    <div className="space-y-6">
      <AdminHeader title="Tableau de bord" description="Vue d'ensemble d'OCC Deliveries." />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {s ? (
          <>
            <Tile label="Utilisateurs" value={s.users} hint={plural(s.admins, 'admin')} />
            <Tile label="Restaurants actifs" value={s.restaurants.active} hint={`${s.restaurants.total} au total · ${s.menuItems} plats`} />
            <Tile label="Commandes" value={s.parties.total} hint={`${ongoing} en cours`} />
            <Tile label="Montant commandé" value={<Money cents={s.orderedTotal} />} hint="hors commandes annulées" />
          </>
        ) : (
          Array.from({ length: 4 }, (_, i) => <Skeleton key={i} className="h-[118px] rounded-lg" />)
        )}
      </div>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <Card>
          <CardBody>
            <div className="mb-3 flex items-baseline justify-between gap-3">
              <h2 className="font-display text-lg font-semibold">Commandes créées par jour</h2>
              <p className="text-sm text-muted tabular-nums">{s ? `${last30} sur 30 jours` : ''}</p>
            </div>
            {s ? (
              <BarChart
                label="Commandes créées par jour (30 derniers jours)"
                data={s.partiesPerDay.map((d) => {
                  const l = dayLabels(d.date)
                  return { label: l.short, title: l.long, value: d.count }
                })}
              />
            ) : (
              <Skeleton className="h-[140px]" />
            )}
          </CardBody>
        </Card>

        <Card>
          <CardBody>
            <h2 className="mb-3 font-display text-lg font-semibold">Par statut</h2>
            <ul className="space-y-2">
              {STATUSES.map((st) => (
                <li key={st} className="flex items-center justify-between gap-3">
                  <Link to={`/admin/commandes?status=${st}`} className="rounded-sm">
                    <Badge variant={STATUS_VARIANT[st]} dot>
                      {STATUS_LABEL[st]}
                    </Badge>
                  </Link>
                  <span className="font-semibold tabular-nums">{s ? s.parties.byStatus[st] ?? 0 : '–'}</span>
                </li>
              ))}
            </ul>
          </CardBody>
        </Card>
      </div>

      <Card>
        <CardBody>
          <h2 className="mb-3 font-display text-lg font-semibold">Restaurants les plus commandés</h2>
          {s && s.topRestaurants.length === 0 && <p className="text-sm text-muted">Aucune commande pour l'instant.</p>}
          <ol className="divide-y divide-border">
            {s?.topRestaurants.map((r, i) => (
              <li key={r.id} className="flex items-center gap-3 py-2.5">
                <span className="w-5 text-sm text-subtle tabular-nums">{i + 1}</span>
                <span aria-hidden className="text-xl">{r.emoji || '🍽️'}</span>
                <Link to={`/admin/restaurants/${r.id}`} className="min-w-0 flex-1 truncate font-semibold hover:underline">
                  {r.name}
                </Link>
                <span className="text-sm text-muted">{plural(r.parties, 'commande')}</span>
                <Money cents={r.amount} className="w-24 text-right font-semibold" />
              </li>
            ))}
          </ol>
        </CardBody>
      </Card>
    </div>
  )
}
