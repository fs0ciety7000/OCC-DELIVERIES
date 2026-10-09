import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { guestApi, teamsApi, usersApi } from '@/lib/api'
import { pb } from '@/lib/pb'
import type { GuestAuthResponse, Team, TeamHistoryPage, User } from '@/lib/types'
import { usualSummary } from './format'
import { GuestBanner } from './GuestBanner'
import { GuestJoinForm } from './GuestJoinForm'
import { InviteGate } from './InviteGate'
import { TeamView } from './TeamPage'
import { UpgradeCard } from './UpgradeCard'

const me = { id: 'u1', collectionId: 'c', collectionName: 'users', name: 'Ana', email: 'ana@occ.be', verified: true } as User

const team = (over: Partial<Team> = {}): Team => ({
  id: 't1',
  name: 'OCC Mons — midi',
  code: 'K7M2QXAB',
  emoji: '🍕',
  color: '#FF6A3D',
  address: 'Rue de Nimy 7, 7000 Mons',
  lat: 0,
  lng: 0,
  usualTime: '12:15',
  usualDays: ['mon', 'tue', 'wed', 'thu', 'fri'],
  defaultCandidates: [],
  defaultSplit: 'equal',
  archived: false,
  created: '2026-10-01 10:00:00.000Z',
  myRole: 'member',
  memberCount: 3,
  members: [
    { id: 'u0', name: 'Bob', color: '#3DD68C', isGuest: false, role: 'owner' },
    { id: 'u1', name: 'Ana', color: '#6AA8FF', isGuest: false, role: 'member' },
    { id: 'g1', name: 'Léa', color: '#B58CFF', isGuest: true, role: 'member' },
  ],
  activeParty: null,
  ...over,
})

const emptyHistory: TeamHistoryPage = { page: 1, perPage: 10, totalItems: 0, totalPages: 0, items: [] }

function wrap(ui: ReactNode, path = '/', route = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path={route} element={ui} />
          <Route path="/party/:id" element={<p>Salle de commande</p>} />
          <Route path="/equipes/:id" element={<p>Page équipe</p>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function signIn(user: User) {
  const payload = btoa(JSON.stringify({ exp: 4102444800, id: user.id, type: 'auth' }))
  pb.authStore.save(`h.${payload}.s`, user as never)
}

afterEach(() => {
  vi.restoreAllMocks()
  pb.authStore.clear()
  localStorage.clear()
})

describe('usualSummary', () => {
  it.each([
    [['mon', 'tue', 'wed', 'thu', 'fri'], '12:15', 'Du lundi au vendredi à 12:15'],
    [['tue', 'thu'], '12:00', 'Mardi, jeudi à 12:00'],
    [[], '12:00', 'À 12:00'],
    [['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'], '', 'Tous les jours'],
    [[], '', ''],
  ] as const)('%j %s → %s', (days, time, want) => {
    expect(usualSummary([...days], time)).toBe(want)
  })
})

describe('TeamView', () => {
  it('liste les membres (rôles, invité·e) et lance la commande du jour', async () => {
    signIn(me)
    vi.spyOn(teamsApi, 'history').mockResolvedValue(emptyHistory)
    const launch = vi.spyOn(teamsApi, 'launch').mockResolvedValue({ party: { id: 'p1' } as never, created: true })
    wrap(<TeamView team={team()} />)
    expect(screen.getByRole('heading', { name: 'OCC Mons — midi' })).toBeInTheDocument()
    expect(screen.getByText('Du lundi au vendredi à 12:15')).toBeInTheDocument()
    const members = screen.getByRole('heading', { name: '3 membres' }).closest('div')!
    expect(within(members).getByText('Propriétaire')).toBeInTheDocument()
    expect(within(members).getByText('Invité·e')).toBeInTheDocument()
    expect(within(members).getByText('(toi)')).toBeInTheDocument()
    // un simple membre ne voit ni réglages ni actions de gestion
    expect(screen.queryByRole('button', { name: "Réglages de l'équipe" })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Retirer/ })).not.toBeInTheDocument()
    expect(await screen.findByText(/Aucune commande pour l'instant/)).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Lancer la commande du jour' }))
    expect(launch).toHaveBeenCalledWith('t1')
    expect(await screen.findByText('Salle de commande')).toBeInTheDocument()
  })

  it('commande en cours mise en avant : rejoindre en un geste (sans code)', async () => {
    signIn(me)
    vi.spyOn(teamsApi, 'history').mockResolvedValue(emptyHistory)
    const join = vi.spyOn(teamsApi, 'joinParty').mockResolvedValue({ party: { id: 'p9', title: 'Midi du lundi' } as never, alreadyMember: false })
    wrap(
      <TeamView
        team={team({
          activeParty: { id: 'p9', code: 'ABCDEF', title: 'Midi du lundi', status: 'voting', created: '', host: { id: 'u0', name: 'Bob' }, memberCount: 2, isMember: false, restaurant: null },
        })}
      />,
    )
    expect(screen.getByRole('heading', { name: 'Midi du lundi' })).toBeInTheDocument()
    expect(screen.getByText(/Lancée par Bob · 2 participants/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Lancer la commande du jour' })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Rejoindre la commande' }))
    expect(join).toHaveBeenCalledWith('p9')
    expect(await screen.findByText('Salle de commande')).toBeInTheDocument()
  })

  it('propriétaire : réglages, gestion des admins et des membres ; invité·e : pas de lancement', async () => {
    signIn(me)
    vi.spyOn(teamsApi, 'history').mockResolvedValue(emptyHistory)
    const update = vi.spyOn(teamsApi, 'update').mockResolvedValue({} as never)
    const remove = vi.spyOn(teamsApi, 'removeMember').mockResolvedValue({ team: team() })
    const { unmount } = wrap(
      <TeamView
        team={team({
          myRole: 'owner',
          members: [
            { id: 'u1', name: 'Ana', isGuest: false, role: 'owner' },
            { id: 'u2', name: 'Bob', isGuest: false, role: 'member' },
            { id: 'g1', name: 'Léa', isGuest: true, role: 'member' },
          ],
        })}
      />,
    )
    expect(screen.getByRole('button', { name: "Réglages de l'équipe" })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Nommer Léa admin' })).not.toBeInTheDocument() // invité·e
    await userEvent.click(screen.getByRole('button', { name: 'Nommer Bob admin' }))
    expect(update).toHaveBeenCalledWith('t1', { admins: ['u2'] })
    await userEvent.click(screen.getByRole('button', { name: "Retirer Léa de l'équipe" }))
    expect(remove).toHaveBeenCalledWith('t1', 'g1')
    expect(screen.queryByRole('button', { name: "Quitter l'équipe" })).not.toBeInTheDocument()
    unmount()

    signIn({ ...me, is_guest: true })
    wrap(<TeamView team={team()} />)
    expect(screen.queryByRole('button', { name: 'Lancer la commande du jour' })).not.toBeInTheDocument()
    expect(screen.getByText(/En invité·e, tu rejoins en un geste/)).toBeInTheDocument()
  })
})

describe('Invités', () => {
  it('formulaire invité : prénom seul + couleur, code transmis', async () => {
    const res: GuestAuthResponse = { token: 't', record: { id: 'g1', name: 'Léa' } as User, party: { id: 'p1', title: 'Midi' }, team: null }
    const join = vi.spyOn(guestApi, 'join').mockResolvedValue(res)
    const onJoined = vi.fn()
    wrap(<GuestJoinForm kind="party" code="K7M2QX" onJoined={onJoined} />)
    await userEvent.click(screen.getByRole('button', { name: 'Rejoindre la commande' }))
    expect(await screen.findByText(/Indique ton prénom/)).toBeInTheDocument()
    expect(join).not.toHaveBeenCalled()
    await userEvent.type(screen.getByLabelText('Ton prénom'), ' Léa ')
    await userEvent.click(screen.getByRole('button', { name: 'Couleur #3DD68C' }))
    await userEvent.click(screen.getByRole('button', { name: 'Rejoindre la commande' }))
    expect(join).toHaveBeenCalledWith({ name: 'Léa', color: '#3DD68C', partyCode: 'K7M2QX' })
    expect(onJoined).toHaveBeenCalledWith(res)
  })

  it("page d'invitation sans session : aperçu, puis « Continuer en invité·e » vers la salle", async () => {
    vi.spyOn(guestApi, 'preview').mockResolvedValue({ kind: 'party', code: 'K7M2QX', title: 'Midi du vendredi', host: 'Bob', joinable: true, memberCount: 3, status: 'lobby' })
    vi.spyOn(guestApi, 'join').mockResolvedValue({ token: 't', record: { id: 'g1', name: 'Léa' } as User, party: { id: 'p1', title: 'Midi du vendredi' }, team: null })
    wrap(<InviteGate kind="party" code="K7M2QX" />, '/j/K7M2QX', '/j/:code')
    expect(await screen.findByRole('heading', { name: 'Midi du vendredi' })).toBeInTheDocument()
    expect(screen.getByText(/Bob t'invite · 3 membres/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Se connecter' })).toHaveAttribute('href', '/login?next=%2Fj%2FK7M2QX')
    await userEvent.click(screen.getByRole('button', { name: 'Continuer en invité·e' }))
    await userEvent.type(screen.getByLabelText('Ton prénom'), 'Léa')
    await userEvent.click(screen.getByRole('button', { name: 'Rejoindre la commande' }))
    expect(await screen.findByText('Salle de commande')).toBeInTheDocument()
  })

  it("lien d'équipe archivée : pas de formulaire", async () => {
    vi.spyOn(guestApi, 'preview').mockResolvedValue({ kind: 'team', code: 'K7M2QXAB', title: 'OCC', joinable: false, memberCount: 2 })
    wrap(<InviteGate kind="team" code="K7M2QXAB" />)
    expect(await screen.findByText('Cette équipe est archivée.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Continuer en invité·e' })).not.toBeInTheDocument()
  })

  it('bandeau invité masquable (mémorisé)', async () => {
    const { rerender } = render(
      <MemoryRouter>
        <GuestBanner user={{ ...me, is_guest: true }} />
      </MemoryRouter>,
    )
    expect(screen.getByText('Tu es invité·e')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Créer mon compte' })).toHaveAttribute('href', '/profile?onglet=infos')
    await userEvent.click(screen.getByRole('button', { name: 'Masquer ce rappel' }))
    expect(screen.queryByText('Tu es invité·e')).not.toBeInTheDocument()
    expect(localStorage.getItem('occ.guestBanner.dismissed')).toBe('1')
    rerender(
      <MemoryRouter>
        <GuestBanner user={me} />
      </MemoryRouter>,
    )
    expect(screen.queryByText('Tu es invité·e')).not.toBeInTheDocument()
  })

  it('créer mon compte : e-mail + mot de passe (même compte)', async () => {
    vi.spyOn(usersApi, 'authMethods').mockResolvedValue({ oauth2: { enabled: false, providers: [] } } as never)
    const upgrade = vi.spyOn(guestApi, 'upgrade').mockResolvedValue({ token: 't', record: { ...me, is_guest: false } })
    wrap(<UpgradeCard user={{ ...me, is_guest: true, name: 'Léa' }} />)
    await userEvent.type(screen.getByLabelText('E-mail'), 'lea@occ.be')
    await userEvent.type(screen.getByLabelText('Mot de passe'), 'motdepasse1')
    await userEvent.type(screen.getByLabelText('Confirme le mot de passe'), 'autre')
    expect(screen.getByText('Les deux mots de passe ne correspondent pas.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Créer mon compte' })).toBeDisabled()
    await userEvent.clear(screen.getByLabelText('Confirme le mot de passe'))
    await userEvent.type(screen.getByLabelText('Confirme le mot de passe'), 'motdepasse1')
    await userEvent.click(screen.getByRole('button', { name: 'Créer mon compte' }))
    expect(upgrade).toHaveBeenCalledWith({ email: 'lea@occ.be', password: 'motdepasse1' })
  })
})
