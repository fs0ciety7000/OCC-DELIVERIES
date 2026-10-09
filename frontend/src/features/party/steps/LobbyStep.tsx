import { Check, Search, Vote as VoteIcon, Zap } from 'lucide-react'
import { useMemo, useState } from 'react'
import { WaitingDots } from '@/components/food/WaitingDots'
import { Badge, Button, Card, CardBody, Chip, EmptyState, Input, Sheet, Skeleton } from '@/components/ui'
import { useNearby } from '@/features/restaurants/hooks'
import { LocationBar } from '@/features/restaurants/LocationBar'
import { RestaurantCard } from '@/features/restaurants/RestaurantCard'
import { RestaurantCover } from '@/features/restaurants/RestaurantCover'
import { partiesApi } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { isoInMinutes } from '@/lib/format'
import { useDebounced } from '@/lib/hooks'
import type { Restaurant } from '@/lib/types'
import { toast } from 'sonner'
import type { PartyCtx } from '../context'
import { useSetCandidates, useTransition } from '../hooks'
import { InviteCard } from '../InviteCard'
import { MemberList } from '../MemberList'

const DURATIONS = [
  { min: 0, label: 'Sans limite' },
  { min: 3, label: '3 min' },
  { min: 5, label: '5 min' },
  { min: 10, label: '10 min' },
] as const

export function LobbyStep({ ctx }: { ctx: PartyCtx }) {
  const { party, isHost } = ctx
  const [q, setQ] = useState('')
  const debounced = useDebounced(q, 300)
  const nearby = useNearby({ q: debounced, radiusKm: 10 })
  const setCandidates = useSetCandidates(party.id)
  const transition = useTransition(party.id)
  const [duration, setDuration] = useState<number>(5)
  const [directOpen, setDirectOpen] = useState(false)
  const [launching, setLaunching] = useState(false)

  const candidateIds = party.candidates ?? []
  const known = useMemo(() => {
    const map = new Map<string, Restaurant>()
    for (const r of party.expand?.candidates ?? []) map.set(r.id, r)
    for (const r of nearby.data ?? []) map.set(r.id, r)
    return map
  }, [party.expand?.candidates, nearby.data])
  const candidates = candidateIds.map((id) => known.get(id)).filter((r): r is Restaurant => !!r)

  const toggle = (id: string) => {
    const next = candidateIds.includes(id) ? candidateIds.filter((c) => c !== id) : [...candidateIds, id]
    setCandidates.mutate(next)
  }

  const launchVote = async () => {
    setLaunching(true)
    if (duration > 0) {
      try {
        await partiesApi.update(party.id, { voting_ends_at: isoInMinutes(duration) })
      } catch (err) {
        toast.error(errorMessage(err))
        setLaunching(false)
        return
      }
    }
    transition.mutate({ to: 'voting' }, { onSettled: () => setLaunching(false) })
  }

  const chooseDirect = (r: Restaurant) => transition.mutate({ to: 'ordering', restaurant: r.id }, { onSuccess: () => setDirectOpen(false) })

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_360px]">
      <div className="order-2 space-y-4 lg:order-1">
        <div className="flex flex-wrap items-end justify-between gap-2">
          <div>
            <h2 className="font-display text-xl font-semibold">On commande où ?</h2>
            <p className="text-sm text-muted">
              {isHost ? 'Choisis au moins 2 restos pour lancer le vote, ou décide directement.' : "L'hôte sélectionne les restos candidats…"}
            </p>
          </div>
          <Badge variant={candidates.length >= 2 ? 'success' : 'neutral'} className="tabular">
            {candidateIds.length} candidat{candidateIds.length > 1 ? 's' : ''}
          </Badge>
        </div>

        {candidates.length > 0 && (
          <ul className="flex flex-wrap gap-2" aria-label="Restos candidats">
            {candidates.map((r) => (
              <li key={r.id}>
                <span className="inline-flex min-h-10 items-center gap-2 rounded-full border border-brand/40 bg-brand/10 py-1 pr-1 pl-1 text-sm font-medium">
                  <RestaurantCover restaurant={r} thumb="80x80" className="size-8 rounded-full" emojiClassName="text-base" />
                  {r.name}
                  {isHost && (
                    <button type="button" onClick={() => toggle(r.id)} className="grid size-8 place-items-center rounded-full text-muted hover:bg-fg/10 hover:text-fg" aria-label={`Retirer ${r.name}`}>
                      ×
                    </button>
                  )}
                </span>
              </li>
            ))}
          </ul>
        )}

        {isHost ? (
          <>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <div className="flex-1" role="search">
                <label htmlFor="cand-search" className="sr-only">
                  Rechercher un resto
                </label>
                <Input id="cand-search" type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Chercher un resto…" leftIcon={<Search className="size-4" />} />
              </div>
              <LocationBar />
            </div>
            {nearby.isPending ? (
              <div className="grid gap-3 sm:grid-cols-2">
                {[0, 1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-56 rounded-lg" />
                ))}
              </div>
            ) : nearby.isError ? (
              <EmptyState tone="danger" emoji="📡" title="Restos indisponibles" description={errorMessage(nearby.error)} />
            ) : (nearby.data?.length ?? 0) === 0 ? (
              <EmptyState emoji="🔍" title="Aucun resto ici" description="Essaie un autre mot-clé." />
            ) : (
              <ul className="grid gap-3 sm:grid-cols-2">
                {nearby.data!.map((r) => {
                  const selected = candidateIds.includes(r.id)
                  return (
                    <li key={r.id}>
                      <RestaurantCard
                        restaurant={r}
                        compact
                        selected={selected}
                        onSelect={() => toggle(r.id)}
                        action={
                          <span
                            aria-hidden
                            className={cn(
                              'grid size-7 shrink-0 place-items-center rounded-full border-2 transition-colors',
                              selected ? 'border-brand bg-brand text-brand-fg' : 'border-border-strong',
                            )}
                          >
                            {selected && <Check className="size-4" strokeWidth={3} />}
                          </span>
                        }
                      />
                    </li>
                  )
                })}
              </ul>
            )}
          </>
        ) : (
          candidates.length === 0 && <EmptyState emoji="🤔" title="Pas encore de candidats" description="Pendant ce temps, invite d'autres collègues !" />
        )}
      </div>

      <aside className="order-1 space-y-4 lg:order-2">
        <InviteCard party={party} />
        <Card>
          <CardBody>
            <h2 className="mb-1 font-display text-lg font-semibold">Dans le salon</h2>
            <MemberList ctx={ctx} />
            {ctx.members.length < 2 ? (
              <WaitingDots label="En attente des collègues…" className="pt-3" />
            ) : (
              !isHost && <WaitingDots label="En attente du lancement du vote…" className="pt-3" />
            )}
          </CardBody>
        </Card>
        {party.notes && (
          <Card>
            <CardBody className="text-sm text-muted">
              <span className="font-semibold text-fg">Mot de l'hôte : </span>
              {party.notes}
            </CardBody>
          </Card>
        )}
      </aside>

      {isHost && (
        <div className="sticky bottom-[calc(var(--tabbar-h)+env(safe-area-inset-bottom)+12px)] z-20 order-3 lg:col-span-2">
          <Card className="space-y-3 border-border-strong bg-elevated/95 p-3 backdrop-blur-xl sm:p-4">
            <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Durée du vote">
              <span className="text-sm text-muted">Durée du vote</span>
              {DURATIONS.map((d) => (
                <Chip key={d.min} selected={duration === d.min} onClick={() => setDuration(d.min)} className="min-h-8 px-3">
                  {d.label}
                </Chip>
              ))}
            </div>
            <div className="flex flex-col gap-2 sm:flex-row">
              <Button variant="secondary" className="sm:flex-1" leftIcon={<Zap className="size-4" />} onClick={() => setDirectOpen(true)}>
                {candidates.length === 1 ? `Commander chez ${candidates[0]!.name}` : 'Choisir directement'}
              </Button>
              <Button className="sm:flex-[2]" size="lg" leftIcon={<VoteIcon className="size-5" />} disabled={candidateIds.length < 2} loading={launching} onClick={launchVote}>
                {candidateIds.length < 2 ? 'Choisis au moins 2 restos' : `Lancer le vote (${candidateIds.length})`}
              </Button>
            </div>
          </Card>
        </div>
      )}

      <Sheet open={directOpen} onClose={() => setDirectOpen(false)} title="On sait déjà !" description="Pas de vote : tout le monde commande dans ce resto." size="lg">
        <ul className="grid gap-2">
          {(candidates.length ? candidates : (nearby.data ?? [])).map((r) => (
            <li key={r.id}>
              <button
                type="button"
                onClick={() => chooseDirect(r)}
                disabled={transition.isPending}
                className="flex w-full items-center gap-3 rounded-md border border-border bg-surface p-2.5 text-left transition-colors hover:border-brand/50 disabled:opacity-60"
              >
                <RestaurantCover restaurant={r} thumb="120x120" className="size-14 shrink-0 rounded-sm" emojiClassName="text-2xl" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-semibold">{r.name}</span>
                  <span className="block truncate text-xs text-muted">{(r.cuisines ?? []).join(' · ')}</span>
                </span>
                <span className="text-sm font-semibold text-brand">Choisir</span>
              </button>
            </li>
          ))}
        </ul>
      </Sheet>
    </div>
  )
}
