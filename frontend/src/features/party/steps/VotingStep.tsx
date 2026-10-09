import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Crown, Gavel } from 'lucide-react'
import { motion } from 'motion/react'
import { Suspense, useMemo, useState } from 'react'
import { VoteBurst, WaitingRider } from '@/components/food'
import { LiquidHeart } from '@/components/food/LiquidHeart'
import { PulseOnChange } from '@/components/food/PulseOnChange'
import { toast } from 'sonner'
import { AvatarStack, Badge, Button, Card, EmptyState, Sheet, Skeleton } from '@/components/ui'
import { PartialMenuBadge } from '@/features/restaurants/PartialMenu'
import { RestaurantMeta } from '@/features/restaurants/RestaurantCard'
import { RestaurantCover } from '@/features/restaurants/RestaurantCover'
import { unvoteAction, voteAction } from '@/lib/offlineActions'
import { OFFLINE_HINT, useOnline } from '@/lib/online'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { haptic } from '@/lib/haptics'
import { itemVariants, listVariants, spring } from '@/lib/motion'
import { qk } from '@/lib/queryKeys'
import type { Restaurant, Vote } from '@/lib/types'
import { rankCandidates } from '../logic'
import type { PartyCtx } from '../context'
import { useTransition, useVotes } from '../hooks'

export function VotingStep({ ctx }: { ctx: PartyCtx }) {
  const { party, me, isHost, members, people } = ctx
  const votes = useVotes(party.id)
  const qc = useQueryClient()
  const transition = useTransition(party.id)
  const online = useOnline()
  const [force, setForce] = useState<Restaurant | null>(null)
  const [closeOpen, setCloseOpen] = useState(false)
  const [bursts, setBursts] = useState<Record<string, number>>({})

  const candidates = useMemo(() => party.expand?.candidates ?? [], [party.expand?.candidates])
  const allVotes = useMemo(() => votes.data ?? [], [votes.data])
  const ranked = useMemo(() => rankCandidates(candidates, allVotes), [candidates, allVotes])
  // Ordre d'affichage stable (celui des candidats), pour éviter que les cartes sautent pendant le vote.
  const leaderId = ranked[0] && ranked[0].count > 0 ? ranked[0].restaurant.id : null
  const voters = new Set(allVotes.map((v) => v.user))
  const maxCount = Math.max(1, members.length)
  const missing = Math.max(0, members.length - voters.size)
  const waitingOthers = voters.has(me.id) && missing > 0

  const toggle = useMutation({
    mutationFn: async (restaurantId: string) => {
      const mine = allVotes.find((v) => v.user === me.id && v.restaurant === restaurantId)
      // hors ligne : mis en file et rejoué au retour du réseau (lib/offlineActions)
      return mine ? unvoteAction(party.id, me.id, restaurantId, mine.id) : voteAction(party.id, me.id, restaurantId)
    },
    onMutate: async (restaurantId) => {
      await qc.cancelQueries({ queryKey: qk.votes(party.id) })
      const prev = qc.getQueryData<Vote[]>(qk.votes(party.id)) ?? []
      const mine = prev.find((v) => v.user === me.id && v.restaurant === restaurantId)
      qc.setQueryData<Vote[]>(
        qk.votes(party.id),
        mine ? prev.filter((v) => v !== mine) : [...prev, { id: `tmp-${restaurantId}`, party: party.id, user: me.id, restaurant: restaurantId }],
      )
      return { prev }
    },
    onError: (err, _v, ctx2) => {
      if (ctx2) qc.setQueryData(qk.votes(party.id), ctx2.prev)
      toast.error(errorMessage(err))
    },
    // en file hors ligne : on garde l'état optimiste (le cache du service worker daterait)
    onSettled: (res) => {
      if (res !== 'queued') void qc.invalidateQueries({ queryKey: qk.votes(party.id) })
    },
  })

  if (votes.isPending) {
    return (
      <div className="grid gap-4 sm:grid-cols-2">
        {[0, 1].map((i) => (
          <Skeleton key={i} className="h-72 rounded-lg" />
        ))}
      </div>
    )
  }

  return (
    <div className="space-y-5 pb-28">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-display text-xl font-semibold">Vote pour tes restos préférés</h2>
          <p className="text-sm text-muted">Tu peux liker plusieurs restos. Le plus aimé l'emporte.</p>
        </div>
        <div className="flex items-center gap-2">
          <PulseOnChange value={voters.size}>
            <Badge className="tabular" aria-live="polite">
              {voters.size}/{members.length} ont voté
            </Badge>
          </PulseOnChange>
        </div>
      </div>

      {candidates.length === 0 ? (
        <EmptyState emoji="🗳️" title="Aucun candidat" description="L'hôte doit rouvrir le salon pour choisir des restos." />
      ) : (
        <motion.ul variants={listVariants} initial="hidden" animate="show" className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {candidates.map((r) => {
            const rv = allVotes.filter((v) => v.restaurant === r.id)
            const mine = rv.some((v) => v.user === me.id)
            const leader = r.id === leaderId
            const pct = Math.round((rv.length / maxCount) * 100)
            const voterUsers = rv.map((v) => people.get(v.user) ?? { id: v.user, name: '' })
            return (
              <motion.li key={r.id} variants={itemVariants}>
                <Card variant={mine ? 'selected' : 'default'} className="flex h-full flex-col overflow-hidden">
                  <div className="relative">
                    <RestaurantCover restaurant={r} className="aspect-[16/8] w-full" />
                    {leader && (
                      <Badge variant="brand" className="absolute top-3 left-3 bg-bg/80 backdrop-blur">
                        <Crown aria-hidden className="size-3" /> En tête
                      </Badge>
                    )}
                  </div>
                  <div className="flex flex-1 flex-col gap-3 p-4">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <h3 className="font-display text-lg leading-6 font-semibold">{r.name}</h3>
                        <RestaurantMeta restaurant={r} className="mt-1" />
                        <PartialMenuBadge restaurant={r} className="mt-2" />
                      </div>
                      <span className="relative shrink-0">
                      <Suspense fallback={null}>
                        <VoteBurst trigger={bursts[r.id] ?? 0} emojis={[r.emoji || '🍽️', '❤️', '✨']} />
                      </Suspense>
                      <motion.button
                        type="button"
                        whileTap={{ scale: 0.85 }}
                        onClick={() => {
                          if (!mine) setBursts((b) => ({ ...b, [r.id]: (b[r.id] ?? 0) + 1 }))
                          haptic('vote')
                          toggle.mutate(r.id)
                        }}
                        aria-pressed={mine}
                        aria-label={mine ? `Retirer mon vote pour ${r.name}` : `Voter pour ${r.name}`}
                        className={cn(
                          'grid size-12 shrink-0 place-items-center rounded-full border transition-colors',
                          mine ? 'border-brand/50 bg-brand/15 text-brand' : 'border-border-strong text-muted hover:text-fg',
                        )}
                      >
                        <LiquidHeart filled={mine} />
                      </motion.button>
                      </span>
                    </div>
                    <div className="mt-auto space-y-2">
                      <div className="h-2 overflow-hidden rounded-full bg-fg/[0.08]" role="meter" aria-valuemin={0} aria-valuemax={maxCount} aria-valuenow={rv.length} aria-label={`${rv.length} vote(s) pour ${r.name}`}>
                        <motion.div className={cn('h-full rounded-full', leader ? 'bg-ember' : 'bg-fg/30')} initial={false} animate={{ width: `${pct}%` }} transition={spring} />
                      </div>
                      <div className="flex min-h-6 items-center justify-between gap-2">
                        <span className="text-sm font-semibold tabular">
                          {rv.length} vote{rv.length > 1 ? 's' : ''}
                        </span>
                        {voterUsers.length > 0 && <AvatarStack users={voterUsers} size={24} max={5} />}
                      </div>
                    </div>
                    {isHost && (
                      <Button variant="ghost" size="sm" leftIcon={<Gavel className="size-4" />} onClick={() => setForce(r)} className="self-start">
                        Imposer ce resto
                      </Button>
                    )}
                  </div>
                </Card>
              </motion.li>
            )
          })}
        </motion.ul>
      )}

      {waitingOthers && (
        <div className="flex items-center gap-3 rounded-lg border border-border bg-surface p-3 sm:p-4" role="status">
          <Suspense fallback={<div className="h-20 w-30 shrink-0" />}>
            <WaitingRider className="h-20 w-30 shrink-0" />
          </Suspense>
          <div className="min-w-0">
            <p className="font-semibold">On attend les autres…</p>
            <p className="text-sm text-muted">
              {missing} {missing > 1 ? 'collègues n’ont' : 'collègue n’a'} pas encore voté.
              {!isHost && ' L’hôte clôturera le vote ensuite.'}
            </p>
          </div>
        </div>
      )}

      {isHost ? (
        <div className="fixed inset-x-0 bottom-[calc(var(--tabbar-h)+env(safe-area-inset-bottom))] z-30 border-t border-border bg-bg/85 p-3 backdrop-blur-xl md:static md:border-0 md:bg-transparent md:p-0">
          <div className="mx-auto flex max-w-[1200px] items-center gap-3">
            <p className="hidden flex-1 text-sm text-muted sm:block">
              {leaderId ? (
                <>
                  En tête : <strong className="text-fg">{ranked[0]?.restaurant.name}</strong>
                </>
              ) : (
                'Pas encore de vote.'
              )}
            </p>
            <Button size="lg" className="flex-1 sm:flex-none" onClick={() => setCloseOpen(true)} disabled={candidates.length === 0 || !online} title={online ? undefined : OFFLINE_HINT}>
              Clore le vote
            </Button>
          </div>
        </div>
      ) : (
        !waitingOthers && <p className="text-center text-sm text-muted">L'hôte clôturera le vote quand tout le monde aura donné son avis.</p>
      )}

      <Sheet
        open={closeOpen}
        onClose={() => setCloseOpen(false)}
        title="Clore le vote ?"
        description="Le resto avec le plus de votes l'emporte (égalité : meilleure note)."
        footer={
          <Button block size="lg" loading={transition.isPending} onClick={() => transition.mutate({ to: 'ordering' }, { onSuccess: () => setCloseOpen(false) })}>
            {ranked[0] ? `Valider — ${ranked[0].restaurant.name}` : 'Valider'}
          </Button>
        }
      >
        <ol className="space-y-2">
          {ranked.map((r, i) => (
            <li key={r.restaurant.id} className="flex items-center gap-3 rounded-md border border-border bg-surface p-2.5">
              <span className="w-5 text-center font-display font-bold text-muted tabular">{i + 1}</span>
              <span className="flex-1 truncate font-medium">{r.restaurant.name}</span>
              <span className="text-sm tabular text-muted">
                {r.count} vote{r.count > 1 ? 's' : ''}
              </span>
            </li>
          ))}
        </ol>
        {voters.size < members.length && (
          <p className="mt-3 text-sm text-warning">
            {members.length - voters.size} membre{members.length - voters.size > 1 ? 's n’ont' : ' n’a'} pas encore voté.
          </p>
        )}
      </Sheet>

      <Sheet
        open={!!force}
        onClose={() => setForce(null)}
        title={`Imposer ${force?.name ?? ''} ?`}
        description="Le vote s'arrête et tout le monde passe commande dans ce resto."
        footer={
          <div className="flex gap-2">
            <Button variant="secondary" className="flex-1" onClick={() => setForce(null)}>
              Annuler
            </Button>
            <Button
              className="flex-1"
              loading={transition.isPending}
              onClick={() => force && transition.mutate({ to: 'ordering', restaurant: force.id }, { onSuccess: () => setForce(null) })}
            >
              Imposer
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted">Les votes resteront visibles dans l'historique.</p>
      </Sheet>
    </div>
  )
}
