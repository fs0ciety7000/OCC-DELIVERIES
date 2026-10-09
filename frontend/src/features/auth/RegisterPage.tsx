import { useState } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router'
import { toast } from 'sonner'
import { Button, Field, Input } from '@/components/ui'
import { usersApi } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { errorMessage } from '@/lib/errors'
import { useConfig } from '@/lib/geo-context'
import { AuthLayout } from './AuthLayout'
import { safeNext } from './safeNext'
import { OAuthButtons } from './OAuthButtons'
import { StrengthMeter } from './PasswordFields'

export function RegisterPage() {
  const [params] = useSearchParams()
  const next = safeNext(params.get('next'))
  const navigate = useNavigate()
  const { isAuthenticated } = useAuth()
  const config = useConfig()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const tooShort = password.length > 0 && password.length < 8

  if (isAuthenticated && !busy) return <Navigate to={next} replace />

  const done = (withPassword: boolean) => {
    toast.success('Bienvenue à bord ! 🎉', {
      description: withPassword && config.data?.mailEnabled ? 'Un e-mail de confirmation vient de partir : clique sur le lien quand tu as un moment.' : undefined,
    })
    navigate(next, { replace: true })
  }

  return (
    <AuthLayout
      title="Rejoins la tablée"
      subtitle="Un compte pour voter, commander et rembourser en un scan."
      footer={
        <>
          Déjà un compte ?{' '}
          <Link to={`/login?next=${encodeURIComponent(next)}`} className="font-semibold text-brand hover:underline">
            Connecte-toi
          </Link>
        </>
      }
    >
      <form
        className="space-y-4"
        onSubmit={async (e) => {
          e.preventDefault()
          if (password.length < 8) return
          setError(null)
          setBusy(true)
          try {
            await usersApi.register(name.trim(), email.trim(), password)
            done(true)
          } catch (err) {
            setError(errorMessage(err, "Impossible de créer le compte. L'e-mail est peut-être déjà utilisé."))
          } finally {
            setBusy(false)
          }
        }}
      >
        <Field label="Prénom (ou surnom)" hint="C'est ce que verront tes collègues.">
          {(p) => <Input {...p} autoComplete="given-name" required maxLength={60} value={name} onChange={(e) => setName(e.target.value)} placeholder="Alice" />}
        </Field>
        <Field label="E-mail">
          {(p) => <Input {...p} type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="prenom@entreprise.be" />}
        </Field>
        <Field label="Mot de passe" hint="8 caractères minimum." error={tooShort ? 'Encore un petit effort : 8 caractères minimum.' : error}>
          {(p) => <Input {...p} type="password" autoComplete="new-password" required minLength={8} value={password} onChange={(e) => setPassword(e.target.value)} />}
        </Field>
        <StrengthMeter password={password} />
        <Button type="submit" block size="lg" loading={busy}>
          Créer mon compte
        </Button>
        <OAuthButtons onSuccess={() => done(false)} />
      </form>
    </AuthLayout>
  )
}
