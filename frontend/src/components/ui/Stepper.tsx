import { Check } from 'lucide-react'
import { motion } from 'motion/react'
import { cn } from '@/lib/cn'
import { ease } from '@/lib/motion'
import type { PartyStatus } from '@/lib/types'
import { PARTY_STEPS, stepIndex } from './steps'

export interface StepperProps {
  status: PartyStatus
  className?: string
}

/**
 * Étapes de la party : Salon → Vote → Commande → Récap → Paiement.
 * Au changement d'étape, le remplissage de la barre grandit (scaleX, courbe Ember) et le
 * point de l'étape courante « pulse » une fois. Mouvement réduit : `MotionConfig` coupe les
 * transformations (état final immédiat).
 */
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
            <span aria-hidden className="relative block h-1">
              <span className="absolute inset-0 overflow-hidden rounded-full bg-fg/10">
                <motion.span
                  className={cn('absolute inset-0 origin-left rounded-full transition-colors duration-200', active ? 'bg-ember' : 'bg-brand')}
                  initial={false}
                  animate={{ scaleX: done || active ? 1 : 0 }}
                  transition={{ duration: 0.5, ease, delay: active ? 0.12 : 0 }}
                />
              </span>
              {active && (
                <motion.span
                  key={status}
                  className="absolute top-1/2 right-0 size-2 -translate-y-1/2 rounded-full bg-brand-2 ring-2 ring-bg"
                  initial={{ scale: 0 }}
                  animate={{ scale: [0, 1.7, 1] }}
                  transition={{ duration: 0.6, ease, delay: 0.45 }}
                />
              )}
            </span>
            <span
              className={cn(
                'flex items-center gap-1 truncate text-[11px] leading-4 font-medium sm:text-xs',
                active ? 'text-fg' : done ? 'text-muted' : 'text-subtle',
              )}
            >
              {done && <Check aria-hidden className="hidden size-3 shrink-0 text-brand sm:block" />}
              <span className="truncate">{step.label}</span>
              <span className="sr-only">{done ? ' (terminé)' : active ? ' (en cours)' : ' (à venir)'}</span>
            </span>
          </li>
        )
      })}
    </ol>
  )
}
