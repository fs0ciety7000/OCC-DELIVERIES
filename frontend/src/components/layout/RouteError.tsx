import { isRouteErrorResponse, Link, useRouteError } from 'react-router'
import { EmptyState } from '@/components/ui'

export function RouteError() {
  const error = useRouteError()
  const notFound = isRouteErrorResponse(error) && error.status === 404
  return (
    <div className="mx-auto max-w-lg py-16">
      <EmptyState
        tone={notFound ? 'default' : 'danger'}
        emoji={notFound ? '🧭' : '🔥'}
        title={notFound ? 'Page introuvable' : 'Quelque chose a brûlé en cuisine'}
        description={notFound ? "Cette page n'existe pas (ou plus)." : 'Recharge la page ; si ça persiste, préviens l’équipe.'}
        action={
          <Link to="/" className="font-semibold text-brand">
            Retour à l'accueil
          </Link>
        }
      />
    </div>
  )
}

export function NotFoundPage() {
  return (
    <div className="mx-auto max-w-lg py-16">
      <EmptyState
        title="404 — assiette vide"
        description="On a cherché partout, cette page n'est pas au menu."
        action={
          <Link to="/" className="font-semibold text-brand">
            Retour à l'accueil
          </Link>
        }
      />
    </div>
  )
}
