import { ClientResponseError } from 'pocketbase'
import { useState } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Button, Field, Input } from '@/components/ui'
import { usersApi } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'
import { AuthLayout } from './AuthLayout'
import { safeNext } from './safeNext'
import { OAuthButtons } from './OAuthButtons'

export function LoginPage() {
  const [params] = useSearchParams()
  const next = safeNext(params.get('next'))
  const navigate = useNavigate()
  const { isAuthenticated } = useAuth()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  if (isAuthenticated && !busy) return <Navigate to={next} replace />

  const done = () => {
    toast.success('Content de te revoir !')
    navigate(next, { replace: true })
  }

  return (
    <AuthLayout
      title="Bon retour 👋"
      subtitle="Connecte-toi pour lancer ou rejoindre une commande."
      footer={
        <>
          Pas encore de compte ?{' '}
          <Link to={`/register?next=${encodeURIComponent(next)}`} className="font-semibold text-brand hover:underline">
            Crée-le en 20 secondes
          </Link>
        </>
      }
    >
      <form
        className="space-y-4"
        onSubmit={async (e) => {
          e.preventDefault()
          setError(null)
          setBusy(true)
          try {
            await usersApi.login(email.trim(), password)
            done()
          } catch (err) {
            // 400 = identifiants refusés (message PocketBase en anglais) ; 403 = compte suspendu (message serveur)
            setError(err instanceof ClientResponseError && err.status === 400 ? 'E-mail ou mot de passe incorrect.' : errorMessage(err, 'E-mail ou mot de passe incorrect.'))
          } finally {
            setBusy(false)
          }
        }}
      >
        <Field label="E-mail">
          {(p) => <Input {...p} type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="prenom@entreprise.be" />}
        </Field>
        <div className="space-y-1.5">
          <Field label="Mot de passe" error={error}>
            {(p) => <Input {...p} type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />}
          </Field>
          <p className="text-right text-sm">
            <Link to={`/auth/mot-de-passe-oublie${email.trim() ? `?email=${encodeURIComponent(email.trim())}` : ''}`} className="font-semibold text-brand hover:underline">
              Mot de passe oublié ?
            </Link>
          </p>
        </div>
        <Button type="submit" block size="lg" loading={busy}>
          Se connecter
        </Button>
        <OAuthButtons onSuccess={done} />
      </form>
    </AuthLayout>
  )
}
