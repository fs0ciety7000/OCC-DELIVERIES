import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, MailCheck, TriangleAlert } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { Button, buttonClass, Field, Input, Spinner } from '@/components/ui'
import { usersApi } from '@/lib/api'
import { logout, useAuth } from '@/lib/auth'
import { errorMessage, hasFieldError } from '@/lib/errors'
import { useConfig } from '@/lib/geo-context'
import { AuthLayout } from './AuthLayout'
import { pairError, type PasswordPair } from './password'
import { PasswordFields } from './PasswordFields'

/* Pages ouvertes depuis les e-mails (liens /auth/…/{TOKEN}) et « Mot de passe oublié ». */

function Outcome({ tone, title, children }: { tone: 'success' | 'danger' | 'info'; title: string; children?: ReactNode }) {
  const Icon = tone === 'success' ? CheckCircle2 : tone === 'danger' ? TriangleAlert : MailCheck
  const color = tone === 'success' ? 'text-success bg-success/12' : tone === 'danger' ? 'text-danger bg-danger/12' : 'text-info bg-info/12'
  return (
    <div className="space-y-3 text-center" role={tone === 'danger' ? 'alert' : 'status'}>
      <span className={`mx-auto grid size-14 place-items-center rounded-full ${color}`}>
        <Icon aria-hidden className="size-7" />
      </span>
      <h2 className="font-display text-xl font-semibold">{title}</h2>
      {children && <div className="space-y-3 text-sm text-muted">{children}</div>}
    </div>
  )
}

const backToLogin = (
  <Link to="/login" className="font-semibold text-brand hover:underline">
    Retour à la connexion
  </Link>
)

/** Messages des jetons refusés par PocketBase (expirés, déjà utilisés…). */
function tokenError(err: unknown, fallback: string): string {
  const msg = errorMessage(err, fallback)
  return hasFieldError(err, 'token') || /token|invalid|expired|validat/i.test(msg) ? fallback : msg
}

/* ----------------------------------------------------- mot de passe oublié */

export function ForgotPasswordPage() {
  const [params] = useSearchParams()
  const config = useConfig()
  const [email, setEmail] = useState(params.get('email') ?? '')
  const [busy, setBusy] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const mailOff = config.data && !config.data.mailEnabled

  return (
    <AuthLayout title="Mot de passe oublié ?" subtitle="Pas de panique : on t'envoie un lien pour en choisir un nouveau." footer={backToLogin}>
      {sent ? (
        <Outcome tone="info" title="Regarde ta boîte mail">
          <p>
            Si un compte existe pour <strong className="text-fg">{email}</strong>, un lien vient de partir. Il est valable 30 minutes.
          </p>
          <p>Rien reçu ? Vérifie les indésirables, ou réessaie dans deux minutes.</p>
        </Outcome>
      ) : mailOff ? (
        <Outcome tone="danger" title="E-mails indisponibles">
          <p>L'envoi d'e-mails n'est pas encore activé sur ce serveur. Demande à un·e admin de t'envoyer un lien de réinitialisation.</p>
        </Outcome>
      ) : (
        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault()
            setError(null)
            setBusy(true)
            try {
              await usersApi.requestPasswordReset(email.trim())
              setSent(true)
            } catch (err) {
              setError(errorMessage(err, "Impossible d'envoyer le lien pour le moment."))
            } finally {
              setBusy(false)
            }
          }}
        >
          <Field label="E-mail du compte" error={error}>
            {(p) => <Input {...p} type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="prenom@entreprise.be" />}
          </Field>
          <Button type="submit" block size="lg" loading={busy}>
            Recevoir le lien
          </Button>
          <p className="text-xs text-subtle">Compte créé avec Google ? Ce lien te permet aussi de définir un mot de passe.</p>
        </form>
      )}
    </AuthLayout>
  )
}

/* ------------------------------------------- nouveau mot de passe (lien) */

export function ResetPasswordPage() {
  const { token = '' } = useParams()
  const [pair, setPair] = useState<PasswordPair>({ password: '', confirm: '' })
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const invalid = pairError(pair)

  return (
    <AuthLayout title="Nouveau mot de passe" subtitle="Choisis-le solide : une petite phrase, c'est parfait." footer={backToLogin}>
      {done ? (
        <Outcome tone="success" title="Mot de passe changé">
          <p>Tes autres sessions ont été déconnectées. Tu peux te connecter avec ton nouveau mot de passe.</p>
          <Link to="/login" className={buttonClass('primary', 'lg', true)}>
            Se connecter
          </Link>
        </Outcome>
      ) : (
        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault()
            if (invalid) return
            setError(null)
            setBusy(true)
            try {
              await usersApi.confirmPasswordReset(token, pair.password)
              logout()
              setDone(true)
            } catch (err) {
              setError(tokenError(err, 'Ce lien a expiré ou a déjà servi. Demande un nouveau lien.'))
            } finally {
              setBusy(false)
            }
          }}
        >
          <PasswordFields value={pair} onChange={setPair} />
          {error && (
            <p role="alert" className="rounded-sm border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">
              {error}{' '}
              <Link to="/auth/mot-de-passe-oublie" className="font-semibold underline">
                Nouveau lien
              </Link>
            </p>
          )}
          <Button type="submit" block size="lg" loading={busy} disabled={!!invalid}>
            Enregistrer le mot de passe
          </Button>
        </form>
      )}
    </AuthLayout>
  )
}

/* ------------------------------------------------- vérification d'adresse */

export function VerifyEmailPage() {
  const { token = '' } = useParams()
  const { isAuthenticated } = useAuth()
  // useQuery : un seul appel même en StrictMode, jamais relancé (jeton à usage unique).
  const verify = useQuery({
    queryKey: ['verifyEmail', token],
    queryFn: async () => {
      await usersApi.confirmVerification(token)
      return true
    },
    retry: false,
    staleTime: Infinity,
    gcTime: Infinity,
  })
  return (
    <AuthLayout title="Confirmation d'adresse" subtitle="Une adresse confirmée, c'est un compte que tu pourras toujours récupérer." footer={isAuthenticated ? <Link to="/" className="font-semibold text-brand hover:underline">Retour à l'accueil</Link> : backToLogin}>
      {verify.isPending ? (
        <div className="flex items-center justify-center gap-3 py-6 text-muted" aria-live="polite">
          <Spinner label="Vérification en cours" /> Vérification en cours…
        </div>
      ) : verify.isError ? (
        <Outcome tone="danger" title="Lien invalide ou expiré">
          <p>{tokenError(verify.error, 'Ce lien a expiré ou a déjà servi.')} Tu peux en redemander un depuis ton profil.</p>
          <Link to={isAuthenticated ? '/profile?onglet=infos' : '/login'} className={buttonClass('secondary', 'md', true)}>
            {isAuthenticated ? 'Aller à mon profil' : 'Se connecter'}
          </Link>
        </Outcome>
      ) : (
        <Outcome tone="success" title="Adresse confirmée 🎉">
          <p>Merci ! Ton compte est vérifié.</p>
          <Link to="/" className={buttonClass('primary', 'lg', true)}>
            C'est parti
          </Link>
        </Outcome>
      )}
    </AuthLayout>
  )
}

/* -------------------------------------------- changement d'adresse (lien) */

export function ConfirmEmailChangePage() {
  const { token = '' } = useParams()
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState<string | null>(null)

  return (
    <AuthLayout title="Nouvelle adresse e-mail" subtitle="Dernière étape : confirme avec ton mot de passe actuel." footer={backToLogin}>
      {done ? (
        <Outcome tone="success" title="Adresse modifiée">
          <p>Par sécurité, toutes tes sessions ont été fermées. Connecte-toi avec ta nouvelle adresse.</p>
          <Link to="/login" className={buttonClass('primary', 'lg', true)}>
            Se connecter
          </Link>
        </Outcome>
      ) : (
        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault()
            setError(null)
            setBusy(true)
            try {
              await usersApi.confirmEmailChange(token, password)
              logout()
              setDone(true)
            } catch (err) {
              setError(hasFieldError(err, 'password') ? 'Mot de passe incorrect.' : tokenError(err, 'Ce lien a expiré ou a déjà servi. Refais la demande depuis ton profil.'))
            } finally {
              setBusy(false)
            }
          }}
        >
          <Field label="Mot de passe actuel" error={error}>
            {(p) => <Input {...p} type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />}
          </Field>
          <Button type="submit" block size="lg" loading={busy}>
            Confirmer la nouvelle adresse
          </Button>
        </form>
      )}
    </AuthLayout>
  )
}
