import { AlertTriangle, Banknote, ChevronDown, Clock, ExternalLink, Link2, QrCode, Smartphone } from 'lucide-react'
import { useEffect, useId, useState, type ReactNode } from 'react'
import { Badge, buttonClass, CopyButton, Money, QRCodeCard, Skeleton } from '@/components/ui'
import { occ } from '@/lib/api'
import { cn } from '@/lib/cn'
import { formatMoney } from '@/lib/format'
import { formatIban } from '@/lib/iban'
import type { DeclareMethod, PaymentMethod, PaymentQR, WalletKind } from '@/lib/types'
import { isLinkMethod, linkFor, METHOD_LABELS } from '../labels'

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
  if (method === 'revolut' || method === 'paypal')
    return (
      <span className={cn('inline-grid h-8 min-w-14 place-items-center rounded-sm bg-fg/[0.08] px-2 font-display text-[13px] font-extrabold tracking-tight text-fg', className)}>
        {method === 'revolut' ? 'Revolut' : 'PayPal'}
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

export function MethodTiles({
  methods,
  value,
  onChange,
  hints = {},
}: {
  methods: DeclareMethod[]
  value: DeclareMethod | null
  onChange: (m: DeclareMethod) => void
  hints?: Partial<Record<DeclareMethod, string>>
}) {
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
            <span className="min-w-0">
              <span className="block text-sm font-semibold">{METHOD_LABELS[m]}</span>
              {hints[m] && <span className="block text-xs text-muted">{hints[m]}</span>}
            </span>
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

const WALLET_GUIDE: Record<WalletKind, (id: string, amount: string) => string[]> = {
  wero: (id, amount) => [
    'Ouvre ton app bancaire (KBC, BNP Paribas Fortis, ING, Belfius…) puis Wero.',
    `Touche « Envoyer » et colle ${id.includes('@') ? "l'e-mail" : 'le numéro'} du payeur.`,
    `Saisis ${amount} et colle la communication, puis valide.`,
  ],
  bancontact: (_id, amount) => [
    'Ouvre ton app Bancontact Pay (ou ton app bancaire).',
    'Choisis « Payer un contact » et colle le numéro du payeur.',
    `Saisis ${amount} et colle la communication, puis valide.`,
  ],
}

function Notice({ tone = 'warning', children }: { tone?: 'warning' | 'info'; children: ReactNode }) {
  return (
    <p className={cn('flex gap-2 rounded-md border p-3 text-sm', tone === 'warning' ? 'border-warning/30 bg-warning/10 text-warning' : 'border-info/30 bg-info/10')}>
      {tone === 'warning' ? <AlertTriangle className="mt-0.5 size-4 shrink-0" /> : <Smartphone className="mt-0.5 size-4 shrink-0" />}
      <span>{children}</span>
    </p>
  )
}

function Reveal({ label, children, defaultOpen = false }: { label: string; children: ReactNode; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(defaultOpen)
  const id = useId()
  return (
    <div className="space-y-3">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((o) => !o)}
        className="flex min-h-11 w-full items-center justify-between gap-2 rounded-md border border-border bg-surface px-3 text-sm font-semibold hover:border-border-strong"
      >
        {label}
        <ChevronDown className={cn('size-4 transition-transform duration-[120ms]', open && 'rotate-180')} />
      </button>
      <div id={id} hidden={!open}>
        {open && children}
      </div>
    </div>
  )
}

/**
 * Contenu détaillé pour la méthode choisie. Aucune fausse promesse : seuls
 * le QR virement (EPC) et les liens marqués `amountPrefilled` remplissent le
 * montant ; Wero / Bancontact Pay n'ont pas de demande de paiement ouverte
 * aux tiers (ADR 0003).
 */
export function MethodDetails({ method, qr, paymentId, mobile = false }: { method: DeclareMethod; qr: PaymentQR; paymentId: string; mobile?: boolean }) {
  const amountText = (qr.amount / 100).toFixed(2).replace('.', ',')
  const money = formatMoney(qr.amount)
  const common = (
    <div className="grid gap-2 sm:grid-cols-2">
      <CopyRow label="Montant" value={amountText} display={<Money cents={qr.amount} />} toastMessage="Montant copié" />
      <CopyRow label="Communication" value={qr.reference} toastMessage="Communication copiée" />
    </div>
  )

  if (method === 'wero' || method === 'bancontact') {
    const id = method === 'wero' ? qr.wero?.id : qr.bancontact?.phone
    const hasQr = method === 'wero' ? qr.wero?.hasQr : qr.bancontact?.hasQr
    const steps = WALLET_GUIDE[method](id ?? '', money)
    const staticQr = hasQr ? (
      <div className="space-y-3">
        <Notice>
          QR sans montant : saisis <strong>{money}</strong> dans l'app.
        </Notice>
        <WalletQrImage paymentId={paymentId} kind={method} label={`QR personnel ${METHOD_LABELS[method]} de ${qr.beneficiary} (sans montant)`} />
      </div>
    ) : null
    return (
      <div className="space-y-4">
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
        {staticQr && (id ? <Reveal label="Afficher son QR personnel (sans montant)">{staticQr}</Reveal> : staticQr)}
      </div>
    )
  }

  if (method === 'qr') {
    if (!qr.epc) return <Notice>Le payeur n'a pas encore renseigné son IBAN.</Notice>
    const card = (
      <QRCodeCard
        value={qr.epc}
        amount={qr.amount}
        size={mobile ? 208 : 240}
        label={`QR virement SEPA de ${money} vers ${qr.beneficiary}`}
        title={mobile ? 'À scanner avec une app bancaire' : 'Ouvre ton app bancaire et scanne'}
        caption={
          <>
            Bénéficiaire : <strong className="text-fg">{qr.beneficiary}</strong>
          </>
        }
      />
    )
    return (
      <div className="space-y-4">
        <p className="text-sm text-muted">
          Ton app bancaire (KBC, BNP Paribas Fortis, ING, Belfius, Argenta…) lit ce QR : <strong className="text-fg">montant et communication sont déjà remplis</strong>, tu n'as plus qu'à
          valider — en virement instantané si ta banque le propose.
        </p>
        {mobile ? (
          <>
            <Notice tone="info">Sur ce téléphone, copie les infos ci-dessous dans ton app bancaire (ou paie avec un lien).</Notice>
            <Reveal label="Afficher le QR pour un collègue">{card}</Reveal>
          </>
        ) : (
          card
        )}
        <div className="grid gap-2 sm:grid-cols-2">
          {qr.iban && <CopyRow label={`IBAN de ${qr.beneficiary}`} value={qr.iban} display={formatIban(qr.iban)} toastMessage="IBAN copié" />}
          <CopyRow label="Montant" value={amountText} display={<Money cents={qr.amount} />} toastMessage="Montant copié" />
          <CopyRow label="Communication" value={qr.reference} toastMessage="Communication copiée" />
        </div>
      </div>
    )
  }

  if (isLinkMethod(method)) {
    const link = linkFor(qr, method)
    if (!link) return <p className="text-sm text-muted">Pas de lien de paiement disponible.</p>
    const name = method === 'link' ? link.label : METHOD_LABELS[method]
    return (
      <div className="space-y-4">
        <a href={link.url} target="_blank" rel="noopener noreferrer" className={buttonClass(link.amountPrefilled ? 'primary' : 'secondary', 'lg', true)}>
          {link.amountPrefilled ? `Payer ${money} avec ${name}` : `Ouvrir ${name}`} <ExternalLink className="size-4" />
        </a>
        {link.amountPrefilled ? (
          <p className="text-sm text-muted">
            Le lien ouvre {name} avec <strong className="text-fg">{money}</strong> déjà rempli{method === 'revolut' ? ' et la communication en note' : ''}. Vérifie le montant avant de valider.
          </p>
        ) : (
          <Notice>
            Ce lien ne pré-remplit pas le montant : saisis <strong>{money}</strong>.
          </Notice>
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
