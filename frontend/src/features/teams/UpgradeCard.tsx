import { useMutation, useQueryClient } from '@tanstack/react-query'
import { UserPlus } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button, Card, CardBody, Field, Input } from '@/components/ui'
import { OAuthButtons } from '@/features/auth/OAuthButtons'
import { guestApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import type { User } from '@/lib/types'

/**
 * Profil d'un·e invité·e : « Créer mon compte » (e-mail + mot de passe, ou Google).
 * Le même compte est converti : commandes, équipes et remboursements sont conservés.
 */
export function UpgradeCard({ user }: { user: User }) {
  const qc = useQueryClient()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const mismatch = confirm.length > 0 && confirm !== password
  const done = () => {
    toast.success('Ton compte est créé : ton historique est conservé.')
    void qc.invalidateQueries()
  }
  const upgrade = useMutation({
    mutationFn: () => guestApi.upgrade({ email: email.trim(), password }),
    onSuccess: done,
  })
  return (
    <Card id="compte">
      <CardBody className="space-y-4">
        <div className="flex items-start gap-3">
          <span aria-hidden className="grid size-10 shrink-0 place-items-center rounded-full bg-info/12 text-info">
            <UserPlus className="size-5" />
          </span>
          <div className="space-y-1">
            <h2 className="font-display text-lg font-semibold">Créer mon compte</h2>
            <p className="text-sm text-muted">
              Tu utilises OCC Deliveries en invité·e, {user.name}. Garde tes commandes et tes équipes, lance tes propres commandes et enregistre tes coordonnées de
              remboursement.
            </p>
          </div>
        </div>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (mismatch || password.length < 8) return
            upgrade.mutate()
          }}
        >
          <Field label="E-mail">{(p) => <Input {...p} type="email" required value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" />}</Field>
          <Field label="Mot de passe" hint="8 caractères minimum." error={password.length > 0 && password.length < 8 ? 'Au moins 8 caractères.' : undefined}>
            {(p) => <Input {...p} type="password" required value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" minLength={8} />}
          </Field>
          <Field label="Confirme le mot de passe" error={mismatch ? 'Les deux mots de passe ne correspondent pas.' : undefined}>
            {(p) => <Input {...p} type="password" required value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" />}
          </Field>
          {upgrade.isError && (
            <p role="alert" className="text-sm text-danger">
              {errorMessage(upgrade.error)}
            </p>
          )}
          <Button type="submit" block loading={upgrade.isPending} disabled={mismatch}>
            Créer mon compte
          </Button>
        </form>
        <OAuthButtons onSuccess={done} />
      </CardBody>
    </Card>
  )
}
