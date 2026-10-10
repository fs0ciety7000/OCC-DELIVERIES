import type { OrderItem, Restaurant, Tally, TallyStanding, Vote } from '@/lib/types'

/** Points d'un resto classé au rang `rank` parmi `candidates` (même règle que le serveur, ADR 0005). */
export function rankPoints(candidates: number, rank: number): number {
  if (rank < 1) return 0
  return Math.max(1, candidates - rank + 1)
}

/** Mon bulletin (1er choix d'abord) d'après les votes de la party. */
export function myRanking(votes: Vote[], userId: string): string[] {
  return votes
    .filter((v) => v.user === userId)
    .sort((a, b) => a.rank - b.rank)
    .map((v) => v.restaurant)
}

/** Ajoute le resto en fin de classement, ou le retire s'il y est déjà. */
export function toggleRanked(ranking: string[], id: string): string[] {
  return ranking.includes(id) ? ranking.filter((r) => r !== id) : [...ranking, id]
}

/** Déplace le resto de `delta` places (−1 = monter), borné. */
export function moveRanked(ranking: string[], id: string, delta: number): string[] {
  const from = ranking.indexOf(id)
  if (from < 0) return ranking
  const to = Math.min(ranking.length - 1, Math.max(0, from + delta))
  if (to === from) return ranking
  const next = [...ranking]
  next.splice(from, 1)
  next.splice(to, 0, id)
  return next
}

/** Votes optimistes : mon bulletin remplacé par `ranking` (les autres membres inchangés). */
export function withMyRanking(votes: Vote[], partyId: string, userId: string, ranking: string[]): Vote[] {
  return [
    ...votes.filter((v) => v.user !== userId),
    ...ranking.map((restaurant, i) => ({ id: `tmp-${restaurant}`, party: partyId, user: userId, restaurant, rank: i + 1 })),
  ]
}

export interface RankedStanding extends TallyStanding {
  restaurant: string
  data: Restaurant
}

/** Classement du serveur joint aux restaurants (ordre du serveur ; candidats absents en fin, à 0). */
export function standingsOf(candidates: Restaurant[], tally: Tally | undefined): RankedStanding[] {
  const byId = new Map(candidates.map((r) => [r.id, r]))
  const out: RankedStanding[] = []
  for (const s of tally?.standings ?? []) {
    const data = byId.get(s.restaurant)
    if (!data) continue
    out.push({ ...s, data })
    byId.delete(s.restaurant)
  }
  for (const data of byId.values()) out.push({ restaurant: data.id, points: 0, firstChoices: 0, voters: 0, data })
  return out
}

/** « 1er », « 2e », « 3e »… */
export function ordinal(n: number): string {
  return n === 1 ? '1er' : `${n}e`
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
