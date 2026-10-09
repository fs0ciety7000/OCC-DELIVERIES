import { useState } from 'react'
import { cn } from '@/lib/cn'
import type { Restaurant } from '@/lib/types'
import { coverUrl, tileStyle } from './visual'

export interface RestaurantCoverProps {
  restaurant: Pick<Restaurant, 'id' | 'name' | 'emoji' | 'cover' | 'cover_url'>
  className?: string
  emojiClassName?: string
  thumb?: string
}

/** Visuel : cover si présente, sinon tuile dégradée générée avec l'emoji. */
export function RestaurantCover({ restaurant, className, emojiClassName, thumb = '640x360' }: RestaurantCoverProps) {
  const src = coverUrl(restaurant, thumb)
  const [failed, setFailed] = useState(false)
  return (
    <div className={cn('relative overflow-hidden bg-elevated', className)} style={src && !failed ? undefined : tileStyle(restaurant.id)}>
      {src && !failed ? (
        <img src={src} alt="" loading="lazy" className="size-full object-cover" onError={() => setFailed(true)} />
      ) : (
        <span aria-hidden className={cn('absolute inset-0 grid place-items-center text-5xl drop-shadow-lg', emojiClassName)}>
          {restaurant.emoji || '🍽️'}
        </span>
      )}
    </div>
  )
}
