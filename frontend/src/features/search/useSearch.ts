import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { use } from 'react'
import { occ } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { GeoContext } from '@/lib/geo-context'
import { qk } from '@/lib/queryKeys'

/** Délai de frappe avant d'interroger le serveur. */
export const SEARCH_DEBOUNCE_MS = 150

/**
 * Recherche globale (TanStack Query). La position (si connue) sert aux distances ; les
 * collègues dépendent de l'utilisateur connecté, d'où sa présence dans la clé.
 */
export function useSearch(q: string, { limit = 6, enabled = true }: { limit?: number; enabled?: boolean } = {}) {
  const coords = use(GeoContext)?.coords ?? null
  const { user } = useAuth()
  const lat = coords?.lat ?? null
  const lng = coords?.lng ?? null
  return useQuery({
    queryKey: qk.search(q, lat, lng, user?.id ?? '', limit),
    queryFn: ({ signal }) => occ.search({ q, limit, ...(lat !== null && lng !== null ? { lat, lng } : {}) }, signal),
    enabled,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })
}
