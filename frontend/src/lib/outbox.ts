/**
 * File d'attente hors ligne (« outbox ») : seules des actions sûres et idempotentes y
 * entrent (prêt·e, vote, retrait de vote, ajout au panier avec clé d'idempotence). Elles
 * sont rejouées dans l'ordre au retour du réseau ; le serveur dédoublonne grâce à
 * `client_key` (voir docs/ARCHITECTURE.md, « Mode hors ligne »).
 */
import type { OrderItemInput } from './types'

export type OutboxAction =
  | { kind: 'ready'; partyId: string; ready: boolean }
  | { kind: 'vote'; partyId: string; userId: string; restaurantId: string }
  | { kind: 'unvote'; partyId: string; userId: string; restaurantId: string }
  | { kind: 'addItem'; partyId: string; input: OrderItemInput; label: string }

export interface OutboxEntry {
  /** Clé d'idempotence envoyée au serveur (`client_key`). */
  id: string
  /** Ordre de rejeu. */
  seq: number
  createdAt: number
  action: OutboxAction
}

export interface OutboxStorage {
  all(): Promise<OutboxEntry[]>
  put(entry: OutboxEntry): Promise<void>
  delete(id: string): Promise<void>
}

/** Stockage en mémoire (tests, navigateurs sans IndexedDB). */
export function memoryStorage(): OutboxStorage {
  const map = new Map<string, OutboxEntry>()
  return {
    all: async () => [...map.values()],
    put: async (e) => void map.set(e.id, e),
    delete: async (id) => void map.delete(id),
  }
}

const DB_STORE = 'actions'

/** Stockage IndexedDB (`occ-outbox`), `null` si indisponible (jsdom, navigation privée stricte). */
export function indexedDbStorage(name = 'occ-outbox'): OutboxStorage | null {
  if (typeof indexedDB === 'undefined') return null
  let dbp: Promise<IDBDatabase> | null = null
  const open = () => {
    dbp ??= new Promise<IDBDatabase>((resolve, reject) => {
      const req = indexedDB.open(name, 1)
      req.onupgradeneeded = () => {
        if (!req.result.objectStoreNames.contains(DB_STORE)) req.result.createObjectStore(DB_STORE, { keyPath: 'id' })
      }
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => {
        dbp = null
        reject(req.error ?? new Error('IndexedDB indisponible'))
      }
    })
    return dbp
  }
  const run = async <T>(mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> => {
    const db = await open()
    return new Promise<T>((resolve, reject) => {
      const tx = db.transaction(DB_STORE, mode)
      const req = fn(tx.objectStore(DB_STORE))
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => reject(req.error ?? new Error('IndexedDB : échec'))
    })
  }
  return {
    all: () => run<OutboxEntry[]>('readonly', (s) => s.getAll() as IDBRequest<OutboxEntry[]>),
    put: async (e) => void (await run('readwrite', (s) => s.put(e))),
    delete: async (id) => void (await run('readwrite', (s) => s.delete(id))),
  }
}

/** Clé d'idempotence (`^[A-Za-z0-9_-]{1,64}$`, contrainte serveur). */
export function newClientKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `k${Date.now().toString(36)}${Math.random().toString(36).slice(2, 12)}`
}

/** Libellé français d'une action (toasts de synchronisation). */
export function actionLabel(a: OutboxAction): string {
  switch (a.kind) {
    case 'ready':
      return a.ready ? 'Je suis prêt·e' : 'Panier rouvert'
    case 'vote':
      return 'Vote'
    case 'unvote':
      return 'Retrait de vote'
    case 'addItem':
      return `Ajout : ${a.label}`
  }
}

export interface ReplayResult {
  done: OutboxEntry[]
  failed: { entry: OutboxEntry; error: unknown }[]
  /** Actions restées en file (réseau toujours indisponible). */
  remaining: number
}

export interface Outbox {
  enqueue(action: OutboxAction, id?: string): Promise<OutboxEntry | null>
  list(): Promise<OutboxEntry[]>
  count(): number
  subscribe(cb: (count: number) => void): () => void
  replay(exec: (entry: OutboxEntry) => Promise<void>, isRetryable: (err: unknown) => boolean): Promise<ReplayResult>
}

/** Au plus `max` actions (les plus anciennes sont abandonnées au-delà). */
export function createOutbox(storage: OutboxStorage, { max = 50 }: { max?: number } = {}): Outbox {
  let seq = 0
  let known = 0
  let replaying: Promise<ReplayResult> | null = null
  const listeners = new Set<(n: number) => void>()
  const notify = (n: number) => {
    known = n
    listeners.forEach((l) => l(n))
  }
  const safeAll = async () => {
    try {
      return (await storage.all()).sort((a, b) => a.seq - b.seq)
    } catch {
      return []
    }
  }
  const refresh = async () => notify((await safeAll()).length)
  void refresh()

  return {
    async enqueue(action, id = newClientKey()) {
      try {
        const list = await safeAll()
        // annulations mutuelles : voter puis retirer son vote hors ligne = rien à envoyer
        if (action.kind === 'unvote' || action.kind === 'vote') {
          const opposite = action.kind === 'vote' ? 'unvote' : 'vote'
          const twin = [...list].reverse().find((e) => e.action.kind === opposite && e.action.partyId === action.partyId && 'restaurantId' in e.action && e.action.restaurantId === action.restaurantId)
          if (twin) {
            await storage.delete(twin.id)
            await refresh()
            return null
          }
        }
        // « prêt·e » : seul le dernier état compte
        if (action.kind === 'ready') {
          for (const e of list) if (e.action.kind === 'ready' && e.action.partyId === action.partyId) await storage.delete(e.id)
        }
        const entry: OutboxEntry = { id, seq: Date.now() * 1000 + (seq++ % 1000), createdAt: Date.now(), action }
        await storage.put(entry)
        const after = await safeAll()
        for (const old of after.slice(0, Math.max(0, after.length - max))) await storage.delete(old.id)
        await refresh()
        return entry
      } catch {
        return null
      }
    },
    list: safeAll,
    count: () => known,
    subscribe(cb) {
      listeners.add(cb)
      cb(known)
      return () => void listeners.delete(cb)
    },
    replay(exec, isRetryable) {
      // un seul rejeu à la fois (événements `online` rapprochés)
      replaying ??= (async () => {
        const result: ReplayResult = { done: [], failed: [], remaining: 0 }
        const list = await safeAll()
        for (let i = 0; i < list.length; i++) {
          const entry = list[i]!
          try {
            await exec(entry)
            result.done.push(entry)
            await storage.delete(entry.id).catch(() => undefined)
          } catch (error) {
            if (isRetryable(error)) {
              result.remaining = list.length - i
              break
            }
            result.failed.push({ entry, error })
            await storage.delete(entry.id).catch(() => undefined)
          }
        }
        await refresh()
        return result
      })().finally(() => {
        replaying = null
      })
      return replaying
    },
  }
}
