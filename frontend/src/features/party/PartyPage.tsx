import { AnimatePresence, motion } from 'motion/react'
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { Link, useParams } from 'react-router'
import { toast } from 'sonner'
import { Button, EmptyState, Skeleton } from '@/components/ui'
import { useAuth } from '@/lib/auth'
import { errorMessage, isNotFound } from '@/lib/errors'
import { fadeUp } from '@/lib/motion'
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
import { VotingStep } from './steps/VotingStep'

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

  const ctx = useMemo<PartyCtx | null>(() => {
    if (!party.data || !user) return null
    const people = new Map<string, User>()
    for (const u of party.data.expand?.members ?? []) people.set(u.id, u)
    for (const m of members.data ?? []) if (m.expand?.user) people.set(m.user, m.expand.user)
    people.set(user.id, { ...people.get(user.id), ...user })
    return { party: party.data, members: members.data ?? [], me: user, isHost: party.data.host === user.id, people }
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
      <AnimatePresence mode="wait">
        <motion.div key={ctx.party.status} {...fadeUp}>
          <StepView ctx={ctx} />
        </motion.div>
      </AnimatePresence>
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
