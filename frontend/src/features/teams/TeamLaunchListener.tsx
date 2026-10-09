import { useCallback } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { teamsApi } from '@/lib/api'
import { pb } from '@/lib/pb'
import type { TeamRecord, User } from '@/lib/types'
import { useMyTeamsRealtime } from './hooks'

/**
 * Monté dans le shell : quand un·e collègue lance une commande pour une de mes équipes,
 * toast « Rejoindre » (un geste, sans code). Rien si j'en fais déjà partie (l'hôte, par ex.).
 */
export function TeamLaunchListener({ user }: { user: User | null }) {
  const navigate = useNavigate()
  const onLaunch = useCallback(
    async (team: TeamRecord) => {
      const partyId = team.last_party
      try {
        await pb.collection('parties').getOne(partyId, { requestKey: null })
        return // déjà membre (rules : visible seulement des membres)
      } catch {
        /* pas encore membre */
      }
      toast(`${team.emoji ? `${team.emoji} ` : ''}${team.name} : la commande du jour est lancée !`, {
        id: `team-launch-${partyId}`,
        duration: 15_000,
        action: {
          label: 'Rejoindre',
          onClick: () => {
            teamsApi
              .joinParty(partyId)
              .then(({ party }) => navigate(`/party/${party.id}`))
              .catch(() => navigate(`/equipes/${team.id}`))
          },
        },
      })
    },
    [navigate],
  )
  useMyTeamsRealtime(user?.id, (t) => void onLaunch(t))
  return null
}
