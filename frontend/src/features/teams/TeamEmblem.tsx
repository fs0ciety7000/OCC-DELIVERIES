import { cn } from '@/lib/cn'
import { isHexColor } from '@/lib/colors'

/** Pastille d'équipe : emoji sur la couleur choisie (donnée utilisateur, comme les avatars). */
export function TeamEmblem({ emoji, color, size = 'md', className }: { emoji?: string; color?: string; size?: 'sm' | 'md' | 'lg'; className?: string }) {
  const dims = size === 'lg' ? 'size-16 text-3xl' : size === 'sm' ? 'size-10 text-xl' : 'size-12 text-2xl'
  return (
    <span
      aria-hidden
      className={cn('grid shrink-0 place-items-center rounded-lg border border-border bg-elevated', dims, className)}
      style={isHexColor(color) ? { backgroundColor: `color-mix(in srgb, ${color} 22%, transparent)`, borderColor: `color-mix(in srgb, ${color} 45%, transparent)` } : undefined}
    >
      {emoji || '🍽️'}
    </span>
  )
}
