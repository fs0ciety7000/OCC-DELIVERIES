import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi, type MockInstance } from 'vitest'
import { occ } from '@/lib/api'
import type { SearchQuery, SearchResult } from '@/lib/types'
import { GlobalSearch } from './GlobalSearch'
import { closeSearch, openSearch } from './store'

const ACTIONS: SearchResult['actions'] = [
  { id: 'new-party', label: 'Lancer une commande', href: '/?lancer=1' },
  { id: 'restaurants', label: 'Restos à proximité', href: '/restaurants' },
]

function result(q: string): SearchResult {
  const base: SearchResult = { query: q, terms: [], fuzzy: false, restaurants: [], dishes: [], people: [], actions: q ? [] : ACTIONS }
  if (q.toLowerCase().startsWith('ram')) {
    return {
      ...base,
      terms: ['ram'],
      restaurants: [{ id: 'r1', name: 'Tomo Râmen', emoji: '🍜', cuisines: ['ramen'], itemsCount: 12, distanceKm: 0.4 }],
      dishes: [{ id: 'd1', name: 'Râmen miso', price: 1450, emoji: '', snippet: 'Bouillon de porc', restaurant: { id: 'r1', name: 'Tomo Râmen', emoji: '🍜' } }],
      people: [
        {
          id: 'u2',
          name: 'Ramona Petit',
          avatar: '',
          color: '#3366ff',
          sharedParties: 2,
          recentParties: [{ id: 'p1', code: 'K7M2QX', title: 'Midi du vendredi', status: 'closed', created: '2026-10-01 10:00:00.000Z' }],
        },
      ],
    }
  }
  return base
}

function LocationProbe() {
  const loc = useLocation()
  return <output data-testid="location">{loc.pathname + loc.search}</output>
}

function renderSearch(onLaunch = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={['/']}>
        <GlobalSearch onLaunch={onLaunch} />
        <input aria-label="Autre champ" />
        <Routes>
          <Route path="*" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return { onLaunch }
}

let spy: MockInstance<typeof occ.search>

beforeEach(() => {
  localStorage.clear()
  spy = vi.spyOn(occ, 'search').mockImplementation(async (q: SearchQuery) => result(q.q))
})

afterEach(() => {
  act(() => closeSearch())
  vi.restoreAllMocks()
})

const queries = () => spy.mock.calls.map((c) => c[0].q)

describe('palette de recherche', () => {
  it('expose un dialogue modal et un combobox relié à sa liste (ARIA)', async () => {
    renderSearch()
    await userEvent.click(screen.getByRole('button', { name: /rechercher/i }))
    const dialog = screen.getByRole('dialog', { name: 'Recherche' })
    expect(dialog).toHaveAttribute('aria-modal', 'true')
    const input = within(dialog).getByRole('combobox')
    expect(input).toHaveFocus()
    expect(input).toHaveAttribute('aria-autocomplete', 'list')
    // vide : raccourcis du serveur + suggestions
    const listbox = await within(dialog).findByRole('listbox')
    expect(input).toHaveAttribute('aria-controls', listbox.id)
    expect(within(listbox).getByRole('group', { name: /raccourcis/i })).toBeInTheDocument()
    expect(within(listbox).getAllByRole('option')[0]).toHaveAttribute('aria-selected', 'true')
    expect(within(dialog).getByRole('group', { name: 'Suggestions' })).toBeInTheDocument()
  })

  it('attend la fin de la frappe (150 ms) avant d’interroger le serveur', async () => {
    renderSearch()
    act(() => openSearch())
    await userEvent.type(screen.getByRole('combobox'), 'ram')
    await screen.findByRole('option', { name: /Râmen miso/ })
    expect(queries()).toContain('ram')
    expect(queries()).not.toContain('r')
    expect(queries()).not.toContain('ra')
  })

  it('navigue au clavier dans les groupes et ouvre le plat choisi (ancre ?plat=)', async () => {
    renderSearch()
    act(() => openSearch())
    const input = screen.getByRole('combobox')
    await userEvent.type(input, 'ram')
    await screen.findByRole('option', { name: /Râmen miso/ })
    const options = screen.getAllByRole('option')
    expect(options).toHaveLength(3)
    expect(screen.getByRole('group', { name: 'Restaurants' })).toBeInTheDocument()
    expect(screen.getByRole('group', { name: 'Plats' })).toBeInTheDocument()
    expect(screen.getByRole('group', { name: 'Collègues' })).toBeInTheDocument()
    expect(input).toHaveAttribute('aria-activedescendant', options[0]!.id)

    await userEvent.keyboard('{ArrowDown}')
    expect(input).toHaveAttribute('aria-activedescendant', options[1]!.id)
    expect(options[1]).toHaveAttribute('aria-selected', 'true')
    expect(options[0]).toHaveAttribute('aria-selected', 'false')
    // ↑ depuis le premier : boucle sur le dernier
    await userEvent.keyboard('{ArrowUp}{ArrowUp}')
    expect(input).toHaveAttribute('aria-activedescendant', options[2]!.id)
    await userEvent.keyboard('{ArrowDown}{ArrowDown}{Enter}')

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent('/restaurants/r1?plat=d1')
  })

  it('surligne les termes trouvés', async () => {
    renderSearch()
    act(() => openSearch())
    await userEvent.type(screen.getByRole('combobox'), 'ram')
    const option = await screen.findByRole('option', { name: /^Tomo Râmen/ })
    expect(option.querySelector('mark')).toHaveTextContent('Râm')
  })

  it('mémorise les recherches récentes et les repropose', async () => {
    renderSearch()
    act(() => openSearch())
    await userEvent.type(screen.getByRole('combobox'), 'ram')
    await userEvent.click(await screen.findByRole('option', { name: /^Tomo Râmen/ }))
    expect(screen.getByTestId('location')).toHaveTextContent('/restaurants/r1')
    expect(JSON.parse(localStorage.getItem('occ:search:recent') ?? '[]')).toEqual(['ram'])

    act(() => openSearch())
    const recent = await screen.findByRole('group', { name: /recherches récentes/i })
    await userEvent.click(within(recent).getByRole('option', { name: 'ram' }))
    expect(screen.getByRole('combobox')).toHaveValue('ram')

    await userEvent.clear(screen.getByRole('combobox'))
    await userEvent.click(screen.getByRole('button', { name: /effacer l’historique/i }))
    expect(screen.queryByRole('group', { name: /recherches récentes/i })).not.toBeInTheDocument()
  })

  it('affiche la fiche d’un·e collègue, Échap revient puis ferme et rend le focus', async () => {
    renderSearch()
    const trigger = screen.getByRole('button', { name: /rechercher/i })
    await userEvent.click(trigger)
    await userEvent.type(screen.getByRole('combobox'), 'ram')
    await userEvent.click(await screen.findByRole('option', { name: /Ramona Petit/ }))
    expect(screen.getByRole('heading', { name: 'Ramona Petit' })).toBeInTheDocument()
    expect(screen.getByText('2 commandes en commun')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Midi du vendredi/ })).toHaveAttribute('href', '/party/p1')

    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('heading', { name: 'Ramona Petit' })).not.toBeInTheDocument()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })

  it('« Lancer une commande » délègue au shell', async () => {
    const { onLaunch } = renderSearch()
    act(() => openSearch())
    await userEvent.click(await screen.findByRole('option', { name: /Lancer une commande/ }))
    expect(onLaunch).toHaveBeenCalledOnce()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('état vide : aucun résultat + suggestions cliquables', async () => {
    renderSearch()
    act(() => openSearch())
    await userEvent.type(screen.getByRole('combobox'), 'zzz')
    expect(await screen.findByText('Aucun résultat pour « zzz »')).toBeInTheDocument()
    await userEvent.click(screen.getAllByRole('button', { name: 'Ramen' })[0]!)
    expect(screen.getByRole('combobox')).toHaveValue('Ramen')
    await screen.findByRole('option', { name: /Râmen miso/ })
  })

  it('erreur serveur : message et « Réessayer »', async () => {
    spy.mockRejectedValue(new Error('Serveur indisponible'))
    renderSearch()
    act(() => openSearch())
    await userEvent.type(screen.getByRole('combobox'), 'ram')
    expect(await screen.findByRole('alert')).toHaveTextContent(/n’a pas abouti/)
    spy.mockImplementation(async (q: SearchQuery) => result(q.q))
    await userEvent.click(screen.getByRole('button', { name: 'Réessayer' }))
    await screen.findByRole('option', { name: /Râmen miso/ })
  })

  it('raccourcis : Ctrl K et « / » ouvrent, sauf « / » tapé dans un champ', async () => {
    renderSearch()
    await userEvent.click(screen.getByRole('textbox', { name: 'Autre champ' }))
    await userEvent.keyboard('/')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Autre champ' })).toHaveValue('/')

    await userEvent.keyboard('{Control>}k{/Control}')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    await userEvent.keyboard('{Control>}k{/Control}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    screen.getByRole('textbox', { name: 'Autre champ' }).blur()
    await userEvent.keyboard('/')
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('combobox')).toHaveValue('')
    await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus())
  })

  it('piège le focus dans le dialogue (Tab boucle)', async () => {
    renderSearch()
    act(() => openSearch())
    const input = screen.getByRole('combobox')
    for (let i = 0; i < 20; i++) await userEvent.tab()
    expect(screen.getByRole('dialog').contains(document.activeElement)).toBe(true)
    await userEvent.tab({ shift: true })
    expect(screen.getByRole('dialog').contains(document.activeElement)).toBe(true)
    input.focus()
  })
})
