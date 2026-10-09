import { useMutation } from '@tanstack/react-query'
import { ArrowLeft, LogOut, MoreHorizontal, Share2, XCircle } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { toast } from 'sonner'
import { PulseOnChange } from '@/components/food/PulseOnChange'
import { AvatarStack, Badge, Button, CopyButton, Sheet, Stepper } from '@/components/ui'
import { occ } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import type { PartyCtx } from './context'
import { inviteUrl, STATUS_LABELS, useTransition } from './hooks'
import { shareInvite } from './share'

export function PartyHeader({ ctx }: { ctx: PartyCtx }) {
  const { party, members, isHost, me } = ctx
  const [menuOpen, setMenuOpen] = useState(false)
  const [confirmCancel, setConfirmCancel] = useState(false)
  const transition = useTransition(party.id)
  const navigate = useNavigate()
  const leave = useMutation({
    mutationFn: () => occ.leave(party.id),
    onSuccess: () => {
      toast('Tu as quitté la commande')
      navigate('/')
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const showReady = party.status === 'ordering' || party.status === 'review'
  const users = members.map((m) => ({ ...(m.expand?.user ?? { id: m.user, name: ctx.people.get(m.user)?.name ?? '' }), ready: m.ready }))
  const readyCount = members.filter((m) => m.ready).length
  const active = party.status !== 'closed' && party.status !== 'cancelled'
  const canLeave = !isHost && ['lobby', 'voting', 'ordering'].includes(party.status)
  const canShare = typeof navigator !== 'undefined' && typeof navigator.share === 'function'

  return (
    <header className="space-y-4">
      <div className="flex items-start gap-2">
        <Link to="/" className="-ml-2 grid size-11 shrink-0 place-items-center rounded-full text-muted hover:bg-fg/[0.06] hover:text-fg" aria-label="Retour à l'accueil">
          <ArrowLeft className="size-5" />
        </Link>
        <div className="min-w-0 flex-1 pt-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate font-display text-2xl leading-[30px] font-bold">{party.title || 'Commande groupée'}</h1>
            <Badge variant={party.status === 'cancelled' ? 'danger' : party.status === 'closed' ? 'success' : 'brand'} dot={active}>
              {STATUS_LABELS[party.status]}
            </Badge>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted">
            {party.expand?.restaurant && (
              <span>
                <span aria-hidden>{party.expand.restaurant.emoji} </span>
                {party.expand.restaurant.name}
              </span>
            )}
            {isHost ? <span>Tu es l'hôte</span> : party.expand?.host && <span>Hôte : {party.expand.host.name}</span>}
          </div>
        </div>
        {active && (
          <Button variant="ghost" size="icon" aria-label="Plus d'actions" onClick={() => setMenuOpen(true)}>
            <MoreHorizontal className="size-5" />
          </Button>
        )}
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          {/* Cible du « panier vivant » (vols de plats des collègues vers leur avatar). */}
          <div data-party-avatars>
            <AvatarStack users={users} size={32} max={6} showReady={showReady} />
          </div>
          <span className="text-sm text-muted tabular" aria-live="polite">
            {members.length} {members.length > 1 ? 'membres' : 'membre'}
            {showReady && (
              <>
                {' · '}
                <PulseOnChange value={readyCount}>
                  {readyCount} prêt{readyCount > 1 ? 's' : ''}
                </PulseOnChange>
              </>
            )}
          </span>
        </div>
        {active && (
          <div className="flex items-center gap-1.5">
            <span className="rounded-sm border border-border bg-elevated px-2.5 py-1 font-display text-sm font-semibold tracking-[0.2em]" aria-label={`Code ${party.code}`}>
              {party.code}
            </span>
            <CopyButton value={inviteUrl(party.code)} label="" ariaLabel="Copier le lien d'invitation" size="icon" variant="ghost" toastMessage="Lien d'invitation copié" />
            {canShare && (
              <Button variant="ghost" size="icon" aria-label="Partager l'invitation" onClick={() => void shareInvite(party)}>
                <Share2 className="size-4" />
              </Button>
            )}
          </div>
        )}
      </div>

      {party.status !== 'cancelled' && <Stepper status={party.status} />}

      <Sheet open={menuOpen} onClose={() => setMenuOpen(false)} title="Actions">
        <div className="space-y-2">
          <CopyButton value={inviteUrl(party.code)} label="Copier le lien d'invitation" className="w-full justify-start" size="md" toastMessage="Lien copié" />
          {canShare && (
            <Button variant="secondary" block className="justify-start" leftIcon={<Share2 className="size-4" />} onClick={() => void shareInvite(party)}>
              Partager l'invitation
            </Button>
          )}
          {canLeave && (
            <Button variant="danger" block className="justify-start" leftIcon={<LogOut className="size-4" />} loading={leave.isPending} onClick={() => leave.mutate()}>
              Quitter la commande
            </Button>
          )}
          {isHost && (
            <Button
              variant="danger"
              block
              className="justify-start"
              leftIcon={<XCircle className="size-4" />}
              onClick={() => {
                setMenuOpen(false)
                setConfirmCancel(true)
              }}
            >
              Annuler la commande
            </Button>
          )}
          {!isHost && !canLeave && <p className="text-sm text-muted">Seul·e l'hôte peut modifier la commande à ce stade.</p>}
          <p className="pt-2 text-xs text-subtle">Connecté·e en tant que {me.name || me.email}</p>
        </div>
      </Sheet>

      <Sheet
        open={confirmCancel}
        onClose={() => setConfirmCancel(false)}
        title="Annuler la commande ?"
        description="Tout le monde sera prévenu. Cette action est définitive."
        footer={
          <div className="flex gap-2">
            <Button variant="secondary" className="flex-1" onClick={() => setConfirmCancel(false)}>
              Garder
            </Button>
            <Button
              variant="danger"
              className="flex-1"
              loading={transition.isPending}
              onClick={() =>
                transition.mutate(
                  { to: 'cancelled' },
                  {
                    onSuccess: () => {
                      setConfirmCancel(false)
                      toast('Commande annulée')
                    },
                  },
                )
              }
            >
              Oui, annuler
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted">Les paniers et votes resteront visibles mais plus personne ne pourra commander.</p>
      </Sheet>
    </header>
  )
}
