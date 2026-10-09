import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { occ } from '@/lib/api'
import type { SearchQuery, SearchResult } from '@/lib/types'
import { DishMatches } from './DishMatches'
import { useDishFocus } from './dishFocus'

const wrap = (ui: ReactNode, path = '/') =>
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[path]}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )

afterEach(() => vi.restoreAllMocks())

describe('Plats correspondants', () => {
  it('liste les plats trouvés vers leur restaurant, ancre ?plat=', async () => {
    const spy = vi.spyOn(occ, 'search').mockImplementation(
      async (q: SearchQuery): Promise<SearchResult> => ({
        query: q.q,
        terms: ['marg'],
        fuzzy: false,
        restaurants: [],
        dishes: [{ id: 'd9', name: 'Margherita', price: 1100, emoji: '🍕', snippet: '', restaurant: { id: 'r9', name: 'Bella', emoji: '🍕' } }],
        people: [],
        actions: [],
      }),
    )
    wrap(<DishMatches q="marg" />)
    const link = await screen.findByRole('link', { name: /Margherita/ })
    expect(link).toHaveAttribute('href', '/restaurants/r9?plat=d9')
    expect(screen.getByRole('heading', { name: /Plats correspondants/ })).toBeInTheDocument()
    expect(spy).toHaveBeenCalledWith(expect.objectContaining({ q: 'marg', limit: 6 }), expect.anything())
  })

  it('ne cherche rien sous 2 caractères', () => {
    const spy = vi.spyOn(occ, 'search')
    const { container } = wrap(<DishMatches q="m" />)
    expect(container).toBeEmptyDOMElement()
    expect(spy).not.toHaveBeenCalled()
  })

  it('n’affiche rien quand aucun plat ne correspond', async () => {
    vi.spyOn(occ, 'search').mockResolvedValue({ query: 'zz', terms: [], fuzzy: false, restaurants: [], dishes: [], people: [], actions: [] })
    wrap(<DishMatches q="zz" />)
    await vi.waitFor(() => expect(screen.queryByLabelText('Plats correspondants')).not.toBeInTheDocument())
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
  })
})

function Menu() {
  useDishFocus(true)
  return (
    <div>
      <section data-section="__popular">
        <div data-dish="d1">
          <button type="button">Ramen (populaires)</button>
        </div>
      </section>
      <section data-section="cat">
        <div data-dish="d1">
          <button type="button">Ramen</button>
        </div>
      </section>
    </div>
  )
}

describe('useDishFocus', () => {
  it('défile jusqu’au plat de sa catégorie et y place le focus', async () => {
    const scroll = vi.fn()
    Element.prototype.scrollIntoView = scroll
    wrap(
      <Routes>
        <Route path="/restaurants/:id" element={<Menu />} />
      </Routes>,
      '/restaurants/r1?plat=d1',
    )
    await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Ramen' })).toHaveFocus())
    expect(scroll).toHaveBeenCalledWith(expect.objectContaining({ block: 'center' }))
  })
})
