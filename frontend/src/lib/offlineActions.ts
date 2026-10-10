/**
 * Actions « rejouables » : envoyées tout de suite si le réseau répond, sinon mises dans
 * la file hors ligne (IndexedDB) et rejouées au retour de la connexion (`replayOutbox`).
 * Chaque action porte une clé d'idempotence : un rejeu ne crée jamais de doublon.
 */
import { useEffect, useState } from 'react'
import { occ, partiesApi } from './api'
import { isNetworkError, isOnline } from './online'
import { createOutbox, indexedDbStorage, memoryStorage, newClientKey, type OutboxAction, type OutboxEntry } from './outbox'
import type { OrderItemInput } from './types'

export const outbox = createOutbox(indexedDbStorage() ?? memoryStorage())

export type ActionResult = 'sent' | 'queued'

async function sendOrQueue(action: OutboxAction, key: string, send: () => Promise<unknown>): Promise<ActionResult> {
  if (!isOnline()) {
    await outbox.enqueue(action, key)
    return 'queued'
  }
  try {
    await send()
    return 'sent'
  } catch (err) {
    if (!isNetworkError(err)) throw err
    await outbox.enqueue(action, key)
    return 'queued'
  }
}

/** Remplace mon bulletin (1er choix d'abord, `[]` = retirer mon vote) ; hors ligne : seul le dernier est gardé. */
export function ballotAction(partyId: string, ranking: string[]): Promise<ActionResult> {
  const key = newClientKey()
  return sendOrQueue({ kind: 'ballot', partyId, ranking }, key, () => partiesApi.ballot(partyId, ranking, key))
}

export function readyAction(partyId: string, ready: boolean): Promise<ActionResult> {
  return sendOrQueue({ kind: 'ready', partyId, ready }, newClientKey(), () => occ.ready(partyId, ready))
}

/** Ajout au panier ; `label` = nom du plat (toasts de synchronisation). */
export function addItemAction(input: OrderItemInput, label: string): Promise<ActionResult> {
  const key = input.client_key || newClientKey()
  const withKey = { ...input, client_key: key }
  return sendOrQueue({ kind: 'addItem', partyId: input.party, input: withKey, label }, key, () => partiesApi.addItem(withKey))
}

/** Ancienne entrée « vote / retrait » (avant le vote par classement) : relit mon bulletin et le réécrit. */
async function replayLegacyVote(a: Extract<OutboxAction, { kind: 'vote' | 'unvote' }>, key: string) {
  const mine = (await partiesApi.votes(a.partyId))
    .filter((v) => v.user === a.userId)
    .sort((x, y) => x.rank - y.rank)
    .map((v) => v.restaurant)
  const has = mine.includes(a.restaurantId)
  if (a.kind === 'vote' ? has : !has) return
  const ranking = a.kind === 'vote' ? [...mine, a.restaurantId] : mine.filter((r) => r !== a.restaurantId)
  await partiesApi.ballot(a.partyId, ranking, key)
}

/** Exécute une action de la file (rejeu). */
export async function executeEntry(entry: OutboxEntry): Promise<void> {
  const a = entry.action
  switch (a.kind) {
    case 'ballot':
      await partiesApi.ballot(a.partyId, a.ranking, entry.id)
      return
    case 'vote':
    case 'unvote':
      await replayLegacyVote(a, entry.id)
      return
    case 'ready':
      await occ.ready(a.partyId, a.ready)
      return
    case 'addItem':
      await partiesApi.addItem({ ...a.input, client_key: a.input.client_key || entry.id })
      return
  }
}

/** Rejoue la file dans l'ordre (arrêt à la première erreur réseau). */
export function replayOutbox() {
  return outbox.replay(executeEntry, isNetworkError)
}

/** Nombre d'actions en attente dans la file hors ligne (réactif). */
export function useOutboxCount(): number {
  const [n, setN] = useState(() => outbox.count())
  useEffect(() => outbox.subscribe(setN), [])
  return n
}
