import { useQuery } from '@tanstack/react-query'
import { occ, restaurantsApi } from '@/lib/api'
import { useGeo } from '@/lib/geo-context'
import { qk } from '@/lib/queryKeys'
import type { MenuCategory, MenuItem } from '@/lib/types'

export function useNearby(opts: { q?: string; radiusKm?: number } = {}) {
  const { coords } = useGeo()
  const radiusKm = opts.radiusKm ?? 5
  const q = opts.q?.trim() ?? ''
  return useQuery({
    queryKey: qk.nearby(coords?.lat ?? 0, coords?.lng ?? 0, radiusKm, q),
    queryFn: () => occ.nearby({ lat: coords!.lat, lng: coords!.lng, radiusKm, q }),
    enabled: !!coords,
    staleTime: 60_000,
    placeholderData: (prev) => prev,
  })
}

export function useRestaurant(id: string | undefined) {
  return useQuery({
    queryKey: qk.restaurant(id ?? ''),
    queryFn: () => restaurantsApi.get(id!),
    enabled: !!id,
    staleTime: 5 * 60_000,
  })
}

export interface MenuSection {
  category: MenuCategory | null
  items: MenuItem[]
}

export interface MenuData {
  categories: MenuCategory[]
  items: MenuItem[]
  sections: MenuSection[]
}

export function buildSections(categories: MenuCategory[], items: MenuItem[]): MenuSection[] {
  const byCat = new Map<string, MenuItem[]>()
  for (const it of items) {
    const list = byCat.get(it.category) ?? []
    list.push(it)
    byCat.set(it.category, list)
  }
  const sections: MenuSection[] = []
  const popular = items.filter((i) => i.popular && i.available !== false)
  if (popular.length >= 2) sections.push({ category: { id: '__popular', name: 'Populaires ⭐', restaurant: '', position: -1 }, items: popular.slice(0, 6) })
  for (const c of categories) {
    const list = byCat.get(c.id)
    if (list?.length) sections.push({ category: c, items: list })
  }
  const orphans = items.filter((i) => !i.category || !categories.some((c) => c.id === i.category))
  if (orphans.length) sections.push({ category: { id: '__other', name: 'Autres', restaurant: '', position: 999 }, items: orphans })
  return sections
}

export function useMenu(restaurantId: string | undefined) {
  return useQuery({
    queryKey: qk.menu(restaurantId ?? ''),
    queryFn: async (): Promise<MenuData> => {
      const [categories, items] = await Promise.all([restaurantsApi.categories(restaurantId!), restaurantsApi.items(restaurantId!)])
      return { categories, items, sections: buildSections(categories, items) }
    },
    enabled: !!restaurantId,
    staleTime: 5 * 60_000,
  })
}
