import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { GeoContext, useConfig, type Coords, type LocationCtx } from './geo-context'

const KEY = 'occ-location'

function readStored(): Coords | null {
  try {
    const raw = sessionStorage.getItem(KEY)
    if (!raw) return null
    const v = JSON.parse(raw) as Coords
    return Number.isFinite(v.lat) && Number.isFinite(v.lng) ? v : null
  } catch {
    return null
  }
}

/** Fournit la position : géolocalisation du navigateur si acceptée, sinon `config.defaultLocation`. */
export function LocationProvider({ children }: { children: ReactNode }) {
  const config = useConfig()
  const [device, setDevice] = useState<Coords | null>(readStored)
  const [status, setStatus] = useState<LocationCtx['status']>(device ? 'ok' : 'idle')

  const request = useCallback(() => {
    if (!('geolocation' in navigator)) {
      setStatus('denied')
      return
    }
    setStatus('locating')
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        const c: Coords = { lat: pos.coords.latitude, lng: pos.coords.longitude, label: 'Ma position', source: 'device' }
        setDevice(c)
        setStatus('ok')
        try {
          sessionStorage.setItem(KEY, JSON.stringify(c))
        } catch {
          /* ignore */
        }
      },
      () => setStatus('denied'),
      { enableHighAccuracy: false, timeout: 8000, maximumAge: 5 * 60_000 },
    )
  }, [])

  const reset = useCallback(() => {
    setDevice(null)
    setStatus('idle')
    try {
      sessionStorage.removeItem(KEY)
    } catch {
      /* ignore */
    }
  }, [])

  const fallback = config.data?.defaultLocation
  const coords = useMemo<Coords | null>(() => {
    if (device) return device
    if (fallback) return { ...fallback, source: 'default' }
    // Backend injoignable : on retombe sur Mons (défaut du contrat).
    if (config.isError) return { lat: 50.4542, lng: 3.9567, label: 'Mons', source: 'default' }
    return null
  }, [device, fallback, config.isError])

  const value = useMemo(() => ({ coords, status, request, reset }), [coords, status, request, reset])
  return <GeoContext value={value}>{children}</GeoContext>
}
