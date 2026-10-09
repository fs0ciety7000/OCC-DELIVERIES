import { formatMoney } from '@/lib/format'
import { cn } from '@/lib/cn'

export interface MoneyProps {
  cents: number | null | undefined
  className?: string
  /** Affiche un signe explicite (« + 1,50 € ») pour les suppléments. */
  signed?: boolean
}

export function Money({ cents, className, signed }: MoneyProps) {
  const value = cents ?? 0
  const text = signed ? `${value < 0 ? '−' : '+'} ${formatMoney(Math.abs(value))}` : formatMoney(value)
  return <span className={cn('tabular whitespace-nowrap', className)}>{text}</span>
}
