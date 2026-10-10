import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ClientResponseError } from 'pocketbase'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PayoutReadiness, Summary } from '@/lib/types'
import type { PartyCtx } from '../context'
import { isPayerNoPayout, payerBlockReason, payoutHints, payoutRequired } from '../payout'
import { PayerSheet } from './PayerSheet'
import { PayoutNudge } from './PayoutNudge'

const summary = vi.fn<() => Promise<Summary>>()
const readiness = vi.fn<() => Promise<PayoutReadiness>>()
const setPayer = vi.fn()
const payoutRequest = vi.fn()
const mine = vi.fn()
const save = vi.fn()

vi.mock('@/lib/api', () => ({
  occ: {
    summary: () => summary(),
    payoutReadiness: () => readiness(),
    setPayer: (...a: unknown[]) => setPayer(...a),
    payoutRequest: (...a: unknown[]) => payoutRequest(...a),
  },
  payoutApi: { mine: (...a: unknown[]) => mine(...a), save: (...a: unknown[]) => save(...a) },
}))
vi.mock('sonner', () => ({ toast: Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() }) }))

const part = (id: string, name: string, total: number) => ({
  user: { id, name },
  ready: true,
  items: [{ id: `i-${id}`, name: 'Tiramisu', optionsLabel: '', note: '', quantity: 1, unitPrice: total, total }],
  subtotal: total,
  sharedFees: 0,
  total,
})

const baseSummary = {
  participants: [part('me', 'Alice', 700), part('bob', 'Bob', 650), part('carol', 'Carol', 600)],
  minOrderReached: true,
} as unknown as Summary

const status = (ready: boolean, iban = false, links: ('revolut' | 'paypal' | 'link')[] = []) => ({ ready, iban, links })
const noneReady: PayoutReadiness = {
  members: [
    { user: 'me', guest: false, payout: status(false) },
    { user: 'bob', guest: false, payout: status(true, true, ['revolut']) },
    { user: 'carol', guest: false, payout: status(false) },
  ],
  readyCount: 1,
  total: 3,
}

const people = new Map([
  ['me', { id: 'me', name: 'Alice' }],
  ['bob', { id: 'bob', name: 'Bob' }],
  ['carol', { id: 'carol', name: 'Carol' }],
])
const ctx = {
  party: { id: 'party1', payer: '', collect_mode: '' },
  me: { id: 'me', name: 'Alice' },
  isHost: true,
  people,
  members: ['me', 'bob', 'carol'].map((u) => ({ id: `m-${u}`, user: u })),
} as unknown as PartyCtx

function renderSheet() {
  const onClose = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <PayerSheet ctx={ctx} variant="validate" open onClose={onClose} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return { onClose }
}

beforeEach(() => {
  vi.clearAllMocks()
  summary.mockResolvedValue(baseSummary)
  readiness.mockResolvedValue(noneReady)
  setPayer.mockResolvedValue({ party: { id: 'party1' }, payments: [] })
  payoutRequest.mockResolvedValue({ sent: true })
  mine.mockResolvedValue(null)
  save.mockResolvedValue({ id: 'pp1', iban: 'BE71096123456769' })
})

describe('payout helpers', () => {
  it('indices lisibles, sans aucune donnée privée', () => {
    expect(payoutHints(status(true, true, ['revolut', 'paypal']))).toEqual(['IBAN ✓', 'Revolut ✓', 'PayPal ✓'])
    expect(payoutHints(status(false))).toEqual([])
    expect(payoutHints(undefined)).toEqual([])
  })

  it('moyen requis seulement si un·e autre membre a commandé', () => {
    expect(payoutRequired('me', baseSummary)).toBe(true)
    expect(payoutRequired('me', { participants: [part('me', 'Alice', 700), { ...part('bob', 'Bob', 0), items: [] }] })).toBe(false)
  })

  it('jamais bloqué en espèces', () => {
    const base = { payer: 'me', meId: 'me', name: 'Alice', status: status(false), required: true }
    expect(payerBlockReason({ ...base, mode: 'cash' })).toBeNull()
    expect(payerBlockReason({ ...base, mode: 'transfer' })).toMatch(/Ajoute ton IBAN/)
    expect(payerBlockReason({ ...base, payer: 'carol', name: 'Carol', mode: 'transfer' })).toMatch(/^Carol n'a encore renseigné/)
  })

  it('reconnaît le code serveur payer_no_payout', () => {
    const err = new ClientResponseError({ status: 409, response: { status: 409, message: 'x', data: { payer: { code: 'payer_no_payout', message: 'x' } } } })
    expect(isPayerNoPayout(err)).toBe(true)
    expect(isPayerNoPayout(new ClientResponseError({ status: 400, response: { data: {} } }))).toBe(false)
  })
})

describe('PayerSheet', () => {
  it('affiche les moyens de chacun et bloque « Valider » pour un payeur sans moyen', async () => {
    renderSheet()
    const group = screen.getByRole('radiogroup', { name: 'Payeur' })
    expect(await within(group).findByText('IBAN ✓ · Revolut ✓')).toBeInTheDocument()
    expect(within(group).getAllByText('Aucun moyen de remboursement')).toHaveLength(2)
    const validate = screen.getByRole('button', { name: 'Valider la commande' })
    await waitFor(() => expect(validate).toBeDisabled())
    expect(validate).toHaveAccessibleDescription(/Ajoute ton IBAN/)
  })

  it('« Pas d’IBAN ? Valider en espèces » valide en mode espèces', async () => {
    const user = userEvent.setup()
    const { onClose } = renderSheet()
    await user.click(await screen.findByRole('button', { name: /Valider en espèces/ }))
    expect(setPayer).toHaveBeenCalledWith('party1', 'me', 'cash')
    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })

  it('choisir « Espèces uniquement » débloque la validation', async () => {
    const user = userEvent.setup()
    renderSheet()
    await screen.findByText('IBAN ✓ · Revolut ✓')
    await user.click(screen.getByRole('radio', { name: /Espèces uniquement/ }))
    const validate = screen.getByRole('button', { name: 'Valider la commande' })
    expect(validate).toBeEnabled()
    await user.click(validate)
    expect(setPayer).toHaveBeenCalledWith('party1', 'me', 'cash')
  })

  it('un payeur avec IBAN est validé par virement', async () => {
    const user = userEvent.setup()
    renderSheet()
    await user.click(await screen.findByRole('radio', { name: /Bob/ }))
    await user.click(screen.getByRole('button', { name: 'Valider la commande' }))
    expect(setPayer).toHaveBeenCalledWith('party1', 'bob', 'transfer')
  })

  it('demande à un·e collègue sans moyen d’ajouter son IBAN', async () => {
    const user = userEvent.setup()
    renderSheet()
    await user.click(await screen.findByRole('radio', { name: /Carol/ }))
    expect(screen.getByText("Carol n'a encore renseigné aucun moyen de remboursement.")).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: "Lui demander d'ajouter son IBAN" }))
    expect(payoutRequest).toHaveBeenCalledWith('party1', 'carol')
    expect(await screen.findByRole('button', { name: 'Demande envoyée' })).toBeDisabled()
  })

  it('ajout rapide de mon IBAN : validation, enregistrement, puis validation possible', async () => {
    const user = userEvent.setup()
    renderSheet()
    await user.click(await screen.findByRole('button', { name: 'Ajouter mon IBAN' }))
    const input = screen.getByLabelText('IBAN')
    await user.type(input, 'be71096123456760')
    expect(await screen.findByText('IBAN invalide (vérifie les chiffres).')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Enregistrer mon IBAN' })).toBeDisabled()

    await user.clear(input)
    await user.type(input, 'be71096123456769')
    expect(input).toHaveValue('BE71 0961 2345 6769')
    readiness.mockResolvedValue({ ...noneReady, members: noneReady.members.map((m) => (m.user === 'me' ? { ...m, payout: status(true, true) } : m)) })
    await user.click(screen.getByRole('button', { name: 'Enregistrer mon IBAN' }))
    expect(save).toHaveBeenCalledWith('me', null, expect.objectContaining({ iban: 'BE71096123456769' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Valider la commande' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: 'Valider la commande' }))
    expect(setPayer).toHaveBeenCalledWith('party1', 'me', 'transfer')
  })
})

describe('PayoutNudge', () => {
  const renderNudge = (c: PartyCtx) =>
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter>
          <PayoutNudge ctx={c} />
        </MemoryRouter>
      </QueryClientProvider>,
    )

  it('invite doucement à ajouter son IBAN, masquable pour la party ; l’hôte voit le compte', async () => {
    localStorage.clear()
    const user = userEvent.setup()
    const { unmount } = renderNudge(ctx)
    expect(await screen.findByText(/Ajoute ton IBAN pour être remboursé·e par virement si tu avances la commande/, undefined, { timeout: 3000 })).toBeInTheDocument()
    expect(screen.getByText('1 membre sur 3 peut être remboursé par virement.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Ajouter mon IBAN' }))
    expect(await screen.findByRole('dialog', undefined, { timeout: 3000 })).toHaveTextContent('Ajoute ton IBAN')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument(), { timeout: 3000 })
    await user.click(screen.getByRole('button', { name: 'Masquer ce conseil pour cette commande' }))
    expect(screen.queryByText(/si tu avances la commande/)).not.toBeInTheDocument()
    unmount()
    renderNudge(ctx)
    await screen.findByText('1 membre sur 3 peut être remboursé par virement.', undefined, { timeout: 3000 })
    expect(screen.queryByText(/si tu avances la commande/)).not.toBeInTheDocument()
  })

  it('rien pour un membre qui a déjà un moyen', async () => {
    localStorage.clear()
    renderNudge({ ...ctx, isHost: false, me: { id: 'bob', name: 'Bob' } } as unknown as PartyCtx)
    await waitFor(() => expect(readiness).toHaveBeenCalled())
    expect(screen.queryByText(/Ajoute ton IBAN/)).not.toBeInTheDocument()
  })
})
