import { AlertTriangle, ChevronDown } from 'lucide-react'
import { Avatar, Badge, Card, CardBody, CopyButton, Money } from '@/components/ui'
import { cn } from '@/lib/cn'
import { plural } from '@/lib/format'
import type { Summary } from '@/lib/types'
import { consolidatedText } from '../labels'

export function TotalsCard({ summary, className }: { summary: Summary; className?: string }) {
  const rows: [string, number][] = [
    ['Articles', summary.itemsSubtotal],
    ['Livraison', summary.deliveryFee],
    ['Frais de service', summary.serviceFee],
    ['Pourboire', summary.tip],
  ]
  return (
    <Card className={className}>
      <CardBody className="space-y-3">
        <dl className="space-y-1.5 text-sm">
          {rows.map(([label, v]) => (
            <div key={label} className="flex justify-between">
              <dt className="text-muted">{label}</dt>
              <dd>
                <Money cents={v} />
              </dd>
            </div>
          ))}
          <div className="flex items-baseline justify-between border-t border-border pt-2.5">
            <dt className="font-semibold">Total</dt>
            <dd>
              <Money cents={summary.grandTotal} className="font-display text-[32px] leading-9 font-bold" />
            </dd>
          </div>
        </dl>
        <p className="text-xs text-subtle">
          Frais partagés répartis {summary.splitMode === 'proportional' ? 'au prorata des paniers' : 'à parts égales'} entre les personnes qui commandent. Prix indicatifs.
        </p>
        {!summary.minOrderReached && summary.restaurant && (
          <div role="alert" className="flex gap-2 rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
            <AlertTriangle aria-hidden className="size-4 shrink-0 translate-y-0.5" />
            <span>
              Minimum de commande non atteint : il faut <Money cents={summary.restaurant.minOrder} /> d'articles (encore{' '}
              <Money cents={Math.max(0, summary.restaurant.minOrder - summary.itemsSubtotal)} />
              ).
            </span>
          </div>
        )}
      </CardBody>
    </Card>
  )
}

export function ParticipantsList({ summary, meId }: { summary: Summary; meId: string }) {
  const withItems = summary.participants.filter((p) => p.items.length > 0)
  const empty = summary.participants.filter((p) => p.items.length === 0)
  return (
    <div className="space-y-2">
      {withItems.map((p) => (
        <details key={p.user.id} className="group rounded-lg border border-border bg-surface shadow-card" open={p.user.id === meId}>
          <summary className="flex min-h-14 cursor-pointer list-none items-center gap-3 px-4 py-2.5 [&::-webkit-details-marker]:hidden">
            <Avatar user={p.user} size={40} ready={p.ready} />
            <div className="min-w-0 flex-1">
              <p className="truncate font-semibold">{p.user.id === meId ? 'Toi' : p.user.name}</p>
              <p className="text-xs text-muted tabular">
                {plural(p.items.reduce((n, i) => n + i.quantity, 0), 'article')} · <Money cents={p.subtotal} /> + <Money cents={p.sharedFees} /> de frais
              </p>
            </div>
            <Money cents={p.total} className="font-display text-lg font-semibold" />
            <ChevronDown aria-hidden className="size-4 text-muted transition-transform group-open:rotate-180" />
          </summary>
          <ul className="space-y-2 border-t border-border px-4 py-3 text-sm">
            {p.items.map((it) => (
              <li key={it.id} className="flex gap-3">
                <span className="w-7 font-semibold text-muted tabular">{it.quantity}×</span>
                <span className="min-w-0 flex-1">
                  {it.name}
                  {it.optionsLabel && <span className="block text-xs text-muted">{it.optionsLabel}</span>}
                  {it.note && <span className="block text-xs text-subtle italic">« {it.note} »</span>}
                </span>
                <Money cents={it.total} />
              </li>
            ))}
          </ul>
        </details>
      ))}
      {empty.length > 0 && (
        <p className="px-1 text-sm text-subtle">
          Sans commande : {empty.map((p) => (p.user.id === meId ? 'toi' : p.user.name)).join(', ')}
        </p>
      )}
    </div>
  )
}

export function ConsolidatedCard({ summary, className }: { summary: Summary; className?: string }) {
  return (
    <Card className={cn(className)}>
      <CardBody className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <h3 className="font-display text-lg font-semibold">Récap consolidé</h3>
          <CopyButton value={consolidatedText(summary)} label="Copier" toastMessage="Récap copié" />
        </div>
        <ul className="divide-y divide-border text-sm">
          {summary.consolidated.map((l, i) => (
            <li key={`${l.menuItem}-${i}`} className="flex gap-3 py-2">
              <Badge variant="brand" className="h-6 min-w-9 justify-center tabular">
                {l.quantity}×
              </Badge>
              <span className="min-w-0 flex-1">
                <span className="font-medium">{l.name}</span>
                {l.optionsLabel && <span className="block text-xs text-muted">{l.optionsLabel}</span>}
                {l.notes.length > 0 && <span className="block text-xs text-subtle italic">{l.notes.map((n) => `« ${n} »`).join(' · ')}</span>}
              </span>
              <Money cents={l.total} className="text-muted" />
            </li>
          ))}
        </ul>
      </CardBody>
    </Card>
  )
}
