import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { occ } from '@/lib/api'
import { passkeysApi } from '@/lib/passkeys'
import type { AppConfig, PasskeyView, User } from '@/lib/types'
import { PasskeysSection } from './PasskeysSection'

const user = { id: 'u1', collectionId: 'c', collectionName: 'users', created: '', updated: '', name: 'Ana', email: 'ana@occ.be', verified: true } as User

const config = (passkeys: boolean): AppConfig => ({
  currency: 'EUR',
  defaultLocation: { lat: 50.45, lng: 3.95, label: 'Mons' },
  providers: [],
  minMenuItems: 0,
  mailEnabled: true,
  passkeys,
})

const pk = (over: Partial<PasskeyView> = {}): PasskeyView => ({
  id: 'pk1',
  name: 'Trousseau iCloud',
  created: '2026-10-01 10:00:00.000Z',
  lastUsedAt: '',
  transports: ['internal', 'hybrid'],
  synced: true,
  ...over,
})

const begin = { publicKey: { challenge: 'AQID', rp: { id: 'localhost', name: 'OCC Deliveries' }, user: { id: 'dTE', name: 'ana@occ.be', displayName: 'Ana' } } }
const credential = { toJSON: () => ({ id: 'new-cred', type: 'public-key' }) }

let create: ReturnType<typeof vi.fn>

function stubWebAuthn() {
  vi.stubGlobal('PublicKeyCredential', function PublicKeyCredential() {})
  create = vi.fn().mockResolvedValue(credential)
  Object.defineProperty(navigator, 'credentials', { configurable: true, value: { create, get: vi.fn() } })
}

function renderSection(u: User = user) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <PasskeysSection user={u} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  stubWebAuthn()
  vi.spyOn(occ, 'config').mockResolvedValue(config(true))
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  Reflect.deleteProperty(navigator, 'credentials')
})

describe('PasskeysSection', () => {
  it('liste les passkeys et en ajoute une sans attente réseau dans le clic', async () => {
    const list = vi.spyOn(passkeysApi, 'list').mockResolvedValueOnce([pk()]).mockResolvedValue([pk(), pk({ id: 'pk2', name: 'Clé USB', synced: false })])
    const beginSpy = vi.spyOn(passkeysApi, 'registerBegin').mockResolvedValue(begin as never)
    const finish = vi.spyOn(passkeysApi, 'registerFinish').mockResolvedValue(pk({ id: 'pk2', name: 'Clé USB' }))
    renderSection()
    expect(await screen.findByText('Trousseau iCloud')).toBeInTheDocument()
    expect(screen.getByText('Synchronisée')).toBeInTheDocument()
    expect(screen.getByText(/jamais utilisée/)).toBeInTheDocument()
    await waitFor(() => expect(beginSpy).toHaveBeenCalledTimes(1)) // options préchargées
    await Promise.resolve()

    await userEvent.click(screen.getByRole('button', { name: 'Ajouter une passkey' }))
    expect(create).toHaveBeenCalledTimes(1)
    const arg = create.mock.calls[0]![0] as CredentialCreationOptions
    expect(Array.from(new Uint8Array(arg.publicKey!.challenge as ArrayBuffer))).toEqual([1, 2, 3])
    await waitFor(() => expect(finish).toHaveBeenCalledWith({ id: 'new-cred', type: 'public-key' }, undefined))
    expect(await screen.findByText('Clé USB')).toBeInTheDocument()
    expect(list).toHaveBeenCalledTimes(2)
  })

  it('annulation dans le navigateur : pas d’erreur, rien n’est envoyé', async () => {
    vi.spyOn(passkeysApi, 'list').mockResolvedValue([])
    vi.spyOn(passkeysApi, 'registerBegin').mockResolvedValue(begin as never)
    const finish = vi.spyOn(passkeysApi, 'registerFinish')
    create.mockRejectedValueOnce(new DOMException('cancel', 'NotAllowedError'))
    renderSection()
    expect(await screen.findByText("Aucune passkey pour l'instant.")).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Ajouter une passkey' }))
    await waitFor(() => expect(create).toHaveBeenCalled())
    expect(finish).not.toHaveBeenCalled()
    await waitFor(() => expect(screen.getByRole('button', { name: 'Ajouter une passkey' })).toBeEnabled())
  })

  it('renomme et supprime après confirmation', async () => {
    vi.spyOn(passkeysApi, 'list').mockResolvedValue([pk({ lastUsedAt: '2026-10-09 10:00:00.000Z' })])
    vi.spyOn(passkeysApi, 'registerBegin').mockResolvedValue(begin as never)
    const rename = vi.spyOn(passkeysApi, 'rename').mockResolvedValue(pk({ name: 'iPhone' }))
    const remove = vi.spyOn(passkeysApi, 'remove').mockResolvedValue(undefined)
    renderSection()
    await userEvent.click(await screen.findByRole('button', { name: 'Renommer Trousseau iCloud' }))
    const input = screen.getByLabelText('Nom de la passkey')
    await userEvent.clear(input)
    await userEvent.type(input, 'iPhone')
    await userEvent.click(screen.getByRole('button', { name: 'Enregistrer' }))
    expect(rename).toHaveBeenCalledWith('pk1', 'iPhone')

    await userEvent.click(await screen.findByRole('button', { name: 'Supprimer Trousseau iCloud' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Supprimer « Trousseau iCloud » ?')).toBeInTheDocument()
    expect(remove).not.toHaveBeenCalled()
    await userEvent.click(within(dialog).getByRole('button', { name: 'Supprimer la passkey' }))
    expect(remove).toHaveBeenCalledWith('pk1')
  })

  it('masquée sans WebAuthn, pour un invité ou si le serveur ne les propose pas', async () => {
    const list = vi.spyOn(passkeysApi, 'list').mockResolvedValue([])
    vi.stubGlobal('PublicKeyCredential', undefined)
    const { unmount } = renderSection()
    expect(screen.queryByText('Passkeys')).not.toBeInTheDocument()
    unmount()

    stubWebAuthn()
    renderSection({ ...user, is_guest: true }).unmount()
    expect(list).not.toHaveBeenCalled()

    vi.spyOn(occ, 'config').mockResolvedValue(config(false))
    vi.spyOn(passkeysApi, 'registerBegin').mockResolvedValue(begin as never)
    renderSection()
    await waitFor(() => expect(occ.config).toHaveBeenCalled())
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.queryByRole('button', { name: 'Ajouter une passkey' })).not.toBeInTheDocument()
  })
})
