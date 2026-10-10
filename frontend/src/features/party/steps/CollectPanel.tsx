import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, ChevronLeft, ChevronRight, Maximize2, RotateCcw, Sun, X } from 'lucide-react'
import { AnimatePresence, motion } from 'motion/react'
import { QRCodeSVG } from 'qrcode.react'
import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router'
import { Avatar, Badge, Button, buttonClass, Card, CardBody, Money, Segmented, Skeleton } from '@/components/ui'
import { occ } from '@/lib/api'
import { cn } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { formatMoney } from '@/lib/format'
import { haptic } from '@/lib/haptics'
import { qk } from '@/lib/queryKeys'
import type { CollectItem, Payment } from '@/lib/types'
import type { PartyCtx } from '../context'
import { usePaymentAction } from '../hooks'
import { METHOD_BADGE, METHOD_LABELS } from '../labels'
import { COLLECT_KIND_LABELS, collectKinds, qrValueFor, stepIndex, type CollectKind } from './collect'
import { StatusBadge } from './PaymentMethods'

/** Une part vue par le payeur : statut temps réel (liste des paiements) + payloads serveur. */
interface Row {
  payment: Payment
  user: { id: string; name: string; avatar?: string; color?: string }
  item: CollectItem | undefined
}

const KIND_HINT: Record<CollectKind, string> = {
  epc: 'À scanner dans l’app bancaire (KBC, BNP Paribas Fortis, ING, Belfius, Argenta…) : montant et communication déjà remplis.',
  revolut: 'À scanner avec l’appareil photo : le lien ouvre Revolut avec le montant. Le ou la collègue vérifie le montant avant de valider.',
  paypal: 'À scanner avec l’appareil photo : le lien ouvre PayPal avec le montant. Le ou la collègue vérifie le montant avant de valider.',
}

/**
 * « Encaisser » (vue du payeur) : un QR **à montant exact** par collègue, à
 * présenter depuis son propre téléphone, et la confirmation de réception au
 * même endroit. Les payloads viennent du serveur (`GET /parties/{id}/payments/qr`).
 */
export function CollectPanel({ ctx, payments }: { ctx: PartyCtx; payments: Payment[] }) {
  const partyId = ctx.party.id
  const collect = useQuery({
    queryKey: qk.paymentsCollect(partyId),
    queryFn: () => occ.collectQR(partyId),
    staleTime: 15_000,
    refetchOnWindowFocus: true,
  })
  const action = usePaymentAction(partyId)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [chosenKind, setChosenKind] = useState<CollectKind | null>(null)
  const [presenting, setPresenting] = useState<number | null>(null)

  const cash = ctx.party.collect_mode === 'cash' || collect.data?.collectMode === 'cash'
  const kinds = cash ? [] : collectKinds(collect.data)
  const kind = chosenKind && kinds.includes(chosenKind) ? chosenKind : (kinds[0] ?? null)
  const byPayment = new Map((collect.data?.items ?? []).map((it) => [it.payment, it]))
  const rows: Row[] = payments
    .map((p) => {
      const item = byPayment.get(p.id)
      const user = ctx.people.get(p.debtor) ?? p.expand?.debtor ?? item?.debtor ?? { id: p.debtor, name: 'Membre' }
      return { payment: p, user, item }
    })
    .sort((a, b) => Number(a.payment.status === 'confirmed') - Number(b.payment.status === 'confirmed') || a.user.name.localeCompare(b.user.name, 'fr'))
  const todo = rows.filter((r) => r.payment.status !== 'confirmed')

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
        <CardBody className="text-sm text-muted">Personne n'a de part à te rembourser.</CardBody>
      </Card>
    )
  }

  return (
    <Card>
      <CardBody className="space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0 flex-1 basis-56">
            <h2 className="font-display text-lg font-semibold">Encaisser</h2>
            <p className="text-sm text-muted">
              {cash ? 'Remboursement en espèces : confirme chaque part dès que tu l’as reçue.' : 'Montre à chaque collègue son QR : le montant exact de sa part est déjà dedans.'}
            </p>
          </div>
          {kind && todo.length > 0 && (
            <Button
              leftIcon={<Maximize2 className="size-4" />}
              onClick={() => setPresenting(rows.indexOf(todo[0]!))}
              className="w-full sm:w-auto"
            >
              Présenter à tour de rôle
            </Button>
          )}
        </div>

        {cash ? null : collect.isPending ? (
          <Skeleton className="h-10 rounded-md" />
        ) : collect.isError ? (
          <p className="rounded-md border border-danger/30 bg-danger/10 p-3 text-sm text-danger">
            QR indisponibles : {errorMessage(collect.error)}{' '}
            <button type="button" className="font-semibold underline" onClick={() => collect.refetch()}>
              Réessayer
            </button>
          </p>
        ) : kinds.length === 0 ? (
          <div className="space-y-3 rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
            <p>Pour générer des QR avec montant, ajoute ton IBAN (ou ton Revolut / PayPal.me) dans ton profil. En attendant, tu peux confirmer les parts reçues ci-dessous.</p>
            <Link to="/profile?onglet=infos" className={buttonClass('secondary', 'sm')}>
              Compléter mon profil
            </Link>
          </div>
        ) : (
          <div className="space-y-2">
            {kinds.length > 1 && kind && (
              <Segmented label="Type de QR" value={kind} onChange={setChosenKind} options={kinds.map((k) => ({ value: k, label: COLLECT_KIND_LABELS[k] }))} className="w-full sm:w-auto" />
            )}
            {kind && <p className="text-xs text-muted">{KIND_HINT[kind]}</p>}
          </div>
        )}

        <ul className="grid grid-cols-1 gap-3 sm:grid-cols-2" aria-label="Parts à encaisser">
          {rows.map((r, i) => (
            <CollectCard
              key={r.payment.id}
              row={r}
              kind={kind}
              busy={busyId === r.payment.id}
              onPresent={() => setPresenting(i)}
              onConfirm={() => run(r.payment, 'confirm')}
              onReset={() => run(r.payment, 'reset')}
            />
          ))}
        </ul>
      </CardBody>

      {presenting !== null && kind && (
        <PresentMode
          rows={rows}
          index={presenting}
          kind={kind}
          kinds={kinds}
          onKind={setChosenKind}
          onIndex={setPresenting}
          onClose={() => setPresenting(null)}
          busyId={busyId}
          onConfirm={(p) => run(p, 'confirm')}
        />
      )}
    </Card>
  )
}

function CollectCard({
  row,
  kind,
  busy,
  onPresent,
  onConfirm,
  onReset,
}: {
  row: Row
  kind: CollectKind | null
  busy: boolean
  onPresent: () => void
  onConfirm: () => void
  onReset: () => void
}) {
  const { payment: p, user, item } = row
  const value = kind ? qrValueFor(item, kind) : null
  const done = p.status === 'confirmed'
  return (
    <li className={cn('flex min-w-0 flex-col gap-3 rounded-md border bg-surface p-3', done ? 'border-success/30' : 'border-border')}>
      <div className="flex items-center gap-3">
        <Avatar user={user} size={40} />
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{user.name}</p>
          <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
            <StatusBadge status={p.status} />
            {p.method && p.method !== 'later' && p.method !== 'self' && <Badge variant={METHOD_BADGE[p.method]}>{METHOD_LABELS[p.method]}</Badge>}
            {p.method === 'later' && p.status === 'pending' && <Badge variant="warning">Plus tard</Badge>}
          </div>
        </div>
        <Money cents={p.amount} className={cn('shrink-0 font-display text-lg font-bold', done && 'text-success')} />
      </div>

      {done ? (
        <p className="flex items-center gap-2 text-sm text-success">
          <CheckCircle2 className="size-5 shrink-0" /> Reçu, merci !
        </p>
      ) : value && kind ? (
        <button
          type="button"
          onClick={onPresent}
          className="group mx-auto rounded-md bg-qr-bg p-2.5 text-qr-fg shadow-card focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand"
          aria-label={`Présenter en plein écran le QR de ${user.name} (${formatMoney(p.amount)})`}
        >
          <QRCodeSVG value={value} size={144} level="M" marginSize={0} bgColor="transparent" fgColor="currentColor" aria-hidden="true" />
        </button>
      ) : kind ? (
        <p className="text-sm text-muted">Pas de QR {COLLECT_KIND_LABELS[kind]} pour cette part.</p>
      ) : null}

      <div className="mt-auto flex flex-wrap justify-end gap-1.5">
        {!done && value && (
          <Button size="sm" variant="ghost" leftIcon={<Maximize2 className="size-3.5" />} onClick={onPresent}>
            Présenter
          </Button>
        )}
        {done ? (
          <Button size="sm" variant="ghost" leftIcon={<RotateCcw className="size-3.5" />} loading={busy} onClick={onReset}>
            Annuler
          </Button>
        ) : (
          <Button size="sm" variant={p.status === 'declared' ? 'primary' : 'secondary'} loading={busy} onClick={onConfirm}>
            Confirmer la réception
          </Button>
        )}
      </div>
    </li>
  )
}

/** Garde l'écran allumé pendant la présentation (Wake Lock API, si disponible). */
function useWakeLock(active: boolean) {
  useEffect(() => {
    if (!active || typeof navigator === 'undefined' || !('wakeLock' in navigator)) return
    let sentinel: WakeLockSentinel | null = null
    let cancelled = false
    const request = () => {
      navigator.wakeLock
        .request('screen')
        .then((s) => {
          if (cancelled) void s.release().catch(() => {})
          else sentinel = s
        })
        .catch(() => {})
    }
    // Le verrou tombe quand l'onglet passe en arrière-plan : on le reprend au retour.
    const onVisible = () => {
      if (document.visibilityState === 'visible') request()
    }
    request()
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      cancelled = true
      document.removeEventListener('visibilitychange', onVisible)
      void sentinel?.release().catch(() => {})
    }
  }, [active])
}

const FOCUSABLE = 'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'

export function PresentMode({
  rows,
  index,
  kind,
  kinds,
  onKind,
  onIndex,
  onClose,
  busyId,
  onConfirm,
}: {
  rows: Row[]
  index: number
  kind: CollectKind
  kinds: CollectKind[]
  onKind: (k: CollectKind) => void
  onIndex: (i: number) => void
  onClose: () => void
  busyId: string | null
  onConfirm: (p: Payment) => void
}) {
  const i = stepIndex(index, 0, rows.length)
  const row = rows[i]
  const panelRef = useRef<HTMLDivElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const [dir, setDir] = useState(1)
  const swipe = useRef<{ x: number; y: number } | null>(null)
  useWakeLock(true)

  const go = (delta: number) => {
    const next = stepIndex(i, delta, rows.length)
    if (next === i) return
    setDir(delta)
    onIndex(next)
  }
  const goRef = useRef(go)
  const closeCb = useRef(onClose)
  useEffect(() => {
    goRef.current = go
    closeCb.current = onClose
  })

  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    // Le reste de l'app devient inerte (lecteurs d'écran, clavier) pendant la présentation.
    const root = document.getElementById('root')
    const wasInert = root?.hasAttribute('inert') ?? true
    if (!wasInert) root?.setAttribute('inert', '')
    closeRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        closeCb.current()
      } else if (e.key === 'ArrowRight') goRef.current(1)
      else if (e.key === 'ArrowLeft') goRef.current(-1)
      else if (e.key === 'Tab' && panelRef.current) {
        const nodes = Array.from(panelRef.current.querySelectorAll<HTMLElement>(FOCUSABLE))
        if (nodes.length === 0) return
        const first = nodes[0]!
        const last = nodes[nodes.length - 1]!
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prevOverflow
      if (!wasInert) root?.removeAttribute('inert')
      previouslyFocused?.focus?.()
    }
  }, [])

  if (!row) return null
  const { payment: p, user, item } = row
  const value = qrValueFor(item, kind)
  const done = p.status === 'confirmed'
  const amount = formatMoney(p.amount)

  const onPointerDown = (e: ReactPointerEvent) => {
    swipe.current = { x: e.clientX, y: e.clientY }
  }
  const onPointerUp = (e: ReactPointerEvent) => {
    const s = swipe.current
    swipe.current = null
    if (!s) return
    const dx = e.clientX - s.x
    if (Math.abs(dx) > 60 && Math.abs(dx) > Math.abs(e.clientY - s.y)) go(dx < 0 ? 1 : -1)
  }

  return createPortal(
    <div
      ref={panelRef}
      role="dialog"
      aria-modal="true"
      aria-label={`Encaisser — ${user.name}, ${amount}`}
      className="fixed inset-0 z-50 flex flex-col overflow-y-auto bg-bg text-fg"
      onPointerDown={onPointerDown}
      onPointerUp={onPointerUp}
      onPointerCancel={() => (swipe.current = null)}
    >
      <div className="mx-auto flex w-full max-w-xl flex-1 flex-col px-4 pt-[max(env(safe-area-inset-top),0.75rem)] pb-[max(env(safe-area-inset-bottom),1rem)]">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm font-semibold text-muted tabular">
            {i + 1} / {rows.length}
          </p>
          {kinds.length > 1 && (
            <Segmented
              label="Type de QR"
              value={kind}
              onChange={onKind}
              options={kinds.map((k) => ({ value: k, label: COLLECT_KIND_LABELS[k] }))}
              className="order-last w-full sm:order-none sm:w-auto"
            />
          )}
          <button ref={closeRef} type="button" onClick={onClose} aria-label="Fermer la présentation" className={buttonClass('ghost', 'icon')}>
            <X className="size-5" />
          </button>
        </div>

        <p className="sr-only" aria-live="polite">
          {`${user.name}, ${amount}${done ? ', reçu' : ''}`}
        </p>
        <AnimatePresence mode="wait" initial={false} custom={dir}>
          <motion.div
            key={`${p.id}-${kind}`}
            custom={dir}
            initial={{ opacity: 0, x: dir * 40 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: dir * -40 }}
            transition={{ duration: 0.18 }}
            className="flex flex-1 flex-col items-center justify-center gap-4 py-4 text-center"
          >
            <div className="flex items-center gap-3">
              <Avatar user={user} size={40} decorative />
              <p className="font-display text-xl font-semibold">{user.name}</p>
            </div>
            <Money cents={p.amount} className={cn('font-display text-[56px] leading-[60px] font-bold', done && 'text-success')} />
            {done ? (
              <p className="flex items-center gap-2 rounded-md border border-success/30 bg-success/10 p-3 font-medium text-success">
                <CheckCircle2 className="size-6" /> Reçu, merci !
              </p>
            ) : value ? (
              <div className="w-[min(80vw,52dvh,26rem)] rounded-lg bg-qr-bg p-4 text-qr-fg shadow-card">
                <QRCodeSVG
                  value={value}
                  size={512}
                  level="M"
                  marginSize={0}
                  bgColor="transparent"
                  fgColor="currentColor"
                  role="img"
                  aria-label={kind === 'epc' ? `QR virement SEPA de ${amount} pour ${user.name}` : `QR du lien ${COLLECT_KIND_LABELS[kind]} de ${amount} pour ${user.name}`}
                  style={{ width: '100%', height: 'auto', display: 'block' }}
                />
              </div>
            ) : (
              <p className="text-sm text-muted">Pas de QR {COLLECT_KIND_LABELS[kind]} pour cette part.</p>
            )}
            {!done && (
              <>
                <p className="max-w-sm text-sm text-muted">
                  {kind === 'epc' ? 'Scanne avec ton app bancaire' : 'Scanne avec l’appareil photo'} — communication : <span className="font-medium text-fg">{p.reference}</span>
                </p>
                <p className="flex items-center gap-1.5 text-xs text-muted">
                  <Sun className="size-3.5 shrink-0" aria-hidden="true" /> Monte la luminosité au maximum pour un scan rapide.
                </p>
              </>
            )}
          </motion.div>
        </AnimatePresence>

        <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-2">
          <Button variant="secondary" size="icon" aria-label="Collègue précédent·e" disabled={i === 0} onClick={() => go(-1)}>
            <ChevronLeft className="size-5" />
          </Button>
          {done ? (
            <Button size="lg" variant={i < rows.length - 1 ? 'primary' : 'secondary'} onClick={() => (i < rows.length - 1 ? go(1) : onClose())}>
              {i < rows.length - 1 ? 'Suivant' : 'Terminer'}
            </Button>
          ) : (
            <Button size="lg" loading={busyId === p.id} onClick={() => onConfirm(p)}>
              J'ai reçu {amount}
            </Button>
          )}
          <Button variant="secondary" size="icon" aria-label="Collègue suivant·e" disabled={i >= rows.length - 1} onClick={() => go(1)}>
            <ChevronRight className="size-5" />
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
