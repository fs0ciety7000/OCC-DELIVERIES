import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronUp, Crown, Gavel, Trophy, X } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { Suspense, useEffect, useMemo, useRef, useState } from 'react'
import { VoteBurst, WaitingRider } from '@/components/food'
import { LiquidHeart } from '@/components/food/LiquidHeart'
import { PulseOnChange } from '@/components/food/PulseOnChange'
import { toast } from 'sonner'
import { AvatarStack, Badge, Button, Card, EmptyState, Sheet, Skeleton } from '@/components/ui'
import { PartialMenuBadge } from '@/features/restaurants/PartialMenu'
import { RestaurantMeta } from '@/features/restaurants/RestaurantCard'
import { RestaurantCover } from '@/features/restaurants/RestaurantCover'
import { ballotAction } from '@/lib/offlineActions'
import { OFFLINE_HINT, useOnline } from '@/lib/online'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { plural } from '@/lib/format'
import { haptic } from '@/lib/haptics'
import { itemVariants, listVariants, spring } from '@/lib/motion'
import { qk } from '@/lib/queryKeys'
import type { Restaurant, Vote } from '@/lib/types'
import { moveRanked, myRanking, ordinal, rankPoints, standingsOf, toggleRanked, withMyRanking } from '../logic'
import type { PartyCtx } from '../context'
import { useTally, useTransition, useVotes } from '../hooks'

const pts = (n: number) => plural(n, 'pt', 'pts')

export function VotingStep({ ctx }: { ctx: PartyCtx }) {
  const { party, me, isHost, members, people } = ctx
  const votes = useVotes(party.id)
  const tally = useTally(party.id)
  const qc = useQueryClient()
  const transition = useTransition(party.id)
  const online = useOnline()
  const [force, setForce] = useState<Restaurant | null>(null)
  const [closeOpen, setCloseOpen] = useState(false)
  const [bursts, setBursts] = useState<Record<string, number>>({})
  const [announce, setAnnounce] = useState('')
  const focusAfter = useRef<{ id: string; dir: 'up' | 'down' } | null>(null)
  const rankingRef = useRef<HTMLOListElement>(null)

  const candidates = useMemo(() => party.expand?.candidates ?? [], [party.expand?.candidates])
  const allVotes = useMemo(() => votes.data ?? [], [votes.data])
  const serverRanking = useMemo(() => myRanking(allVotes, me.id), [allVotes, me.id])
  // bulletin en cours d'envoi : prioritaire sur les relectures temps réel intermédiaires
  // (sinon un 3e geste partirait d'un état périmé)
  const [pending, setPending] = useState<string[] | null>(null)
  const ranking = pending ?? serverRanking
  const standings = useMemo(() => standingsOf(candidates, tally.data), [candidates, tally.data])
  const byRestaurant = useMemo(() => new Map(standings.map((s) => [s.restaurant, s])), [standings])
  const names = useMemo(() => new Map(candidates.map((r) => [r.id, r.name])), [candidates])
  // K : points d'un 1er choix (nombre de candidats, fourni par le serveur)
  const k = tally.data?.candidates || candidates.length
  const leader = standings[0] && standings[0].points > 0 ? standings[0] : null
  const voters = new Set(allVotes.map((v) => v.user))
  // jauge : part des points possibles (tout le monde met ce resto en 1er choix)
  const maxPoints = Math.max(1, k * Math.max(1, members.length))
  const missing = Math.max(0, members.length - voters.size)
  const waitingOthers = voters.has(me.id) && missing > 0

  const ballotKey = ['ballot', party.id]
  const save = useMutation({
    mutationKey: ballotKey,
    // un bulletin après l'autre, dans l'ordre des gestes (chaque envoi est l'état complet)
    scope: { id: `ballot-${party.id}` },
    // hors ligne : mis en file (dernier bulletin seulement) et rejoué au retour du réseau
    mutationFn: (next: string[]) => ballotAction(party.id, next),
    onError: (err) => toast.error(errorMessage(err)),
    // une fois le dernier bulletin traité : relecture des votes + classement (sauf en file hors
    // ligne : on garde l'état optimiste, le cache du service worker daterait)
    onSettled: async (res) => {
      if (qc.isMutating({ mutationKey: ballotKey }) > 1) return
      if (res !== 'queued') await qc.invalidateQueries({ queryKey: qk.votes(party.id) })
      setPending(null)
    },
  })

  /** Applique tout de suite (optimiste) puis envoie le bulletin complet. */
  const change = (next: string[], message: string) => {
    void qc.cancelQueries({ queryKey: qk.votes(party.id), exact: true })
    qc.setQueryData<Vote[]>(qk.votes(party.id), (prev) => withMyRanking(prev ?? [], party.id, me.id, next))
    setPending(next)
    setAnnounce(message)
    save.mutate(next)
  }

  const toggle = (r: Restaurant) => {
    const next = toggleRanked(ranking, r.id)
    const added = next.length > ranking.length
    if (added) setBursts((b) => ({ ...b, [r.id]: (b[r.id] ?? 0) + 1 }))
    haptic('vote')
    change(next, added ? `${r.name} ajouté : ton ${ordinal(next.length)} choix.` : `${r.name} retiré de ton classement.`)
  }

  const move = (id: string, dir: 'up' | 'down') => {
    const next = moveRanked(ranking, id, dir === 'up' ? -1 : 1)
    if (next === ranking) return
    focusAfter.current = { id, dir }
    change(next, `${names.get(id) ?? 'Ce resto'} est maintenant ton ${ordinal(next.indexOf(id) + 1)} choix.`)
  }

  // garder le focus sur le bouton déplacé (ou son voisin s'il devient inactif en haut / en bas)
  useEffect(() => {
    const f = focusAfter.current
    if (!f) return
    focusAfter.current = null
    const btn = (dir: string) => rankingRef.current?.querySelector<HTMLButtonElement>(`[data-move="${f.id}"][data-dir="${dir}"]`)
    const target = btn(f.dir)
    ;(target && !target.disabled ? target : btn(f.dir === 'up' ? 'down' : 'up'))?.focus()
  }, [ranking])

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
      <p className="sr-only" aria-live="polite" aria-atomic="true">
        {announce}
      </p>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-display text-xl font-semibold">Classe tes restos préférés</h2>
          <p className="text-sm text-muted">
            Touche les restos qui te vont, dans l’ordre. Ton 1er choix vaut le plus de points.
          </p>
        </div>
        <PulseOnChange value={voters.size}>
          <Badge className="tabular" aria-live="polite">
            {voters.size}/{members.length} ont voté
          </Badge>
        </PulseOnChange>
      </div>

      {candidates.length === 0 ? (
        <EmptyState emoji="🗳️" title="Aucun candidat" description="L'hôte doit rouvrir le salon pour choisir des restos." />
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card className="p-4">
              <div className="flex items-baseline justify-between gap-2">
                <h3 className="font-display text-lg font-semibold">Mon classement</h3>
                {ranking.length > 0 && (
                  <Button variant="ghost" size="sm" className="min-h-11" onClick={() => change([], 'Ton classement est vidé.')}>
                    Tout retirer
                  </Button>
                )}
              </div>
              <p className="mt-1 text-sm text-muted">
                {k > 1 ? `1er choix ${pts(k)}, 2e ${pts(rankPoints(k, 2))}${k > 2 ? '…' : ''} Ne classe que ceux qui te vont.` : 'Ne classe que ceux qui te vont.'}
              </p>
              {ranking.length === 0 ? (
                <p className="mt-3 rounded-md border border-dashed border-border-strong p-4 text-center text-sm text-muted">
                  Touche le cœur d’un resto pour en faire ton 1er choix.
                </p>
              ) : (
                <ol ref={rankingRef} className="mt-3 space-y-2" aria-label="Mon classement">
                  <AnimatePresence initial={false}>
                    {ranking.map((id, i) => {
                      const r = candidates.find((c) => c.id === id)
                      if (!r) return null
                      const rank = i + 1
                      return (
                        <motion.li
                          key={id}
                          layout
                          initial={{ opacity: 0, y: 6 }}
                          animate={{ opacity: 1, y: 0 }}
                          exit={{ opacity: 0, x: -12 }}
                          transition={spring}
                          className="flex items-center gap-2 rounded-md border border-border bg-elevated p-1.5 pl-2"
                        >
                          <RankBadge rank={rank} />
                          <span className="min-w-0 flex-1">
                            <span className="block truncate font-semibold">{r.name}</span>
                            <span className="text-xs text-muted tabular">+{pts(rankPoints(k, rank))}</span>
                          </span>
                          <Button
                            variant="ghost"
                            size="icon"
                            data-move={id}
                            data-dir="up"
                            disabled={i === 0}
                            onClick={() => move(id, 'up')}
                            aria-label={`Monter ${r.name} (actuellement ${ordinal(rank)} choix)`}
                          >
                            <ChevronUp aria-hidden className="size-5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            data-move={id}
                            data-dir="down"
                            disabled={i === ranking.length - 1}
                            onClick={() => move(id, 'down')}
                            aria-label={`Descendre ${r.name} (actuellement ${ordinal(rank)} choix)`}
                          >
                            <ChevronDown aria-hidden className="size-5" />
                          </Button>
                          <Button variant="ghost" size="icon" onClick={() => toggle(r)} aria-label={`Retirer ${r.name} de mon classement`}>
                            <X aria-hidden className="size-5 text-muted" />
                          </Button>
                        </motion.li>
                      )
                    })}
                  </AnimatePresence>
                </ol>
              )}
            </Card>

            <Card className="p-4">
              <h3 className="flex items-center gap-2 font-display text-lg font-semibold">
                <Trophy aria-hidden className="size-4 text-brand" /> Classement en direct
              </h3>
              <p className="mt-1 text-sm text-muted">Égalité : le plus de 1ers choix, puis la meilleure note.</p>
              <ol className="mt-3 space-y-2" aria-label="Classement en direct">
                {standings.map((s, i) => {
                  const top = leader?.restaurant === s.restaurant
                  return (
                    <motion.li key={s.restaurant} layout transition={spring} className="space-y-1">
                      <div className="flex items-center gap-2 text-sm">
                        <span className="w-5 text-center font-display font-bold text-muted tabular">{i + 1}</span>
                        <span className="min-w-0 flex-1 truncate font-medium">{s.data.name}</span>
                        {s.firstChoices > 0 && <span className="text-xs text-muted tabular">{plural(s.firstChoices, '1er choix', '1ers choix')}</span>}
                        <span className={cn('font-semibold tabular', top && 'text-brand')}>{pts(s.points)}</span>
                      </div>
                      <div
                        className="ml-7 h-2 overflow-hidden rounded-full bg-fg/[0.08]"
                        role="meter"
                        aria-valuemin={0}
                        aria-valuemax={maxPoints}
                        aria-valuenow={s.points}
                        aria-label={`${s.data.name} : ${pts(s.points)}`}
                      >
                        <motion.div
                          className={cn('h-full rounded-full', top ? 'bg-ember' : 'bg-fg/30')}
                          initial={false}
                          animate={{ width: `${Math.round((s.points / maxPoints) * 100)}%` }}
                          transition={spring}
                        />
                      </div>
                    </motion.li>
                  )
                })}
              </ol>
            </Card>
          </div>

          <motion.ul variants={listVariants} initial="hidden" animate="show" className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {/* Ordre d'affichage stable (celui des candidats), pour éviter que les cartes sautent pendant le vote. */}
            {candidates.map((r) => {
              const rank = ranking.indexOf(r.id) + 1
              const mine = rank > 0
              const s = byRestaurant.get(r.id)
              const isLeader = leader?.restaurant === r.id
              const rv = allVotes.filter((v) => v.restaurant === r.id)
              const voterUsers = rv.map((v) => people.get(v.user) ?? { id: v.user, name: '' })
              return (
                <motion.li key={r.id} variants={itemVariants}>
                  <Card variant={mine ? 'selected' : 'default'} className="flex h-full flex-col overflow-hidden">
                    <div className="relative">
                      <RestaurantCover restaurant={r} className="aspect-[16/8] w-full" />
                      {isLeader && (
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
                            onClick={() => toggle(r)}
                            aria-pressed={mine}
                            aria-label={mine ? `Retirer ${r.name} de mon classement (${ordinal(rank)} choix)` : `Ajouter ${r.name} à mon classement`}
                            className={cn(
                              'grid size-12 shrink-0 place-items-center rounded-full border transition-colors',
                              mine ? 'border-brand/50 bg-brand/15 text-brand' : 'border-border-strong text-muted hover:text-fg',
                            )}
                          >
                            {mine ? (
                              <motion.span key={rank} initial={{ scale: 0.6 }} animate={{ scale: 1 }} transition={spring} className="font-display text-base font-bold tabular">
                                {ordinal(rank)}
                              </motion.span>
                            ) : (
                              <LiquidHeart filled={false} />
                            )}
                          </motion.button>
                        </span>
                      </div>
                      <div className="mt-auto space-y-2">
                        <div
                          className="h-2 overflow-hidden rounded-full bg-fg/[0.08]"
                          role="meter"
                          aria-valuemin={0}
                          aria-valuemax={maxPoints}
                          aria-valuenow={s?.points ?? 0}
                          aria-label={`${pts(s?.points ?? 0)} pour ${r.name}`}
                        >
                          <motion.div
                            className={cn('h-full rounded-full', isLeader ? 'bg-ember' : 'bg-fg/30')}
                            initial={false}
                            animate={{ width: `${Math.round(((s?.points ?? 0) / maxPoints) * 100)}%` }}
                            transition={spring}
                          />
                        </div>
                        <div className="flex min-h-6 items-center justify-between gap-2">
                          <span className="text-sm font-semibold tabular">
                            {pts(s?.points ?? 0)}
                            <span className="font-normal text-muted"> · {plural(s?.voters ?? 0, 'votant')}</span>
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
        </>
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
              {leader ? (
                <>
                  En tête : <strong className="text-fg">{leader.data.name}</strong> ({pts(leader.points)})
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
        description="Le resto avec le plus de points l'emporte (égalité : plus de 1ers choix, puis meilleure note)."
        footer={
          <Button block size="lg" loading={transition.isPending} onClick={() => transition.mutate({ to: 'ordering' }, { onSuccess: () => setCloseOpen(false) })}>
            {standings[0] ? `Valider — ${standings[0].data.name}` : 'Valider'}
          </Button>
        }
      >
        <ol className="space-y-2">
          {standings.map((s, i) => (
            <li key={s.restaurant} className="flex items-center gap-3 rounded-md border border-border bg-surface p-2.5">
              <span className="w-5 text-center font-display font-bold text-muted tabular">{i + 1}</span>
              <span className="flex-1 truncate font-medium">{s.data.name}</span>
              <span className="text-sm tabular text-muted">{pts(s.points)}</span>
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

/** Pastille de rang : 1er choix en braise, les suivants en neutre. */
function RankBadge({ rank }: { rank: number }) {
  return (
    <span
      aria-hidden
      className={cn(
        'grid size-9 shrink-0 place-items-center rounded-full font-display text-sm font-bold tabular',
        rank === 1 ? 'bg-ember text-brand-fg' : 'border border-border-strong text-fg',
      )}
    >
      {rank}
    </span>
  )
}
