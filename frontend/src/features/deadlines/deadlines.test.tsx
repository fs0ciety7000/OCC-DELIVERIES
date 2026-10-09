import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { partiesApi } from '@/lib/api'
import type { Party } from '@/lib/types'
import { PartyDeadlines } from './PartyDeadlines'

const party = (over: Partial<Party> = {}): Party =>
  ({
    id: 'p1', code: 'K7M2QX', title: 'Midi', host: 'u1', members: ['u1', 'u2'], status: 'voting', candidates: [], restaurant: '',
    provider: '', delivery_address: '', notes: '', voting_ends_at: '', ordering_ends_at: '', split_mode: 'equal',
    delivery_fee: 0, service_fee: 0, tip: 0, payer: '', dispatch: null, closed_at: '', auto_close_disabled: false, auto_events: [],
    ...over,
  }) as Party

function renderIt(p: Party, isHost: boolean) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <PartyDeadlines party={p} isHost={isHost} />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe('PartyDeadlines', () => {
  it('membre : heure limite à l’heure de Bruxelles et mode de clôture, sans réglages', () => {
    renderIt(party({ voting_ends_at: '2026-10-09 09:45:00.000Z' }), false)
    expect(screen.getByText(/Fin du vote à/)).toHaveTextContent('Fin du vote à 11:45')
    expect(screen.getByText('Clôture automatique')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '+5 min' })).not.toBeInTheDocument()
  })

  it('membre sans heure limite : rien', () => {
    const { container } = renderIt(party({ status: 'ordering' }), false)
    expect(container).toBeEmptyDOMElement()
  })

  it('hôte : raccourcis, heure précise et interrupteur de clôture automatique', async () => {
    const update = vi.spyOn(partiesApi, 'update').mockImplementation(async (_id, data) => party({ ...(data as Partial<Party>) }))
    renderIt(party({ status: 'ordering', ordering_ends_at: '2026-10-09 10:10:00.000Z' }), true)
    expect(screen.getByText(/Fin de la commande à/)).toHaveTextContent('12:10')
    // Réglages repliés hors desktop.
    expect(screen.queryByRole('button', { name: '+10 min' })).not.toBeInTheDocument()
    const toggle = screen.getByRole('button', { name: 'Modifier' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await userEvent.click(toggle)
    expect(screen.getByRole('button', { name: 'Masquer' })).toHaveAttribute('aria-expanded', 'true')
    await userEvent.click(screen.getByRole('button', { name: '+10 min' }))
    expect(update).toHaveBeenLastCalledWith('p1', { ordering_ends_at: expect.stringMatching(/:00\.000Z$/) })
    await userEvent.click(screen.getByRole('switch', { name: 'Clôturer automatiquement à l’heure limite' }))
    expect(update).toHaveBeenLastCalledWith('p1', { auto_close_disabled: true })
    await userEvent.click(screen.getByRole('button', { name: 'Retirer' }))
    expect(update).toHaveBeenLastCalledWith('p1', { ordering_ends_at: '' })
  })

  it('journal des actions automatiques, plus récent d’abord', async () => {
    renderIt(
      party({
        status: 'review',
        auto_events: [
          { kind: 'reminder_vote', at: '2026-10-09T09:43:00Z', text: 'Rappel envoyé : fin du vote à 11:45' },
          { kind: 'vote_closed', at: '2026-10-09T09:45:00Z', text: 'Vote clôturé automatiquement à 11:45 — Pizza Nonna' },
          { kind: 'reminder_order', at: '2026-10-09T10:08:00Z', text: 'Rappel envoyé : fin de la commande à 12:10' },
          { kind: 'ordering_closed', at: '2026-10-09T10:10:00Z', text: 'Commande clôturée automatiquement à 12:10' },
        ],
      }),
      false,
    )
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)
    expect(items[0]).toHaveTextContent('Commande clôturée automatiquement à 12:10')
    await userEvent.click(screen.getByRole('button', { name: 'Tout afficher (4)' }))
    expect(screen.getAllByRole('listitem')).toHaveLength(4)
  })
})
