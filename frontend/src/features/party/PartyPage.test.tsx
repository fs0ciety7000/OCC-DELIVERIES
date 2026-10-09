import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
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

vi.mock('@/lib/api', () => ({
  PARTY_EXPAND: '',
  occ: {
    config: vi.fn(async () => ({ currency: 'EUR', defaultLocation: { lat: 50, lng: 3, label: 'Mons' }, providers: [{ id: 'ubereats', name: 'Uber Eats', color: '', enabled: true }] })),
    nearby: vi.fn(async () => [{ ...resto, distanceKm: 0.4 }, { ...resto2, distanceKm: 1.2 }]),
    summary: vi.fn(async () => summary),
    paymentQR: vi.fn(async () => ({ amount: 1400, reference: 'OCC K7M2QX Bob', beneficiary: 'Alice', epc: 'BCD', iban: 'BE71096123456769', links: [], wero: { id: '+32470123456', hasQr: false }, bancontact: null, methods: ['qr', 'wero', 'cash', 'later'] })),
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
    votes: vi.fn(async () => [{ id: 'v1', party: 'p1', user: 'u2', restaurant: 'r2' }]),
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
    ['voting', /Vote pour tes restos/],
    ['ordering', /On commande chez Pizza Nonna/],
    ['review', /Qui a pris quoi/],
    ['paying', /Les parts/],
    ['closed', /Tout est réglé|Commande clôturée/],
    ['cancelled', /Commande annulée/],
  ])('%s', async (s, text) => {
    status = s
    const { unmount } = await renderParty()
    expect(await screen.findByText(text, {}, { timeout: 3000 })).toBeInTheDocument()
    unmount()
  })
})

describe('PartyPage — vue du payeur', () => {
  it('signale un profil de remboursement vide', async () => {
    status = 'paying'
    const { unmount } = await renderParty()
    expect(await screen.findByText(/Aucun moyen renseigné/, {}, { timeout: 3000 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Compléter mon profil' })).toHaveAttribute('href', '/profile?onglet=infos')
    unmount()
  })
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
