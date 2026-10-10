import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Landmark } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { toast } from 'sonner'
import { Badge, Button, buttonClass, Field, Input, Sheet } from '@/components/ui'
import { payoutApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { formatIban, isValidIban, normalizeIban } from '@/lib/iban'
import { qk } from '@/lib/queryKeys'
import type { User } from '@/lib/types'

/**
 * Ajout rapide de l'IBAN sans quitter la party (mêmes validation et
 * normalisation que le profil ; le serveur revalide). Les autres champs du
 * profil sont conservés ; Revolut / PayPal se renseignent dans le profil.
 */
export function PayoutQuickAddForm({ me, partyId, onSaved, autoFocus }: { me: User; partyId: string; onSaved?: () => void; autoFocus?: boolean }) {
  const qc = useQueryClient()
  const profile = useQuery({ queryKey: qk.payout(me.id), queryFn: () => payoutApi.mine(me.id), enabled: !me.is_guest })
  const [iban, setIban] = useState('')
  const [holder, setHolder] = useState('')
  const [touched, setTouched] = useState(false)
  // Erreur dès qu'un IBAN complet (≥ 16 car., longueur belge) est faux, ou à la sortie du champ.
  const error = iban && !isValidIban(iban) && (touched || normalizeIban(iban).length >= 16) ? 'IBAN invalide (vérifie les chiffres).' : undefined
  const valid = !!iban && isValidIban(iban)

  const save = useMutation({
    mutationFn: () => {
      const p = profile.data
      return payoutApi.save(me.id, p?.id ?? null, {
        holder_name: holder.trim() || p?.holder_name || '',
        iban: normalizeIban(iban),
        bic: p?.bic ?? '',
        revolut_tag: p?.revolut_tag ?? '',
        paypal_me: p?.paypal_me ?? '',
        payment_link: p?.payment_link ?? '',
      })
    },
    onSuccess: (p) => {
      qc.setQueryData(qk.payout(me.id), p)
      void qc.invalidateQueries({ queryKey: qk.payoutReadiness(partyId) })
      toast.success('IBAN enregistré', { description: 'Tes collègues pourront te rembourser par QR virement.' })
      onSaved?.()
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  if (me.is_guest) {
    return (
      <div className="space-y-3 rounded-md border border-border bg-elevated p-3 text-sm">
        <p>Tu participes en invité·e : crée ton compte pour enregistrer un IBAN et être remboursé·e par virement.</p>
        <Link to="/profile" className={buttonClass('secondary', 'sm')}>
          Créer mon compte
        </Link>
      </div>
    )
  }

  return (
    <form
      className="space-y-3"
      aria-label="Ajouter mon IBAN"
      onSubmit={(e) => {
        e.preventDefault()
        setTouched(true)
        if (valid) save.mutate()
      }}
    >
      <div className="flex items-center gap-2">
        <Landmark aria-hidden className="size-4 text-brand" />
        <span className="font-semibold">Virement (QR SEPA)</span>
        <Badge variant="brand">Recommandé</Badge>
      </div>
      <Field label="IBAN" error={error} hint={valid ? <span className="text-success">IBAN valide ✓</span> : 'Tes collègues scannent un QR avec leur app bancaire : montant et communication déjà remplis.'}>
        {(p) => (
          <Input
            {...p}
            value={iban}
            onChange={(e) => setIban(formatIban(e.target.value))}
            onBlur={() => setTouched(true)}
            placeholder="BE71 0961 2345 6769"
            autoComplete="off"
            spellCheck={false}
            autoFocus={autoFocus}
            className="tabular uppercase"
          />
        )}
      </Field>
      <Field label="Titulaire du compte" optional>
        {(p) => <Input {...p} value={holder} onChange={(e) => setHolder(e.target.value)} autoComplete="name" placeholder={profile.data?.holder_name || me.name} />}
      </Field>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <Button type="submit" disabled={!valid || profile.isPending} loading={save.isPending} className="sm:flex-1">
          Enregistrer mon IBAN
        </Button>
        <Link to="/profile?onglet=infos" className={buttonClass('ghost', 'md')}>
          Plutôt Revolut / PayPal ?
        </Link>
      </div>
      <p className="text-xs text-muted">Privé : ni l'hôte ni tes collègues ne voient ton IBAN, seulement le QR de leur part.</p>
    </form>
  )
}

/** La même chose dans une sheet (carte d'alerte, lien de notification `?iban=1`). */
export function PayoutQuickAddSheet({ me, partyId, open, onClose }: { me: User; partyId: string; open: boolean; onClose: () => void }) {
  return (
    <Sheet open={open} onClose={onClose} title="Ajoute ton IBAN" description="Pour être remboursé·e par virement si tu avances la commande.">
      {open && <PayoutQuickAddForm me={me} partyId={partyId} onSaved={onClose} autoFocus />}
    </Sheet>
  )
}
