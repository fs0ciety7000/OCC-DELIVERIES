import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { adminApi } from '@/lib/api'
import type { SyncDiscoverResult, SyncRun, SyncSource } from '@/lib/types'
import { DiscoverCard } from './DiscoverCard'

const zero = { restaurants_created: 0, restaurants_updated: 0, restaurants_stale: 0, items_created: 0, items_updated: 0, items_price_changed: 0, items_unavailable: 0 }

const found: SyncDiscoverResult = {
  query: 'https://www.takeaway.com/be-fr/menu/snack-a-la-gare',
  kind: 'takeaway',
  slug: 'snack-a-la-gare',
  tried: [
    { host: 'www.snack-a-la-gare.be', status: 'found', message: 'Snack à la Gare : 196 plats' },
    { host: 'snack-a-la-gare.be', status: 'skipped' },
  ],
  found: [
    {
      url: 'https://www.snack-a-la-gare.be/',
      host: 'www.snack-a-la-gare.be',
      name: 'Snack à la Gare',
      address: '7 Rue Léopold II, 7000 Mons',
      items: 196,
      categories: 13,
      distanceKm: 0.9,
      alreadySource: false,
    },
    {
      url: 'https://www.tomomons.be/',
      host: 'www.tomomons.be',
      name: 'Tomo',
      address: '',
      items: 67,
      categories: 7,
      alreadySource: true,
      sourceId: 's9',
    },
  ],
  network: 4,
  durationMs: 6600,
  interrupted: false,
}

function renderCard() {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <DiscoverCard />
    </QueryClientProvider>,
  )
}

describe('DiscoverCard', () => {
  afterEach(() => vi.restoreAllMocks())

  it('refuse une recherche vide sans appeler le serveur', async () => {
    const spy = vi.spyOn(adminApi, 'discoverSync')
    renderCard()
    await userEvent.click(screen.getByRole('button', { name: 'Découvrir' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(/Colle un lien takeaway.com/)
    expect(spy).not.toHaveBeenCalled()
  })

  it('affiche les sites trouvés, ajoute la source puis la synchronise seule', async () => {
    const discover = vi.spyOn(adminApi, 'discoverSync').mockResolvedValue(found)
    const source = { id: 's1', provider: 'takeaway-site', label: 'Snack à la Gare', url: 'https://www.snack-a-la-gare.be/' } as SyncSource
    const add = vi.spyOn(adminApi, 'addSyncSite').mockResolvedValue({ source, created: true })
    const running: SyncRun = { id: 'r1', started_at: '2026-10-09 10:00:00.000Z', finished_at: '', status: 'running', trigger: 'manual', stats: zero, sources: [], changesCount: 0, error: '' }
    const start = vi.spyOn(adminApi, 'startSync').mockResolvedValue({ run: running })
    vi.spyOn(adminApi, 'syncRun').mockResolvedValue({
      run: { ...running, status: 'success', finished_at: '2026-10-09 10:00:20.000Z', stats: { ...zero, restaurants_created: 1, items_created: 196 } },
    })

    renderCard()
    await userEvent.type(screen.getByLabelText('Lien takeaway.com ou nom du resto'), found.query)
    await userEvent.click(screen.getByRole('button', { name: 'Découvrir' }))
    expect(discover).toHaveBeenCalledWith(found.query)

    const list = await screen.findByRole('list', { name: 'Sites trouvés' })
    const items = within(list).getAllByRole('listitem')
    const snack = items[0]!
    const tomo = items[1]!
    expect(within(snack).getByText('Snack à la Gare')).toBeInTheDocument()
    expect(within(snack).getByText('7 Rue Léopold II, 7000 Mons')).toBeInTheDocument()
    expect(within(snack).getByText(/196 plats · 13 catégories · à 900 m/)).toBeInTheDocument()
    expect(within(snack).getByRole('link', { name: /www\.snack-a-la-gare\.be/ })).toHaveAttribute('href', 'https://www.snack-a-la-gare.be/')
    expect(within(tomo).getByText('Déjà suivi')).toBeInTheDocument()
    expect(within(tomo).queryByRole('button', { name: 'Ajouter et synchroniser' })).not.toBeInTheDocument()

    await userEvent.click(within(snack).getByRole('button', { name: 'Ajouter et synchroniser' }))
    expect(add).toHaveBeenCalledWith('https://www.snack-a-la-gare.be/', 'Snack à la Gare')
    expect(start).toHaveBeenCalledWith('s1')
    expect(await within(snack).findByText('Réussie')).toBeInTheDocument()
    expect(within(snack).getByText(/1 restaurant créé/)).toBeInTheDocument()
    expect(within(snack).queryByRole('button', { name: 'Ajouter et synchroniser' })).not.toBeInTheDocument()
  })

  it('sans résultat : liste les adresses vérifiées et renvoie vers l’ajout manuel', async () => {
    vi.spyOn(adminApi, 'discoverSync').mockResolvedValue({
      ...found,
      query: 'Chez Personne',
      kind: 'name',
      found: [],
      tried: [
        { host: 'www.chez-personne.be', status: 'absent', message: 'nom de domaine inexistant' },
        { host: 'www.personne.be', status: 'other', message: 'site sans le modèle Takeaway' },
        { host: 'personne.be', status: 'skipped' },
      ],
    })
    renderCard()
    await userEvent.type(screen.getByLabelText('Lien takeaway.com ou nom du resto'), 'Chez Personne')
    await userEvent.click(screen.getByRole('button', { name: 'Découvrir' }))
    expect(await screen.findByText('Aucun site Takeaway trouvé.')).toBeInTheDocument()
    expect(screen.getByText(/« Ajouter une source »/)).toBeInTheDocument()
    await userEvent.click(screen.getByText('2 adresses vérifiées'))
    const tried = screen.getByRole('list', { name: 'Adresses vérifiées' })
    expect(within(tried).getByText('www.chez-personne.be')).toBeInTheDocument()
    expect(within(tried).getByText('Autre site')).toBeInTheDocument()
    expect(within(tried).queryByText('personne.be')).not.toBeInTheDocument()
  })
})
