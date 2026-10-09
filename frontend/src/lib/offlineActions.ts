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

export function voteAction(partyId: string, userId: string, restaurantId: string): Promise<ActionResult> {
  const key = newClientKey()
  return sendOrQueue({ kind: 'vote', partyId, userId, restaurantId }, key, () => partiesApi.vote(partyId, userId, restaurantId, key))
}

/** `voteId` : identifiant connu du vote (absent / « tmp-… » si le vote n'est pas encore envoyé). */
export function unvoteAction(partyId: string, userId: string, restaurantId: string, voteId?: string): Promise<ActionResult> {
  const action: OutboxAction = { kind: 'unvote', partyId, userId, restaurantId }
  const known = voteId && !voteId.startsWith('tmp-') ? voteId : null
  return sendOrQueue(action, newClientKey(), () => (known ? partiesApi.unvote(known) : removeMyVotes(action)))
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

async function removeMyVotes(a: Extract<OutboxAction, { kind: 'unvote' }>) {
  const votes = await partiesApi.votes(a.partyId)
  for (const v of votes) {
    if (v.user !== a.userId || v.restaurant !== a.restaurantId) continue
    try {
      await partiesApi.unvote(v.id)
    } catch (err) {
      if ((err as { status?: number }).status !== 404) throw err
    }
  }
}

/** Exécute une action de la file (rejeu). */
export async function executeEntry(entry: OutboxEntry): Promise<void> {
  const a = entry.action
  switch (a.kind) {
    case 'vote':
      await partiesApi.vote(a.partyId, a.userId, a.restaurantId, entry.id)
      return
    case 'unvote':
      await removeMyVotes(a)
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
