import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { occ, usersApi } from '@/lib/api'
import { pb } from '@/lib/pb'
import type { AccountInfo, User } from '@/lib/types'
import { SecurityCard } from './SecurityCard'
import { VerifyEmailBanner } from './VerifyEmailBanner'

const user = { id: 'u1', collectionId: 'c', collectionName: 'users', created: '', updated: '', name: 'Ana', email: 'ana@occ.be', verified: false } as User

const account = (over: Partial<AccountInfo> = {}): AccountInfo => ({
  email: 'ana@occ.be',
  verified: true,
  passwordSet: true,
  providers: [{ id: 'ea1', provider: 'google', created: '2026-10-01 10:00:00.000Z' }],
  mailEnabled: true,
  ...over,
})

function renderCard(acc: AccountInfo) {
  vi.spyOn(usersApi, 'account').mockResolvedValue(acc)
  vi.spyOn(usersApi, 'authMethods').mockResolvedValue({ oauth2: { enabled: true, providers: [{ name: 'google', displayName: 'Google' }] } } as never)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SecurityCard user={user} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
  pb.authStore.clear()
})

describe('SecurityCard', () => {
  it('compte Google sans mot de passe : lien pour en choisir un, dissociation bloquée', async () => {
    const link = vi.spyOn(usersApi, 'requestPasswordReset').mockResolvedValue(true)
    renderCard(account({ passwordSet: false, verified: false }))
    expect(await screen.findByText('Google connecté')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Dissocier/ })).toBeDisabled()
    expect(screen.getByText('Non vérifiée')).toBeInTheDocument()
    expect(screen.queryByLabelText('Mot de passe actuel')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Nouvelle adresse')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Recevoir un lien pour choisir un mot de passe' }))
    expect(link).toHaveBeenCalledWith('ana@occ.be')
  })

  it('change le mot de passe et signale un mot de passe actuel faux', async () => {
    const { ClientResponseError } = await import('pocketbase')
    const change = vi
      .spyOn(usersApi, 'changePassword')
      .mockRejectedValueOnce(new ClientResponseError({ status: 400, response: { message: 'x', data: { oldPassword: { message: 'Invalid' } } } }))
      .mockResolvedValueOnce(undefined)
    renderCard(account())
    await userEvent.type(await screen.findByLabelText('Mot de passe actuel'), 'ancien-mdp')
    await userEvent.type(screen.getByLabelText('Nouveau mot de passe'), 'Nouveau-Mdp-2026')
    await userEvent.type(screen.getByLabelText('Confirme le mot de passe'), 'Nouveau-Mdp-2026')
    await userEvent.click(screen.getByRole('button', { name: 'Changer le mot de passe' }))
    expect(await screen.findByText('Mot de passe actuel incorrect.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Changer le mot de passe' }))
    expect(change).toHaveBeenLastCalledWith({ id: 'u1', email: 'ana@occ.be' }, 'ancien-mdp', 'Nouveau-Mdp-2026')
  })

  it('dissocie Google quand un mot de passe existe et demande un changement d’adresse', async () => {
    const unlink = vi.spyOn(usersApi, 'unlinkProvider').mockResolvedValue({ ok: true })
    const change = vi.spyOn(usersApi, 'requestEmailChange').mockResolvedValue(true)
    renderCard(account())
    await userEvent.click(await screen.findByRole('button', { name: /Dissocier/ }))
    expect(unlink).toHaveBeenCalledWith('google')
    await userEvent.type(screen.getByLabelText('Nouvelle adresse'), 'ana@nouveau.be')
    await userEvent.click(screen.getByRole('button', { name: "Changer d'adresse" }))
    expect(change).toHaveBeenCalledWith('ana@nouveau.be')
  })

  it('suppression : il faut taper SUPPRIMER', async () => {
    const del = vi.spyOn(usersApi, 'deleteMe').mockResolvedValue(undefined)
    renderCard(account())
    await userEvent.click(await screen.findByRole('button', { name: 'Supprimer mon compte' }))
    const dialog = await screen.findByRole('dialog')
    const confirm = within(dialog).getByRole('button', { name: 'Supprimer définitivement' })
    expect(confirm).toBeDisabled()
    await userEvent.type(within(dialog).getByLabelText('Tape SUPPRIMER pour confirmer'), 'supprimer')
    expect(confirm).toBeEnabled()
    await userEvent.click(confirm)
    expect(del).toHaveBeenCalledWith('supprimer')
  })
})

describe('VerifyEmailBanner', () => {
  it('rappelle de confirmer l’adresse et renvoie l’e-mail', async () => {
    vi.spyOn(occ, 'config').mockResolvedValue({ currency: 'EUR', defaultLocation: { lat: 0, lng: 0, label: '' }, providers: [], minMenuItems: 0, mailEnabled: true })
    const resend = vi.spyOn(usersApi, 'requestVerification').mockResolvedValue(true)
    const qc = new QueryClient()
    render(
      <QueryClientProvider client={qc}>
        <VerifyEmailBanner user={user} dismissible />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('Confirme ton adresse e-mail.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: "Renvoyer l'e-mail" }))
    expect(resend).toHaveBeenCalledWith('ana@occ.be')
    await userEvent.click(screen.getByRole('button', { name: 'Masquer ce rappel' }))
    expect(screen.queryByText('Confirme ton adresse e-mail.')).not.toBeInTheDocument()
  })

  it('rien pour un compte vérifié', async () => {
    vi.spyOn(occ, 'config').mockResolvedValue({ currency: 'EUR', defaultLocation: { lat: 0, lng: 0, label: '' }, providers: [], minMenuItems: 0, mailEnabled: true })
    const qc = new QueryClient()
    const { container } = render(
      <QueryClientProvider client={qc}>
        <VerifyEmailBanner user={{ ...user, verified: true }} />
      </QueryClientProvider>,
    )
    await new Promise((r) => setTimeout(r, 20))
    expect(container).toBeEmptyDOMElement()
  })
})
