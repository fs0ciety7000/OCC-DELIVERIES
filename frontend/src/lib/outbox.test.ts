import { ClientResponseError } from 'pocketbase'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { partiesApi, occ } from './api'
import { createOutbox, memoryStorage, newClientKey, actionLabel, type OutboxEntry } from './outbox'
import { addItemAction, executeEntry, outbox, readyAction, replayOutbox, voteAction } from './offlineActions'
import { isNetworkError } from './online'

const network = () => new ClientResponseError({ status: 0, response: {} })
const refused = () => new ClientResponseError({ status: 400, response: { message: 'Le vote n’est pas ouvert.' } })

describe('file hors ligne', () => {
  it('rejoue dans l’ordre et s’arrête à la première erreur réseau', async () => {
    const box = createOutbox(memoryStorage())
    await box.enqueue({ kind: 'vote', partyId: 'p', userId: 'u', restaurantId: 'r1' })
    await box.enqueue({ kind: 'ready', partyId: 'p', ready: true })
    await box.enqueue({ kind: 'vote', partyId: 'p', userId: 'u', restaurantId: 'r2' })
    const seen: string[] = []
    let fail = true
    const exec = async (e: OutboxEntry) => {
      const label = e.action.kind === 'vote' ? e.action.restaurantId : e.action.kind
      if (label === 'ready' && fail) throw network()
      seen.push(label)
    }
    const first = await box.replay(exec, isNetworkError)
    expect(seen).toEqual(['r1'])
    expect(first.remaining).toBe(2)
    expect(box.count()).toBe(2)
    fail = false
    const second = await box.replay(exec, isNetworkError)
    expect(seen).toEqual(['r1', 'ready', 'r2'])
    expect(second.done).toHaveLength(2)
    expect(box.count()).toBe(0)
  })

  it('abandonne une action refusée par le serveur et continue', async () => {
    const box = createOutbox(memoryStorage())
    await box.enqueue({ kind: 'vote', partyId: 'p', userId: 'u', restaurantId: 'r1' })
    await box.enqueue({ kind: 'ready', partyId: 'p', ready: true })
    const res = await box.replay(async (e) => {
      if (e.action.kind === 'vote') throw refused()
    }, isNetworkError)
    expect(res.failed).toHaveLength(1)
    expect(res.done).toHaveLength(1)
    expect(await box.list()).toHaveLength(0)
  })

  it('annule un vote puis son retrait, ne garde que le dernier « prêt·e », borne la file', async () => {
    const box = createOutbox(memoryStorage(), { max: 3 })
    await box.enqueue({ kind: 'vote', partyId: 'p', userId: 'u', restaurantId: 'r1' })
    expect(await box.enqueue({ kind: 'unvote', partyId: 'p', userId: 'u', restaurantId: 'r1' })).toBeNull()
    expect(await box.list()).toHaveLength(0)
    await box.enqueue({ kind: 'ready', partyId: 'p', ready: true })
    await box.enqueue({ kind: 'ready', partyId: 'p', ready: false })
    const list = await box.list()
    expect(list).toHaveLength(1)
    expect(list[0]!.action).toEqual({ kind: 'ready', partyId: 'p', ready: false })
    for (const r of ['a', 'b', 'c', 'd']) await box.enqueue({ kind: 'vote', partyId: 'p', userId: 'u', restaurantId: r })
    expect((await box.list()).map((e) => (e.action.kind === 'vote' ? e.action.restaurantId : e.action.kind))).toEqual(['b', 'c', 'd'])
  })

  it('clés compatibles avec le serveur et libellés français', () => {
    expect(newClientKey()).toMatch(/^[A-Za-z0-9_-]{1,64}$/)
    expect(actionLabel({ kind: 'addItem', partyId: 'p', label: 'Margherita', input: {} as never })).toBe('Ajout : Margherita')
    expect(actionLabel({ kind: 'ready', partyId: 'p', ready: true })).toBe('Je suis prêt·e')
  })
})

describe('actions rejouables', () => {
  afterEach(async () => {
    vi.restoreAllMocks()
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: true })
    await outbox.replay(async () => undefined, () => false) // vide la file partagée
  })

  it('hors ligne : mise en file puis rejeu avec la même clé d’idempotence', async () => {
    Object.defineProperty(navigator, 'onLine', { configurable: true, value: false })
    const add = vi.spyOn(partiesApi, 'addItem').mockResolvedValue({} as never)
    const ready = vi.spyOn(occ, 'ready').mockResolvedValue({} as never)
    const input = { party: 'p1', user: 'u1', menu_item: 'm1', quantity: 2, selected_options: [], note: '' }
    expect(await addItemAction(input, 'Margherita')).toBe('queued')
    expect(await readyAction('p1', true)).toBe('queued')
    expect(add).not.toHaveBeenCalled()
    expect(outbox.count()).toBe(2)

    Object.defineProperty(navigator, 'onLine', { configurable: true, value: true })
    const res = await replayOutbox()
    expect(res.done.map((d) => d.action.kind)).toEqual(['addItem', 'ready'])
    const sent = add.mock.calls[0]![0]
    expect(sent.client_key).toBe(res.done[0]!.id)
    expect(sent.quantity).toBe(2)
    expect(ready).toHaveBeenCalledWith('p1', true)
  })

  it('en ligne mais serveur injoignable : mise en file ; refus du serveur : erreur', async () => {
    vi.spyOn(partiesApi, 'vote').mockRejectedValueOnce(network()).mockRejectedValueOnce(refused())
    expect(await voteAction('p1', 'u1', 'r1')).toBe('queued')
    await expect(voteAction('p1', 'u1', 'r2')).rejects.toThrow()
    expect(outbox.count()).toBe(1)
  })

  it('rejeu d’un retrait de vote : supprime mes votes pour ce resto (404 ignoré)', async () => {
    vi.spyOn(partiesApi, 'votes').mockResolvedValue([
      { id: 'v1', party: 'p1', user: 'u1', restaurant: 'r1' },
      { id: 'v2', party: 'p1', user: 'u2', restaurant: 'r1' },
    ])
    const del = vi.spyOn(partiesApi, 'unvote').mockRejectedValueOnce(new ClientResponseError({ status: 404, response: {} }))
    await executeEntry({ id: 'k', seq: 1, createdAt: 0, action: { kind: 'unvote', partyId: 'p1', userId: 'u1', restaurantId: 'r1' } })
    expect(del).toHaveBeenCalledTimes(1)
    expect(del).toHaveBeenCalledWith('v1')
  })
})
