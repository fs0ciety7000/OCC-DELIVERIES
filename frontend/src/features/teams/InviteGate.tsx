import { useQuery } from '@tanstack/react-query'
import { Users } from 'lucide-react'
import { useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router'
import { toast } from 'sonner'
import { buttonClass, Card, EmptyState, Skeleton } from '@/components/ui'
import { guestApi } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import type { GuestAuthResponse } from '@/lib/types'
import { GuestJoinForm } from './GuestJoinForm'
import { teamKeys } from './keys'

/**
 * Page d'invitation pour un·e visiteur·se non connecté·e (`/j/:code`, `/e/:code`) :
 * aperçu (titre, hôte, membres), connexion / inscription, ou « Continuer en invité·e ».
 */
export function InviteGate({ kind, code }: { kind: 'party' | 'team'; code: string }) {
  const navigate = useNavigate()
  const location = useLocation()
  const [guest, setGuest] = useState(false)
  const preview = useQuery({ queryKey: teamKeys.invite(code), queryFn: () => guestApi.preview(code), retry: false })
  const next = encodeURIComponent(location.pathname)

  const onJoined = (res: GuestAuthResponse) => {
    if (res.party) {
      toast.success(`Bienvenue dans « ${res.party.title || 'la commande'} » !`)
      navigate(`/party/${res.party.id}`, { replace: true })
    } else if (res.team) {
      toast.success(`Bienvenue dans l'équipe « ${res.team.name} » !`)
      navigate(`/equipes/${res.team.id}`, { replace: true })
    }
  }

  if (preview.isError) {
    return (
      <EmptyState
        tone="danger"
        emoji="🚪"
        title="Lien d'invitation introuvable"
        description={errorMessage(preview.error)}
        action={<Link to="/">Retour à l'accueil</Link>}
      />
    )
  }
  const p = preview.data
  const what = kind === 'party' ? 'la commande' : "l'équipe"
  return (
    <div className="mx-auto flex w-full max-w-md flex-col gap-6 py-6 sm:py-12">
      <div className="space-y-2 text-center">
        <p className="text-sm font-semibold tracking-wide text-brand uppercase">{kind === 'party' ? 'Invitation à une commande' : "Invitation d'équipe"}</p>
        {p ? (
          <h1 className="font-display text-[32px] leading-9 font-bold">
            {p.emoji ? `${p.emoji} ` : ''}
            {p.title || (kind === 'party' ? 'Commande groupée' : 'Équipe')}
          </h1>
        ) : (
          <Skeleton className="mx-auto h-9 w-2/3" />
        )}
        {p && (
          <p className="inline-flex items-center gap-1.5 text-muted">
            <Users aria-hidden className="size-4" />
            {p.host ? `${p.host} t'invite · ` : ''}
            {p.memberCount} {p.memberCount > 1 ? 'membres' : 'membre'}
          </p>
        )}
      </div>
      {p && !p.joinable ? (
        <Card className="p-5 text-center text-muted">{kind === 'party' ? "Cette commande n'accepte plus de nouveaux participants." : 'Cette équipe est archivée.'}</Card>
      ) : (
        <Card className="space-y-4 p-5 sm:p-6">
          {guest ? (
            <>
              <h2 className="font-display text-lg font-semibold">Continuer en invité·e</h2>
              <GuestJoinForm kind={kind} code={code} onJoined={onJoined} />
              <button type="button" className="min-h-11 w-full text-sm font-semibold text-muted hover:text-fg" onClick={() => setGuest(false)}>
                J'ai déjà un compte
              </button>
            </>
          ) : (
            <>
              <p className="text-sm text-muted">Connecte-toi pour rejoindre {what} avec ton compte, ou entre juste ton prénom.</p>
              <div className="grid gap-2">
                <Link to={`/login?next=${next}`} className={cn(buttonClass('primary', 'lg'), 'w-full')}>
                  Se connecter
                </Link>
                <Link to={`/register?next=${next}`} className={cn(buttonClass('secondary', 'md'), 'w-full')}>
                  Créer un compte
                </Link>
                <button type="button" className={cn(buttonClass('ghost', 'md'), 'w-full')} onClick={() => setGuest(true)}>
                  Continuer en invité·e
                </button>
              </div>
            </>
          )}
        </Card>
      )}
    </div>
  )
}
