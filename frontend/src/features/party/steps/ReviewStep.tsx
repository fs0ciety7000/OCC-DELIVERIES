import { FileDown, Phone, RotateCcw, Send, Wallet } from 'lucide-react'
import { useState } from 'react'
import { Avatar, Badge, Button, Card, CardBody, EmptyState, Field, Input, Money, Segmented, Sheet, Skeleton } from '@/components/ui'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { centsToInput, formatTime, parseMoneyToCents } from '@/lib/format'
import { useConfig } from '@/lib/geo-context'
import type { Dispatch, DispatchMethod, SplitMode, Summary } from '@/lib/types'
import type { PartyCtx } from '../context'
import { useDispatch, useFees, useSetPayer, useSummary, useTransition } from '../hooks'
import { DISPATCH_LABELS } from '../labels'
import { DispatchedBanner } from './DispatchedBanner'
import { DispatchSheet } from './DispatchSheet'
import { ConsolidatedCard, ParticipantsList, TotalsCard } from './SummaryViews'

export function ReviewStep({ ctx }: { ctx: PartyCtx }) {
  const { party, isHost, me } = ctx
  const summary = useSummary(party.id)
  const [payerOpen, setPayerOpen] = useState(false)
  const transition = useTransition(party.id)
  // La traversée « Commande envoyée » attend que l'hôte ferme la sheet d'envoi.
  const [dispatchSheetOpen, setDispatchSheetOpen] = useState(false)

  if (summary.isPending) {
    return (
      <div className="grid gap-4 lg:grid-cols-2">
        <Skeleton className="h-64 rounded-lg" />
        <Skeleton className="h-64 rounded-lg" />
      </div>
    )
  }
  if (summary.isError) {
    return <EmptyState tone="danger" emoji="🧾" title="Récap indisponible" description={errorMessage(summary.error)} action={<Button onClick={() => summary.refetch()}>Réessayer</Button>} />
  }
  const s = summary.data

  return (
    <div className="grid gap-6 pb-28 lg:grid-cols-[minmax(0,1fr)_380px]">
      <div className="min-w-0 space-y-6">
        {party.dispatch && <DispatchedBanner party={party} paused={dispatchSheetOpen} />}
        {!isHost && (
          <Card className="border-info/30 bg-info/5">
            <CardBody className="text-sm">
              <strong className="font-semibold">Les paniers sont verrouillés.</strong>{' '}
              <span className="text-muted">L'hôte passe la commande et désignera qui a payé. Voici ta part estimée.</span>
            </CardBody>
          </Card>
        )}
        <section className="space-y-3" aria-labelledby="h-parts">
          <h2 id="h-parts" className="font-display text-xl font-semibold">
            Qui a pris quoi
          </h2>
          <ParticipantsList summary={s} meId={me.id} people={ctx.people} />
        </section>
        <ConsolidatedCard summary={s} />
      </div>

      <aside className="space-y-4 lg:sticky lg:top-[calc(var(--header-h)+16px)] lg:self-start">
        <TotalsCard summary={s} />
        {isHost && <FeesEditor key={`${party.delivery_fee}-${party.service_fee}-${party.tip}-${party.split_mode}`} ctx={ctx} summary={s} />}
        {isHost && <DispatchPanel ctx={ctx} onSheetChange={setDispatchSheetOpen} />}
      </aside>

      {isHost && (
        <div className="fixed inset-x-0 bottom-[calc(var(--tabbar-h)+env(safe-area-inset-bottom))] z-30 border-t border-border bg-bg/85 backdrop-blur-xl">
          <div className="mx-auto flex max-w-[1200px] gap-2 px-4 py-3 sm:px-8">
            <Button variant="secondary" size="lg" leftIcon={<RotateCcw className="size-4" />} loading={transition.isPending} onClick={() => transition.mutate({ to: 'ordering' })}>
              <span className="hidden sm:inline">Rouvrir la commande</span>
              <span className="sm:hidden">Rouvrir</span>
            </Button>
            <Button size="lg" className="flex-1" leftIcon={<Wallet className="size-5" />} onClick={() => setPayerOpen(true)}>
              Qui a payé ?
            </Button>
          </div>
        </div>
      )}

      <PayerSheet ctx={ctx} summary={s} open={payerOpen} onClose={() => setPayerOpen(false)} />
    </div>
  )
}

function FeesEditor({ ctx, summary }: { ctx: PartyCtx; summary: Summary }) {
  const { party } = ctx
  const fees = useFees(party.id)
  const [delivery, setDelivery] = useState(centsToInput(party.delivery_fee || summary.deliveryFee))
  const [service, setService] = useState(centsToInput(party.service_fee || 0))
  const [tip, setTip] = useState(centsToInput(party.tip || 0))
  const [split, setSplit] = useState<SplitMode>((party.split_mode || summary.splitMode || 'equal') as SplitMode)
  const parsed = { delivery: parseMoneyToCents(delivery), service: parseMoneyToCents(service), tip: parseMoneyToCents(tip) }
  const invalid = Object.values(parsed).some((v) => v === null)
  const dirty =
    parsed.delivery !== party.delivery_fee || parsed.service !== (party.service_fee || 0) || parsed.tip !== (party.tip || 0) || split !== (party.split_mode || 'equal')

  const moneyField = (label: string, value: string, set: (v: string) => void, key: keyof typeof parsed) => (
    <Field label={label} error={parsed[key] === null ? 'Montant invalide' : undefined}>
      {(p) => (
        <div className="relative">
          <Input {...p} inputMode="decimal" value={value} onChange={(e) => set(e.target.value)} className="pr-8 tabular" />
          <span aria-hidden className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-sm text-subtle">
            €
          </span>
        </div>
      )}
    </Field>
  )

  return (
    <Card>
      <CardBody>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (invalid) return
            fees.mutate({ delivery_fee: parsed.delivery!, service_fee: parsed.service!, tip: parsed.tip!, split_mode: split })
          }}
        >
          <h3 className="font-display text-lg font-semibold">Frais partagés</h3>
          <div className="grid grid-cols-3 gap-2">
            {moneyField('Livraison', delivery, setDelivery, 'delivery')}
            {moneyField('Service', service, setService, 'service')}
            {moneyField('Pourboire', tip, setTip, 'tip')}
          </div>
          <Segmented<SplitMode>
            label="Répartition des frais"
            value={split}
            onChange={setSplit}
            className="w-full"
            options={[
              { value: 'equal', label: 'Parts égales' },
              { value: 'proportional', label: 'Au prorata' },
            ]}
          />
          <Button type="submit" variant="secondary" block disabled={!dirty || invalid} loading={fees.isPending}>
            Mettre à jour les frais
          </Button>
        </form>
      </CardBody>
    </Card>
  )
}

const DISPATCH_TILES: { method: DispatchMethod; title: string; hint: string; className: string; icon?: React.ReactNode }[] = [
  { method: 'ubereats', title: 'Uber Eats', hint: 'Lien du resto + récap à saisir', className: 'border-ubereats/30 hover:border-ubereats/60 [&_[data-mark]]:bg-ubereats/15 [&_[data-mark]]:text-ubereats-ink' },
  { method: 'takeaway', title: 'Takeaway', hint: 'Lien Takeaway.com + récap', className: 'border-takeaway/30 hover:border-takeaway/60 [&_[data-mark]]:bg-takeaway/15 [&_[data-mark]]:text-takeaway-ink' },
  { method: 'deliveroo', title: 'Deliveroo', hint: 'Lien Deliveroo + récap', className: 'border-deliveroo/30 hover:border-deliveroo/60 [&_[data-mark]]:bg-deliveroo/15 [&_[data-mark]]:text-deliveroo-ink' },
  { method: 'weloveat', title: 'weloveat', hint: 'Lien weloveat.be + récap', className: 'border-weloveat-ink/30 hover:border-weloveat-ink/60 [&_[data-mark]]:bg-weloveat-ink/15 [&_[data-mark]]:text-weloveat-ink' },
  { method: 'export', title: 'Exporter', hint: 'CSV, TXT ou JSON', className: 'hover:border-border-strong', icon: <FileDown className="size-4" /> },
  { method: 'phone', title: 'Téléphone', hint: 'Script à dicter au resto', className: 'hover:border-border-strong', icon: <Phone className="size-4" /> },
]

const isPlatform = (m: DispatchMethod) => m !== 'export' && m !== 'phone'

function DispatchPanel({ ctx, onSheetChange }: { ctx: PartyCtx; onSheetChange?: (open: boolean) => void }) {
  const { party } = ctx
  const config = useConfig()
  const dispatch = useDispatch(party.id)
  const [result, setResult] = useState<Dispatch | null>(null)
  const enabledProviders = new Set((config.data?.providers ?? []).filter((p) => p.enabled).map((p) => p.id))
  const tiles = DISPATCH_TILES.filter((t) => (isPlatform(t.method) ? !config.data || enabledProviders.has(t.method) : true))

  return (
    <Card>
      <CardBody className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <h3 className="font-display text-lg font-semibold">Passer la commande</h3>
          {party.dispatch && (
            <Badge variant="success">
              {DISPATCH_LABELS[party.dispatch.method]} · {formatTime(party.dispatch.at)}
            </Badge>
          )}
        </div>
        <div className="grid grid-cols-2 gap-2">
          {tiles.map((t) => (
            <button
              key={t.method}
              type="button"
              disabled={dispatch.isPending}
              onClick={() =>
                dispatch.mutate(t.method, {
                  onSuccess: (r) => {
                    setResult(r.dispatch)
                    onSheetChange?.(true)
                  },
                })
              }
              className={cn(
                'flex min-h-22 flex-col items-start gap-1.5 rounded-md border border-border bg-elevated p-3 text-left transition-colors disabled:opacity-60',
                t.className,
                party.dispatch?.method === t.method && 'ring-2 ring-brand/50',
              )}
            >
              <span data-mark className="inline-flex items-center gap-1.5 rounded-full bg-fg/[0.07] px-2 py-0.5 text-xs font-bold">
                {t.icon ?? <Send className="size-3.5" />} {t.title}
              </span>
              <span className="text-xs text-muted">{t.hint}</span>
            </button>
          ))}
        </div>
      </CardBody>
      <DispatchSheet dispatch={result} party={party} phone={party.expand?.restaurant?.phone} restaurant={party.expand?.restaurant} onClose={() => {
          setResult(null)
          onSheetChange?.(false)
        }}
      />
    </Card>
  )
}

function PayerSheet({ ctx, summary, open, onClose }: { ctx: PartyCtx; summary: Summary; open: boolean; onClose: () => void }) {
  const { party, me, members, people } = ctx
  const setPayer = useSetPayer(party.id)
  const [selected, setSelected] = useState<string>(party.payer || me.id)
  const totals = new Map(summary.participants.map((p) => [p.user.id, p.total]))
  const candidates = members.map((m) => m.expand?.user ?? people.get(m.user) ?? { id: m.user, name: '' })

  return (
    <Sheet
      open={open}
      onClose={onClose}
      title="Qui a avancé l'argent ?"
      description="Chacun·e remboursera sa part à cette personne (QR virement depuis son app bancaire, Revolut, PayPal, espèces…)."
      footer={
        <Button block size="lg" loading={setPayer.isPending} onClick={() => setPayer.mutate(selected, { onSuccess: onClose })}>
          Valider et passer aux remboursements
        </Button>
      }
    >
      <div role="radiogroup" aria-label="Payeur" className="space-y-2">
        {candidates.map((u) => {
          const on = selected === u.id
          return (
            <button
              key={u.id}
              type="button"
              role="radio"
              aria-checked={on}
              onClick={() => setSelected(u.id)}
              className={cn(
                'flex min-h-14 w-full items-center gap-3 rounded-md border bg-surface px-3 py-2 text-left transition-colors',
                on ? 'border-brand/60 bg-brand/[0.06]' : 'border-border hover:border-border-strong',
              )}
            >
              <Avatar user={u} size={40} decorative />
              <span className="min-w-0 flex-1">
                <span className="block truncate font-semibold">{u.id === me.id ? `${u.name} (toi)` : u.name}</span>
                <span className="block text-xs text-muted">Sa part : <Money cents={totals.get(u.id) ?? 0} /></span>
              </span>
              <span aria-hidden className={cn('grid size-5 place-items-center rounded-full border-2', on ? 'border-brand' : 'border-border-strong')}>
                {on && <span className="size-2.5 rounded-full bg-brand" />}
              </span>
            </button>
          )
        })}
      </div>
      {!summary.minOrderReached && <p className="mt-3 text-sm text-warning">Attention : le minimum de commande n'est pas atteint.</p>}
    </Sheet>
  )
}
