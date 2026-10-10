import { Landmark, X } from 'lucide-react'
import { useState } from 'react'
import { Button, Card, CardBody } from '@/components/ui'
import type { PartyCtx } from '../context'
import { usePayoutReadiness } from '../hooks'
import { dismissNudge, isNudgeDismissed } from '../payout'
import { PayoutQuickAddSheet } from './PayoutQuickAdd'

/**
 * Alerte douce pendant les paniers et le récap (ADR 0003, mise à jour 4) :
 * un membre sans moyen de remboursement est invité à ajouter son IBAN (carte
 * masquable par party) ; l'hôte voit combien de membres peuvent être
 * remboursés par virement. Jamais bloquant : les espèces restent possibles.
 */
export function PayoutNudge({ ctx }: { ctx: PartyCtx }) {
  const { party, me, isHost } = ctx
  const readiness = usePayoutReadiness(party.id)
  const [dismissed, setDismissed] = useState(() => isNudgeDismissed(party.id))
  const [open, setOpen] = useState(false)
  const data = readiness.data
  if (!data) return null
  const mine = data.members.find((m) => m.user === me.id)
  const showMine = !!mine && !mine.payout.ready && !mine.guest && !dismissed

  return (
    <>
      {showMine && (
        <Card className="border-info/30 bg-info/5">
          <CardBody className="flex items-start gap-3">
            <Landmark aria-hidden className="mt-0.5 size-5 shrink-0 text-info" />
            <div className="min-w-0 flex-1 space-y-2">
              <p className="text-sm">
                <strong className="font-semibold">Ajoute ton IBAN pour être remboursé·e par virement si tu avances la commande.</strong>{' '}
                <span className="text-muted">Sinon, tes collègues te rembourseront en espèces.</span>
              </p>
              <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
                Ajouter mon IBAN
              </Button>
            </div>
            <button
              type="button"
              aria-label="Masquer ce conseil pour cette commande"
              className="-mt-2 -mr-2 grid size-11 shrink-0 place-items-center rounded-md text-muted hover:bg-fg/[0.06] hover:text-fg"
              onClick={() => {
                dismissNudge(party.id)
                setDismissed(true)
              }}
            >
              <X aria-hidden className="size-4" />
            </button>
          </CardBody>
        </Card>
      )}
      {isHost && data.total > 1 && (
        <p className="flex items-center gap-2 text-sm text-muted" aria-live="polite">
          <Landmark aria-hidden className="size-4 shrink-0" />
          {data.readyCount === 0
            ? 'Personne n’a encore renseigné d’IBAN : le remboursement se fera en espèces, sauf si quelqu’un en ajoute un.'
            : `${data.readyCount} membre${data.readyCount > 1 ? 's' : ''} sur ${data.total} ${data.readyCount > 1 ? 'peuvent' : 'peut'} être remboursé${data.readyCount > 1 ? 's' : ''} par virement.`}
        </p>
      )}
      <PayoutQuickAddSheet me={me} partyId={party.id} open={open} onClose={() => setOpen(false)} />
    </>
  )
}
