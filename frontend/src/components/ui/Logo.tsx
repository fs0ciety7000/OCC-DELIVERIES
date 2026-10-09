import { cn } from '@/lib/cn'

export function LogoMark({ className }: { className?: string }) {
  return (
    <span aria-hidden className={cn('relative grid size-8 place-items-center rounded-[10px] bg-ember shadow-glow', className)}>
      <svg viewBox="0 0 24 24" className="size-[60%] text-brand-fg" fill="currentColor">
        <path d="M12 2c1.2 2.8 4.8 5 4.8 9.8 0 3.6-2.2 7-4.8 7s-4.8-3.4-4.8-7c0-2.2 1-3.6 2-4.6.2 1.4.8 2.4 1.8 2.8C10.6 7.2 11.2 4.6 12 2Z" />
        <rect x="6" y="20" width="12" height="2" rx="1" opacity=".6" />
      </svg>
    </span>
  )
}

export function Logo({ className }: { className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-2.5', className)}>
      <LogoMark />
      <span className="font-display text-lg leading-none font-bold tracking-tight">
        OCC <span className="text-muted font-semibold">Deliveries</span>
      </span>
    </span>
  )
}
