import { useQuery } from '@tanstack/react-query'
import { Avatar, Card, CardBody } from '@/components/ui'
import { teamsApi } from '@/lib/api'
import type { Party } from '@/lib/types'
import { teamKeys } from './keys'

const OPEN = new Set(['lobby', 'voting', 'ordering'])

/**
 * Commande d'équipe : « Membres de l'équipe pas encore là » (ils peuvent rejoindre en un
 * geste depuis la page d'équipe ou le toast de lancement).
 */
export function TeamMissingMembers({ party }: { party: Pick<Party, 'id' | 'team' | 'status' | 'members'> }) {
  const enabled = !!party.team && OPEN.has(party.status)
  const info = useQuery({
    queryKey: [...teamKeys.partyTeam(party.id), party.members.length],
    queryFn: () => teamsApi.partyTeam(party.id),
    enabled,
    staleTime: 15_000,
  })
  if (!enabled || !info.data?.team || info.data.missing.length === 0) return null
  const { team, missing } = info.data
  return (
    <Card aria-labelledby="h-team-missing">
      <CardBody className="space-y-2">
        <h2 id="h-team-missing" className="text-sm font-semibold">
          Membres de l'équipe pas encore là <span className="font-normal text-subtle">· {team.emoji ? `${team.emoji} ` : ''}{team.name}</span>
        </h2>
        <ul className="flex flex-wrap gap-x-4 gap-y-2">
          {missing.map((m) => (
            <li key={m.id} className="flex items-center gap-2 text-sm text-muted">
              <Avatar user={m} size={24} decorative className="opacity-60" />
              {m.name}
              {m.isGuest && <span className="text-xs text-subtle">(invité·e)</span>}
            </li>
          ))}
        </ul>
        <p className="text-xs text-subtle">Ils rejoignent en un geste depuis la page de l'équipe, sans code.</p>
      </CardBody>
    </Card>
  )
}
