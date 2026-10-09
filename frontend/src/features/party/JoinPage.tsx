import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Button, EmptyState, Spinner } from '@/components/ui'
import { occ } from '@/lib/api'
import { InviteGate } from '@/features/teams/InviteGate'
import { useAuth } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'
import { normalizeCode } from './hooks'

/** `/j/:code` : sans session → connexion ou « Continuer en invité·e » ; sinon rejoint directement. */
export function JoinPage() {
  const { code = '' } = useParams()
  const { isAuthenticated } = useAuth()
  // Ouvert sans session : la page d'invitation redirige elle-même après « Continuer en invité·e ».
  const [gated] = useState(!isAuthenticated)
  const clean = normalizeCode(code)
  if ((gated || !isAuthenticated) && clean.length === 6) return <InviteGate kind="party" code={clean} />
  return <JoinPageAuthed />
}

/** Rejoint la party (idempotent) puis redirige vers la salle. */
function JoinPageAuthed() {
  const { code = '' } = useParams()
  const navigate = useNavigate()
  const clean = normalizeCode(code)
  const join = useMutation({
    mutationFn: () => occ.join(clean),
    onSuccess: ({ party, alreadyMember }) => {
      // Déjà membre (lien réouvert, page quittée…) : on retourne simplement dans le salon.
      if (alreadyMember) toast(`Te revoilà dans « ${party.title || 'la commande'} »`)
      else toast.success(`Bienvenue dans « ${party.title || 'la commande'} » !`)
      navigate(`/party/${party.id}`, { replace: true })
    },
  })
  const started = useRef(false)
  const { mutate } = join
  useEffect(() => {
    if (started.current || clean.length !== 6) return
    started.current = true
    mutate()
  }, [clean, mutate])

  if (clean.length !== 6) {
    return <EmptyState emoji="🔢" title="Code invalide" description="Un code fait 6 caractères, par exemple K7M2QX." action={<Link to="/">Retour à l'accueil</Link>} />
  }
  if (join.isError) {
    return (
      <EmptyState
        tone="danger"
        emoji="🚪"
        title="Impossible de rejoindre"
        description={errorMessage(join.error)}
        action={
          <>
            <Button onClick={() => join.mutate()}>Réessayer</Button>
            <Button variant="secondary" onClick={() => navigate('/')}>
              Accueil
            </Button>
          </>
        }
      />
    )
  }
  return (
    <div className="flex flex-col items-center gap-4 py-20 text-center" aria-live="polite">
      <Spinner className="size-8 text-brand" />
      <p className="font-display text-xl font-semibold">On t'ouvre la porte…</p>
      <p className="text-sm text-muted">
        Code <span className="font-semibold tracking-widest text-fg">{clean}</span>
      </p>
    </div>
  )
}
