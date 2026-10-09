import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui'
import { usersApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { qk } from '@/lib/queryKeys'

/** Boutons OAuth2 affichés seulement si des fournisseurs sont configurés côté PocketBase. */
export function OAuthButtons({ onSuccess }: { onSuccess: () => void }) {
  const methods = useQuery({ queryKey: qk.authMethods, queryFn: usersApi.authMethods, staleTime: 10 * 60_000, retry: false })
  const [busy, setBusy] = useState<string | null>(null)
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
            variant="secondary"
            block
            loading={busy === p.name}
            onClick={async () => {
              setBusy(p.name)
              try {
                await usersApi.oauth(p.name)
                onSuccess()
              } catch (err) {
                toast.error(errorMessage(err, 'Connexion annulée.'))
              } finally {
                setBusy(null)
              }
            }}
          >
            Continuer avec {p.displayName || p.name}
          </Button>
        ))}
      </div>
    </div>
  )
}
