import type { ButtonHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

export interface ChipProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  selected?: boolean
}

export function Chip({ selected, className, ...rest }: ChipProps) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      className={cn(
        'inline-flex min-h-9 shrink-0 items-center gap-1.5 rounded-full border px-3.5 text-sm font-medium whitespace-nowrap transition-colors duration-[120ms]',
        selected ? 'border-brand/50 bg-brand/12 text-fg' : 'border-border bg-surface text-muted hover:border-border-strong hover:text-fg',
        className,
      )}
      {...rest}
    />
  )
}
