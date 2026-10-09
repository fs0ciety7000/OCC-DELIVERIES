import { beforeAll, describe, expect, it } from 'vitest'
import code from '../../public/outils/occ-menu-export.js?raw'

type Extracted = {
  restaurant: {
    slug: string
    name: string
    cuisines: string[]
    address: string
    lat: number
    rating: number
    providers: { id: string; url: string }[]
    categories: { name: string; items: { name: string; price: number; tags: string[] }[] }[]
  }
  stats: { categories: number; items: number; source: string }
}
type Api = {
  extract: (doc: Document, href: string, extra?: unknown[]) => Extracted
  toCents: (v: unknown, cents?: boolean) => number
}

let api: Api

beforeAll(() => {
  const g = globalThis as unknown as { __OCC_EXPORT_NO_RUN__: boolean; OCCMenuExport: Api }
  g.__OCC_EXPORT_NO_RUN__ = true
  new Function(code)()
  api = g.OCCMenuExport
})

function doc(html: string): Document {
  return new DOMParser().parseFromString(html, 'text/html')
}

describe('occ-menu-export', () => {
  it('convertit les prix en centimes (formats FR/EN)', () => {
    expect(api.toCents('12,50')).toBe(1250)
    expect(api.toCents('€ 9.9')).toBe(990)
    expect(api.toCents(7)).toBe(700)
    expect(api.toCents(1450, true)).toBe(1450)
    expect(api.toCents('')).toBe(0)
  })

  it('lit un menu schema.org JSON-LD', () => {
    const ld = {
      '@context': 'https://schema.org',
      '@type': 'Restaurant',
      name: 'Chez Test',
      servesCuisine: ['Pizza', 'Italien'],
      address: { streetAddress: 'Grand-Place 1', postalCode: '7000', addressLocality: 'Mons' },
      geo: { latitude: '50.4542', longitude: '3.9519' },
      aggregateRating: { ratingValue: '4.56', reviewCount: '120' },
      hasMenu: {
        '@type': 'Menu',
        hasMenuSection: [
          {
            '@type': 'MenuSection',
            name: 'Pizzas',
            hasMenuItem: [
              { '@type': 'MenuItem', name: 'Margherita', offers: { price: '11.50', priceCurrency: 'EUR' }, suitableForDiet: 'https://schema.org/VegetarianDiet' },
              { '@type': 'MenuItem', name: 'Sans prix' },
            ],
          },
        ],
      },
    }
    const d = doc(`<html><head><script type="application/ld+json">${JSON.stringify(ld)}</script></head><body></body></html>`)
    const res = api.extract(d, 'https://www.ubereats.com/be/store/chez-test/abc?x=1')
    expect(res.stats).toEqual({ categories: 1, items: 1, source: 'json-ld' })
    expect(res.restaurant.name).toBe('Chez Test')
    expect(res.restaurant.slug).toBe('chez-test')
    expect(res.restaurant.address).toBe('Grand-Place 1, 7000 Mons')
    expect(res.restaurant.lat).toBeCloseTo(50.4542)
    expect(res.restaurant.rating).toBe(4.6)
    expect(res.restaurant.cuisines).toEqual(['pizza', 'italien'])
    expect(res.restaurant.providers).toEqual([{ id: 'ubereats', url: 'https://www.ubereats.com/be/store/chez-test/abc' }])
    expect(res.restaurant.categories[0]?.items[0]).toMatchObject({ name: 'Margherita', price: 1150, tags: ['veggie'] })
  })

  it('se rabat sur un état applicatif embarqué (centimes entiers)', () => {
    const state = {
      store: {
        title: 'Snack du Beffroi',
        location: { address: 'Rue X 2, 7000 Mons' },
        sections: [{ title: 'Frites', itemList: [{ title: 'Grande frite', price: 450 }, { title: 'Petite frite', price: 350 }] }],
      },
    }
    const d = doc(`<html><head><title>Snack du Beffroi | Uber Eats</title><script type="application/json">${JSON.stringify(state)}</script></head></html>`)
    const res = api.extract(d, 'https://www.takeaway.com/be-fr/menu/snack')
    expect(res.stats.source).toBe('état embarqué')
    expect(res.restaurant.name).toBe('Snack du Beffroi')
    expect(res.restaurant.providers[0]?.id).toBe('takeaway')
    expect(res.restaurant.categories[0]?.items.map((i) => i.price)).toEqual([450, 350])
  })

  it('ne renvoie rien quand la page ne contient pas de menu', () => {
    const res = api.extract(doc('<html><head><title>Liste</title></head></html>'), 'https://example.org/')
    expect(res.stats.items).toBe(0)
  })

  it('lit les données chargées par la page (catégories par identifiants, prix dans les variantes)', () => {
    const api_ = {
      restaurant: { name: 'Snack à la Gare' },
      menu: {
        categories: [
          { name: 'Dürüms', itemIds: ['a', 'b'] },
          { name: 'Boissons', itemIds: ['c'] },
        ],
        items: {
          a: { id: 'a', name: 'Dürüm poulet', variations: [{ basePrice: 9.5 }] },
          b: { id: 'b', name: 'Dürüm mixte', variations: [{ basePrice: 10 }] },
          c: { id: 'c', name: 'Coca-Cola 500 ml', variations: [{ basePrice: 2.5 }] },
        },
      },
    }
    const d = doc('<html><head><title>Snack à la Gare | Takeaway.com</title></head><body></body></html>')
    const res = api.extract(d, 'https://www.takeaway.com/be-fr/menu/snack-a-la-gare', [api_])
    expect(res.stats).toMatchObject({ categories: 2, items: 3, source: 'données chargées' })
    expect(res.restaurant.name).toBe('Snack à la Gare')
    expect(res.restaurant.providers[0]?.id).toBe('takeaway')
    expect(res.restaurant.categories[0]?.items.map((i) => i.price)).toEqual([950, 1000])
  })

  it('se rabat sur la page affichée quand les prix sont visibles', () => {
    const d = doc(`<html><body><main>
      <h2>Frites</h2>
      <div class="card"><h3>Grande frite</h3><span>€ 4,50</span></div>
      <div class="card"><h3>Petite frite</h3><span>3,50 €</span></div>
      <h2>Boissons</h2>
      <div class="card"><h3>Eau pétillante</h3><span>Unavailable</span></div>
    </main></body></html>`)
    const res = api.extract(d, 'https://www.takeaway.com/be-fr/menu/x')
    expect(res.stats.source).toBe('page affichée')
    expect(res.restaurant.categories).toHaveLength(1)
    expect(res.restaurant.categories[0]?.items.map((i) => [i.name, i.price])).toEqual([
      ['Grande frite', 450],
      ['Petite frite', 350],
    ])
  })
})
