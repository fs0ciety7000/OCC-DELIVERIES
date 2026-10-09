import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { adminApi } from '@/lib/api'
import type { AdminSettings, Restaurant } from '@/lib/types'
import { IncompleteMenusCard } from './IncompleteMenusCard'
import { countIncomplete, isIncomplete, parseThreshold } from './incomplete'
import { RestaurantsAdminPage } from './RestaurantsAdminPage'

function resto(id: string, items: number, active = true): Restaurant {
  return {
    id, collectionId: 'c', collectionName: 'restaurants', created: '', updated: '',
    name: `Resto ${id}`, slug: `resto-${id}`, description: '', emoji: '', cuisines: [], address: '', lat: 50.45, lng: 3.95, phone: '',
    rating: 4, rating_count: 0, price_level: 2, eta_min: 20, eta_max: 30, delivery_fee: 0, min_order: 0, providers: [], active, items_count: items,
  } as Restaurant
}

const list = [resto('a', 3), resto('b', 12), resto('c', 0, false), resto('d', 9)]

describe('incomplete', () => {
  it('applique la même règle que le serveur', () => {
    expect(isIncomplete({ items_count: 3 }, 0)).toBe(false)
    expect(isIncomplete({ items_count: 9 }, 10)).toBe(true)
    expect(isIncomplete({ items_count: 10 }, 10)).toBe(false)
    expect(isIncomplete({}, 1)).toBe(true)
    // les restaurants désactivés ne comptent pas
    expect(countIncomplete(list, 10)).toBe(2)
    expect(countIncomplete(list, 4)).toBe(1)
    expect(countIncomplete(list, 0)).toBe(0)
  })

  it('valide le seuil saisi', () => {
    expect(parseThreshold('10')).toEqual({ value: 10 })
    expect(parseThreshold(' 100 ')).toEqual({ value: 100 })
    for (const bad of ['', '0', '101', '2.5', 'dix', '-3']) expect(parseThreshold(bad)).toHaveProperty('error')
  })
})

describe('IncompleteMenusCard', () => {
  const on: AdminSettings = { minMenuItems: 10, hiddenRestaurants: 2, activeRestaurants: 3 }
  const off: AdminSettings = { minMenuItems: 0, hiddenRestaurants: 0, activeRestaurants: 3 }

  it('active le filtre avec le seuil saisi (10 par défaut)', async () => {
    const onSave = vi.fn()
    render(<IncompleteMenusCard settings={off} restaurants={list} onSave={onSave} />)
    const sw = screen.getByRole('switch', { name: 'Masquer les restaurants incomplets' })
    expect(sw).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByLabelText('Masquer les restaurants de moins de')).toHaveValue(10)
    expect(screen.getByText(/2 restaurants seraient masqués avec ce seuil/)).toBeInTheDocument()

    await userEvent.click(sw)
    expect(onSave).toHaveBeenCalledWith(10)
  })

  it('affiche le nombre masqué, change le seuil et désactive', async () => {
    const onSave = vi.fn()
    const onShowHidden = vi.fn()
    render(<IncompleteMenusCard settings={on} restaurants={list} onSave={onSave} onShowHidden={onShowHidden} />)
    expect(screen.getByRole('switch', { name: 'Masquer les restaurants incomplets' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByText('2 restaurants masqués')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /Voir les restaurants masqués/ }))
    expect(onShowHidden).toHaveBeenCalled()

    const input = screen.getByLabelText('Masquer les restaurants de moins de')
    await userEvent.clear(input)
    await userEvent.type(input, '4')
    expect(screen.getByText(/1 restaurant serait masqué avec ce seuil/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Appliquer' }))
    expect(onSave).toHaveBeenLastCalledWith(4)

    await userEvent.clear(input)
    await userEvent.type(input, '500')
    expect(screen.getByRole('alert')).toHaveTextContent('1 à 100')
    expect(screen.queryByRole('button', { name: 'Appliquer' })).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('switch', { name: 'Masquer les restaurants incomplets' }))
    expect(onSave).toHaveBeenLastCalledWith(0)
  })
})

describe('RestaurantsAdminPage — cartes incomplètes', () => {
  afterEach(() => vi.restoreAllMocks())

  it('signale les restaurants masqués et les filtre', async () => {
    vi.spyOn(adminApi, 'restaurants').mockResolvedValue(list)
    vi.spyOn(adminApi, 'settings').mockResolvedValue({ minMenuItems: 10, hiddenRestaurants: 2, activeRestaurants: 3 })
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter initialEntries={['/admin/restaurants?filtre=incompletes']}>
          <RestaurantsAdminPage />
        </MemoryRouter>
      </QueryClientProvider>,
    )
    const ul = await screen.findByRole('list', { name: 'Restaurants' })
    expect(await within(ul).findByText('Resto a')).toBeInTheDocument()
    expect(within(ul).getByText('Resto d')).toBeInTheDocument()
    expect(within(ul).queryByText('Resto b')).not.toBeInTheDocument()
    expect(within(ul).queryByText('Resto c')).not.toBeInTheDocument()
    expect(within(ul).getByText(/Masqué : carte incomplète \(3 plats\)/)).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Incomplets' })).toHaveAttribute('aria-checked', 'true')

    await userEvent.click(screen.getByRole('radio', { name: 'Tous' }))
    expect(within(ul).getByText('Resto b')).toBeInTheDocument()
    expect(within(ul).getAllByText(/Masqué : carte incomplète/)).toHaveLength(2)
  })
})
