import type { Restaurant } from '@/lib/types'

/** Crédit ODbL : affiché quand une coordonnée vient d'OpenStreetMap. */
export function OsmAttribution({ restaurant }: { restaurant: Pick<Restaurant, 'enriched_from'> }) {
  const src = restaurant.enriched_from
  if (!src || src.provider !== 'osm' || !(src.fields?.length > 0)) return null
  return (
    <p className="text-xs text-subtle">
      Coordonnées : ©{' '}
      <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener noreferrer" className="underline underline-offset-2 hover:text-fg">
        contributeurs OpenStreetMap
      </a>
    </p>
  )
}
