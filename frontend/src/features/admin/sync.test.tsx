import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { adminApi } from '@/lib/api'
import type { SyncRun, SyncSource, SyncStatus } from '@/lib/types'
import { SyncPage } from './SyncPage'
import { formatDuration, parseLastStatus, runDuration, statsSummary } from './syncFormat'

const zero = { restaurants_created: 0, restaurants_updated: 0, restaurants_stale: 0, items_created: 0, items_updated: 0, items_price_changed: 0, items_unavailable: 0 }

describe('syncFormat', () => {
  it('lit le statut d’une source', () => {
    expect(parseLastStatus('ok: 40 restaurants')).toEqual({ status: 'ok', message: '40 restaurants' })
    expect(parseLastStatus('blocked: accès refusé par https://x (HTTP 403)')).toEqual({ status: 'blocked', message: 'accès refusé par https://x (HTTP 403)' })
    expect(parseLastStatus('failed: aucun restaurant lu')?.status).toBe('failed')
    expect(parseLastStatus('')).toBeNull()
  })

  it('formate les durées', () => {
    expect(formatDuration(4_000)).toBe('4 s')
    expect(formatDuration(95_000)).toBe('1 min 35 s')
    expect(formatDuration(3_900_000)).toBe('1 h 05')
    expect(runDuration('2026-10-09 03:30:00.000Z', '2026-10-09 03:38:20.000Z')).toBe(500_000)
  })

  it('résume les compteurs sans les zéros', () => {
    expect(statsSummary(zero)).toEqual([])
    expect(statsSummary({ ...zero, restaurants_created: 1, items_created: 2400, items_updated: 5, items_price_changed: 3, items_unavailable: 1 })).toEqual([
      '1 restaurant créé',
      `${(2400).toLocaleString('fr-BE')} plats ajoutés`,
      '3 prix modifiés',
      '1 autre plat modifié',
      '1 plat indisponible',
    ])
  })
})

const run: SyncRun = {
  id: 'r1',
  started_at: '2026-10-09 01:30:00.000Z',
  finished_at: '2026-10-09 01:38:00.000Z',
  status: 'partial',
  trigger: 'cron',
  stats: { ...zero, items_price_changed: 2, items_updated: 2 },
  sources: [],
  changesCount: 2,
  error: '1 source bloquée',
}

describe('SyncPage', () => {
  afterEach(() => vi.restoreAllMocks())

  it('affiche l’état, l’historique et les sources', async () => {
    const status: SyncStatus = { enabled: true, cron: '30 3 * * *', timezone: 'Europe/Brussels', running: null, lastRun: run, nextRunAt: '2026-10-10T01:30:00Z' }
    const source: SyncSource = {
      id: 's1', provider: 'deliveroo', label: 'Deliveroo — Mons', url: 'https://deliveroo.be/fr/restaurants/brussels/mons-center',
      city: 'mons', priority: 50, enabled: true, options: false, last_run_at: '2026-10-09 01:35:00.000Z', last_status: 'blocked: HTTP 403',
    }
    vi.spyOn(adminApi, 'syncStatus').mockResolvedValue(status)
    vi.spyOn(adminApi, 'syncRuns').mockResolvedValue({ page: 1, perPage: 20, totalItems: 1, items: [run] })
    vi.spyOn(adminApi, 'syncSources').mockResolvedValue([source])
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <MemoryRouter>
          <SyncPage />
        </MemoryRouter>
      </QueryClientProvider>,
    )
    expect(await screen.findByText(/Dernière synchronisation/)).toBeInTheDocument()
    expect(screen.getByText('1 source bloquée')).toBeInTheDocument()
    expect(screen.getByText(/Prochaine synchronisation/)).toBeInTheDocument()
    expect(await screen.findByText('Deliveroo — Mons')).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: 'Activer : Deliveroo — Mons' })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getAllByText('Bloquée').length).toBeGreaterThan(0)
    expect(await screen.findByRole('button', { expanded: false, name: /Partielle/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Synchroniser maintenant' })).toBeEnabled()
  })
})
