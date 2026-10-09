import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { UnsubscribeFunc } from 'pocketbase'
import { useEffect, useRef } from 'react'
import { teamsApi } from '@/lib/api'
import { pb } from '@/lib/pb'
import type { TeamRecord } from '@/lib/types'
import { teamKeys } from './keys'

export function useMyTeams(userId: string | undefined) {
  return useQuery({
    queryKey: teamKeys.mine(userId ?? ''),
    queryFn: teamsApi.mine,
    enabled: !!userId,
    staleTime: 30_000,
  })
}

export function useTeam(id: string | undefined) {
  return useQuery({ queryKey: teamKeys.team(id ?? ''), queryFn: () => teamsApi.get(id!), enabled: !!id })
}

/**
 * Abonnement realtime aux équipes dont je suis membre (rule `members.id ?=`) :
 * quand un·e collègue lance la commande du jour (`last_party` change), `onLaunch`
 * est appelé (toast « Rejoindre ») et les vues d'équipe sont invalidées.
 */
export function useMyTeamsRealtime(userId: string | undefined, onLaunch?: (team: TeamRecord) => void) {
  const qc = useQueryClient()
  const cbRef = useRef(onLaunch)
  useEffect(() => {
    cbRef.current = onLaunch
  }, [onLaunch])

  useEffect(() => {
    if (!userId) return
    let cancelled = false
    let unsub: UnsubscribeFunc | null = null
    const lastParty = new Map<string, string>()
    pb.collection('teams')
      .subscribe<TeamRecord>('*', (e) => {
        void qc.invalidateQueries({ queryKey: teamKeys.all })
        const before = lastParty.get(e.record.id)
        lastParty.set(e.record.id, e.record.last_party)
        if (e.action !== 'update' || !e.record.last_party || e.record.last_party === before) return
        // Événement déclenché par une nouvelle commande d'équipe (et pas par un réglage).
        const launchedAt = Date.parse(e.record.last_launch_at?.replace(' ', 'T') ?? '')
        if (Number.isFinite(launchedAt) && Date.now() - launchedAt > 2 * 60_000) return
        cbRef.current?.(e.record)
      })
      .then((u) => {
        if (cancelled) void u().catch(() => undefined)
        else unsub = u
      })
      .catch(() => undefined)
    return () => {
      cancelled = true
      if (unsub) void unsub().catch(() => undefined)
    }
  }, [userId, qc])
}
