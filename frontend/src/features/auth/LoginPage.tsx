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
            setError(errorMessage(err, 'E-mail ou mot de passe incorrect.'))
          } finally {
            setBusy(false)
          }
        }}
      >
        <Field label="E-mail">
          {(p) => <Input {...p} type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="prenom@entreprise.be" />}
        </Field>
        <Field label="Mot de passe" error={error}>
          {(p) => <Input {...p} type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />}
        </Field>
        <Button type="submit" block size="lg" loading={busy}>
          Se connecter
        </Button>
        <OAuthButtons onSuccess={done} />
      </form>
    </AuthLayout>
  )
}
