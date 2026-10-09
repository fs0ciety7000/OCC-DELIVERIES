import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Link2, Link2Off, Mail, ShieldAlert, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Badge, Button, Card, CardBody, Field, Input, Sheet, Skeleton } from '@/components/ui'
import { pairError, type PasswordPair } from '@/features/auth/password'
import { PasswordFields } from '@/features/auth/PasswordFields'
import { providerLabel } from '@/features/auth/providers'
import { PopupBlockedError, usersApi } from '@/lib/api'
import { logout } from '@/lib/auth'
import { errorMessage, hasFieldError } from '@/lib/errors'
import { formatRelativeTime } from '@/lib/format'
import { qk } from '@/lib/queryKeys'
import type { AccountInfo, User } from '@/lib/types'

/** Mot à taper pour supprimer son compte (même règle que le serveur). */
export const DELETE_WORD = 'SUPPRIMER'

/** Profil → Mes infos → Sécurité : e-mail, mot de passe, comptes connectés. */
export function SecurityCard({ user }: { user: User }) {
  const account = useQuery({ queryKey: qk.account(user.id), queryFn: usersApi.account })
  if (account.isPending) return <Skeleton className="h-72 rounded-lg" />
  if (account.isError)
    return (
      <Card>
        <CardBody className="text-sm text-danger">{errorMessage(account.error)}</CardBody>
      </Card>
    )
  const acc = account.data
  return (
    <Card>
      <CardBody className="space-y-6">
        <div className="space-y-1">
          <h2 className="font-display text-lg font-semibold">Sécurité</h2>
          <p className="text-sm text-muted">Ton adresse, ton mot de passe et les comptes avec lesquels tu te connectes.</p>
        </div>
        <EmailSection user={user} acc={acc} />
        <PasswordSection user={user} acc={acc} />
        <ProvidersSection user={user} acc={acc} />
      </CardBody>
    </Card>
  )
}

function SectionTitle({ icon: Icon, children, id }: { icon: typeof Mail; children: string; id: string }) {
  return (
    <h3 id={id} className="flex items-center gap-2 font-semibold">
      <Icon aria-hidden className="size-4 text-brand" /> {children}
    </h3>
  )
}

/* --------------------------------------------------------------- e-mail */

function EmailSection({ user, acc }: { user: User; acc: AccountInfo }) {
  const [newEmail, setNewEmail] = useState('')
  const resend = useMutation({
    mutationFn: () => usersApi.requestVerification(acc.email),
    onSuccess: () => toast.success(`E-mail de confirmation envoyé à ${acc.email}`),
    onError: (e) => toast.error(errorMessage(e)),
  })
  const change = useMutation({
    mutationFn: () => usersApi.requestEmailChange(newEmail.trim()),
    onSuccess: () => {
      toast.success('Regarde ta nouvelle boîte mail', { description: `Clique sur le lien envoyé à ${newEmail.trim()} pour confirmer le changement.` })
      setNewEmail('')
    },
    onError: (e) => toast.error(hasFieldError(e, 'newEmail') ? 'Adresse invalide ou déjà utilisée par un autre compte.' : errorMessage(e)),
  })
  const same = newEmail.trim().toLowerCase() === (user.email ?? acc.email).toLowerCase()
  return (
    <section className="space-y-3" aria-labelledby="sec-email">
      <SectionTitle icon={Mail} id="sec-email">
        Adresse e-mail
      </SectionTitle>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <span className="font-medium break-all">{acc.email}</span>
        {acc.verified ? <Badge variant="success">Vérifiée</Badge> : <Badge variant="warning">Non vérifiée</Badge>}
        {!acc.verified && acc.mailEnabled && (
          <Button variant="ghost" size="sm" loading={resend.isPending} disabled={resend.isSuccess} onClick={() => resend.mutate()}>
            {resend.isSuccess ? 'E-mail envoyé' : "Renvoyer l'e-mail de confirmation"}
          </Button>
        )}
      </div>
      {!acc.mailEnabled ? (
        <p className="text-xs text-subtle">Changement d'adresse indisponible : l'envoi d'e-mails n'est pas configuré sur ce serveur.</p>
      ) : !acc.passwordSet ? (
        <p className="text-xs text-subtle">Pour changer d'adresse, choisis d'abord un mot de passe (ci-dessous) : il sert à confirmer le changement.</p>
      ) : (
        <form
          className="flex flex-col gap-2 sm:flex-row sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            if (newEmail.trim() && !same) change.mutate()
          }}
        >
          <Field label="Nouvelle adresse" className="min-w-0 flex-1" hint="Un lien de confirmation y sera envoyé ; l'adresse actuelle reste valable d'ici là.">
            {(p) => <Input {...p} type="email" autoComplete="email" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} placeholder="nouvelle@adresse.be" />}
          </Field>
          <Button type="submit" variant="secondary" loading={change.isPending} disabled={!newEmail.trim() || same} className="sm:mb-5">
            Changer d'adresse
          </Button>
        </form>
      )}
    </section>
  )
}

/* --------------------------------------------------------- mot de passe */

function PasswordSection({ user, acc }: { user: User; acc: AccountInfo }) {
  const qc = useQueryClient()
  const [current, setCurrent] = useState('')
  const [pair, setPair] = useState<PasswordPair>({ password: '', confirm: '' })
  const [currentError, setCurrentError] = useState<string | null>(null)
  const save = useMutation({
    mutationFn: () => usersApi.changePassword({ id: user.id, email: acc.email }, current, pair.password),
    onSuccess: () => {
      toast.success('Mot de passe changé', { description: 'Tes autres appareils ont été déconnectés.' })
      setCurrent('')
      setPair({ password: '', confirm: '' })
      void qc.invalidateQueries({ queryKey: qk.account(user.id) })
    },
    onError: (e) => {
      if (hasFieldError(e, 'oldPassword')) setCurrentError('Mot de passe actuel incorrect.')
      else toast.error(errorMessage(e))
    },
  })
  const sendLink = useMutation({
    mutationFn: () => usersApi.requestPasswordReset(acc.email),
    onSuccess: () => toast.success(`Lien envoyé à ${acc.email}`, { description: 'Il est valable 30 minutes.' }),
    onError: (e) => toast.error(errorMessage(e)),
  })
  const invalid = pairError(pair)
  return (
    <section className="space-y-3 border-t border-border pt-5" aria-labelledby="sec-password">
      <SectionTitle icon={KeyRound} id="sec-password">
        Mot de passe
      </SectionTitle>
      {acc.passwordSet ? (
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            setCurrentError(null)
            if (!invalid && current) save.mutate()
          }}
        >
          <Field label="Mot de passe actuel" error={currentError}>
            {(p) => <Input {...p} type="password" autoComplete="current-password" required value={current} onChange={(e) => setCurrent(e.target.value)} />}
          </Field>
          <PasswordFields value={pair} onChange={setPair} />
          <Button type="submit" variant="secondary" loading={save.isPending} disabled={!!invalid || !current}>
            Changer le mot de passe
          </Button>
        </form>
      ) : (
        <div className="space-y-2 text-sm">
          <p className="text-muted">Tu te connectes avec Google, sans mot de passe. Pour pouvoir aussi te connecter avec ton e-mail, reçois un lien pour en choisir un.</p>
          {acc.mailEnabled ? (
            <Button variant="secondary" size="sm" loading={sendLink.isPending} disabled={sendLink.isSuccess} onClick={() => sendLink.mutate()}>
              {sendLink.isSuccess ? 'Lien envoyé' : 'Recevoir un lien pour choisir un mot de passe'}
            </Button>
          ) : (
            <p className="text-xs text-subtle">Indisponible : l'envoi d'e-mails n'est pas configuré sur ce serveur.</p>
          )}
        </div>
      )}
    </section>
  )
}

/* ------------------------------------------------------ comptes connectés */

function ProvidersSection({ user, acc }: { user: User; acc: AccountInfo }) {
  const qc = useQueryClient()
  const methods = useQuery({ queryKey: qk.authMethods, queryFn: usersApi.authMethods, staleTime: 10 * 60_000, retry: false })
  const available = methods.data?.oauth2?.enabled ? methods.data.oauth2.providers : []
  const refresh = () => qc.invalidateQueries({ queryKey: qk.account(user.id) })
  const unlink = useMutation({
    mutationFn: (provider: string) => usersApi.unlinkProvider(provider),
    onSuccess: (_, provider) => {
      toast.success(`${providerLabel(provider)} dissocié`)
      void refresh()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  // Pas de useMutation ici : la fenêtre Google doit s'ouvrir dans le clic (bloqueurs).
  const [linking, setLinking] = useState<string | null>(null)
  const link = (provider: string) => {
    setLinking(provider)
    usersApi
      .oauth(provider)
      .then(() => {
        toast.success(`${providerLabel(provider)} associé à ton compte`)
        void refresh()
      })
      .catch((e: unknown) => toast.error(e instanceof PopupBlockedError ? e.message : errorMessage(e, 'Association annulée.')))
      .finally(() => setLinking(null))
  }
  const names = Array.from(new Set([...acc.providers.map((p) => p.provider), ...available.map((p) => p.name)]))
  if (names.length === 0) return null
  return (
    <section className="space-y-3 border-t border-border pt-5" aria-labelledby="sec-providers">
      <SectionTitle icon={Link2} id="sec-providers">
        Comptes connectés
      </SectionTitle>
      <ul className="space-y-2">
        {names.map((name) => {
          const linked = acc.providers.find((p) => p.provider === name)
          const label = providerLabel(name, available.find((p) => p.name === name)?.displayName)
          return (
            <li key={name} className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-elevated/40 px-3 py-2.5">
              <span className="font-semibold">{label}</span>
              {linked ? <Badge variant="success">{label} connecté</Badge> : <Badge>Non connecté</Badge>}
              {linked && <span className="text-xs text-subtle">depuis {formatRelativeTime(linked.created)}</span>}
              <span className="ml-auto">
                {linked ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    leftIcon={<Link2Off className="size-4" />}
                    disabled={!acc.passwordSet}
                    title={acc.passwordSet ? undefined : "Choisis d'abord un mot de passe : sans lui, tu ne pourrais plus te connecter."}
                    loading={unlink.isPending && unlink.variables === name}
                    onClick={() => unlink.mutate(name)}
                  >
                    Dissocier
                  </Button>
                ) : (
                  available.some((p) => p.name === name) && (
                    <Button variant="secondary" size="sm" loading={linking === name} onClick={() => link(name)}>
                      Associer {label}
                    </Button>
                  )
                )}
              </span>
              {linked && !acc.passwordSet && <p className="w-full text-xs text-subtle">Pour dissocier {label}, choisis d'abord un mot de passe.</p>}
            </li>
          )
        })}
      </ul>
    </section>
  )
}

/* ------------------------------------------------- suppression du compte */

/** Profil → Mes infos, juste avant « Se déconnecter » : suppression définitive (anonymisation). */
export function DeleteAccountCard() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState('')
  const ok = typed.trim().toUpperCase() === DELETE_WORD
  const del = useMutation({
    mutationFn: () => usersApi.deleteMe(typed),
    onSuccess: () => {
      logout()
      toast('Ton compte a été supprimé. À bientôt peut-être !')
      navigate('/', { replace: true })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  const close = () => {
    setOpen(false)
    setTyped('')
  }
  return (
    <Card className="border-danger/30">
      <CardBody className="space-y-3">
        <h2 className="flex items-center gap-2 font-display text-lg font-semibold">
          <ShieldAlert aria-hidden className="size-5 text-danger" /> Supprimer mon compte
        </h2>
        <p className="text-sm text-muted">
          Ton nom, ton adresse, ta photo et tes coordonnées de remboursement sont effacés. Les commandes passées restent dans l'historique de tes collègues sous « Compte supprimé », avec leurs montants.
        </p>
        <Button variant="danger" leftIcon={<Trash2 className="size-4" />} onClick={() => setOpen(true)}>
          Supprimer mon compte
        </Button>
      </CardBody>
      <Sheet
        open={open}
        onClose={close}
        title="Supprimer ton compte ?"
        description="C'est définitif : tu ne pourras plus te connecter à ce compte."
        footer={
          <div className="flex gap-2">
            <Button variant="secondary" block onClick={close}>
              Annuler
            </Button>
            <Button variant="danger" block disabled={!ok} loading={del.isPending} onClick={() => del.mutate()}>
              Supprimer définitivement
            </Button>
          </div>
        }
      >
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (ok) del.mutate()
          }}
        >
          <p className="text-sm text-muted">Les commandes en cours que tu organises ou pour lesquelles on te doit de l'argent doivent d'abord être clôturées.</p>
          <Field label={`Tape ${DELETE_WORD} pour confirmer`}>
            {(p) => <Input {...p} data-autofocus autoComplete="off" autoCapitalize="characters" spellCheck={false} value={typed} onChange={(e) => setTyped(e.target.value)} placeholder={DELETE_WORD} />}
          </Field>
        </form>
      </Sheet>
    </Card>
  )
}
