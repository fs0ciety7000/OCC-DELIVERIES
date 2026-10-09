import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Button, EmptyState, Spinner } from '@/components/ui'
import { teamsApi } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'
import { normalizeTeamCode, TEAM_CODE_LENGTH } from './format'
import { InviteGate } from './InviteGate'
import { teamKeys } from './keys'

/** `/e/:code` : lien fixe d'une équipe — rejoint (idempotent) puis ouvre la page d'équipe. */
export function TeamJoinPage() {
  const { code = '' } = useParams()
  const { isAuthenticated } = useAuth()
  // Ouvert sans session : la page d'invitation gère tout (l'invité·e connecté·e par le formulaire est redirigé·e par elle).
  const [gated] = useState(!isAuthenticated)
  const clean = normalizeTeamCode(code)
  if (clean.length !== TEAM_CODE_LENGTH) {
    return <EmptyState emoji="🔗" title="Lien d'équipe invalide" description="Un lien d'équipe se termine par 8 caractères, par exemple /e/K7M2QXAB." action={<Link to="/">Retour à l'accueil</Link>} />
  }
  if (gated || !isAuthenticated) return <InviteGate kind="team" code={clean} />
  return <TeamJoinAuthed code={clean} />
}

function TeamJoinAuthed({ code }: { code: string }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const join = useMutation({
    mutationFn: () => teamsApi.join(code),
    onSuccess: ({ team, alreadyMember }) => {
      if (alreadyMember) toast(`Te revoilà dans « ${team.name} »`)
      else toast.success(`Bienvenue dans l'équipe « ${team.name} » !`)
      void qc.invalidateQueries({ queryKey: teamKeys.all })
      navigate(`/equipes/${team.id}`, { replace: true })
    },
  })
  const started = useRef(false)
  const { mutate } = join
  useEffect(() => {
    if (started.current) return
    started.current = true
    mutate()
  }, [mutate])

  if (join.isError) {
    return (
      <EmptyState
        tone="danger"
        emoji="🚪"
        title="Impossible de rejoindre l'équipe"
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
      <p className="font-display text-xl font-semibold">On t'ajoute à l'équipe…</p>
    </div>
  )
}
