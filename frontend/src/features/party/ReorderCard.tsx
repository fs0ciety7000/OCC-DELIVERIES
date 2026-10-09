import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { History } from 'lucide-react'
import { toast } from 'sonner'
import { Button, Card, CardBody, Money } from '@/components/ui'
import { occ } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { formatDate, plural } from '@/lib/format'
import { qk } from '@/lib/queryKeys'

const SHOWN = 4

/**
 * « Comme la dernière fois ? » : si j'ai déjà commandé dans ce restaurant, propose de remettre
 * mes plats dans le panier. Le serveur revalide tout (disponibilité, options, prix).
 */
export function ReorderCard({ partyId }: { partyId: string }) {
  const qc = useQueryClient()
  const preview = useQuery({ queryKey: qk.reorder(partyId), queryFn: () => occ.reorderPreview(partyId), staleTime: 60_000 })
  const apply = useMutation({
    mutationFn: () => occ.reorder(partyId),
    onSuccess: (res) => {
      const n = res.added.reduce((s, a) => s + a.quantity, 0)
      if (n > 0) toast.success(`${plural(n, 'plat')} remis dans ton panier`)
      if (res.skipped.length > 0) {
        toast.warning(n > 0 ? 'Certains plats ne sont plus proposés' : 'Aucun plat de ta dernière commande n’est encore proposé', {
          description: res.skipped.map((s) => `${s.name} : ${s.reason}`).join(' · '),
        })
      }
      void qc.invalidateQueries({ queryKey: qk.items(partyId) })
      void qc.invalidateQueries({ queryKey: qk.members(partyId) })
      void qc.invalidateQueries({ queryKey: qk.summary(partyId) })
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  const data = preview.data
  // Pas de commande précédente (ou chargement / erreur) : rien à proposer, l'écran reste inchangé.
  if (!data?.source) return null
  const available = data.items.filter((i) => i.available)
  const unavailable = data.items.filter((i) => !i.available)
  if (available.length === 0) return null
  const total = available.reduce((s, i) => s + i.unitPrice * i.quantity, 0)

  return (
    <Card className="border-brand/30">
      <CardBody className="space-y-3">
        <div className="flex items-start gap-3">
          <span aria-hidden className="grid size-10 shrink-0 place-items-center rounded-full bg-brand/12 text-brand">
            <History className="size-5" />
          </span>
          <div className="min-w-0">
            <h3 className="font-display text-lg leading-6 font-semibold">Comme la dernière fois ?</h3>
            <p className="text-sm text-muted">
              Ta commande du {formatDate(data.source.created)}
              {data.source.title ? ` (« ${data.source.title} »)` : ''}.
            </p>
          </div>
        </div>
        <ul className="space-y-1 text-sm" aria-label="Plats de ta dernière commande">
          {available.slice(0, SHOWN).map((i, k) => (
            <li key={`${i.menuItem}-${k}`} className="flex gap-2">
              <span className="w-6 shrink-0 text-muted tabular">{i.quantity}×</span>
              <span className="min-w-0 flex-1 truncate">
                {i.name}
                {i.optionsLabel && <span className="text-muted"> · {i.optionsLabel}</span>}
              </span>
            </li>
          ))}
          {available.length > SHOWN && <li className="text-xs text-subtle">+ {plural(available.length - SHOWN, 'autre plat', 'autres plats')}</li>}
        </ul>
        {unavailable.length > 0 && (
          <p className="text-xs text-warning">
            Plus proposé : {unavailable.map((i) => i.name).join(', ')}
          </p>
        )}
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-sm text-muted">
            Environ <Money cents={total} className="font-semibold text-fg" />
          </span>
          <Button variant="secondary" loading={apply.isPending} onClick={() => apply.mutate()}>
            Reprendre ma dernière commande
          </Button>
        </div>
      </CardBody>
    </Card>
  )
}
