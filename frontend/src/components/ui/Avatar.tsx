import { useEffect, useRef } from 'react'
import { cn } from '@/lib/cn'
import { fallbackColor, isHexColor, readableOn } from '@/lib/colors'
import { initials } from '@/lib/format'
import { fileUrl } from '@/lib/pb'
import { usePulse } from '@/lib/pulse'

export interface AvatarUser {
  id: string
  name?: string
  avatar?: string
  color?: string
}

export type AvatarSize = 24 | 32 | 40 | 56

const sizeClass: Record<AvatarSize, string> = {
  24: 'size-6 text-[10px]',
  32: 'size-8 text-xs',
  40: 'size-10 text-sm',
  56: 'size-14 text-lg',
}

export interface AvatarProps {
  user: AvatarUser
  size?: AvatarSize
  ready?: boolean
  className?: string
  /** Pas de nom accessible (déjà annoncé à côté). */
  decorative?: boolean
}

export function Avatar({ user, size = 32, ready, className, decorative }: AvatarProps) {
  const color = isHexColor(user.color) ? user.color : fallbackColor(user.id)
  const src = fileUrl('users', user.id, user.avatar, '100x100')
  const tone = readableOn(color)
  const pulseCount = usePulse(user.id)
  const ref = useRef<HTMLSpanElement>(null)

  // Pulse 600 ms quand ce collègue agit en direct (vote, prêt, paiement…).
  useEffect(() => {
    const el = ref.current
    if (!el || pulseCount === 0) return
    el.classList.remove('animate-live')
    void el.offsetWidth
    el.classList.add('animate-live')
  }, [pulseCount])

  const label = decorative ? undefined : `${user.name || 'Membre'}${ready ? ' — prêt·e' : ''}`
  return (
    <span
      ref={ref}
      role={decorative ? undefined : 'img'}
      aria-label={label}
      aria-hidden={decorative || undefined}
      title={user.name}
      className={cn(
        'relative inline-grid shrink-0 place-items-center overflow-hidden rounded-full font-semibold ring-2 ring-bg select-none',
        tone === 'ink' ? 'text-ink' : 'text-paper',
        ready && 'ring-success',
        sizeClass[size],
        className,
      )}
      style={{ backgroundColor: color }}
    >
      {src ? <img src={src} alt="" className="size-full object-cover" loading="lazy" /> : initials(user.name)}
    </span>
  )
}

export interface AvatarStackProps {
  users: (AvatarUser & { ready?: boolean })[]
  size?: AvatarSize
  max?: number
  className?: string
  showReady?: boolean
}

export function AvatarStack({ users, size = 32, max = 5, className, showReady }: AvatarStackProps) {
  const shown = users.slice(0, max)
  const rest = users.length - shown.length
  const names = users.map((u) => u.name || 'Membre').join(', ')
  return (
    <div className={cn('flex items-center -space-x-2', className)} role="group" aria-label={`${users.length} participant·es : ${names}`}>
      {shown.map((u) => (
        <Avatar key={u.id} user={u} size={size} ready={showReady && u.ready} decorative />
      ))}
      {rest > 0 && (
        <span aria-hidden className={cn('relative inline-grid place-items-center rounded-full bg-elevated font-semibold text-muted ring-2 ring-bg tabular', sizeClass[size])}>
          +{rest}
        </span>
      )}
    </div>
  )
}
