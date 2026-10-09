import type { HTMLAttributes, Ref } from 'react'
import { cn } from '@/lib/cn'

export type CardVariant = 'default' | 'interactive' | 'selected'

const variants: Record<CardVariant, string> = {
  default: '',
  interactive:
    'transition-[transform,border-color,box-shadow] duration-[120ms] ease-ember hover:-translate-y-0.5 hover:border-border-strong motion-reduce:hover:translate-y-0',
  selected: 'border-brand/70 shadow-glow',
}

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  variant?: CardVariant
  ref?: Ref<HTMLDivElement>
}

export function Card({ variant = 'default', className, ref, ...rest }: CardProps) {
  return <div ref={ref} className={cn('rounded-lg border border-border bg-surface shadow-card', variants[variant], className)} {...rest} />
}

export function CardHeader({ className, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('flex items-start justify-between gap-3 p-4 pb-0 sm:p-5 sm:pb-0', className)} {...rest} />
}

export function CardBody({ className, ...rest }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('p-4 sm:p-5', className)} {...rest} />
}

export function CardTitle({ className, ...rest }: HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cn('font-display text-lg font-semibold leading-6', className)} {...rest} />
}
