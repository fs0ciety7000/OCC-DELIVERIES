import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { STATUS_LABELS } from '@/features/party/hooks'
import type { HistoryEntry, HistoryPage, MyStats } from '@/lib/types'
import { HistoryCard } from './HistoryCard'
import { historyBadge } from './historyBadge'

const base: HistoryEntry = {
  id: 'p1', code: 'K7M2QX', title: 'Midi du vendredi', status: 'closed', created: '2026-10-09 10:00:00.000Z', closedAt: '2026-10-09 13:00:00.000Z',
  restaurant: { id: 'r1', name: 'Pizza Nonna', emoji: '🍕', cover: '', cover_url: '', active: true },
  provider: 'ubereats', host: { id: 'u2', name: 'Bob' }, isHost: false, memberCount: 3,
  items: [
    { menuItem: 'm1', name: 'Margherita', optionsLabel: 'Large, Fromage', note: 'bien cuite', quantity: 2, unitPrice: 1450, total: 2900 },
    { menuItem: 'm2', name: 'Tiramisu', optionsLabel: '', note: '', quantity: 1, unitPrice: 600, total: 600 },
  ],
  subtotal: 3500, sharedFees: 133, total: 3633, grandTotal: 6099,
  payer: { id: 'u2', name: 'Bob' }, payment: { id: 'pay1', method: 'wero', status: 'confirmed', amount: 3633 },
}

const page = (items: HistoryEntry[], p = 1, totalPages = 1): HistoryPage => ({ page: p, perPage: 10, totalItems: items.length + (totalPages - 1) * 10, totalPages, items })
const stats: MyStats = { orders: 4, totalSpent: 5210, favoriteRestaurant: { id: 'r1', name: 'Pizza Nonna', emoji: '🍕', orders: 3 }, favoriteDish: { name: 'Margherita', quantity: 5, orders: 3 } }

let fail = false
let statsData: MyStats = stats
const history = vi.fn(async (_page?: number): Promise<HistoryPage> => page([base]))
vi.mock('@/lib/api', () => ({
  occ: {
    history: async (p?: number) => {
      if (fail) throw new Error('Serveur injoignable')
      return history(p)
    },
    myStats: vi.fn(async () => statsData),
  },
  partiesApi: {},
}))

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/', element: ui }, { path: '/party/:id', element: <p>party page</p> }])
  return render(
    <QueryClientProvider client={qc}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

describe('historyBadge', () => {
  it.each<[Partial<HistoryEntry>, string, string]>([
    [{}, 'Remboursé', 'success'],
    [{ payment: { id: 'x', method: 'self', status: 'confirmed', amount: 0 } }, "Tu as avancé l'argent", 'brand'],
    [{ payment: { id: 'x', method: 'wero', status: 'declared', amount: 1 } }, 'Déclaré · Wero', 'info'],
    [{ payment: { id: 'x', method: '', status: 'pending', amount: 1 }, status: 'paying' }, 'À rembourser', 'warning'],
    [{ status: 'cancelled' }, 'Annulée', 'danger'],
    [{ payment: null, status: 'ordering' }, STATUS_LABELS.ordering, 'brand'],
    [{ items: [], payment: null }, 'Sans article', 'neutral'],
  ])('%o → %s', (patch, label, variant) => {
    expect(historyBadge({ ...base, ...patch })).toEqual({ label, variant })
  })
})

describe('HistoryCard', () => {
  it('affiche ma part, le statut et déplie mes plats', () => {
    const onRelaunch = vi.fn()
    wrap(<HistoryCard entry={base} onRelaunch={onRelaunch} />)
    expect(screen.getByRole('heading', { name: 'Pizza Nonna' })).toBeInTheDocument()
    expect(screen.getByText('Remboursé')).toBeInTheDocument()
    expect(screen.getAllByText(/36,33/).length).toBeGreaterThan(0)

    const toggle = screen.getByRole('button', { name: /Mes plats \(3\)/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Large, Fromage')).not.toBeInTheDocument()
    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    const list = screen.getByRole('list', { name: 'Mes plats' })
    expect(within(list).getByText('Margherita')).toBeInTheDocument()
    expect(within(list).getByText('Large, Fromage')).toBeInTheDocument()
    expect(within(list).getByText('« bien cuite »')).toBeInTheDocument()
    expect(screen.getByText(/Payé par Bob · remboursé via Wero/)).toBeInTheDocument()

    expect(screen.getByRole('link', { name: /Voir la commande « Midi du vendredi »/ })).toHaveAttribute('href', '/party/p1')
    fireEvent.click(screen.getByRole('button', { name: /Relancer ici/ }))
    expect(onRelaunch).toHaveBeenCalledWith(base.restaurant)
  })

  it('commande en cours : « Reprendre », pas de relance ; annulée : pas de lien', () => {
    const { unmount } = wrap(<HistoryCard entry={{ ...base, status: 'ordering', payment: null }} onRelaunch={vi.fn()} />)
    expect(screen.getByRole('link', { name: /Reprendre la commande/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Relancer ici/ })).not.toBeInTheDocument()
    unmount()
    wrap(<HistoryCard entry={{ ...base, status: 'cancelled' }} onRelaunch={vi.fn()} />)
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByText('Annulée')).toBeInTheDocument()
  })

  it('pas de second badge identique au premier (récap sans paiement)', () => {
    wrap(<HistoryCard entry={{ ...base, status: 'review', payment: null, payer: null }} />)
    expect(screen.getAllByText(STATUS_LABELS.review)).toHaveLength(1)
  })

  it('second badge de statut quand il apporte une info', () => {
    wrap(<HistoryCard entry={{ ...base, status: 'paying' }} />)
    expect(screen.getByText('Remboursé')).toBeInTheDocument()
    expect(screen.getByText(STATUS_LABELS.paying)).toBeInTheDocument()
  })

  it('sans plat : pas de bouton « Aucun plat », part affichée « — »', () => {
    wrap(<HistoryCard entry={{ ...base, status: 'ordering', items: [], payment: null, payer: null, total: 0 }} />)
    expect(screen.queryByRole('button', { name: /Aucun plat|Mes plats/ })).not.toBeInTheDocument()
    expect(screen.getByText('—')).toBeInTheDocument()
    expect(screen.queryByText(/0,00/)).not.toBeInTheDocument()
  })

  it('restaurant désactivé : pas de relance', () => {
    wrap(<HistoryCard entry={{ ...base, restaurant: { ...base.restaurant!, active: false } }} onRelaunch={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /Relancer ici/ })).not.toBeInTheDocument()
  })
})

describe('OrderHistory', () => {
  beforeEach(() => history.mockReset())

  it('statistiques + liste paginée (« Voir plus »)', async () => {
    const second = { ...base, id: 'p2', title: 'Soirée', restaurant: { ...base.restaurant!, id: 'r2', name: 'Sushi Go' } }
    history.mockImplementation(async (p?: number) => (p === 2 ? page([second], 2, 2) : page([base], 1, 2)))
    const { OrderHistory } = await import('./OrderHistory')
    wrap(<OrderHistory userId="u1" />)
    expect(await screen.findByRole('heading', { name: 'Pizza Nonna' })).toBeInTheDocument()
    const statsList = screen.getByLabelText('Mes statistiques')
    expect(within(statsList).getByText('Commandes passées')).toBeInTheDocument()
    expect(within(statsList).getByText('4')).toBeInTheDocument()
    expect(within(statsList).getByText(/52,10/)).toBeInTheDocument()
    expect(within(statsList).getByText('🍕 Pizza Nonna')).toBeInTheDocument()
    expect(within(statsList).getByText('5× commandé')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Voir plus de commandes' }))
    expect(await screen.findByRole('heading', { name: 'Sushi Go' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Voir plus de commandes' })).not.toBeInTheDocument()
    expect(history).toHaveBeenCalledWith(2)
  })

  it('état vide illustré', async () => {
    history.mockImplementation(async () => page([]))
    const { OrderHistory } = await import('./OrderHistory')
    wrap(<OrderHistory userId="u1" />)
    expect(await screen.findByText('Pas encore de commande')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Lancer une commande/ })).toBeInTheDocument()
  })

  it('aucune commande passée : pas de tuiles de statistiques', async () => {
    statsData = { orders: 0, totalSpent: 0, favoriteRestaurant: null, favoriteDish: null }
    history.mockImplementation(async () => page([{ ...base, status: 'ordering', payment: null }]))
    const { OrderHistory } = await import('./OrderHistory')
    wrap(<OrderHistory userId="u1" />)
    expect(await screen.findByRole('heading', { name: 'Pizza Nonna' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Mes statistiques')).not.toBeInTheDocument()
    statsData = stats
  })

  it('erreur : message + réessayer', async () => {
    fail = true
    const { OrderHistory } = await import('./OrderHistory')
    wrap(<OrderHistory userId="u1" />)
    expect(await screen.findByText('Historique indisponible')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Réessayer' })).toBeInTheDocument()
    fail = false
  })
})
