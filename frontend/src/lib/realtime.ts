import { useQueryClient } from '@tanstack/react-query'
import type { RecordSubscription, UnsubscribeFunc } from 'pocketbase'
import { useEffect, useRef } from 'react'
import { toast } from 'sonner'
import { pb } from './pb'
import { pulse, ringBell } from './pulse'
import { qk } from './queryKeys'
import type { Party, PartyMember, Payment, Vote, OrderItem } from './types'

interface Options {
  /** Utilisateur courant (évite de se notifier soi-même). */
  meId?: string
  /** Appelé quand la party elle-même change (statut…). */
  onPartyChange?: (party: Party, action: string) => void
}

/**
 * Abonnements realtime d'une party : parties / party_members / votes / order_items / payments
 * filtrés par party → invalidation des requêtes TanStack correspondantes + pulse des avatars.
 */
export function usePartyRealtime(partyId: string | undefined, { meId, onPartyChange }: Options = {}) {
  const qc = useQueryClient()
  const cbRef = useRef(onPartyChange)
  useEffect(() => {
    cbRef.current = onPartyChange
  }, [onPartyChange])

  useEffect(() => {
    if (!partyId) return
    let cancelled = false
    const unsubs: UnsubscribeFunc[] = []
    const filter = pb.filter('party = {:p}', { p: partyId })
    // État « prêt » connu par membre, pour ne sonner la cloche qu'au passage false → true.
    const readyBefore = new Map<string, boolean>()
    for (const m of qc.getQueryData<PartyMember[]>(qk.members(partyId)) ?? []) readyBefore.set(m.id, m.ready)
    const inv = (...keys: (readonly unknown[])[]) => keys.forEach((queryKey) => void qc.invalidateQueries({ queryKey }))

    const add = async (p: Promise<UnsubscribeFunc>) => {
      try {
        const u = await p
        if (cancelled) void u().catch(() => undefined)
        else unsubs.push(u)
      } catch {
        /* realtime indisponible : les requêtes restent fonctionnelles via refetch */
      }
    }

    void add(
      pb.collection('parties').subscribe<Party>(partyId, (e: RecordSubscription<Party>) => {
        inv(qk.partyDetail(partyId), qk.summary(partyId))
        if (meId) inv(qk.myParties(meId))
        cbRef.current?.(e.record, e.action)
      }),
    )
    void add(
      pb.collection('party_members').subscribe<PartyMember>(
        '*',
        (e) => {
          inv(qk.members(partyId), qk.summary(partyId), qk.partyDetail(partyId))
          pulse(e.record.user)
          if (e.action === 'update' && e.record.ready && !readyBefore.get(e.record.id)) ringBell(e.record.user)
          readyBefore.set(e.record.id, !!e.record.ready)
          if (e.action === 'create' && e.record.user !== meId) {
            const name = e.record.expand?.user?.name
            toast(`${name || 'Quelqu’un'} a rejoint la commande 👋`)
          }
        },
        { filter, expand: 'user' },
      ),
    )
    void add(
      pb.collection('votes').subscribe<Vote>(
        '*',
        (e) => {
          inv(qk.votes(partyId))
          pulse(e.record.user)
        },
        { filter },
      ),
    )
    void add(
      pb.collection('order_items').subscribe<OrderItem>(
        '*',
        (e) => {
          inv(qk.items(partyId), qk.summary(partyId), qk.members(partyId))
          pulse(e.record.user)
        },
        { filter },
      ),
    )
    void add(
      pb.collection('payments').subscribe<Payment>(
        '*',
        (e) => {
          inv(qk.payments(partyId), qk.partyDetail(partyId))
          pulse(e.record.debtor)
        },
        { filter },
      ),
    )
    // Après une reconnexion SSE, on resynchronise tout (évènements potentiellement manqués).
    let first = true
    void add(
      pb.realtime.subscribe('PB_CONNECT', () => {
        if (first) {
          first = false
          return
        }
        inv(qk.party(partyId))
      }),
    )

    return () => {
      cancelled = true
      unsubs.forEach((u) => void u().catch(() => undefined))
    }
  }, [partyId, meId, qc])
}

/**
 * Abonnement global (AppShell) aux parties dont je suis membre : les rules PocketBase
 * filtrent les évènements SSE (`members.id ?= @request.auth.id`). Invalide « mes commandes
 * en cours » / l'historique et signale chaque changement de statut via `onStatusChange`.
 */
export function useMyPartiesRealtime(userId: string | undefined, onStatusChange?: (party: Party, previous: Party['status']) => void) {
  const qc = useQueryClient()
  const cbRef = useRef(onStatusChange)
  useEffect(() => {
    cbRef.current = onStatusChange
  }, [onStatusChange])

  useEffect(() => {
    if (!userId) return
    let cancelled = false
    const unsubs: UnsubscribeFunc[] = []
    // Statut connu par party : évènements reçus, sinon dernière liste « en cours » en cache.
    const known = new Map<string, Party['status']>()
    const cachedStatus = (id: string) => qc.getQueryData<Party[]>(qk.myParties(userId))?.find((p) => p.id === id)?.status
    const refresh = () => {
      void qc.invalidateQueries({ queryKey: qk.myParties(userId) })
      void qc.invalidateQueries({ queryKey: qk.history(userId) })
    }
    const add = async (p: Promise<UnsubscribeFunc>) => {
      try {
        const u = await p
        if (cancelled) void u().catch(() => undefined)
        else unsubs.push(u)
      } catch {
        /* realtime indisponible : refetch au focus */
      }
    }
    void add(
      pb.collection('parties').subscribe<Party>('*', (e) => {
        const before = known.get(e.record.id) ?? cachedStatus(e.record.id)
        refresh()
        if (e.action === 'delete') {
          known.delete(e.record.id)
          return
        }
        known.set(e.record.id, e.record.status)
        if (e.action === 'update' && before && before !== e.record.status) cbRef.current?.(e.record, before)
      }),
    )
    let first = true
    void add(
      pb.realtime.subscribe('PB_CONNECT', () => {
        if (first) {
          first = false
          return
        }
        refresh()
      }),
    )
    return () => {
      cancelled = true
      unsubs.forEach((u) => void u().catch(() => undefined))
    }
  }, [userId, qc])
}
