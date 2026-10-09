import type { ReactNode } from 'react'
import { EmptyPlate } from '@/components/food/EmptyPlate'
import { cn } from '@/lib/cn'

export interface EmptyStateProps {
  /** Sans emoji : illustration animée « assiette vide ». */
  emoji?: string
  /** Illustration personnalisée (prioritaire sur `emoji`), ex. `<EmptyBag />`. */
  illustration?: ReactNode
  title: string
  description?: ReactNode
  action?: ReactNode
  className?: string
  tone?: 'default' | 'danger'
}

export function EmptyState({ emoji, illustration, title, description, action, className, tone = 'default' }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center gap-3 px-6 py-12 text-center', className)} role={tone === 'danger' ? 'alert' : undefined}>
      {illustration ? (
        illustration
      ) : emoji ? (
        <div
          aria-hidden
          className={cn(
            'grid size-16 place-items-center rounded-xl border text-3xl',
            tone === 'danger' ? 'border-danger/30 bg-danger/10' : 'border-border bg-elevated',
          )}
        >
          {emoji}
        </div>
      ) : (
        <EmptyPlate />
      )}
      <h3 className="font-display text-xl font-semibold">{title}</h3>
      {description && <p className="max-w-sm text-sm text-muted">{description}</p>}
      {action && <div className="mt-2 flex flex-wrap justify-center gap-2">{action}</div>}
    </div>
  )
}
