import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { CollectQR, Payment } from '@/lib/types'
import type { PartyCtx } from '../context'
import { collectKinds, qrValueFor, stepIndex } from './collect'
import { CollectPanel } from './CollectPanel'

const collectQR = vi.fn<(id: string) => Promise<CollectQR>>()
const mutate = vi.fn()

vi.mock('@/lib/api', () => ({ occ: { collectQR: (id: string) => collectQR(id) } }))
vi.mock('../hooks', () => ({ usePaymentAction: () => ({ mutate, isPending: false }) }))

const epc = (cents: string, name: string) => `BCD\n002\n1\nSCT\n\nBob Martin\nBE71096123456769\nEUR${cents}\n\n\nOCC K7M2QX ${name}`

const data: CollectQR = {
  beneficiary: 'Bob Martin',
  iban: 'BE71096123456769',
  items: [
    {
      payment: 'p1',
      debtor: { id: 'u1', name: 'Alice', avatar: '', color: '' },
      amount: 1240,
      status: 'pending',
      method: '',
      reference: 'OCC K7M2QX Alice',
      epc: epc('12.40', 'Alice'),
      links: [
        { kind: 'revolut', label: 'Revolut', url: 'https://revolut.me/bobm?amount=1240&currency=EUR&note=OCC+K7M2QX+Alice', amountPrefilled: true },
        { kind: 'link', label: 'lydia-app.com', url: 'https://lydia-app.com/x', amountPrefilled: false },
      ],
    },
    {
      payment: 'p2',
      debtor: { id: 'u2', name: 'Carol', avatar: '', color: '' },
      amount: 980,
      status: 'declared',
      method: 'qr',
      reference: 'OCC K7M2QX Carol',
      epc: epc('9.80', 'Carol'),
      links: [{ kind: 'revolut', label: 'Revolut', url: 'https://revolut.me/bobm?amount=980&currency=EUR&note=OCC+K7M2QX+Carol', amountPrefilled: true }],
    },
  ],
}

const pay = (id: string, debtor: string, amount: number, status: Payment['status'], method: Payment['method'] = ''): Payment => ({
  id,
  collectionId: '',
  collectionName: 'payments',
  created: '',
  updated: '',
  party: 'party1',
  debtor,
  creditor: 'bob',
  amount,
  method,
  status,
  reference: '',
  declared_at: '',
  confirmed_at: '',
})

const people = new Map([
  ['u1', { id: 'u1', name: 'Alice' }],
  ['u2', { id: 'u2', name: 'Carol' }],
])
const ctx = { party: { id: 'party1' }, people, me: { id: 'bob', name: 'Bob' }, isHost: false, members: [] } as unknown as PartyCtx

function renderPanel(payments = [pay('p1', 'u1', 1240, 'pending'), pay('p2', 'u2', 980, 'declared', 'qr')]) {
  return render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <CollectPanel ctx={ctx} payments={payments} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('collect — helpers', () => {
  it('types de QR disponibles : EPC puis liens à montant pré-rempli seulement', () => {
    expect(collectKinds(data)).toEqual(['epc', 'revolut'])
    expect(collectKinds(undefined)).toEqual([])
    expect(collectKinds({ ...data, items: data.items.map((it) => ({ ...it, epc: null, links: [] })) })).toEqual([])
  })

  it('contenu du QR : payload EPC ou lien avec montant', () => {
    expect(qrValueFor(data.items[0], 'epc')).toContain('EUR12.40')
    expect(qrValueFor(data.items[0], 'revolut')).toBe('https://revolut.me/bobm?amount=1240&currency=EUR&note=OCC+K7M2QX+Alice')
    expect(qrValueFor(data.items[0], 'paypal')).toBeNull()
    expect(qrValueFor(undefined, 'epc')).toBeNull()
  })

  it('navigation bornée', () => {
    expect(stepIndex(0, -1, 3)).toBe(0)
    expect(stepIndex(1, 1, 3)).toBe(2)
    expect(stepIndex(2, 1, 3)).toBe(2)
    expect(stepIndex(0, 1, 0)).toBe(0)
  })
})

describe('CollectPanel (vue du payeur)', () => {
  const request = vi.fn()
  const release = vi.fn(async () => {})
  beforeEach(() => {
    collectQR.mockReset().mockResolvedValue(data)
    mutate.mockReset()
    request.mockReset().mockResolvedValue({ release })
    release.mockClear()
    Object.defineProperty(navigator, 'wakeLock', { value: { request }, configurable: true })
  })
  afterEach(() => {
    Reflect.deleteProperty(navigator, 'wakeLock')
  })

  it('une carte par collègue avec montant, statut et QR à présenter', async () => {
    renderPanel()
    const list = screen.getByRole('list', { name: 'Parts à encaisser' })
    const cards = within(list).getAllByRole('listitem')
    expect(cards).toHaveLength(2)
    expect(within(cards[0]!).getByText('Alice')).toBeInTheDocument()
    expect(within(cards[0]!).getByText(/12,40/)).toBeInTheDocument()
    expect(within(cards[0]!).getByText('En attente')).toBeInTheDocument()
    expect(within(cards[1]!).getByText('Déclaré')).toBeInTheDocument()
    expect(await within(cards[0]!).findByRole('button', { name: /Présenter en plein écran le QR de Alice \(12,40\s€\)/ })).toBeInTheDocument()
    expect(collectQR).toHaveBeenCalledWith('party1')
    // EPC par défaut, Revolut proposé (le lien Lydia sans montant ne compte pas)
    const group = screen.getByRole('radiogroup', { name: 'Type de QR' })
    expect(within(group).getAllByRole('radio').map((r) => r.textContent)).toEqual(['Virement', 'Revolut'])
    expect(screen.getByText(/app bancaire/)).toBeInTheDocument()
    await userEvent.click(within(group).getByRole('radio', { name: 'Revolut' }))
    expect(screen.getByText(/appareil photo/)).toBeInTheDocument()
  })

  it('confirmer la réception depuis la carte', async () => {
    renderPanel()
    const card = screen.getAllByRole('listitem')[1]!
    await userEvent.click(within(card).getByRole('button', { name: 'Confirmer la réception' }))
    expect(mutate).toHaveBeenCalledWith({ paymentId: 'p2', action: 'confirm' }, expect.anything())
  })

  it('présenter : plein écran, collègue suivant, Échap ferme, écran maintenu allumé', async () => {
    renderPanel()
    await userEvent.click(await screen.findByRole('button', { name: 'Présenter à tour de rôle' }))
    const dialog = screen.getByRole('dialog', { name: /Encaisser — Alice, 12,40/ })
    expect(within(dialog).getByRole('img', { name: /QR virement SEPA de 12,40\s€ pour Alice/ })).toBeInTheDocument()
    expect(within(dialog).getByText('1 / 2')).toBeInTheDocument()
    expect(request).toHaveBeenCalledWith('screen')

    await userEvent.click(within(dialog).getByRole('button', { name: 'Collègue suivant·e' }))
    const second = await screen.findByRole('dialog', { name: /Carol, 9,80/ })
    expect(within(second).getByText('2 / 2')).toBeInTheDocument()

    await userEvent.click(within(second).getByRole('radio', { name: 'Revolut' }))
    expect(await screen.findByRole('img', { name: /QR du lien Revolut de 9,80\s€ pour Carol/ })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /J'ai reçu 9,80/ }))
    expect(mutate).toHaveBeenCalledWith({ paymentId: 'p2', action: 'confirm' }, expect.anything())

    await userEvent.keyboard('{ArrowLeft}')
    expect(await screen.findByRole('dialog', { name: /Alice/ })).toBeInTheDocument()
    await userEvent.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(release).toHaveBeenCalled()
  })

  it('sans IBAN ni lien à montant : invitation à compléter le profil, confirmation toujours possible', async () => {
    collectQR.mockResolvedValue({ ...data, iban: null, items: data.items.map((it) => ({ ...it, epc: null, links: [] })) })
    renderPanel()
    expect(await screen.findByRole('link', { name: 'Compléter mon profil' })).toHaveAttribute('href', '/profile?onglet=infos')
    expect(screen.queryByRole('button', { name: 'Présenter à tour de rôle' })).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Confirmer la réception' })).toHaveLength(2)
  })

  it('mode espèces : montants et confirmation, ni QR ni invitation à compléter le profil', async () => {
    collectQR.mockResolvedValue({ collectMode: 'cash', beneficiary: 'Bob', iban: null, items: data.items.map((it) => ({ ...it, epc: null, links: [] })) })
    renderPanel()
    expect(await screen.findByText(/Remboursement en espèces/)).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Compléter mon profil' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Présenter/ })).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Confirmer la réception' })).toHaveLength(2)
  })
})
