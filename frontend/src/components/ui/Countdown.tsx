import { Timer } from 'lucide-react'
import { cn } from '@/lib/cn'
import { formatCountdown, parseDate } from '@/lib/format'
import { useNow } from '@/lib/hooks'

export interface CountdownProps {
  to: string | Date | null | undefined
  label?: string
  className?: string
}

/** Compte à rebours mm:ss (indicatif). Rien si pas d'échéance. */
export function Countdown({ to, label = 'Temps restant', className }: CountdownProps) {
  const now = useNow(1000)
  const target = typeof to === 'string' ? parseDate(to) : (to ?? null)
  if (!target) return null
  const ms = target.getTime() - now.getTime()
  const over = ms <= 0
  const urgent = !over && ms < 60_000
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-semibold tabular',
        over ? 'border-border text-subtle' : urgent ? 'border-danger/30 bg-danger/10 text-danger' : 'border-warning/30 bg-warning/10 text-warning',
        className,
      )}
      aria-label={over ? `${label} : temps écoulé` : `${label} : ${formatCountdown(ms)}`}
    >
      <Timer aria-hidden className="size-3.5" />
      <span aria-hidden>{over ? 'Temps écoulé' : formatCountdown(ms)}</span>
    </span>
  )
}
