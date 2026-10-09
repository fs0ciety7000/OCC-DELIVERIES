import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { NearbyRestaurant } from '@/lib/types'
import { PartialMenuBadge, PartialMenuBanner } from './PartialMenu'
import { RestaurantCard } from './RestaurantCard'
import { uberEatsUrl } from './visual'

function restaurant(over: Partial<NearbyRestaurant> = {}): NearbyRestaurant {
  return {
    id: 'r1',
    collectionId: 'c',
    collectionName: 'restaurants',
    created: '',
    updated: '',
    name: 'Le Nouveau Wok',
    slug: 'le-nouveau-wok',
    description: '',
    emoji: '🥡',
    cuisines: ['chinois'],
    address: '',
    lat: 50.4542,
    lng: 3.9567,
    phone: '',
    rating: 4.2,
    rating_count: 30,
    price_level: 2,
    eta_min: 30,
    eta_max: 45,
    delivery_fee: 0,
    min_order: 0,
    providers: [{ id: 'ubereats', url: 'https://www.ubereats.com/be/store/nouveau-wok/bbb' }],
    active: true,
    distanceKm: 1.2,
    ...over,
  } as NearbyRestaurant
}

const renderCard = (r: NearbyRestaurant) =>
  render(
    <MemoryRouter>
      <RestaurantCard restaurant={r} to={`/restaurants/${r.id}`} />
    </MemoryRouter>,
  )

describe('Carte partielle (instantané Uber Eats)', () => {
  it('affiche le badge « Aperçu du menu » sur la carte du restaurant', () => {
    renderCard(restaurant({ partial_menu: true }))
    expect(screen.getByText('Aperçu du menu')).toBeInTheDocument()
  })

  it("n'affiche pas le badge pour une carte complète", () => {
    renderCard(restaurant({ partial_menu: false }))
    expect(screen.queryByText('Aperçu du menu')).not.toBeInTheDocument()
    render(<PartialMenuBadge restaurant={{}} />)
    expect(screen.queryByText('Aperçu du menu')).not.toBeInTheDocument()
  })

  it('masque la distance quand la position est approximative', () => {
    const { unmount } = renderCard(restaurant())
    expect(screen.getByText('1,2 km')).toBeInTheDocument()
    unmount()
    renderCard(restaurant({ geo_approx: true }))
    expect(screen.queryByText(/km$/)).not.toBeInTheDocument()
  })

  it('affiche le bandeau avec un lien vers la carte complète sur Uber Eats (nouvel onglet)', () => {
    render(<PartialMenuBanner restaurant={restaurant({ partial_menu: true })} />)
    expect(screen.getByRole('complementary', { name: 'Carte partielle' })).toHaveTextContent(
      'Carte partielle : seuls quelques plats sont connus ici. La carte complète est sur Uber Eats.',
    )
    const link = screen.getByRole('link', { name: /carte complète de Le Nouveau Wok sur Uber Eats/ })
    expect(link).toHaveAttribute('href', 'https://www.ubereats.com/be/store/nouveau-wok/bbb')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', expect.stringContaining('noopener'))
  })

  it("n'affiche pas de bandeau pour une carte complète, ni de lien sans adresse Uber Eats", () => {
    const { container, unmount } = render(<PartialMenuBanner restaurant={restaurant()} />)
    expect(container).toBeEmptyDOMElement()
    unmount()
    render(<PartialMenuBanner restaurant={restaurant({ partial_menu: true, providers: [] })} />)
    expect(screen.getByRole('complementary')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(uberEatsUrl({ providers: null })).toBe('')
  })
})
