import { ExternalLink, Info } from 'lucide-react'
import { Badge, buttonClass } from '@/components/ui'
import { cn } from '@/lib/cn'
import type { Restaurant } from '@/lib/types'
import { uberEatsUrl } from './visual'

/** Badge « Aperçu du menu » : seuls quelques plats sont connus (instantané Uber Eats). */
export function PartialMenuBadge({ restaurant, className }: { restaurant: Pick<Restaurant, 'partial_menu'>; className?: string }) {
  if (!restaurant.partial_menu) return null
  return (
    <Badge variant="info" className={className} title="Seuls quelques plats sont connus ici">
      Aperçu du menu
    </Badge>
  )
}

/** Bandeau calme pour un restaurant à carte partielle, avec le lien vers la carte complète sur Uber Eats. */
export function PartialMenuBanner({ restaurant, className }: { restaurant: Pick<Restaurant, 'partial_menu' | 'providers' | 'name'>; className?: string }) {
  if (!restaurant.partial_menu) return null
  const url = uberEatsUrl(restaurant)
  return (
    <aside
      aria-label="Carte partielle"
      className={cn('flex flex-col gap-3 rounded-md border border-info/25 bg-info/10 p-3.5 text-sm sm:flex-row sm:items-center sm:justify-between', className)}
    >
      <p className="flex items-start gap-2 text-fg">
        <Info aria-hidden className="mt-0.5 size-4 shrink-0 text-info" />
        <span>
          <strong className="font-semibold">Carte partielle :</strong> seuls quelques plats sont connus ici. La carte complète est sur Uber Eats.
        </span>
      </p>
      {url && (
        <a
          href={url}
          target="_blank"
          rel="noreferrer noopener"
          className={cn(buttonClass('secondary', 'sm'), 'shrink-0 self-start sm:self-auto')}
          aria-label={`Voir la carte complète de ${restaurant.name} sur Uber Eats (nouvel onglet)`}
        >
          Voir sur Uber Eats <ExternalLink aria-hidden className="size-3.5" />
        </a>
      )}
    </aside>
  )
}
