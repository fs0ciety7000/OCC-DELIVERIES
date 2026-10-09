import type { OrderItem, Restaurant, Vote } from '@/lib/types'

/** Classement : votes ↓, puis note ↓, puis nom (même règle que le serveur). */
export function rankCandidates(candidates: Restaurant[], votes: Vote[]) {
  const counts = new Map<string, number>()
  for (const v of votes) counts.set(v.restaurant, (counts.get(v.restaurant) ?? 0) + 1)
  return [...candidates]
    .map((r) => ({ restaurant: r, count: counts.get(r.id) ?? 0 }))
    .sort((a, b) => b.count - a.count || (b.restaurant.rating ?? 0) - (a.restaurant.rating ?? 0) || a.restaurant.name.localeCompare(b.restaurant.name, 'fr'))
}

export interface CartStats {
  count: number
  total: number
}

/** Nombre d'articles et sous-total (prix serveur) par utilisateur. */
export function cartStatsByUser(items: OrderItem[]): Map<string, CartStats> {
  const out = new Map<string, CartStats>()
  for (const it of items) {
    const s = out.get(it.user) ?? { count: 0, total: 0 }
    s.count += it.quantity
    s.total += it.total
    out.set(it.user, s)
  }
  return out
}
