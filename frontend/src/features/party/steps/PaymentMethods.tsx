import { AlertTriangle, Banknote, ChevronDown, Clock, ExternalLink, Link2, QrCode, Smartphone } from 'lucide-react'
import { useId, useState, type ReactNode } from 'react'
import { Badge, buttonClass, CopyButton, Money, QRCodeCard } from '@/components/ui'
import { cn } from '@/lib/cn'
import { formatMoney } from '@/lib/format'
import { formatIban } from '@/lib/iban'
import type { DeclareMethod, PaymentMethod, PaymentQR } from '@/lib/types'
import { isLinkMethod, linkFor, METHOD_LABELS } from '../labels'

/** Pastille « wordmark » (pas de logo officiel). */
export function MethodMark({ method, className }: { method: PaymentMethod; className?: string }) {
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
    // anciens paiements Wero / Bancontact Pay (moyens retirés)
    wero: <Smartphone className="size-4" />,
    bancontact: <Smartphone className="size-4" />,
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
 * montant (Wero / Bancontact Pay, sans format tiers à montant, ont été
 * retirés : ADR 0003 mise à jour 3).
 */
export function MethodDetails({ method, qr, mobile = false }: { method: DeclareMethod; qr: PaymentQR; mobile?: boolean }) {
  const amountText = (qr.amount / 100).toFixed(2).replace('.', ',')
  const money = formatMoney(qr.amount)
  const common = (
    <div className="grid gap-2 sm:grid-cols-2">
      <CopyRow label="Montant" value={amountText} display={<Money cents={qr.amount} />} toastMessage="Montant copié" />
      <CopyRow label="Communication" value={qr.reference} toastMessage="Communication copiée" />
    </div>
  )

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
          valider — en virement instantané si ta banque le propose. C'est la même app que celle où vit Wero : pas besoin de Wero pour rembourser.
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
