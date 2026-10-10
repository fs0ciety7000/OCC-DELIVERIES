import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { occ, partiesApi, type PartyFeesInput } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { readyAction } from '@/lib/offlineActions'
import { pb } from '@/lib/pb'
import { isPayerNoPayout } from './payout'
import { qk } from '@/lib/queryKeys'
import type { CollectMode, DeclareMethod, DispatchMethod, Party, PartyMember, PartyStatus, PaymentAction } from '@/lib/types'

export function useParty(id: string | undefined) {
  return useQuery({ queryKey: qk.partyDetail(id ?? ''), queryFn: () => partiesApi.get(id!), enabled: !!id })
}

export function useMembers(id: string | undefined) {
  return useQuery({ queryKey: qk.members(id ?? ''), queryFn: () => partiesApi.members(id!), enabled: !!id })
}

export function useVotes(id: string | undefined, enabled = true) {
  return useQuery({ queryKey: qk.votes(id ?? ''), queryFn: () => partiesApi.votes(id!), enabled: !!id && enabled })
}

/** Classement en direct (serveur) ; rechargé avec les votes (clé sous `votes`). */
export function useTally(id: string | undefined, enabled = true) {
  return useQuery({ queryKey: qk.tally(id ?? ''), queryFn: () => partiesApi.tally(id!), enabled: !!id && enabled })
}

export function useOrderItems(id: string | undefined, enabled = true) {
  return useQuery({ queryKey: qk.items(id ?? ''), queryFn: () => partiesApi.orderItems(id!), enabled: !!id && enabled })
}

export function usePayments(id: string | undefined, enabled = true) {
  return useQuery({ queryKey: qk.payments(id ?? ''), queryFn: () => partiesApi.payments(id!), enabled: !!id && enabled })
}

export function useSummary(id: string | undefined, enabled = true) {
  return useQuery({ queryKey: qk.summary(id ?? ''), queryFn: () => occ.summary(id!), enabled: !!id && enabled })
}

export function useMyParties(userId: string | undefined) {
  return useQuery({ queryKey: qk.myParties(userId ?? ''), queryFn: () => partiesApi.mineActive(userId!), enabled: !!userId })
}

/** Met à jour le cache avec la party renvoyée par le serveur (en gardant l'expand connu). */
function useSetParty() {
  const qc = useQueryClient()
  return (party: Party) => {
    qc.setQueryData<Party>(qk.partyDetail(party.id), (prev) => (prev ? { ...prev, ...party, expand: party.expand ?? prev.expand } : party))
    void qc.invalidateQueries({ queryKey: qk.party(party.id) })
  }
}

const onError = (err: unknown) => toast.error(errorMessage(err))

export function useTransition(partyId: string) {
  const setParty = useSetParty()
  return useMutation({
    mutationFn: (v: { to: PartyStatus; restaurant?: string }) => occ.transition(partyId, v),
    onSuccess: (res) => setParty(res.party),
    onError,
  })
}

export function useUpdateParty(partyId: string) {
  const setParty = useSetParty()
  return useMutation({
    mutationFn: (data: Parameters<typeof partiesApi.update>[1]) => partiesApi.update(partyId, data),
    onSuccess: setParty,
    onError,
  })
}

export function useSetCandidates(partyId: string) {
  const qc = useQueryClient()
  const setParty = useSetParty()
  return useMutation({
    mutationFn: (candidates: string[]) => partiesApi.setCandidates(partyId, candidates),
    onMutate: async (candidates) => {
      await qc.cancelQueries({ queryKey: qk.partyDetail(partyId) })
      const prev = qc.getQueryData<Party>(qk.partyDetail(partyId))
      if (prev) qc.setQueryData<Party>(qk.partyDetail(partyId), { ...prev, candidates })
      return { prev }
    },
    onError: (err, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(qk.partyDetail(partyId), ctx.prev)
      onError(err)
    },
    onSuccess: setParty,
  })
}

export function useFees(partyId: string) {
  const setParty = useSetParty()
  return useMutation({
    mutationFn: (fees: PartyFeesInput) => partiesApi.update(partyId, fees),
    onSuccess: (p) => {
      setParty(p)
      toast.success('Frais mis à jour')
    },
    onError,
  })
}

export function useReady(partyId: string) {
  const qc = useQueryClient()
  return useMutation({
    // hors ligne : mis en file et rejoué au retour du réseau (lib/offlineActions)
    mutationFn: (ready: boolean) => readyAction(partyId, ready),
    onSuccess: (res, ready) => {
      if (res === 'queued') {
        const me = pb.authStore.record?.id
        qc.setQueryData<PartyMember[]>(qk.members(partyId), (list) => list?.map((m) => (m.user === me ? { ...m, ready } : m)))
        return
      }
      void qc.invalidateQueries({ queryKey: qk.members(partyId) })
      void qc.invalidateQueries({ queryKey: qk.summary(partyId) })
    },
    onError,
  })
}

export function useDispatch(partyId: string) {
  const setParty = useSetParty()
  return useMutation({
    mutationFn: (method: DispatchMethod) => occ.dispatch(partyId, method),
    onSuccess: (res) => setParty(res.party),
    onError,
  })
}

export function useSetPayer(partyId: string) {
  const setParty = useSetParty()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { payer: string; collectMode: CollectMode }) => occ.setPayer(partyId, v.payer, v.collectMode),
    onSuccess: (res) => {
      qc.setQueryData(qk.payments(partyId), res.payments)
      setParty(res.party)
    },
    onError: (err) => {
      // Course : le profil a changé entre-temps → on relit les moyens de chacun (l'écran se met à jour).
      if (isPayerNoPayout(err)) void qc.invalidateQueries({ queryKey: qk.payoutReadiness(partyId) })
      onError(err)
    },
  })
}

/**
 * Moyens de remboursement des membres (booléens, jamais l'IBAN). Les profils sont
 * privés (pas de temps réel) : relus au focus et régulièrement tant que l'écran est ouvert.
 */
export function usePayoutReadiness(partyId: string, enabled = true) {
  return useQuery({
    queryKey: qk.payoutReadiness(partyId),
    queryFn: () => occ.payoutReadiness(partyId),
    enabled: !!partyId && enabled,
    // relu à chaque ouverture du choix du payeur (petit endpoint, profils sans temps réel)
    staleTime: 0,
    refetchOnWindowFocus: true,
    refetchInterval: enabled ? 30_000 : false,
  })
}

export function usePayoutRequest(partyId: string) {
  return useMutation({
    mutationFn: (userId: string) => occ.payoutRequest(partyId, userId),
    onSuccess: () => toast.success('Demande envoyée', { description: 'Une notification lui demande d’ajouter son IBAN.' }),
    onError,
  })
}

export function usePaymentAction(partyId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (v: { paymentId: string; action: PaymentAction; method?: DeclareMethod }) => occ.paymentAction(v.paymentId, v.action, v.method),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.payments(partyId) })
      void qc.invalidateQueries({ queryKey: qk.partyDetail(partyId) })
    },
    onError,
  })
}

export const STATUS_LABELS: Record<PartyStatus, string> = {
  lobby: 'Salon',
  voting: 'Vote en cours',
  ordering: 'Paniers ouverts',
  review: 'Récap',
  paying: 'Remboursements',
  closed: 'Terminée',
  cancelled: 'Annulée',
}

export function inviteUrl(code: string): string {
  return `${window.location.origin}/j/${code}`
}

/** Alphabet des codes : sans 0/O/1/I. */
export function normalizeCode(input: string): string {
  return input
    .toUpperCase()
    .replace(/[^A-HJ-NP-Z2-9]/g, '')
    .slice(0, 6)
}
