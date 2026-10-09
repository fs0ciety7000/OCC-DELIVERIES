import { cn } from '@/lib/cn'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
export type ButtonSize = 'sm' | 'md' | 'lg' | 'icon'

const base =
  'relative inline-flex items-center justify-center gap-2 whitespace-nowrap font-semibold select-none transition-[transform,background-color,box-shadow,border-color,color,opacity] duration-[var(--press-duration)] ease-ember press disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand'

const variants: Record<ButtonVariant, string> = {
  primary: 'bg-ember text-brand-fg shadow-glow hover:brightness-[1.06]',
  secondary: 'bg-surface text-fg border border-border-strong hover:bg-elevated hover:border-fg/25',
  ghost: 'text-fg hover:bg-fg/[0.06]',
  danger: 'bg-danger/12 text-danger border border-danger/30 hover:bg-danger/20',
}

const sizes: Record<ButtonSize, string> = {
  sm: 'min-h-9 px-3 text-sm rounded-sm',
  md: 'min-h-11 px-4 text-[15px] rounded-md',
  lg: 'min-h-13 px-6 text-base rounded-md',
  icon: 'size-11 rounded-md',
}

export function buttonClass(variant: ButtonVariant = 'primary', size: ButtonSize = 'md', block = false, className?: string) {
  return cn(base, variants[variant], sizes[size], block && 'w-full', className)
}
