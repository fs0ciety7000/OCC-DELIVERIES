import { useCallback, useEffect, useMemo, useRef } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Button, EmptyState, Skeleton } from '@/components/ui'
import { useAuth } from '@/lib/auth'
import { errorMessage, isNotFound } from '@/lib/errors'
import { forgetParty, readLastParty, rememberParty } from '@/lib/lastParty'
import { usePartyRealtime } from '@/lib/realtime'
import type { Party, PartyStatus, User } from '@/lib/types'
import type { PartyCtx } from './context'
import { useMembers, useParty } from './hooks'
import { PartyHeader } from './PartyHeader'
import { ClosedStep } from './steps/ClosedStep'
import { LobbyStep } from './steps/LobbyStep'
import { OrderingStep } from './steps/OrderingStep'
import { PayingStep } from './steps/PayingStep'
import { ReviewStep } from './steps/ReviewStep'
import { StepTransition } from './steps/StepTransition'
import { VotingStep } from './steps/VotingStep'
import { PartyDeadlines } from '@/features/deadlines/PartyDeadlines'
import { distinctColors } from '@/lib/colors'
import { TeamMissingMembers } from '@/features/teams/TeamMissingMembers'
import { PayoutQuickAddSheet } from './steps/PayoutQuickAdd'

const STATUS_TOASTS: Partial<Record<PartyStatus, string>> = {
  voting: 'Le vote est ouvert — à vos cœurs ! ❤️',
  ordering: 'Le resto est choisi : à vos paniers ! 🛒',
  review: "L'hôte prépare le récap 🧾",
  paying: 'Place aux remboursements 💸',
  closed: 'Tout est réglé, bon appétit ! 🎉',
  cancelled: "La commande a été annulée par l'hôte.",
}

export function PartyPage() {
  const { id } = useParams()
  const { user } = useAuth()
  const party = useParty(id)
  const members = useMembers(id)
  // `?iban=1` : lien de la notification « Ajoute ton IBAN » → ajout rapide sans quitter la party.
  const [search, setSearch] = useSearchParams()
  const ibanOpen = search.get('iban') === '1'
  const lastStatus = useRef<PartyStatus | null>(null)

  const onPartyChange = useCallback((p: Party, action: string) => {
    if (action === 'delete') {
      toast("Cette commande n'existe plus.")
      return
    }
    if (lastStatus.current && lastStatus.current !== p.status) {
      const msg = STATUS_TOASTS[p.status]
      if (msg) toast(msg)
    }
    lastStatus.current = p.status
  }, [])
  const status = party.data?.status
  useEffect(() => {
    if (status) lastStatus.current = status
  }, [status])

  usePartyRealtime(id, { meId: user?.id, onPartyChange })

  // Souvenir local de la dernière commande ouverte (repli du bandeau « Commande en cours »).
  const p = party.data
  useEffect(() => {
    if (p && user) rememberParty(user.id, { id: p.id, title: p.title, status: p.status, code: p.code })
  }, [p, user])
  const notFound = party.isError && isNotFound(party.error)
  useEffect(() => {
    if (notFound && user && readLastParty(user.id)?.id === id) forgetParty(user.id)
  }, [notFound, user, id])

  const ctx = useMemo<PartyCtx | null>(() => {
    if (!party.data || !user) return null
    const people = new Map<string, User>()
    for (const u of party.data.expand?.members ?? []) people.set(u.id, u)
    for (const m of members.data ?? []) if (m.expand?.user) people.set(m.user, m.expand.user)
    people.set(user.id, { ...people.get(user.id), ...user })
    // Couleurs d'avatar distinctes dans la party (deux collègues avec la même couleur stockée
    // ne se confondent pas) : appliquées une fois ici, donc identiques sur toutes les étapes.
    const order = [...(members.data ?? []).map((m) => m.user), ...people.keys()]
    const colors = distinctColors(order.map((uid) => people.get(uid) ?? { id: uid }))
    for (const [uid, u] of people) people.set(uid, { ...u, color: colors.get(uid) ?? u.color })
    const list = (members.data ?? []).map((m) => (m.expand?.user ? { ...m, expand: { ...m.expand, user: people.get(m.user) ?? m.expand.user } } : m))
    return { party: party.data, members: list, me: user, isHost: party.data.host === user.id, people }
  }, [party.data, members.data, user])

  if (party.isError) {
    const nf = isNotFound(party.error)
    return (
      <EmptyState
        tone={nf ? 'default' : 'danger'}
        emoji={nf ? '🔒' : '📡'}
        title={nf ? 'Commande introuvable' : 'Chargement impossible'}
        description={nf ? "Elle n'existe pas ou tu n'en fais pas partie. Demande le code à l'hôte !" : errorMessage(party.error)}
        action={
          nf ? (
            <Link to="/" className="font-semibold text-brand">
              Retour à l'accueil
            </Link>
          ) : (
            <Button onClick={() => party.refetch()}>Réessayer</Button>
          )
        }
      />
    )
  }

  if (!ctx) {
    return (
      <div className="space-y-4" aria-busy="true" aria-label="Chargement de la commande">
        <Skeleton className="h-8 w-2/3" />
        <Skeleton className="h-4 w-1/3" />
        <div className="flex gap-1.5">
          {[0, 1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="h-1 flex-1" />
          ))}
        </div>
        <Skeleton className="h-48 w-full rounded-lg" />
        <Skeleton className="h-32 w-full rounded-lg" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PartyHeader ctx={ctx} />
      <TeamMissingMembers party={ctx.party} />
      <PartyDeadlines party={ctx.party} isHost={ctx.isHost} />
      <StepTransition status={ctx.party.status}>
        <StepView ctx={ctx} />
      </StepTransition>
      <PayoutQuickAddSheet
        me={ctx.me}
        partyId={ctx.party.id}
        open={ibanOpen}
        onClose={() =>
          setSearch(
            (prev) => {
              const next = new URLSearchParams(prev)
              next.delete('iban')
              return next
            },
            { replace: true },
          )
        }
      />
    </div>
  )
}

function StepView({ ctx }: { ctx: PartyCtx }) {
  switch (ctx.party.status) {
    case 'lobby':
      return <LobbyStep ctx={ctx} />
    case 'voting':
      return <VotingStep ctx={ctx} />
    case 'ordering':
      return <OrderingStep ctx={ctx} />
    case 'review':
      return <ReviewStep ctx={ctx} />
    case 'paying':
      return <PayingStep ctx={ctx} />
    case 'closed':
      return <ClosedStep ctx={ctx} />
    case 'cancelled':
      return (
        <EmptyState
          emoji="🫗"
          title="Commande annulée"
          description="L'hôte a annulé cette commande. On se rattrape au prochain midi ?"
          action={
            <Link to="/" className="font-semibold text-brand">
              Retour à l'accueil
            </Link>
          }
        />
      )
    default:
      return null
  }
}
