import { useQuery } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui'
import { PopupBlockedError, usersApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { qk } from '@/lib/queryKeys'
import { providerLabel } from './providers'

/**
 * Boutons « Continuer avec Google » : affichés seulement si PocketBase annonce le
 * fournisseur (`listAuthMethods`, OCC_GOOGLE_CLIENT_ID / _SECRET côté serveur).
 * Même bouton pour la connexion et l'inscription : un compte est créé au premier passage,
 * ou lié au compte existant de même adresse.
 */
export function OAuthButtons({ onSuccess }: { onSuccess: () => void }) {
  const methods = useQuery({ queryKey: qk.authMethods, queryFn: usersApi.authMethods, staleTime: 10 * 60_000, retry: false })
  const [busy, setBusy] = useState<string | null>(null)
  // Un essai abandonné (fenêtre fermée) ne doit pas rattraper l'essai suivant.
  const attempt = useRef(0)
  const providers = methods.data?.oauth2?.enabled ? methods.data.oauth2.providers : []
  if (!providers.length) return null
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3 text-xs text-subtle">
        <span className="h-px flex-1 bg-border" /> ou <span className="h-px flex-1 bg-border" />
      </div>
      <div className="grid gap-2">
        {providers.map((p) => (
          <Button
            key={p.name}
            type="button"
            variant="secondary"
            block
            loading={busy === p.name}
            onClick={async () => {
              const id = ++attempt.current
              setBusy(p.name)
              try {
                await usersApi.oauth(p.name)
                if (id === attempt.current) onSuccess()
              } catch (err) {
                if (id !== attempt.current) return
                if (err instanceof PopupBlockedError) toast.error(err.message, { duration: 8000 })
                else toast.error(errorMessage(err, 'Connexion annulée.'))
              } finally {
                if (id === attempt.current) setBusy(null)
              }
            }}
          >
            Continuer avec {providerLabel(p.name, p.displayName)}
          </Button>
        ))}
        {busy && (
          <p className="text-center text-xs text-muted" aria-live="polite">
            Termine la connexion dans la fenêtre qui s'est ouverte.{' '}
            <button
              type="button"
              className="font-semibold text-brand underline-offset-2 hover:underline"
              onClick={() => {
                attempt.current++
                setBusy(null)
              }}
            >
              Annuler
            </button>
          </p>
        )}
      </div>
    </div>
  )
}
