import { ChevronRight, Plus } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { AvatarStack, Badge, Button, Card, Skeleton } from '@/components/ui'
import { STATUS_LABELS } from '@/features/party/hooks'
import type { User } from '@/lib/types'
import { CreateTeamSheet } from './CreateTeamSheet'
import { usualSummary } from './format'
import { useMyTeams } from './hooks'
import { TeamEmblem } from './TeamEmblem'

/** Accueil : « Mes équipes » (salons permanents) + création (comptes uniquement). */
export function TeamsSection({ user }: { user: User }) {
  const teams = useMyTeams(user.id)
  const [createOpen, setCreateOpen] = useState(false)
  const list = (teams.data ?? []).filter((t) => !t.archived)
  // Un·e invité·e sans équipe n'a rien à faire ici.
  if (user.is_guest && list.length === 0) return null
  return (
    <section className="space-y-3" aria-labelledby="h-teams">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <h2 id="h-teams" className="font-display text-2xl font-semibold">
          Mes équipes
        </h2>
        {!user.is_guest && (
          <Button variant="ghost" size="sm" leftIcon={<Plus className="size-4" />} onClick={() => setCreateOpen(true)}>
            Créer une équipe
          </Button>
        )}
      </div>
      {teams.isPending ? (
        <Skeleton className="h-20 rounded-lg" />
      ) : list.length === 0 ? (
        <Card className="p-4 text-sm text-muted">
          Un salon permanent pour ton équipe : un lien fixe à épingler, et la commande du jour se lance en un geste avec l'adresse du bureau et vos restos favoris.
        </Card>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2">
          {list.map((t) => (
            <li key={t.id}>
              <Link to={`/equipes/${t.id}`} className="block rounded-lg">
                <Card variant={t.activeParty ? 'selected' : 'interactive'} className="flex items-center gap-3 p-3.5">
                  <TeamEmblem emoji={t.emoji} color={t.color} />
                  <div className="min-w-0 flex-1 space-y-1">
                    <p className="truncate font-semibold">{t.name}</p>
                    {t.activeParty ? (
                      <Badge variant="brand" dot>
                        {t.activeParty.isMember ? STATUS_LABELS[t.activeParty.status] : 'Commande en cours — rejoindre'}
                      </Badge>
                    ) : (
                      <p className="truncate text-xs text-subtle">{usualSummary(t.usualDays, t.usualTime) || `${t.memberCount} membres`}</p>
                    )}
                  </div>
                  <AvatarStack users={t.members} size={24} max={3} />
                  <ChevronRight aria-hidden className="size-5 text-subtle" />
                </Card>
              </Link>
            </li>
          ))}
        </ul>
      )}
      <CreateTeamSheet open={createOpen} onClose={() => setCreateOpen(false)} />
    </section>
  )
}
