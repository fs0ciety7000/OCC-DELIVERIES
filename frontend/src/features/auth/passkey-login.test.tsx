import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ClientResponseError } from 'pocketbase'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { occ, usersApi } from '@/lib/api'
import { passkeysApi } from '@/lib/passkeys'
import { pb } from '@/lib/pb'
import type { AppConfig, User } from '@/lib/types'
import { LoginPage } from './LoginPage'

const config = (passkeys: boolean): AppConfig => ({
  currency: 'EUR',
  defaultLocation: { lat: 50.45, lng: 3.95, label: 'Mons' },
  providers: [],
  minMenuItems: 0,
  mailEnabled: true,
  passkeys,
})

const record = { id: 'u1', collectionId: '_pb_users_auth_', collectionName: 'users', name: 'Ana', email: 'ana@occ.be' } as unknown as User
// jeton factice non expiré (le SDK lit `exp`)
const token = `x.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 3600, type: 'auth', collectionId: '_pb_users_auth_' }))}.y`
const begin = (conditional = false) => ({ publicKey: { challenge: conditional ? 'BAUG' : 'AQID', rpId: 'localhost', userVerification: 'required' }, ...(conditional ? { mediation: 'conditional' } : {}) })
const credential = { toJSON: () => ({ id: 'cred', type: 'public-key' }) }

let get: ReturnType<typeof vi.fn>

function stubWebAuthn(conditional: boolean) {
  vi.stubGlobal('PublicKeyCredential', Object.assign(function PublicKeyCredential() {}, { isConditionalMediationAvailable: async () => conditional }))
  get = vi.fn()
  Object.defineProperty(navigator, 'credentials', { configurable: true, value: { create: vi.fn(), get } })
}

function renderLogin() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/login?next=/parties']}>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/parties" element={<p>Mes commandes</p>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.spyOn(occ, 'config').mockResolvedValue(config(true))
  vi.spyOn(usersApi, 'authMethods').mockResolvedValue({ oauth2: { enabled: false, providers: [] } } as never)
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  Reflect.deleteProperty(navigator, 'credentials')
  pb.authStore.clear()
})

describe('Connexion avec une passkey', () => {
  it('bouton : fenêtre du navigateur puis session PocketBase', async () => {
    stubWebAuthn(false)
    vi.spyOn(passkeysApi, 'loginBegin').mockImplementation(async (c) => begin(c) as never)
    const finish = vi.spyOn(passkeysApi, 'loginFinish').mockResolvedValue({ token, record })
    get.mockResolvedValue(credential)
    renderLogin()
    const button = await screen.findByRole('button', { name: 'Se connecter avec une passkey' })
    expect(screen.getByLabelText('E-mail')).toHaveAttribute('autocomplete', 'email')
    await waitFor(() => expect(passkeysApi.loginBegin).toHaveBeenCalledWith())
    await Promise.resolve()

    await userEvent.click(button)
    expect(get).toHaveBeenCalledTimes(1) // options préchargées : appel direct dans le clic
    const arg = get.mock.calls[0]![0] as CredentialRequestOptions
    expect(arg.mediation).toBeUndefined()
    expect(Array.from(new Uint8Array(arg.publicKey!.challenge as ArrayBuffer))).toEqual([1, 2, 3])
    await waitFor(() => expect(finish).toHaveBeenCalledWith({ id: 'cred', type: 'public-key' }))
    expect(await screen.findByText('Mes commandes')).toBeInTheDocument()
    expect(pb.authStore.token).toBe(token)
  })

  it('autoremplissage : champ « username webauthn » et demande conditionnelle', async () => {
    stubWebAuthn(true)
    vi.spyOn(passkeysApi, 'loginBegin').mockImplementation(async (c) => begin(c) as never)
    vi.spyOn(passkeysApi, 'loginFinish').mockResolvedValue({ token, record })
    let choose: (c: unknown) => void = () => {}
    get.mockImplementation(() => new Promise((resolve) => (choose = resolve)))
    renderLogin()
    await waitFor(() => expect(screen.getByLabelText('E-mail')).toHaveAttribute('autocomplete', 'username webauthn'))
    await waitFor(() => expect(get).toHaveBeenCalled())
    const arg = get.mock.calls[0]![0] as CredentialRequestOptions
    expect(arg.mediation).toBe('conditional')
    expect(arg.signal).toBeInstanceOf(AbortSignal)
    expect(passkeysApi.loginBegin).toHaveBeenCalledWith(true)
    choose(credential) // l'utilisateur choisit sa passkey dans la liste du champ
    expect(await screen.findByText('Mes commandes')).toBeInTheDocument()
  })

  it('erreur du serveur affichée ; annulation silencieuse', async () => {
    stubWebAuthn(false)
    vi.spyOn(passkeysApi, 'loginBegin').mockImplementation(async (c) => begin(c) as never)
    const message = "Cette passkey n'est plus liée à un compte OCC Deliveries."
    vi.spyOn(passkeysApi, 'loginFinish').mockRejectedValue(new ClientResponseError({ status: 400, response: { status: 400, message, data: {} } }))
    get.mockRejectedValueOnce(new DOMException('cancel', 'NotAllowedError')).mockResolvedValue(credential)
    renderLogin()
    const button = await screen.findByRole('button', { name: 'Se connecter avec une passkey' })
    await userEvent.click(button)
    await waitFor(() => expect(button).toBeEnabled())
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    await userEvent.click(button)
    expect(await screen.findByRole('alert')).toHaveTextContent(message)
  })

  it('pas de bouton si le serveur ne propose pas les passkeys', async () => {
    stubWebAuthn(true)
    vi.spyOn(occ, 'config').mockResolvedValue(config(false))
    const beginSpy = vi.spyOn(passkeysApi, 'loginBegin')
    renderLogin()
    await screen.findByRole('button', { name: 'Se connecter' })
    await waitFor(() => expect(occ.config).toHaveBeenCalled())
    await new Promise((r) => setTimeout(r, 0))
    expect(screen.queryByRole('button', { name: 'Se connecter avec une passkey' })).not.toBeInTheDocument()
    expect(beginSpy).not.toHaveBeenCalled()
    expect(screen.getByLabelText('E-mail')).toHaveAttribute('autocomplete', 'email')
  })
})
