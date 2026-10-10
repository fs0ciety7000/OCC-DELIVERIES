import type { HTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

export type BadgeVariant = 'neutral' | 'brand' | 'success' | 'warning' | 'danger' | 'info' | 'ubereats' | 'takeaway' | 'deliveroo' | 'weloveat'

const variants: Record<BadgeVariant, string> = {
  neutral: 'bg-fg/[0.07] text-muted border-border',
  brand: 'bg-brand/12 text-brand border-brand/25',
  success: 'bg-success/12 text-success border-success/25',
  warning: 'bg-warning/12 text-warning border-warning/25',
  danger: 'bg-danger/12 text-danger border-danger/25',
  info: 'bg-info/12 text-info border-info/25',
  ubereats: 'bg-ubereats/14 text-ubereats-ink border-ubereats/30',
  takeaway: 'bg-takeaway/14 text-takeaway-ink border-takeaway/30',
  deliveroo: 'bg-deliveroo/14 text-deliveroo-ink border-deliveroo/30',
  weloveat: 'bg-weloveat-ink/14 text-weloveat-ink border-weloveat-ink/30',
}

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant
  dot?: boolean
}

export function Badge({ variant = 'neutral', dot, className, children, ...rest }: BadgeProps) {
  return (
    <span
      className={cn('inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-semibold leading-4 whitespace-nowrap', variants[variant], className)}
      {...rest}
    >
      {dot && <span aria-hidden className="size-1.5 rounded-full bg-current" />}
      {children}
    </span>
  )
}
