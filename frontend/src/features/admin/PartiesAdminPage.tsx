import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Ban, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Avatar, Badge, Button, buttonClass, Card, CardBody, Chip, EmptyState, Money, Sheet, Skeleton } from '@/components/ui'
import { adminApi, partiesApi } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { formatRelativeTime, plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { Party, PartyStatus } from '@/lib/types'
import { AdminHeader } from './AdminLayout'
import { STATUS_LABEL, STATUS_VARIANT, STATUSES } from './labels'

const cancellable = (s: PartyStatus) => s !== 'closed' && s !== 'cancelled'

function useCancel(onDone?: () => void) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => adminApi.cancelParty(id),
    onSuccess: () => {
      toast.success('Commande annulée')
      void qc.invalidateQueries({ queryKey: qk.admin.all })
      onDone?.()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
}

function CancelSheet({ party, onClose }: { party: Party | null; onClose: () => void }) {
  const cancel = useCancel(onClose)
  return (
    <Sheet
      open={!!party}
      onClose={onClose}
      title="Annuler cette commande ?"
      description={party ? `« ${party.title || party.code} » passera en « Annulée » pour tous ses membres. Action irréversible.` : undefined}
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>
            Garder
          </Button>
          <Button variant="danger" loading={cancel.isPending} onClick={() => party && cancel.mutate(party.id)}>
            Forcer l'annulation
          </Button>
        </div>
      }
    >
      <span className="sr-only">Confirmation d'annulation</span>
    </Sheet>
  )
}

export function PartiesAdminPage() {
  const [params, setParams] = useSearchParams()
  const status = (STATUSES as string[]).includes(params.get('status') ?? '') ? (params.get('status') as PartyStatus) : ''
  const [page, setPage] = useState(1)
  const [toCancel, setToCancel] = useState<Party | null>(null)
  const list = useQuery({ queryKey: qk.admin.parties(status, page), queryFn: () => adminApi.parties(status, page), placeholderData: keepPreviousData })

  const setStatus = (s: PartyStatus | '') => {
    setPage(1)
    setParams(s ? { status: s } : {}, { replace: true })
  }

  return (
    <div>
      <AdminHeader title="Commandes" description={list.data ? plural(list.data.totalItems, 'commande') : undefined} />
      <div className="relative -mx-4 mb-4 flex gap-2 overflow-x-auto px-4 pb-1" role="group" aria-label="Filtrer par statut">
        <Chip selected={status === ''} onClick={() => setStatus('')}>
          Toutes
        </Chip>
        {STATUSES.map((s) => (
          <Chip key={s} selected={status === s} onClick={() => setStatus(s)}>
            {STATUS_LABEL[s]}
          </Chip>
        ))}
      </div>

      {list.isPending && <Skeleton className="h-64 rounded-lg" />}
      {list.isError && <EmptyState tone="danger" emoji="⚠️" title="Chargement impossible" description={errorMessage(list.error)} />}
      {list.data?.items.length === 0 && <EmptyState title="Aucune commande" description={status ? `Aucune commande « ${STATUS_LABEL[status]} ».` : 'Les commandes groupées apparaîtront ici.'} />}

      <ul className="space-y-2">
        {list.data?.items.map((p) => (
          <li key={p.id}>
            <Card className="flex items-center gap-3 p-3 sm:p-4">
              <Link to={`/admin/commandes/${p.id}`} className="flex min-w-0 flex-1 items-center gap-3 rounded-md">
                <span aria-hidden className="text-xl">{p.expand?.restaurant?.emoji || '🛍️'}</span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate font-semibold">{p.title || 'Sans titre'}</span>
                    <Badge variant={STATUS_VARIANT[p.status]} dot>
                      {STATUS_LABEL[p.status]}
                    </Badge>
                  </div>
                  <p className="truncate text-xs text-subtle">
                    {p.code} · {p.expand?.host?.name ?? 'hôte inconnu'} · {plural(p.members.length, 'membre')}
                    {p.expand?.restaurant ? ` · ${p.expand.restaurant.name}` : ''} · {formatRelativeTime(p.created)}
                  </p>
                </div>
                <ChevronRight className="size-4 shrink-0 text-subtle" aria-hidden />
              </Link>
              {cancellable(p.status) && (
                <Button variant="ghost" size="icon" aria-label={`Annuler ${p.title || p.code}`} onClick={() => setToCancel(p)}>
                  <Ban className="size-4" />
                </Button>
              )}
            </Card>
          </li>
        ))}
      </ul>
      {list.data && list.data.totalPages > 1 && (
        <div className="mt-4 flex items-center justify-center gap-3">
          <Button variant="secondary" size="sm" disabled={page <= 1} onClick={() => setPage(page - 1)}>
            Précédent
          </Button>
          <span className="text-sm text-muted tabular-nums">
            {page} / {list.data.totalPages}
          </span>
          <Button variant="secondary" size="sm" disabled={page >= list.data.totalPages} onClick={() => setPage(page + 1)}>
            Suivant
          </Button>
        </div>
      )}
      <CancelSheet party={toCancel} onClose={() => setToCancel(null)} />
    </div>
  )
}

export function PartyAdminDetailPage() {
  const { id = '' } = useParams()
  const [cancelOpen, setCancelOpen] = useState(false)
  const detail = useQuery({
    queryKey: qk.admin.party(id),
    queryFn: async () => {
      const [party, members, items, payments] = await Promise.all([partiesApi.get(id), partiesApi.members(id), partiesApi.orderItems(id), partiesApi.payments(id)])
      return { party, members, items, payments }
    },
    enabled: !!id,
  })
  if (detail.isPending) return <Skeleton className="h-80 rounded-lg" />
  if (detail.isError) {
    return <EmptyState tone="danger" emoji="🛍️" title="Commande introuvable" description={errorMessage(detail.error)} action={<Link to="/admin/commandes" className={buttonClass('secondary')}>Retour</Link>} />
  }
  const { party, members, items, payments } = detail.data
  const total = items.reduce((n, i) => n + i.total, 0)
  const nameOf = (uid: string) => members.find((m) => m.user === uid)?.expand?.user?.name ?? 'Ancien membre'

  return (
    <div className="space-y-4">
      <Link to="/admin/commandes" className={cn(buttonClass('ghost', 'sm'), '-ml-3')}>
        <ArrowLeft className="size-4" aria-hidden /> Commandes
      </Link>
      <AdminHeader
        title={party.title || 'Sans titre'}
        description={
          <span className="flex flex-wrap items-center gap-2">
            <Badge variant={STATUS_VARIANT[party.status]} dot>
              {STATUS_LABEL[party.status]}
            </Badge>
            <span>
              Code {party.code} · créée {formatRelativeTime(party.created)}
              {party.expand?.restaurant ? ` · ${party.expand.restaurant.name}` : ''}
            </span>
          </span>
        }
        actions={
          cancellable(party.status) && (
            <Button variant="danger" size="sm" leftIcon={<Ban className="size-4" />} onClick={() => setCancelOpen(true)}>
              Forcer l'annulation
            </Button>
          )
        }
      />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardBody>
            <h2 className="mb-3 font-display text-lg font-semibold">Membres ({members.length})</h2>
            <ul className="space-y-2">
              {members.map((m) => (
                <li key={m.id} className="flex items-center gap-3">
                  {m.expand?.user && <Avatar user={m.expand.user} size={32} decorative />}
                  <span className="flex-1 truncate">{m.expand?.user?.name ?? m.user}</span>
                  {m.role === 'host' && <Badge variant="brand">Hôte</Badge>}
                  {m.ready && <Badge variant="success">Prêt</Badge>}
                </li>
              ))}
            </ul>
          </CardBody>
        </Card>
        <Card>
          <CardBody>
            <h2 className="mb-3 font-display text-lg font-semibold">Paiements</h2>
            {payments.length === 0 ? (
              <p className="text-sm text-muted">Pas encore de payeur désigné.</p>
            ) : (
              <ul className="space-y-2">
                {payments.map((p) => (
                  <li key={p.id} className="flex items-center gap-3 text-sm">
                    <span className="flex-1 truncate">
                      {p.expand?.debtor?.name ?? nameOf(p.debtor)} → {p.expand?.creditor?.name ?? nameOf(p.creditor)}
                    </span>
                    <Badge variant={p.status === 'confirmed' ? 'success' : p.status === 'declared' ? 'warning' : 'neutral'}>
                      {p.status === 'confirmed' ? 'Confirmé' : p.status === 'declared' ? 'Déclaré' : 'En attente'}
                    </Badge>
                    <Money cents={p.amount} className="font-semibold" />
                  </li>
                ))}
              </ul>
            )}
          </CardBody>
        </Card>
      </div>
      <Card>
        <CardBody>
          <div className="mb-3 flex items-baseline justify-between">
            <h2 className="font-display text-lg font-semibold">Articles ({items.length})</h2>
            <Money cents={total} className="font-semibold" />
          </div>
          {items.length === 0 ? (
            <p className="text-sm text-muted">Aucun article.</p>
          ) : (
            <ul className="divide-y divide-border text-sm">
              {items.map((i) => (
                <li key={i.id} className="flex gap-3 py-2">
                  <span className="w-8 text-subtle tabular-nums">{i.quantity}×</span>
                  <span className="min-w-0 flex-1">
                    <span className="font-medium">{i.name}</span>
                    {i.options_label && <span className="text-muted"> · {i.options_label}</span>}
                    <span className="block text-xs text-subtle">{nameOf(i.user)}{i.note ? ` · « ${i.note} »` : ''}</span>
                  </span>
                  <Money cents={i.total} />
                </li>
              ))}
            </ul>
          )}
        </CardBody>
      </Card>
      {cancelOpen && <CancelSheet party={party} onClose={() => (setCancelOpen(false), void detail.refetch())} />}
    </div>
  )
}
