import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { pb } from '@/lib/pb'
import type { Party, PartyStatus, Restaurant, Summary } from '@/lib/types'

const me = { id: 'u1', name: 'Alice', color: '#FF6A3D', collectionName: 'users' }
const bob = { id: 'u2', name: 'Bob', color: '#6AA8FF', collectionName: 'users' }
const resto: Restaurant = {
  id: 'r1', name: 'Pizza Nonna', slug: 'nonna', description: '', emoji: '🍕', cuisines: ['pizza'], address: '', lat: 0, lng: 0, phone: '065 00 00 00',
  rating: 4.5, rating_count: 10, price_level: 2, eta_min: 20, eta_max: 30, delivery_fee: 299, min_order: 1500, providers: [{ id: 'ubereats', url: 'https://x' }], active: true,
}
const resto2 = { ...resto, id: 'r2', name: 'Sushi Go', emoji: '🍣' }

let status: PartyStatus = 'lobby'
let partialMenu = false
const party = (): Party => ({
  id: 'p1', code: 'K7M2QX', title: 'Midi du vendredi', host: 'u1', members: ['u1', 'u2'], status, candidates: ['r1', 'r2'], restaurant: 'r1', provider: '',
  delivery_address: '', notes: '', voting_ends_at: '', ordering_ends_at: '', split_mode: 'equal', delivery_fee: 299, service_fee: 0, tip: 0, payer: 'u1',
  dispatch: null, closed_at: '',
  expand: { host: me, members: [me, bob], candidates: [{ ...resto, partial_menu: partialMenu }, resto2], restaurant: { ...resto, partial_menu: partialMenu } },
})
const summary: Summary = {
  partyId: 'p1', currency: 'EUR', status: 'review', restaurant: { id: 'r1', name: 'Pizza Nonna', minOrder: 1500, deliveryFee: 299 },
  participants: [
    { user: me, ready: true, items: [{ id: 'o1', name: 'Margherita', optionsLabel: 'Large', note: '', quantity: 1, unitPrice: 1250, total: 1250 }], subtotal: 1250, sharedFees: 150, total: 1400 },
    { user: bob, ready: false, items: [], subtotal: 0, sharedFees: 0, total: 0 },
  ],
  consolidated: [{ menuItem: 'm1', name: 'Margherita', optionsLabel: 'Large', quantity: 1, total: 1250, notes: [] }],
  itemsSubtotal: 1250, deliveryFee: 299, serviceFee: 0, tip: 0, sharedFees: 299, grandTotal: 1549, minOrderReached: false, allReady: false, splitMode: 'equal',
}

let myBallot: string[] = []
const tally = {
  candidates: 2, voters: 1, winner: 'r2',
  standings: [{ restaurant: 'r2', points: 2, firstChoices: 1, voters: 1 }, { restaurant: 'r1', points: 0, firstChoices: 0, voters: 0 }],
}

vi.mock('@/lib/api', () => ({
  PARTY_EXPAND: '',
  occ: {
    config: vi.fn(async () => ({ currency: 'EUR', defaultLocation: { lat: 50, lng: 3, label: 'Mons' }, providers: [{ id: 'ubereats', name: 'Uber Eats', color: '', enabled: true }] })),
    nearby: vi.fn(async () => [{ ...resto, distanceKm: 0.4 }, { ...resto2, distanceKm: 1.2 }]),
    summary: vi.fn(async () => summary),
    paymentQR: vi.fn(async () => ({ amount: 1400, reference: 'OCC K7M2QX Bob', beneficiary: 'Alice', epc: 'BCD', iban: 'BE71096123456769', links: [], wero: { id: '+32470123456', hasQr: false }, bancontact: null, methods: ['qr', 'wero', 'cash', 'later'] })),
    collectQR: vi.fn(async () => ({ beneficiary: 'Alice', iban: 'BE71096123456769', items: [{ payment: 'pay2', debtor: bob, amount: 149, status: 'declared', method: 'wero', reference: 'OCC K7M2QX Bob', epc: 'BCD', links: [] }] })),
  },
  restaurantsApi: {
    categories: vi.fn(async () => [{ id: 'c1', restaurant: 'r1', name: 'Pizzas', position: 1 }]),
    items: vi.fn(async () => [{ id: 'm1', restaurant: 'r1', category: 'c1', name: 'Margherita', description: '', price: 1250, emoji: '🍕', tags: ['veggie'], option_groups: [], popular: true, available: true, position: 1 }]),
  },
  partiesApi: {
    get: vi.fn(async () => party()),
    members: vi.fn(async () => [
      { id: 'pm1', party: 'p1', user: 'u1', role: 'host', ready: true, expand: { user: me } },
      { id: 'pm2', party: 'p1', user: 'u2', role: 'member', ready: false, expand: { user: bob } },
    ]),
    votes: vi.fn(async () => [
      { id: 'v1', party: 'p1', user: 'u2', restaurant: 'r2', rank: 1 },
      ...myBallot.map((restaurant, i) => ({ id: `m${i}`, party: 'p1', user: 'u1', restaurant, rank: i + 1 })),
    ]),
    tally: vi.fn(async () => tally),
    ballot: vi.fn(async (_p: string, ranking: string[]) => {
      myBallot = ranking
      return { ballot: ranking, tally }
    }),
    orderItems: vi.fn(async () => [{ id: 'o1', party: 'p1', user: 'u1', menu_item: 'm1', quantity: 1, selected_options: [], note: '', name: 'Margherita', options_label: '', unit_price: 1250, total: 1250 }]),
    payments: vi.fn(async () => [
      { id: 'pay1', party: 'p1', debtor: 'u1', creditor: 'u1', amount: 1400, method: 'self', status: 'confirmed', reference: '', declared_at: '', confirmed_at: '' },
      { id: 'pay2', party: 'p1', debtor: 'u2', creditor: 'u1', amount: 149, method: 'wero', status: 'declared', reference: 'OCC K7M2QX Bob', declared_at: '', confirmed_at: '', expand: { debtor: bob } },
    ]),
  },
  usersApi: {},
  payoutApi: { mine: vi.fn(async () => null) },
}))

beforeAll(() => {
  // Jeton factice non expiré (exp lointain) pour que authStore soit valide.
  const payload = btoa(JSON.stringify({ exp: 4102444800, id: 'u1', type: 'auth' })).replace(/=+$/, '')
  pb.authStore.save(`h.${payload}.s`, me as never)
  vi.spyOn(pb.collection('parties'), 'subscribe').mockResolvedValue(async () => undefined)
  vi.spyOn(pb.realtime, 'subscribe').mockResolvedValue(async () => undefined)
})
afterAll(() => pb.authStore.clear())

async function renderParty() {
  const { LocationProvider } = await import('@/lib/geo')
  const { PartyPage } = await import('./PartyPage')
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const router = createMemoryRouter([{ path: '/party/:id', element: <PartyPage /> }], { initialEntries: ['/party/p1'] })
  return render(
    <QueryClientProvider client={qc}>
      <LocationProvider>
        <RouterProvider router={router} />
      </LocationProvider>
    </QueryClientProvider>,
  )
}

describe('PartyPage — chaque étape se rend', () => {
  it.each<[PartyStatus, RegExp]>([
    ['lobby', /On commande où/],
    ['voting', /Classe tes restos/],
    ['ordering', /On commande chez Pizza Nonna/],
    ['review', /Qui a pris quoi/],
    ['paying', /Encaisser/],
    ['closed', /Tout est réglé|Commande clôturée/],
    ['cancelled', /Commande annulée/],
  ])('%s', async (s, text) => {
    status = s
    const { unmount } = await renderParty()
    expect(await screen.findByText(text, {}, { timeout: 3000 })).toBeInTheDocument()
    unmount()
  }, 20000)
})

describe('PartyPage — vue du payeur', () => {
  it('signale un profil de remboursement vide', async () => {
    status = 'paying'
    const { unmount } = await renderParty()
    expect(await screen.findByText(/Aucun moyen renseigné/, {}, { timeout: 3000 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Compléter mon profil' })).toHaveAttribute('href', '/profile?onglet=infos')
    unmount()
  }, 20000)
})

describe('PartyPage — restaurant à carte partielle (instantané Uber Eats)', () => {
  it('montre le badge au vote et le bandeau à la commande', async () => {
    partialMenu = true
    status = 'voting'
    let view = await renderParty()
    expect(await screen.findByText('Aperçu du menu', {}, { timeout: 3000 })).toBeInTheDocument()
    view.unmount()

    status = 'ordering'
    view = await renderParty()
    expect(await screen.findByText(/seuls quelques plats sont connus ici/, {}, { timeout: 3000 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /carte complète de Pizza Nonna sur Uber Eats/ })).toHaveAttribute('target', '_blank')
    view.unmount()
    partialMenu = false
  })
})

describe('PartyPage — vote par classement', () => {
  it('ajoute un resto à mon classement, le réordonne et envoie le bulletin complet', async () => {
    status = 'voting'
    const { partiesApi } = await import('@/lib/api')
    const ballot = vi.mocked(partiesApi.ballot)
    ballot.mockClear()
    const { unmount } = await renderParty()
    // classement en direct venu du serveur : Sushi Go en tête avec 2 pts
    expect(await screen.findByRole('meter', { name: 'Sushi Go : 2 pts' }, { timeout: 5000 })).toBeInTheDocument()
    expect(screen.getAllByText('En tête').length).toBeGreaterThan(0)
    expect(screen.getByText(/Ton 1er choix vaut le plus de points/)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Ajouter Pizza Nonna à mon classement' }))
    fireEvent.click(screen.getByRole('button', { name: 'Ajouter Sushi Go à mon classement' }))
    await waitFor(() => expect(ballot).toHaveBeenCalledTimes(2))
    expect(ballot.mock.calls[0]!.slice(0, 2)).toEqual(['p1', ['r1']])
    expect(ballot.mock.calls[1]!.slice(0, 2)).toEqual(['p1', ['r1', 'r2']])
    expect(screen.getByRole('button', { name: 'Retirer Pizza Nonna de mon classement (1er choix)' })).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(screen.getByRole('button', { name: 'Monter Sushi Go (actuellement 2e choix)' }))
    await waitFor(() => expect(ballot).toHaveBeenCalledTimes(3))
    expect(ballot.mock.calls[2]!.slice(0, 2)).toEqual(['p1', ['r2', 'r1']])
    expect(screen.getByText('Sushi Go est maintenant ton 1er choix.')).toBeInTheDocument()
    unmount()
  }, 20000)
})
