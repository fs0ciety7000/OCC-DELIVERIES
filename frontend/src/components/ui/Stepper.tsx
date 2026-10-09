import { Check } from 'lucide-react'
import { cn } from '@/lib/cn'
import type { PartyStatus } from '@/lib/types'
import { PARTY_STEPS, stepIndex } from './steps'

export interface StepperProps {
  status: PartyStatus
  className?: string
}

/** Étapes de la party : Salon → Vote → Commande → Récap → Paiement. */
export function Stepper({ status, className }: StepperProps) {
  const current = stepIndex(status)
  return (
    <ol className={cn('flex w-full items-center gap-1.5', className)} aria-label="Étapes de la commande">
      {PARTY_STEPS.map((step, i) => {
        const done = i < current
        const active = i === current
        return (
          <li
            key={step.status}
            className="flex min-w-0 flex-1 flex-col gap-1.5"
            aria-current={active ? 'step' : undefined}
            data-state={done ? 'done' : active ? 'active' : 'todo'}
          >
            <span
              aria-hidden
              className={cn(
                'h-1 rounded-full transition-colors duration-200',
                done ? 'bg-brand' : active ? 'bg-ember' : 'bg-fg/10',
              )}
            />
            <span
              className={cn(
                'flex items-center gap-1 truncate text-[11px] leading-4 font-medium sm:text-xs',
                active ? 'text-fg' : done ? 'text-muted' : 'text-subtle',
              )}
            >
              {done && <Check aria-hidden className="size-3 shrink-0 text-brand" />}
              <span className="truncate">{step.label}</span>
              <span className="sr-only">{done ? ' (terminé)' : active ? ' (en cours)' : ' (à venir)'}</span>
            </span>
          </li>
        )
      })}
    </ol>
  )
}
