import { useQuery } from '@tanstack/react-query'
import { createContext, use } from 'react'
import { occ } from './api'
import { qk } from './queryKeys'

export interface Coords {
  lat: number
  lng: number
  label: string
  source: 'default' | 'device'
}

export interface LocationCtx {
  coords: Coords | null
  status: 'idle' | 'locating' | 'denied' | 'ok'
  request: () => void
  reset: () => void
}

export const GeoContext = createContext<LocationCtx | null>(null)

export function useConfig() {
  return useQuery({ queryKey: qk.config, queryFn: occ.config, staleTime: 60 * 60_000 })
}

/** Position courante (navigateur si acceptée, sinon `config.defaultLocation`). */
export function useGeo(): LocationCtx {
  const v = use(GeoContext)
  if (!v) throw new Error('LocationProvider manquant')
  return v
}
