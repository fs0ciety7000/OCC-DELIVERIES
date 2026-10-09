import { Banknote, Clock, ExternalLink, Link2, QrCode } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { Badge, CopyButton, Money, QRCodeCard, Skeleton } from '@/components/ui'
import { occ } from '@/lib/api'
import { cn } from '@/lib/cn'
import { formatMoney } from '@/lib/format'
import type { DeclareMethod, PaymentMethod, PaymentQR, WalletKind } from '@/lib/types'
import { METHOD_LABELS } from '../labels'

/** Pastille « wordmark » (pas de logo officiel). */
export function MethodMark({ method, className }: { method: PaymentMethod; className?: string }) {
  if (method === 'wero')
    return <span className={cn('inline-grid h-8 min-w-14 place-items-center rounded-sm bg-wero px-2 font-display text-[15px] font-extrabold tracking-tight text-wero-fg', className)}>wero</span>
  if (method === 'bancontact')
    return (
      <span className={cn('inline-grid h-8 min-w-14 place-items-center rounded-sm bg-bancontact px-2 text-[11px] leading-3 font-extrabold text-bancontact-fg', className)}>
        Bancontact
        <span className="font-semibold opacity-85">Pay</span>
      </span>
    )
  const icons: Partial<Record<PaymentMethod, ReactNode>> = {
    qr: <QrCode className="size-4" />,
    link: <Link2 className="size-4" />,
    cash: <Banknote className="size-4" />,
    later: <Clock className="size-4" />,
  }
  return <span className={cn('inline-grid size-8 place-items-center rounded-sm bg-fg/[0.08] text-fg', className)}>{icons[method]}</span>
}

export function MethodTiles({ methods, value, onChange }: { methods: DeclareMethod[]; value: DeclareMethod | null; onChange: (m: DeclareMethod) => void }) {
  return (
    <div role="radiogroup" aria-label="Moyen de remboursement" className="grid grid-cols-2 gap-2 sm:grid-cols-3">
      {methods.map((m) => {
        const on = value === m
        return (
          <button
            key={m}
            type="button"
            role="radio"
            aria-checked={on}
            onClick={() => onChange(m)}
            className={cn(
              'flex min-h-20 flex-col items-start justify-between gap-2 rounded-md border bg-elevated p-3 text-left transition-[border-color,box-shadow] duration-[120ms]',
              on ? 'border-brand/70 shadow-glow' : 'border-border hover:border-border-strong',
            )}
          >
            <MethodMark method={m} />
            <span className="text-sm font-semibold">{METHOD_LABELS[m]}</span>
          </button>
        )
      })}
    </div>
  )
}

function CopyRow({ label, value, display, toastMessage }: { label: string; value: string; display?: ReactNode; toastMessage: string }) {
  return (
    <div className="flex min-h-12 items-center gap-3 rounded-md border border-border bg-surface px-3 py-2">
      <div className="min-w-0 flex-1">
        <p className="text-xs text-muted">{label}</p>
        <p className="truncate font-semibold tabular">{display ?? value}</p>
      </div>
      <CopyButton value={value} label="" ariaLabel={`Copier ${label.toLowerCase()}`} size="icon" variant="ghost" toastMessage={toastMessage} />
    </div>
  )
}

/** Image du QR « recevoir » du payeur, récupérée avec le jeton d'auth (blob URL). */
export function WalletQrImage({ paymentId, kind, label }: { paymentId: string; kind: WalletKind; label: string }) {
  const [state, setState] = useState<{ src: string | null; error: boolean; loading: boolean }>({ src: null, error: false, loading: true })
  useEffect(() => {
    let url: string | null = null
    let cancelled = false
    occ
      .walletQrObjectUrl(paymentId, kind)
      .then((u) => {
        if (cancelled) {
          if (u) URL.revokeObjectURL(u)
          return
        }
        url = u
        setState({ src: u, error: false, loading: false })
      })
      .catch(() => {
        if (!cancelled) setState({ src: null, error: true, loading: false })
      })
    return () => {
      cancelled = true
      if (url) URL.revokeObjectURL(url)
    }
  }, [paymentId, kind])

  if (state.loading) return <Skeleton className="mx-auto size-56 rounded-md" />
  if (!state.src) return state.error ? <p className="text-center text-sm text-muted">QR du payeur indisponible — utilise son numéro ci-dessous.</p> : null
  return <QRCodeCard imageSrc={state.src} label={label} size={200} title="Scanne avec ton app bancaire" />
}

const WALLET_GUIDE: Record<WalletKind, (id: string) => string[]> = {
  wero: (id) => [
    'Ouvre ton app bancaire (KBC, BNP, ING, Belfius…) puis Wero.',
    `Touche « Envoyer » et colle ${id.includes('@') ? "l'e-mail" : 'le numéro'} du payeur, ou scanne son QR.`,
    'Colle le montant et la communication, puis valide.',
  ],
  bancontact: () => [
    'Ouvre ton app Bancontact Pay (ou ton app bancaire).',
    'Choisis « Envoyer de l’argent » et colle le numéro du payeur, ou scanne son QR.',
    'Colle le montant et la communication, puis valide.',
  ],
}

/** Contenu détaillé pour la méthode choisie (sans fausse promesse de deep link Wero / Bancontact). */
export function MethodDetails({ method, qr, paymentId }: { method: DeclareMethod; qr: PaymentQR; paymentId: string }) {
  const amountText = (qr.amount / 100).toFixed(2).replace('.', ',')
  const common = (
    <div className="grid gap-2 sm:grid-cols-2">
      <CopyRow label="Montant" value={amountText} display={<Money cents={qr.amount} />} toastMessage="Montant copié" />
      <CopyRow label="Communication" value={qr.reference} toastMessage="Communication copiée" />
    </div>
  )

  if (method === 'wero' || method === 'bancontact') {
    const id = method === 'wero' ? qr.wero?.id : qr.bancontact?.phone
    const hasQr = method === 'wero' ? qr.wero?.hasQr : qr.bancontact?.hasQr
    const steps = WALLET_GUIDE[method](id ?? '')
    return (
      <div className="space-y-4">
        {hasQr && <WalletQrImage paymentId={paymentId} kind={method} label={`QR ${METHOD_LABELS[method]} de ${qr.beneficiary}`} />}
        {id && <CopyRow label={`${METHOD_LABELS[method]} de ${qr.beneficiary}`} value={id} toastMessage="Identifiant copié" />}
        {common}
        <ol className="space-y-2" aria-label={`Comment payer avec ${METHOD_LABELS[method]}`}>
          {steps.map((s, i) => (
            <li key={i} className="flex gap-3 text-sm">
              <span className={cn('grid size-6 shrink-0 place-items-center rounded-full text-xs font-bold', method === 'wero' ? 'bg-wero text-wero-fg' : 'bg-bancontact text-bancontact-fg')}>{i + 1}</span>
              <span className="pt-0.5">{s}</span>
            </li>
          ))}
        </ol>
      </div>
    )
  }

  if (method === 'qr') {
    return (
      <div className="space-y-4">
        {qr.epc ? (
          <QRCodeCard
            value={qr.epc}
            amount={qr.amount}
            label={`QR virement SEPA de ${formatMoney(qr.amount)} vers ${qr.beneficiary}`}
            title="Scanne avec ton app bancaire"
            caption={<>Bénéficiaire : <strong className="text-fg">{qr.beneficiary}</strong></>}
          />
        ) : (
          <p className="rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning">Le payeur n'a pas encore renseigné son IBAN.</p>
        )}
        {common}
      </div>
    )
  }

  if (method === 'link') {
    return (
      <div className="space-y-4">
        {qr.link ? (
          <a
            href={qr.link}
            target="_blank"
            rel="noopener noreferrer"
            className="flex min-h-12 items-center justify-center gap-2 rounded-md border border-border-strong bg-surface px-4 font-semibold hover:bg-elevated"
          >
            Ouvrir le lien de {qr.beneficiary} <ExternalLink className="size-4" />
          </a>
        ) : (
          <p className="text-sm text-muted">Pas de lien de paiement disponible.</p>
        )}
        {common}
      </div>
    )
  }

  if (method === 'cash') {
    return (
      <p className="rounded-md border border-border bg-surface p-3 text-sm">
        Donne <Money cents={qr.amount} className="font-semibold" /> en main propre à <strong>{qr.beneficiary}</strong>. Il ou elle confirmera la réception.
      </p>
    )
  }

  return <p className="rounded-md border border-border bg-surface p-3 text-sm text-muted">Pas de souci : ta part reste « en attente » et {qr.beneficiary} sera prévenu·e. Reviens ici quand tu veux.</p>
}

export function StatusBadge({ status }: { status: 'pending' | 'declared' | 'confirmed' }) {
  if (status === 'confirmed') return <Badge variant="success">Confirmé</Badge>
  if (status === 'declared') return <Badge variant="info">Déclaré</Badge>
  return <Badge variant="warning">En attente</Badge>
}
