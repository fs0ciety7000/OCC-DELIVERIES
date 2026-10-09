import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronRight, Clock, LogOut, MapPin, Rocket, Settings, ShieldCheck, UserMinus } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { toast } from 'sonner'
import { Avatar, Badge, Button, buttonClass, Card, CardBody, EmptyState, Money, Skeleton } from '@/components/ui'
import { STATUS_LABELS } from '@/features/party/hooks'
import { distinctColors } from '@/lib/colors'
import { teamsApi } from '@/lib/api'
import { useAuth } from '@/lib/auth'
import { errorMessage, isNotFound } from '@/lib/errors'
import { formatDate, plural } from '@/lib/format'
import type { Team, TeamMember } from '@/lib/types'
import { canManage, ROLE_LABELS, usualSummary } from './format'
import { useTeam } from './hooks'
import { teamKeys } from './keys'
import { TeamEmblem } from './TeamEmblem'
import { TeamInviteCard } from './TeamInviteCard'
import { TeamSettingsSheet } from './TeamSettingsSheet'

/** `/equipes/:id` — salon d'équipe permanent. */
export function TeamPage() {
  const { id } = useParams()
  const team = useTeam(id)
  if (team.isError) {
    const nf = isNotFound(team.error) || (team.error as { status?: number })?.status === 403
    return (
      <EmptyState
        tone={nf ? 'default' : 'danger'}
        emoji={nf ? '🔒' : '📡'}
        title={nf ? 'Équipe introuvable' : 'Chargement impossible'}
        description={nf ? "Elle n'existe pas ou tu n'en fais pas partie. Demande le lien de l'équipe à un·e collègue !" : errorMessage(team.error)}
        action={
          <Link to="/" className="font-semibold text-brand">
            Retour à l'accueil
          </Link>
        }
      />
    )
  }
  if (!team.data) {
    return (
      <div className="space-y-4" aria-busy="true" aria-label="Chargement de l'équipe">
        <Skeleton className="h-16 w-2/3" />
        <Skeleton className="h-32 w-full rounded-lg" />
        <Skeleton className="h-48 w-full rounded-lg" />
      </div>
    )
  }
  return <TeamView team={team.data} />
}

export function TeamView({ team }: { team: Team }) {
  const { user } = useAuth()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const manager = canManage(team.myRole)
  const isGuest = !!user?.is_guest
  const usual = usualSummary(team.usualDays, team.usualTime)
  const refresh = () => void qc.invalidateQueries({ queryKey: teamKeys.all })

  const launch = useMutation({
    mutationFn: () => teamsApi.launch(team.id),
    onSuccess: ({ party, created }) => {
      toast.success(created ? "C'est parti : l'équipe est prévenue !" : 'Une commande est déjà en cours : on te la met.')
      refresh()
      navigate(`/party/${party.id}`)
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  const joinParty = useMutation({
    mutationFn: (partyId: string) => teamsApi.joinParty(partyId),
    onSuccess: ({ party }) => {
      toast.success(`Bienvenue dans « ${party.title || 'la commande'} » !`)
      refresh()
      navigate(`/party/${party.id}`)
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  const leave = useMutation({
    mutationFn: () => teamsApi.leave(team.id),
    onSuccess: () => {
      toast(`Tu as quitté « ${team.name} »`)
      refresh()
      navigate('/')
    },
    onError: (e) => toast.error(errorMessage(e)),
  })

  const active = team.activeParty
  return (
    <div className="mx-auto max-w-[680px] space-y-6">
      <header className="flex items-start gap-4">
        <TeamEmblem emoji={team.emoji} color={team.color} size="lg" />
        <div className="min-w-0 flex-1 space-y-1">
          <h1 className="font-display text-[32px] leading-9 font-bold break-words">{team.name}</h1>
          {usual && (
            <p className="flex items-center gap-1.5 text-sm text-muted">
              <Clock aria-hidden className="size-4 shrink-0" /> {usual}
            </p>
          )}
          {team.address && (
            <p className="flex items-center gap-1.5 text-sm text-muted">
              <MapPin aria-hidden className="size-4 shrink-0" /> <span className="truncate">{team.address}</span>
            </p>
          )}
          {team.archived && <Badge variant="warning">Archivée</Badge>}
        </div>
        {manager && (
          <Button variant="ghost" size="icon" aria-label="Réglages de l'équipe" onClick={() => setSettingsOpen(true)}>
            <Settings className="size-5" />
          </Button>
        )}
      </header>

      {active ? (
        <Card variant="selected" aria-labelledby="h-team-active">
          <CardBody className="space-y-3">
            <div className="flex items-center justify-between gap-2">
              <h2 id="h-team-active" className="min-w-0 truncate font-display text-lg font-semibold">
                {active.title || 'Commande du jour'}
              </h2>
              <Badge variant="brand" dot className="shrink-0">
                {STATUS_LABELS[active.status]}
              </Badge>
            </div>
            <div className="flex items-center gap-3">
              <span aria-hidden className="text-3xl">
                {active.restaurant?.emoji || '🗳️'}
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm text-muted">
                  Lancée par {active.host.name} · {plural(active.memberCount, 'participant')}
                  {active.restaurant ? ` · ${active.restaurant.name}` : ''}
                </p>
              </div>
            </div>
            {active.isMember ? (
              <Link to={`/party/${active.id}`} className={buttonClass('secondary', 'md', true)}>
                Ouvrir la commande <ChevronRight aria-hidden className="size-4" />
              </Link>
            ) : (
              <Button block size="lg" loading={joinParty.isPending} onClick={() => joinParty.mutate(active.id)}>
                Rejoindre la commande
              </Button>
            )}
          </CardBody>
        </Card>
      ) : isGuest ? (
        <Card>
          <CardBody className="text-sm text-muted">Pas de commande en cours. En invité·e, tu rejoins en un geste celles que lance l'équipe.</CardBody>
        </Card>
      ) : (
        !team.archived && (
          <Button block size="lg" leftIcon={<Rocket className="size-5" />} loading={launch.isPending} onClick={() => launch.mutate()}>
            Lancer la commande du jour
          </Button>
        )
      )}

      <MembersCard team={team} meId={user?.id} />
      <TeamInviteCard team={team} />
      <TeamHistory teamId={team.id} />

      {team.myRole !== 'owner' && (
        <Button variant="ghost" block leftIcon={<LogOut className="size-4" />} loading={leave.isPending} onClick={() => leave.mutate()}>
          Quitter l'équipe
        </Button>
      )}
      {manager && settingsOpen && <TeamSettingsSheet team={team} open onClose={() => setSettingsOpen(false)} />}
    </div>
  )
}

function MembersCard({ team, meId }: { team: Team; meId?: string }) {
  const qc = useQueryClient()
  const admins = team.members.filter((m) => m.role === 'admin').map((m) => m.id)
  const setAdmins = useMutation({
    mutationFn: (ids: string[]) => teamsApi.update(team.id, { admins: ids }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: teamKeys.all }),
    onError: (e) => toast.error(errorMessage(e)),
  })
  const remove = useMutation({
    mutationFn: (m: TeamMember) => teamsApi.removeMember(team.id, m.id),
    onSuccess: (_, m) => {
      toast(`${m.name} a été retiré·e de l'équipe`)
      void qc.invalidateQueries({ queryKey: teamKeys.all })
    },
    onError: (e) => toast.error(errorMessage(e)),
  })
  // Liste d'avatars : couleurs rendues distinctes au sein de l'équipe.
  const colors = distinctColors(team.members)
  return (
    <Card aria-labelledby="h-team-members">
      <CardBody className="space-y-3">
        <h2 id="h-team-members" className="font-display text-lg font-semibold">
          {plural(team.memberCount, 'membre')}
        </h2>
        <ul className="divide-y divide-border">
          {team.members.map((m) => {
            const color = colors.get(m.id) ?? m.color
            const canRemove = m.id !== meId && m.role !== 'owner' && (team.myRole === 'owner' || (team.myRole === 'admin' && m.role === 'member'))
            const canToggleAdmin = team.myRole === 'owner' && m.role !== 'owner' && !m.isGuest
            return (
              <li key={m.id} className="flex items-center gap-3 py-2.5">
                <Avatar user={{ ...m, color }} size={40} decorative />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-semibold">
                    {m.name}
                    {m.id === meId && <span className="font-normal text-subtle"> (toi)</span>}
                  </p>
                  <div className="flex flex-wrap gap-1.5">
                    {m.role && m.role !== 'member' && <Badge variant={m.role === 'owner' ? 'brand' : 'info'}>{ROLE_LABELS[m.role]}</Badge>}
                    {m.isGuest && <Badge variant="neutral">Invité·e</Badge>}
                  </div>
                </div>
                {canToggleAdmin && (
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={m.role === 'admin' ? `Retirer les droits d'admin de ${m.name}` : `Nommer ${m.name} admin`}
                    aria-pressed={m.role === 'admin'}
                    onClick={() => setAdmins.mutate(m.role === 'admin' ? admins.filter((x) => x !== m.id) : [...admins, m.id])}
                  >
                    <ShieldCheck className={m.role === 'admin' ? 'size-4 text-info' : 'size-4'} />
                  </Button>
                )}
                {canRemove && (
                  <Button variant="ghost" size="icon" aria-label={`Retirer ${m.name} de l'équipe`} onClick={() => remove.mutate(m)}>
                    <UserMinus className="size-4" />
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      </CardBody>
    </Card>
  )
}

function TeamHistory({ teamId }: { teamId: string }) {
  const history = useInfiniteQuery({
    queryKey: teamKeys.history(teamId),
    queryFn: ({ pageParam }) => teamsApi.history(teamId, pageParam),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page < last.totalPages ? last.page + 1 : undefined),
  })
  const items = history.data?.pages.flatMap((p) => p.items) ?? []
  return (
    <section aria-labelledby="h-team-history" className="space-y-3">
      <h2 id="h-team-history" className="font-display text-lg font-semibold">
        Historique de l'équipe
      </h2>
      {history.isPending ? (
        <Skeleton className="h-20 w-full rounded-lg" />
      ) : items.length === 0 ? (
        <p className="text-sm text-muted">Aucune commande pour l'instant : la première sera la bonne !</p>
      ) : (
        <ul className="space-y-2">
          {items.map(({ party: p, isMember }) => {
            const row = (
              <Card variant={isMember ? 'interactive' : 'default'} className="flex items-center gap-3 p-3">
                <span aria-hidden className="grid size-10 shrink-0 place-items-center rounded-md bg-elevated text-xl">
                  {p.restaurant?.emoji || '🍽️'}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate font-semibold">{p.restaurant?.name ?? p.title ?? 'Commande groupée'}</p>
                  <p className="truncate text-xs text-subtle">
                    {formatDate(p.created)} · {plural(p.memberCount, 'participant')} · {STATUS_LABELS[p.status]}
                  </p>
                </div>
                {isMember && p.total > 0 && <Money cents={p.total} className="text-sm font-semibold" />}
              </Card>
            )
            return <li key={p.id}>{isMember ? <Link to={`/party/${p.id}`} className="block rounded-lg" aria-label={`${p.title || 'Commande'} du ${formatDate(p.created)}`}>{row}</Link> : row}</li>
          })}
        </ul>
      )}
      {history.hasNextPage && (
        <Button variant="ghost" block loading={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>
          Voir plus
        </Button>
      )}
    </section>
  )
}
