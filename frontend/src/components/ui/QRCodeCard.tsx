import { QRCodeSVG } from 'qrcode.react'
import type { ReactNode } from 'react'
import { cn } from '@/lib/cn'
import { Money } from './Money'
import { CopyButton } from './CopyButton'

export interface QRCodeCardProps {
  /** Contenu encodé (payload EPC, lien d'invitation…). */
  value?: string
  title?: ReactNode
  caption?: ReactNode
  amount?: number
  copyValue?: string
  copyLabel?: string
  size?: number
  className?: string
  label: string
}

/** QR toujours noir sur fond blanc, quel que soit le thème (lisibilité scanners). */
export function QRCodeCard({ value, title, caption, amount, copyValue, copyLabel = 'Copier', size = 208, className, label }: QRCodeCardProps) {
  return (
    <figure className={cn('flex flex-col items-center gap-3 rounded-lg border border-border bg-surface p-4 text-center shadow-card', className)}>
      {title && <figcaption className="text-sm font-medium text-muted">{title}</figcaption>}
      <div className="rounded-md bg-qr-bg p-3 text-qr-fg shadow-card">
        {value ? (
          <QRCodeSVG value={value} size={size} level="M" marginSize={0} bgColor="transparent" fgColor="currentColor" role="img" aria-label={label} title={label} />
        ) : null}
      </div>
      {amount != null && <Money cents={amount} className="font-display text-3xl font-bold" />}
      {caption && <div className="text-xs text-muted">{caption}</div>}
      {copyValue && <CopyButton value={copyValue} label={copyLabel} />}
    </figure>
  )
}
