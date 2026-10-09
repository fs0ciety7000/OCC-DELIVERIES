import { Minus, Plus, Trash2 } from 'lucide-react'
import { cn } from '@/lib/cn'

export interface QuantityStepperProps {
  value: number
  onChange: (value: number) => void
  min?: number
  max?: number
  /** Si fourni, le « − » à la valeur min devient une corbeille. */
  onRemove?: () => void
  disabled?: boolean
  size?: 'sm' | 'md'
  label?: string
}

export function QuantityStepper({ value, onChange, min = 1, max = 20, onRemove, disabled, size = 'md', label = 'Quantité' }: QuantityStepperProps) {
  const btn = cn(
    'grid place-items-center rounded-full text-fg transition-colors hover:bg-fg/[0.08] disabled:opacity-40 disabled:pointer-events-none',
    size === 'sm' ? 'size-9' : 'size-11',
  )
  const atMin = value <= min
  return (
    <div role="group" aria-label={label} className={cn('inline-flex items-center rounded-full border border-border bg-elevated', disabled && 'opacity-60')}>
      {atMin && onRemove ? (
        <button type="button" className={btn} onClick={onRemove} disabled={disabled} aria-label="Retirer">
          <Trash2 className="size-4 text-danger" />
        </button>
      ) : (
        <button type="button" className={btn} onClick={() => onChange(Math.max(min, value - 1))} disabled={disabled || atMin} aria-label="Diminuer">
          <Minus className="size-4" />
        </button>
      )}
      <output aria-live="polite" className={cn('tabular text-center font-semibold', size === 'sm' ? 'w-6 text-sm' : 'w-8')}>
        {value}
      </output>
      <button type="button" className={btn} onClick={() => onChange(Math.min(max, value + 1))} disabled={disabled || value >= max} aria-label="Augmenter">
        <Plus className="size-4" />
      </button>
    </div>
  )
}
