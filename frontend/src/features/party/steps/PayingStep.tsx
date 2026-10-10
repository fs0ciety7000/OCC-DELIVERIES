import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, Lock, RotateCcw, UserRoundCog } from 'lucide-react'
import { motion } from 'motion/react'
import { Suspense, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router'
import { PaymentCoin } from '@/components/food'
import { Avatar, Badge, Button, buttonClass, Card, CardBody, EmptyState, Money, Sheet, Skeleton } from '@/components/ui'
import { occ, payoutApi } from '@/lib/api'
import { useMediaQuery } from '@/lib/hooks'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { formatRelativeTime } from '@/lib/format'
import { haptic } from '@/lib/haptics'
import { spring } from '@/lib/motion'
import { qk } from '@/lib/queryKeys'
import type { DeclareMethod, Payment } from '@/lib/types'
import type { PartyCtx } from '../context'
import { usePaymentAction, usePayments, useSetPayer, useTransition } from '../hooks'
import { availableMethods, declareLabel, isLinkMethod, linkFor, METHOD_BADGE, METHOD_LABELS, methodHint, orderForDevice, payoutMethods } from '../labels'
import { CollectPanel } from './CollectPanel'
import { DispatchedBanner } from './DispatchedBanner'
import { MethodDetails, MethodMark, MethodTiles, StatusBadge } from './PaymentMethods'

export function PayingStep({ ctx }: { ctx: PartyCtx }) {
  const { party, me, isHost } = ctx
  const payments = usePayments(party.id)
  const transition = useTransition(party.id)
  const [closeOpen, setCloseOpen] = useState(false)
  const [payerOpen, setPayerOpen] = useState(false)

  if (payments.isPending) {
    return (
      <div className="grid gap-4 lg:grid-cols-2">
        <Skeleton className="h-96 rounded-lg" />
        <Skeleton className="h-64 rounded-lg" />
      </div>
    )
  }
  if (payments.isError) {
    return <EmptyState tone="danger" emoji="💸" title="Paiements indisponibles" description={errorMessage(payments.error)} action={<Button onClick={() => payments.refetch()}>Réessayer</Button>} />
  }

  const list = payments.data
  const mine = list.find((p) => p.debtor === me.id)
  const payerId = party.payer || list[0]?.creditor
  const iAmPayer = payerId === me.id
  const payer = payerId ? ctx.people.get(payerId) : undefined
  const owed = list.filter((p) => p.debtor !== p.creditor && p.method !== 'self')
  const total = owed.reduce((s, p) => s + p.amount, 0)
  const confirmed = owed.filter((p) => p.status === 'confirmed').reduce((s, p) => s + p.amount, 0)
  const pct = total ? Math.round((confirmed / total) * 100) : 100

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_400px]">
      <div className="min-w-0 space-y-4">
        {party.dispatch && <DispatchedBanner party={party} />}
        <Card>
          <CardBody className="space-y-3">
            <div className="flex items-center gap-3">
              {payer && <Avatar user={payer} size={40} />}
              <div className="min-w-0 flex-1">
                <p className="text-sm text-muted">{iAmPayer ? 'Tu as avancé l’argent' : 'À rembourser à'}</p>
                <p className={cn('font-display text-lg leading-6 font-semibold', iAmPayer ? 'text-pretty' : 'truncate')}>{iAmPayer ? 'Merci pour l’avance 🙌' : (payer?.name ?? 'Le payeur')}</p>
              </div>
              <div className="shrink-0 text-right">
                <p className="text-xs text-muted">Remboursé</p>
                <p className="text-sm font-semibold tabular">
                  <Money cents={confirmed} /> / <Money cents={total} />
                </p>
              </div>
            </div>
            <div className="h-2.5 overflow-hidden rounded-full bg-fg/[0.08]" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct} aria-label="Progression des remboursements">
              <motion.div className="h-full rounded-full bg-ember" initial={false} animate={{ width: `${pct}%` }} transition={spring} />
            </div>
          </CardBody>
        </Card>

        {mine && !iAmPayer && mine.method !== 'self' ? (
          <MyShare ctx={ctx} payment={mine} />
        ) : !iAmPayer ? (
          <Card>
            <CardBody className="text-sm text-muted">Tu n'as rien commandé : rien à rembourser 😉</CardBody>
          </Card>
        ) : (
          <CollectPanel ctx={ctx} payments={owed} />
        )}
      </div>

      <aside className="space-y-4">
        {iAmPayer && <PayerMethods userId={me.id} />}
        {/* Le payeur confirme depuis « Encaisser » : pas de seconde liste avec les mêmes boutons. */}
        {!iAmPayer && <PaymentsList ctx={ctx} payments={owed} canManage={isHost} />}
        {isHost && (
          <div className="flex flex-col gap-2">
            <Button variant="secondary" leftIcon={<UserRoundCog className="size-4" />} onClick={() => setPayerOpen(true)}>
              Changer de payeur
            </Button>
            <Button variant="ghost" leftIcon={<Lock className="size-4" />} onClick={() => setCloseOpen(true)}>
              Clôturer la commande
            </Button>
          </div>
        )}
      </aside>

      <Sheet
        open={closeOpen}
        onClose={() => setCloseOpen(false)}
        title="Clôturer maintenant ?"
        description={pct < 100 ? 'Certaines parts ne sont pas encore confirmées.' : 'Tout est remboursé 🎉'}
        footer={
          <Button block size="lg" loading={transition.isPending} onClick={() => transition.mutate({ to: 'closed' }, { onSuccess: () => setCloseOpen(false) })}>
            Clôturer
          </Button>
        }
      >
        <p className="text-sm text-muted">La commande passera dans l'historique. Les parts non confirmées resteront visibles.</p>
      </Sheet>
      <ChangePayerSheet ctx={ctx} open={payerOpen} onClose={() => setPayerOpen(false)} />
    </div>
  )
}

function MyShare({ ctx, payment }: { ctx: PartyCtx; payment: Payment }) {
  const { party } = ctx
  const action = usePaymentAction(party.id)
  // Le profil du payeur est privé (pas de realtime) : on relit régulièrement
  // pour voir apparaître un moyen qu'il vient d'ajouter.
  const qr = useQuery({
    queryKey: qk.paymentQR(payment.id),
    queryFn: () => occ.paymentQR(payment.id),
    enabled: payment.status !== 'confirmed',
    staleTime: 15_000,
    refetchOnWindowFocus: true,
    refetchInterval: payment.status === 'pending' ? 60_000 : false,
  })
  const desktop = useMediaQuery('(min-width: 1024px)')
  const methods = orderForDevice(availableMethods(qr.data), qr.data, !desktop)
  const hints = Object.fromEntries(methods.map((m) => [m, methodHint(m, qr.data)])) as Partial<Record<DeclareMethod, string>>
  const [chosen, setChosen] = useState<DeclareMethod | null>(null)
  const [changing, setChanging] = useState(false)
  const method = chosen ?? methods[0] ?? null
  const declared = payment.status === 'declared'
  // Vibration courte quand le payeur confirme ma part (évènement realtime, pas au chargement).
  const lastStatus = useRef(payment.status)
  useEffect(() => {
    if (lastStatus.current !== 'confirmed' && payment.status === 'confirmed') haptic('paid')
    lastStatus.current = payment.status
  }, [payment.status])
  const showPicker = payment.status === 'pending' || changing

  return (
    <Card className="overflow-hidden">
      <div className="space-y-1 border-b border-border bg-elevated/60 p-4 sm:p-5">
        <div className="flex items-center justify-between gap-2">
          <h2 className="font-display text-lg font-semibold">Ma part</h2>
          <StatusBadge status={payment.status} />
        </div>
        <Money cents={payment.amount} className="block font-display text-[40px] leading-[44px] font-bold" />
        <p className="text-sm text-muted">
          Communication : <span className="font-medium text-fg">{payment.reference}</span>
        </p>
      </div>
      <CardBody className="space-y-4">
        {payment.status === 'confirmed' ? (
          <>
          <Suspense fallback={null}>
            <PaymentCoin size={80} />
          </Suspense>
          <div className="flex items-center gap-3 rounded-md border border-success/30 bg-success/10 p-3 text-success">
            <CheckCircle2 className="size-6 shrink-0" />
            <p className="text-sm font-medium">C'est réglé, merci ! {payment.confirmed_at && <span className="text-muted">({formatRelativeTime(payment.confirmed_at)})</span>}</p>
          </div>
          </>
        ) : declared && !changing ? (
          <div className="space-y-3">
            <div className="flex items-center gap-3 rounded-md border border-info/30 bg-info/10 p-3">
              <Badge variant={METHOD_BADGE[payment.method || 'qr']}>{METHOD_LABELS[payment.method || 'qr']}</Badge>
              <p className="text-sm">En attente de confirmation par le payeur.</p>
            </div>
            <Button variant="ghost" size="sm" onClick={() => setChanging(true)}>
              Changer de moyen
            </Button>
          </div>
        ) : null}

        {payment.status !== 'confirmed' && showPicker && (
          <>
            {qr.isPending ? (
              <Skeleton className="h-40 rounded-md" />
            ) : qr.isError ? (
              <p className="text-sm text-danger">{errorMessage(qr.error)}</p>
            ) : (
              <>
                <MethodTiles methods={methods} value={method} onChange={setChosen} hints={hints} />
                {method && qr.data && (
                  <motion.div key={method} initial={{ opacity: 0, y: 6 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.2 }}>
                    <MethodDetails method={method} qr={qr.data} paymentId={payment.id} mobile={!desktop} />
                  </motion.div>
                )}
                {method && (
                  <Button
                    block
                    size="lg"
                    // Un seul primary par moyen : le lien prérempli (Revolut, PayPal…) passe avant « J'ai payé ».
                    variant={method === 'later' || (isLinkMethod(method) && !!linkFor(qr.data, method)?.amountPrefilled) ? 'secondary' : 'primary'}
                    loading={action.isPending}
                    onClick={() =>
                      action.mutate(
                        { paymentId: payment.id, action: 'declare', method },
                        { onSuccess: () => setChanging(false) },
                      )
                    }
                  >
                    {declareLabel(method)}
                  </Button>
                )}
              </>
            )}
          </>
        )}
      </CardBody>
    </Card>
  )
}

function PayerMethods({ userId }: { userId: string }) {
  const profile = useQuery({ queryKey: qk.payout(userId), queryFn: () => payoutApi.mine(userId) })
  if (!profile.isSuccess) return null
  const methods = payoutMethods(profile.data)
  return (
    <Card>
      <CardBody className="space-y-3">
        <h2 className="font-display text-lg font-semibold">Tes moyens de remboursement</h2>
        {methods.length ? (
          <>
            <p className="text-sm text-muted">Tes collègues voient, avec leur montant exact :</p>
            <ul className="flex flex-wrap gap-2" aria-label="Moyens proposés">
              {methods.map((m) => (
                <li key={m} className="flex items-center gap-2 rounded-md border border-border bg-surface py-1 pr-3 pl-1 text-sm font-medium">
                  <MethodMark method={m} /> {METHOD_LABELS[m]}
                </li>
              ))}
            </ul>
          </>
        ) : (
          <p className="rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
            Aucun moyen renseigné : tes collègues ne peuvent te rembourser qu'en espèces. Ajoute ton IBAN, Revolut, PayPal ou Wero.
          </p>
        )}
        <Link to="/profile?onglet=infos" className={buttonClass('secondary', 'sm')}>
          {methods.length ? 'Modifier dans mon profil' : 'Compléter mon profil'}
        </Link>
      </CardBody>
    </Card>
  )
}

function PaymentsList({ ctx, payments, canManage }: { ctx: PartyCtx; payments: Payment[]; canManage: boolean }) {
  const action = usePaymentAction(ctx.party.id)
  const [busyId, setBusyId] = useState<string | null>(null)
  const run = (p: Payment, a: 'confirm' | 'reset') => {
    setBusyId(p.id)
    action.mutate(
      { paymentId: p.id, action: a },
      {
        onSuccess: () => {
          if (a === 'confirm') haptic('paid')
        },
        onSettled: () => setBusyId(null),
      },
    )
  }
  if (payments.length === 0) {
    return (
      <Card>
        <CardBody className="text-sm text-muted">Personne n'a de part à rembourser.</CardBody>
      </Card>
    )
  }
  return (
    <Card>
      <CardBody className="space-y-2">
        <h2 className="font-display text-lg font-semibold">Les parts</h2>
        <ul className="divide-y divide-border">
          {payments.map((p) => {
            const u = ctx.people.get(p.debtor) ?? p.expand?.debtor ?? { id: p.debtor, name: '' }
            return (
              <li key={p.id} className="flex flex-wrap items-center gap-3 py-2.5">
                <Avatar user={u} size={40} />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">{p.debtor === ctx.me.id ? 'Toi' : u.name}</p>
                  <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                    <StatusBadge status={p.status} />
                    {p.method && p.method !== 'later' && <Badge variant={METHOD_BADGE[p.method]}>{METHOD_LABELS[p.method]}</Badge>}
                    {p.method === 'later' && p.status === 'pending' && <Badge variant="warning">Plus tard</Badge>}
                  </div>
                </div>
                <Money cents={p.amount} className={cn('font-semibold', p.status === 'confirmed' && 'text-success')} />
                {canManage && (
                  <div className="flex w-full justify-end gap-1.5 sm:w-auto">
                    {p.status !== 'confirmed' ? (
                      <Button size="sm" variant={p.status === 'declared' ? 'primary' : 'secondary'} loading={busyId === p.id} onClick={() => run(p, 'confirm')}>
                        Confirmer
                      </Button>
                    ) : (
                      <Button size="sm" variant="ghost" leftIcon={<RotateCcw className="size-3.5" />} loading={busyId === p.id} onClick={() => run(p, 'reset')}>
                        Annuler
                      </Button>
                    )}
                  </div>
                )}
              </li>
            )
          })}
        </ul>
      </CardBody>
    </Card>
  )
}

function ChangePayerSheet({ ctx, open, onClose }: { ctx: PartyCtx; open: boolean; onClose: () => void }) {
  const setPayer = useSetPayer(ctx.party.id)
  return (
    <Sheet open={open} onClose={onClose} title="Changer de payeur" description="Possible tant qu'aucun remboursement n'a été confirmé.">
      <ul className="space-y-2">
        {ctx.members.map((m) => {
          const u = m.expand?.user ?? ctx.people.get(m.user) ?? { id: m.user, name: '' }
          const current = ctx.party.payer === m.user
          return (
            <li key={m.id}>
              <button
                type="button"
                disabled={current || setPayer.isPending}
                onClick={() => setPayer.mutate(m.user, { onSuccess: onClose })}
                className="flex min-h-14 w-full items-center gap-3 rounded-md border border-border bg-surface px-3 text-left hover:border-border-strong disabled:opacity-60"
              >
                <Avatar user={u} size={32} decorative />
                <span className="flex-1 font-medium">{u.name}</span>
                {current && <Badge variant="brand">Actuel</Badge>}
              </button>
            </li>
          )
        })}
      </ul>
    </Sheet>
  )
}
