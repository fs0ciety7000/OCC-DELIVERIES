import { useMutation } from '@tanstack/react-query'
import { MailWarning, X } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui'
import { usersApi } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { useConfig } from '@/lib/geo-context'
import type { User } from '@/lib/types'

const DISMISS_KEY = 'occ.verifyBanner.dismissed'

function readDismissed(): boolean {
  try {
    return sessionStorage.getItem(DISMISS_KEY) === '1'
  } catch {
    return false
  }
}

/**
 * Bandeau discret « Confirme ton adresse e-mail » : un compte non vérifié reste
 * pleinement utilisable (on ne bloque pas les collègues), on le rappelle seulement.
 * Affiché si le serveur peut envoyer des e-mails. `dismissible` : masquable pour la session.
 */
export function VerifyEmailBanner({ user, dismissible = false, className }: { user: User | null; dismissible?: boolean; className?: string }) {
  const config = useConfig()
  const [dismissed, setDismissed] = useState(() => dismissible && readDismissed())
  const resend = useMutation({
    mutationFn: () => usersApi.requestVerification(user?.email ?? ''),
    onSuccess: () => toast.success(`E-mail envoyé à ${user?.email}`, { description: 'Pense à regarder dans les indésirables.' }),
    onError: (e) => toast.error(errorMessage(e, "Impossible d'envoyer l'e-mail pour le moment.")),
  })
  if (!user || user.is_guest || user.verified !== false || !user.email || !config.data?.mailEnabled || dismissed) return null
  return (
    <aside aria-label="Adresse e-mail à confirmer" className={cn('flex items-start gap-3 rounded-md border border-warning/30 bg-warning/10 px-3 py-2.5 text-sm sm:items-center', className)}>
      <MailWarning aria-hidden className="mt-0.5 size-5 shrink-0 text-warning sm:mt-0" />
      <p className="min-w-0 flex-1">
        <span className="font-semibold">Confirme ton adresse e-mail.</span>{' '}
        <span className="text-muted">Le lien envoyé à {user.email} sécurise ton compte (mot de passe oublié…).</span>
      </p>
      <Button variant="ghost" size="sm" loading={resend.isPending} disabled={resend.isSuccess} onClick={() => resend.mutate()}>
        {resend.isSuccess ? 'Envoyé' : "Renvoyer l'e-mail"}
      </Button>
      {dismissible && (
        <button
          type="button"
          aria-label="Masquer ce rappel"
          className="-my-1.5 grid size-11 shrink-0 place-items-center rounded-full text-muted hover:bg-fg/[0.06] hover:text-fg"
          onClick={() => {
            try {
              sessionStorage.setItem(DISMISS_KEY, '1')
            } catch {
              /* stockage indisponible : masqué jusqu'au rechargement */
            }
            setDismissed(true)
          }}
        >
          <X className="size-4" />
        </button>
      )}
    </aside>
  )
}
