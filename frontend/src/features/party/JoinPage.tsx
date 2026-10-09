import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Button, EmptyState, Spinner } from '@/components/ui'
import { occ } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { normalizeCode } from './hooks'

/** `/j/:code` : rejoint la party (idempotent) puis redirige vers la salle. */
export function JoinPage() {
  const { code = '' } = useParams()
  const navigate = useNavigate()
  const clean = normalizeCode(code)
  const join = useMutation({
    mutationFn: () => occ.join(clean),
    onSuccess: ({ party }) => {
      toast.success(`Bienvenue dans « ${party.title || 'la commande'} » !`)
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
