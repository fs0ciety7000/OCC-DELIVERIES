import type { Party, PartyMember, User } from '@/lib/types'

/** Données communes passées à chaque étape de la party. */
export interface PartyCtx {
  party: Party
  members: PartyMember[]
  me: User
  isHost: boolean
  /** id utilisateur → utilisateur (membres + expand). */
  people: Map<string, User>
}

export function personName(ctx: Pick<PartyCtx, 'people' | 'me'>, userId: string): string {
  if (userId === ctx.me.id) return 'Toi'
  return ctx.people.get(userId)?.name || 'Un·e collègue'
}
