import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { adminApi } from '@/lib/api'
import { pb } from '@/lib/pb'
import type { AdminUser, MailStatus } from '@/lib/types'
import { filterQuery } from './userFilters'
import { UsersAdminPage } from './UsersAdminPage'

const base: AdminUser = {
  id: 'u2', name: 'Bob', email: 'bob@occ.be', role: 'user', color: '#FF6A3D', avatar: '', verified: true,
  created: '2026-10-01 10:00:00.000Z', parties: 3, banned: false, bannedReason: '', bannedAt: '', deleted: false, deletedAt: '',
  passwordSet: true, providers: ['google'], lastLoginAt: '2026-10-08 10:00:00.000Z',
}
const users: AdminUser[] = [
  { ...base, id: 'u1', name: 'Ana', email: 'ana@occ.be', role: 'admin' },
  base,
  { ...base, id: 'u3', name: 'Cléo', email: 'cleo@occ.be', banned: true, bannedReason: 'spam', bannedAt: '2026-10-09 08:00:00.000Z', providers: [], verified: false },
]
const mailOff: MailStatus = { enabled: false, host: '', port: 0, tls: false, senderAddress: '', senderName: '', fromEnv: false }

function setup(mail: MailStatus = mailOff) {
  const payload = btoa(JSON.stringify({ exp: 4102444800, id: 'u1', type: 'auth' }))
  pb.authStore.save(`h.${payload}.s`, { id: 'u1', collectionId: 'c', collectionName: 'users', name: 'Ana', role: 'admin' } as never)
  const list = vi.spyOn(adminApi, 'users').mockResolvedValue({ page: 1, perPage: 50, totalItems: users.length, items: users })
  vi.spyOn(adminApi, 'mailStatus').mockResolvedValue(mail)
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <UsersAdminPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return { list }
}

beforeEach(() => vi.restoreAllMocks())
afterEach(() => pb.authStore.clear())

describe('filterQuery', () => {
  it('traduit les filtres en paramètres', () => {
    expect(filterQuery('all')).toEqual({})
    expect(filterQuery('admins')).toEqual({ role: 'admin' })
    expect(filterQuery('banned')).toEqual({ status: 'banned' })
    expect(filterQuery('unverified')).toEqual({ status: 'unverified' })
  })
})

describe('UsersAdminPage', () => {
  it('affiche badges, filtre et état des e-mails', async () => {
    const { list } = setup()
    expect(await screen.findByText('Cléo')).toBeInTheDocument()
    expect(screen.getByText('Suspendu')).toBeInTheDocument()
    expect(screen.getByText(/— spam/)).toBeInTheDocument()
    expect(screen.getAllByText('Google')).toHaveLength(2)
    expect(await screen.findByText('Désactivés')).toBeInTheDocument()
    // e-mails désactivés : ni bouton de test, ni alerte « Non vérifié » (personne ne peut vérifier)
    expect(screen.queryByRole('button', { name: 'Envoyer un e-mail de test' })).not.toBeInTheDocument()
    expect(screen.queryByText('Non vérifié')).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('radio', { name: 'Suspendus' }))
    expect(list).toHaveBeenLastCalledWith('', 1, { status: 'banned' })
  })

  it('suspend avec un motif après confirmation', async () => {
    setup()
    const ban = vi.spyOn(adminApi, 'ban').mockResolvedValue({ user: { ...base, banned: true } })
    await userEvent.click(await screen.findByRole('button', { name: 'Actions pour Bob' }))
    const menu = await screen.findByRole('dialog')
    // e-mails désactivés : pas de lien de réinitialisation
    expect(within(menu).getByRole('button', { name: /Envoyer un lien de réinitialisation/ })).toBeDisabled()
    await userEvent.click(within(menu).getByRole('button', { name: /Suspendre le compte/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Suspendre Bob ?' })
    await userEvent.type(within(dialog).getByLabelText(/Motif/), 'compte en double')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Suspendre' }))
    expect(ban).toHaveBeenCalledWith('u2', 'compte en double')
  })

  it('protège son propre compte et réactive un compte suspendu', async () => {
    setup({ ...mailOff, enabled: true, host: 'smtp.example.com', port: 587, senderAddress: 'noreply@fs0ciety.org', senderName: 'OCC Deliveries', fromEnv: true })
    const unban = vi.spyOn(adminApi, 'unban').mockResolvedValue({ user: base })
    expect(await screen.findByText('Non vérifié')).toBeInTheDocument()
    await userEvent.click(await screen.findByRole('button', { name: 'Actions pour Ana' }))
    let menu = await screen.findByRole('dialog')
    expect(within(menu).getByRole('button', { name: /Suspendre le compte/ })).toBeDisabled()
    expect(within(menu).getByRole('button', { name: /Supprimer le compte/ })).toBeDisabled()
    expect(within(menu).getByRole('button', { name: /Retirer les droits admin/ })).toBeDisabled()
    expect(within(menu).getByRole('button', { name: /Envoyer un lien de réinitialisation/ })).toBeEnabled()
    await userEvent.keyboard('{Escape}')

    await userEvent.click(screen.getByRole('button', { name: 'Actions pour Cléo' }))
    menu = await screen.findByRole('dialog')
    await userEvent.click(within(menu).getByRole('button', { name: /Réactiver le compte/ }))
    expect(unban).toHaveBeenCalledWith('u3')
  })

  it('envoie un lien de réinitialisation et supprime (anonymise) un compte', async () => {
    setup({ ...mailOff, enabled: true, host: 'smtp.example.com', port: 587, senderAddress: 'noreply@fs0ciety.org', senderName: 'OCC Deliveries', fromEnv: true })
    const reset = vi.spyOn(adminApi, 'sendPasswordReset').mockResolvedValue({ sent: true, email: 'bob@occ.be' })
    const del = vi.spyOn(adminApi, 'deleteUser').mockResolvedValue({ user: { ...base, deleted: true } })
    const test = vi.spyOn(adminApi, 'mailTest').mockResolvedValue({ sent: true, to: 'ana@occ.be' })

    await userEvent.click(await screen.findByRole('button', { name: 'Envoyer un e-mail de test' }))
    expect(test).toHaveBeenCalled()

    await userEvent.click(screen.getByRole('button', { name: 'Actions pour Bob' }))
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: /Envoyer un lien de réinitialisation/ }))
    await userEvent.click(within(await screen.findByRole('dialog', { name: 'Envoyer un lien à Bob ?' })).getByRole('button', { name: 'Envoyer le lien' }))
    expect(reset).toHaveBeenCalledWith('u2')

    await userEvent.click(screen.getByRole('button', { name: 'Actions pour Bob' }))
    await userEvent.click(within(await screen.findByRole('dialog')).getByRole('button', { name: /Supprimer le compte/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Supprimer le compte de Bob ?' })
    expect(dialog).toHaveTextContent('Compte supprimé')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer le compte' }))
    expect(del).toHaveBeenCalledWith('u2')
  })
})
