/** Clés TanStack Query des équipes (séparées de `lib/queryKeys` pour rester additif). */
export const teamKeys = {
  all: ['teams'] as const,
  mine: (userId: string) => ['teams', 'mine', userId] as const,
  team: (id: string) => ['teams', 'detail', id] as const,
  history: (id: string) => ['teams', 'history', id] as const,
  partyTeam: (partyId: string) => ['party', partyId, 'team'] as const,
  invite: (code: string) => ['invite', code] as const,
}
