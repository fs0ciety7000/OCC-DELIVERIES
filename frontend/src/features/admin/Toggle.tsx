import { cn } from '@/lib/cn'

/** Interrupteur accessible (`role="switch"`), cible ≥ 44 px. */
export function Toggle({
  checked,
  onChange,
  label,
  disabled,
  showLabel = false,
}: {
  checked: boolean
  onChange: (value: boolean) => void
  label: string
  disabled?: boolean
  showLabel?: boolean
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={showLabel ? undefined : label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className="inline-flex min-h-11 items-center gap-2.5 rounded-full text-sm font-medium disabled:opacity-50"
    >
      <span
        aria-hidden
        className={cn(
          'relative inline-block h-6 w-10 shrink-0 rounded-full border transition-colors duration-[120ms]',
          checked ? 'border-brand/60 bg-brand' : 'border-border-strong bg-elevated',
        )}
      >
        <span
          className={cn(
            'absolute top-0.5 left-0.5 size-[18px] rounded-full bg-fg shadow-card transition-transform duration-[120ms] motion-reduce:transition-none',
            checked && 'translate-x-4 bg-brand-fg',
          )}
        />
      </span>
      {showLabel && <span>{label}</span>}
    </button>
  )
}
