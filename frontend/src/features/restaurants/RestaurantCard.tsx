import { Bike, Clock, MapPin, Star } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { Badge, Card, Money } from '@/components/ui'
import { cn } from '@/lib/cn'
import { formatDistance, formatEta, formatRating, priceLevel } from '@/lib/format'
import type { NearbyRestaurant, Restaurant } from '@/lib/types'
import { RestaurantCover } from './RestaurantCover'
import { cuisineLabel } from './visual'

export function ProviderBadges({ restaurant, className }: { restaurant: Pick<Restaurant, 'providers'>; className?: string }) {
  const providers = restaurant.providers ?? []
  if (!providers.length) return null
  return (
    <div className={cn('flex flex-wrap gap-1.5', className)}>
      {providers.map((p) => (
        <Badge key={p.id} variant={p.id === 'ubereats' ? 'ubereats' : 'takeaway'}>
          {p.id === 'ubereats' ? 'Uber Eats' : 'Takeaway'}
        </Badge>
      ))}
    </div>
  )
}

export function RestaurantMeta({ restaurant, className }: { restaurant: Restaurant | NearbyRestaurant; className?: string }) {
  const distance = 'distanceKm' in restaurant ? restaurant.distanceKm : undefined
  return (
    <ul className={cn('flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] leading-5 text-muted', className)}>
      {restaurant.rating > 0 && (
        <li className="inline-flex items-center gap-1 font-semibold text-fg">
          <Star aria-hidden className="size-3.5 fill-brand-2 text-brand-2" />
          <span className="tabular">{formatRating(restaurant.rating)}</span>
          {restaurant.rating_count > 0 && <span className="font-normal text-subtle tabular">({restaurant.rating_count})</span>}
          <span className="sr-only">sur 5</span>
        </li>
      )}
      {(restaurant.eta_min > 0 || restaurant.eta_max > 0) && (
        <li className="inline-flex items-center gap-1">
          <Clock aria-hidden className="size-3.5" />
          <span className="tabular">{formatEta(restaurant.eta_min, restaurant.eta_max)}</span>
        </li>
      )}
      <li className="inline-flex items-center gap-1">
        <Bike aria-hidden className="size-3.5" />
        {restaurant.delivery_fee > 0 ? <Money cents={restaurant.delivery_fee} /> : <span>Livraison offerte</span>}
      </li>
      {distance != null && (
        <li className="inline-flex items-center gap-1">
          <MapPin aria-hidden className="size-3.5" />
          <span className="tabular">{formatDistance(distance)}</span>
        </li>
      )}
      {restaurant.price_level > 0 && <li aria-label={`Gamme de prix ${restaurant.price_level} sur 4`}>{priceLevel(restaurant.price_level)}</li>}
    </ul>
  )
}

export interface RestaurantCardProps {
  restaurant: Restaurant | NearbyRestaurant
  to?: string
  onSelect?: () => void
  selected?: boolean
  action?: ReactNode
  compact?: boolean
  className?: string
}

export function RestaurantCard({ restaurant, to, onSelect, selected, action, compact, className }: RestaurantCardProps) {
  const cuisines = (restaurant.cuisines ?? []).slice(0, 3).map(cuisineLabel).join(' · ')
  const body = (
    <>
      <RestaurantCover restaurant={restaurant} className={cn('w-full', compact ? 'aspect-[16/9]' : 'aspect-[16/8]')} />
      <div className="flex flex-1 flex-col gap-1.5 p-3.5 sm:p-4">
        <div className="flex items-start justify-between gap-2">
          <h3 className="font-display text-[17px] leading-6 font-semibold">{restaurant.name}</h3>
          {action}
        </div>
        {cuisines && <p className="text-[13px] text-subtle">{cuisines}</p>}
        <RestaurantMeta restaurant={restaurant} />
        {!compact && <ProviderBadges restaurant={restaurant} className="mt-1" />}
      </div>
    </>
  )
  const cls = cn('flex h-full flex-col overflow-hidden', className)
  if (to) {
    return (
      <Card variant={selected ? 'selected' : 'interactive'} className={cls}>
        <Link to={to} className="flex h-full flex-col rounded-lg focus-visible:outline-offset-[-2px]" aria-label={restaurant.name}>
          {body}
        </Link>
      </Card>
    )
  }
  if (onSelect) {
    return (
      <Card variant={selected ? 'selected' : 'interactive'} className={cls}>
        <button type="button" onClick={onSelect} aria-pressed={selected} className="flex h-full flex-col rounded-lg text-left focus-visible:outline-offset-[-2px]">
          {body}
        </button>
      </Card>
    )
  }
  return (
    <Card variant={selected ? 'selected' : 'default'} className={cls}>
      {body}
    </Card>
  )
}

export function RestaurantCardSkeleton() {
  return (
    <Card className="overflow-hidden">
      <div className="skeleton aspect-[16/8] w-full" />
      <div className="space-y-2 p-4">
        <div className="skeleton h-4 w-2/3 rounded-sm" />
        <div className="skeleton h-3 w-1/2 rounded-sm" />
        <div className="skeleton h-3 w-3/4 rounded-sm" />
      </div>
    </Card>
  )
}
