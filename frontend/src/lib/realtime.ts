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
