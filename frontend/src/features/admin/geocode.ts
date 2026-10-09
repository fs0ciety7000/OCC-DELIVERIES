export interface GeocodeResult {
  lat: number
  lng: number
  label: string
}

/**
 * Géocodage côté client via Nominatim (OpenStreetMap). Usage ponctuel par un admin,
 * conforme à la politique d'usage (1 requête à la demande, pas d'automatisation).
 */
export async function geocode(address: string, signal?: AbortSignal): Promise<GeocodeResult | null> {
  const q = address.trim()
  if (!q) return null
  const url = `https://nominatim.openstreetmap.org/search?format=json&limit=1&accept-language=fr&q=${encodeURIComponent(q)}`
  const res = await fetch(url, { signal, headers: { Accept: 'application/json' } })
  if (!res.ok) throw new Error('Le service de géocodage ne répond pas.')
  const list = (await res.json()) as { lat: string; lon: string; display_name: string }[]
  const first = list[0]
  if (!first) return null
  return { lat: Math.round(Number(first.lat) * 1e6) / 1e6, lng: Math.round(Number(first.lon) * 1e6) / 1e6, label: first.display_name }
}

/** « Chez Mario & Fils » → « chez-mario-fils ». */
export function slugify(name: string): string {
  return name
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/&/g, ' ')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 120)
}
