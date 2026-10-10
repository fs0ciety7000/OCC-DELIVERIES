import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Fingerprint, Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Field, Input, Sheet, Skeleton } from '@/components/ui'
import { useConfig } from '@/lib/geo-context'
import { errorMessage } from '@/lib/errors'
import { formatRelativeTime } from '@/lib/format'
import { passkeysApi, PrefetchedOptions, registerPasskey, type CreationBegin } from '@/lib/passkeys'
import { qk } from '@/lib/queryKeys'
import type { PasskeyView, User } from '@/lib/types'
import { passkeysSupported, webAuthnErrorMessage } from '@/lib/webauthn'

/**
 * Profil → Sécurité → Passkeys : liste, renommer, supprimer, ajouter.
 * Masqué si le navigateur ne gère pas WebAuthn ou si le serveur n'est pas configuré.
 */
export function PasskeysSection({ user }: { user: User }) {
  if (!passkeysSupported() || user.is_guest) return null
  return <PasskeysSectionInner user={user} />
}

function PasskeysSectionInner({ user }: { user: User }) {
  const config = useConfig()
  if (config.data?.passkeys !== true) return null
  return <PasskeysList user={user} />
}

function PasskeysList({ user }: { user: User }) {
  const qc = useQueryClient()
  const list = useQuery({ queryKey: qk.passkeys(user.id), queryFn: passkeysApi.list })
  // Options chargées d'avance : navigator.credentials.create est appelé directement dans le clic.
  const [prefetch] = useState(() => new PrefetchedOptions<CreationBegin>(() => passkeysApi.registerBegin()))
  useEffect(() => {
    prefetch.warm()
    const timer = setInterval(() => prefetch.warm(), 60_000)
    return () => clearInterval(timer)
  }, [prefetch])
  const [adding, setAdding] = useState(false)
  const refresh = () => qc.invalidateQueries({ queryKey: qk.passkeys(user.id) })

  // Pas de useMutation : la fenêtre du navigateur doit s'ouvrir dans le geste.
  const add = () => {
    setAdding(true)
    registerPasskey(prefetch.take())
      .then((pk) => {
        toast.success('Passkey ajoutée', { description: `« ${pk.name} » : tu peux maintenant te connecter sans mot de passe.` })
        void refresh()
      })
      .catch((e: unknown) => {
        const msg = webAuthnErrorMessage(e)
        if (msg) toast.error(msg)
        else if (!(e instanceof DOMException)) toast.error(errorMessage(e))
      })
      .finally(() => {
        setAdding(false)
        prefetch.warm()
      })
  }

  const items = list.data ?? []
  return (
    <section className="space-y-3 border-t border-border pt-5" aria-labelledby="sec-passkeys">
      <h3 id="sec-passkeys" className="flex items-center gap-2 font-semibold">
        <Fingerprint aria-hidden className="size-4 text-brand" /> Passkeys
      </h3>
      <p className="text-sm text-muted">Connecte-toi d'un geste — empreinte, visage ou code de ton appareil — sans taper de mot de passe.</p>
      {list.isPending ? (
        <Skeleton className="h-16 rounded-md" />
      ) : list.isError ? (
        <p className="text-sm text-danger">{errorMessage(list.error)}</p>
      ) : items.length === 0 ? (
        <p className="text-sm text-subtle">Aucune passkey pour l'instant.</p>
      ) : (
        <ul className="space-y-2">
          {items.map((pk) => (
            <PasskeyRow key={pk.id} pk={pk} onChanged={refresh} />
          ))}
        </ul>
      )}
      <Button variant="secondary" size="sm" leftIcon={<Plus className="size-4" />} loading={adding} onClick={add}>
        Ajouter une passkey
      </Button>
    </section>
  )
}

function PasskeyRow({ pk, onChanged }: { pk: PasskeyView; onChanged: () => void }) {
  const [editing, setEditing] = useState(false)
  const [name, setName] = useState(pk.name)
  const [confirming, setConfirming] = useState(false)
  const rename = useMutation({
    mutationFn: () => passkeysApi.rename(pk.id, name.trim()),
    onSuccess: () => {
      setEditing(false)
      onChanged()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  const remove = useMutation({
    mutationFn: () => passkeysApi.remove(pk.id),
    onSuccess: () => {
      setConfirming(false)
      toast.success('Passkey supprimée', { description: 'Pense aussi à la retirer de ton appareil ou de ton gestionnaire de mots de passe.' })
      onChanged()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  return (
    <li className="space-y-2 rounded-md border border-border bg-elevated/40 px-3 py-2.5">
      {editing ? (
        <form
          className="flex flex-col gap-2 sm:flex-row sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            if (name.trim()) rename.mutate()
          }}
        >
          <Field label="Nom de la passkey" className="min-w-0 flex-1">
            {(p) => <Input {...p} autoFocus maxLength={60} value={name} onChange={(e) => setName(e.target.value)} />}
          </Field>
          <div className="flex gap-2 sm:mb-5">
            <Button type="submit" size="sm" loading={rename.isPending} disabled={!name.trim()}>
              Enregistrer
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setEditing(false)
                setName(pk.name)
              }}
            >
              Annuler
            </Button>
          </div>
        </form>
      ) : (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span className="min-w-0 font-semibold break-words">{pk.name}</span>
          {pk.synced && <Badge variant="info">Synchronisée</Badge>}
          <span className="ml-auto flex gap-1">
            <Button variant="ghost" size="sm" leftIcon={<Pencil className="size-4" />} aria-label={`Renommer ${pk.name}`} onClick={() => setEditing(true)}>
              Renommer
            </Button>
            <Button variant="ghost" size="sm" className="text-danger" leftIcon={<Trash2 className="size-4" />} aria-label={`Supprimer ${pk.name}`} onClick={() => setConfirming(true)}>
              Supprimer
            </Button>
          </span>
          <p className="w-full text-xs text-subtle">
            Ajoutée {formatRelativeTime(pk.created)} · {pk.lastUsedAt ? `utilisée ${formatRelativeTime(pk.lastUsedAt)}` : 'jamais utilisée'}
          </p>
        </div>
      )}
      <Sheet
        open={confirming}
        onClose={() => setConfirming(false)}
        title={`Supprimer « ${pk.name} » ?`}
        description="Tu ne pourras plus te connecter avec cette passkey. Ton mot de passe et tes autres moyens de connexion ne changent pas."
        footer={
          <div className="flex gap-2">
            <Button variant="secondary" block onClick={() => setConfirming(false)}>
              Annuler
            </Button>
            <Button variant="danger" block loading={remove.isPending} onClick={() => remove.mutate()}>
              Supprimer la passkey
            </Button>
          </div>
        }
      >
        <p className="text-sm text-muted">Elle restera peut-être proposée par ton appareil : supprime-la aussi de ton trousseau ou gestionnaire de mots de passe.</p>
      </Sheet>
    </li>
  )
}
