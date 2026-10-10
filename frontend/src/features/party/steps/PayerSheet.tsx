import { Banknote, BellRing, Landmark } from 'lucide-react'
import { useId, useState } from 'react'
import { Avatar, Button, Money, Sheet } from '@/components/ui'
import { cn } from '@/lib/cn'
import type { CollectMode, User } from '@/lib/types'
import type { PartyCtx } from '../context'
import { usePayoutReadiness, usePayoutRequest, useSetPayer, useSummary } from '../hooks'
import { payerBlockReason, payoutHints, payoutRequired } from '../payout'
import { PayoutQuickAddForm } from './PayoutQuickAdd'

const MODES: { value: CollectMode; title: string; hint: string; icon: React.ReactNode }[] = [
  { value: 'transfer', title: 'Virement / Revolut / PayPal', hint: 'QR virement ou lien avec le montant de chacun·e', icon: <Landmark aria-hidden className="size-4" /> },
  { value: 'cash', title: 'Espèces uniquement', hint: 'De la main à la main, aucun IBAN nécessaire', icon: <Banknote aria-hidden className="size-4" /> },
]

/**
 * Choix du payeur et de son mode de remboursement (ADR 0003, mise à jour 4) :
 * `review → paying` (« Valider la commande ») ou changement en `paying`.
 * En mode virement, un payeur sans moyen de remboursement ne peut pas être
 * validé : ajout rapide de l'IBAN pour soi, demande par notification pour un·e
 * collègue, ou validation en espèces. Le serveur refait la vérification (409).
 */
export function PayerSheet({ ctx, open, onClose, variant }: { ctx: PartyCtx; open: boolean; onClose: () => void; variant: 'validate' | 'change' }) {
  const { party, me, members, people } = ctx
  const summary = useSummary(party.id, open)
  const readiness = usePayoutReadiness(party.id, open)
  const setPayer = useSetPayer(party.id)
  const ask = usePayoutRequest(party.id)
  const [selected, setSelected] = useState<string>(party.payer || me.id)
  const [chosenMode, setChosenMode] = useState<CollectMode | null>(null)
  const [quickAdd, setQuickAdd] = useState(false)
  const reasonId = useId()

  const statusById = new Map((readiness.data?.members ?? []).map((m) => [m.user, m]))
  const totals = new Map((summary.data?.participants ?? []).map((p) => [p.user.id, p.total]))
  const candidates: User[] = members.map((m) => m.expand?.user ?? people.get(m.user) ?? { id: m.user, name: '' })
  const target = candidates.find((u) => u.id === selected)
  const entry = statusById.get(selected)
  const required = payoutRequired(selected, summary.data)
  const mode: CollectMode = chosenMode ?? (variant === 'change' && party.collect_mode ? party.collect_mode : 'transfer')
  const reason = payerBlockReason({ payer: selected, meId: me.id, name: target?.name ?? '', mode, status: entry?.payout, required })
  const blocked = mode === 'transfer' && required && entry !== undefined && !entry.payout.ready
  const unchanged = variant === 'change' && selected === party.payer && mode === (party.collect_mode || 'transfer')
  const busy = setPayer.isPending

  const submit = (collectMode: CollectMode) => setPayer.mutate({ payer: selected, collectMode }, { onSuccess: onClose })

  return (
    <Sheet
      open={open}
      onClose={onClose}
      title={variant === 'validate' ? "Qui a avancé l'argent ?" : 'Changer de payeur'}
      description={
        variant === 'validate'
          ? 'Chacun·e remboursera sa part à cette personne, comme elle le préfère.'
          : "Possible tant qu'aucun remboursement n'a été confirmé : les parts sont recalculées."
      }
      footer={
        <div className="space-y-2">
          {reason && (
            <p id={reasonId} className="text-sm text-muted" role="status">
              {reason}
            </p>
          )}
          <Button block size="lg" disabled={!!reason || unchanged || summary.isPending} aria-describedby={reason ? reasonId : undefined} loading={busy && setPayer.variables?.collectMode === mode} onClick={() => submit(mode)}>
            {variant === 'validate' ? 'Valider la commande' : 'Confirmer le changement'}
          </Button>
          {blocked && (
            <Button block variant="secondary" leftIcon={<Banknote className="size-4" />} disabled={busy && setPayer.variables?.collectMode !== 'cash'} loading={busy && setPayer.variables?.collectMode === 'cash'} onClick={() => submit('cash')}>
              Pas d'IBAN ? Valider en espèces
            </Button>
          )}
        </div>
      }
    >
      <div className="space-y-5">
        <div role="radiogroup" aria-label="Payeur" className="space-y-2">
          {candidates.map((u) => {
            const on = selected === u.id
            const m = statusById.get(u.id)
            const hints = payoutHints(m?.payout)
            return (
              <button
                key={u.id}
                type="button"
                role="radio"
                aria-checked={on}
                onClick={() => {
                  setSelected(u.id)
                  setQuickAdd(false)
                }}
                className={cn(
                  'flex min-h-14 w-full items-center gap-3 rounded-md border bg-surface px-3 py-2 text-left transition-colors',
                  on ? 'border-brand/60 bg-brand/[0.06]' : 'border-border hover:border-border-strong',
                )}
              >
                <Avatar user={u} size={40} decorative />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-2">
                    <span className="truncate font-semibold">{u.id === me.id ? `${u.name} (toi)` : u.name}</span>
                    {variant === 'change' && party.payer === u.id && <span className="text-xs font-semibold text-brand">Actuel</span>}
                  </span>
                  <span className="block text-xs text-muted">
                    Sa part : <Money cents={totals.get(u.id) ?? 0} />
                  </span>
                  {m && (
                    <span className={cn('block text-xs', hints.length ? 'font-medium text-success' : 'text-muted')}>
                      {hints.length ? hints.join(' · ') : m.guest ? 'Invité·e · espèces uniquement' : 'Aucun moyen de remboursement'}
                    </span>
                  )}
                </span>
                <span aria-hidden className={cn('grid size-5 shrink-0 place-items-center rounded-full border-2', on ? 'border-brand' : 'border-border-strong')}>
                  {on && <span className="size-2.5 rounded-full bg-brand" />}
                </span>
              </button>
            )
          })}
        </div>

        <fieldset className="space-y-2">
          <legend className="mb-2 text-sm font-semibold">Remboursement de {selected === me.id ? 'ta' : 'sa'} avance</legend>
          <div role="radiogroup" aria-label="Mode de remboursement" className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {MODES.map((o) => {
              const on = mode === o.value
              return (
                <button
                  key={o.value}
                  type="button"
                  role="radio"
                  aria-checked={on}
                  onClick={() => setChosenMode(o.value)}
                  className={cn(
                    'flex min-h-14 items-start gap-2 rounded-md border bg-surface p-3 text-left transition-colors',
                    on ? 'border-brand/60 bg-brand/[0.06]' : 'border-border hover:border-border-strong',
                  )}
                >
                  <span className={cn('mt-0.5', on ? 'text-brand' : 'text-muted')}>{o.icon}</span>
                  <span className="min-w-0">
                    <span className="block text-sm font-semibold">{o.title}</span>
                    <span className="block text-xs text-muted">{o.hint}</span>
                  </span>
                </button>
              )
            })}
          </div>
        </fieldset>

        {blocked && selected === me.id && (
          <div className="space-y-3 rounded-md border border-warning/30 bg-warning/10 p-3">
            <p className="text-sm text-fg">
              <strong className="font-semibold">Aucun moyen de remboursement dans ton profil.</strong> Ajoute ton IBAN pour que tes collègues te remboursent par QR virement — ou valide en espèces.
            </p>
            {quickAdd ? (
              <PayoutQuickAddForm me={me} partyId={party.id} onSaved={() => setQuickAdd(false)} autoFocus />
            ) : (
              <Button variant="secondary" leftIcon={<Landmark className="size-4" />} onClick={() => setQuickAdd(true)}>
                Ajouter mon IBAN
              </Button>
            )}
          </div>
        )}
        {blocked && selected !== me.id && target && (
          <div className="space-y-3 rounded-md border border-warning/30 bg-warning/10 p-3">
            <p className="text-sm text-fg">
              <strong className="font-semibold">{target.name} n'a encore renseigné aucun moyen de remboursement.</strong>{' '}
              {entry?.guest ? 'En invité·e, il ou elle doit d’abord créer son compte.' : 'Demande-lui d’ajouter son IBAN, ou valide en espèces.'}
            </p>
            {!entry?.guest && (
              <Button variant="secondary" leftIcon={<BellRing className="size-4" />} loading={ask.isPending} disabled={ask.isSuccess && ask.variables === selected} onClick={() => ask.mutate(selected)}>
                {ask.isSuccess && ask.variables === selected ? 'Demande envoyée' : "Lui demander d'ajouter son IBAN"}
              </Button>
            )}
          </div>
        )}

        {summary.data && !summary.data.minOrderReached && <p className="text-sm text-warning">Attention : le minimum de commande n'est pas atteint.</p>}
      </div>
    </Sheet>
  )
}
