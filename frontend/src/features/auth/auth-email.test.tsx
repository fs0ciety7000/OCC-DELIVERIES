import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ClientResponseError } from 'pocketbase'
import type { ReactNode } from 'react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { occ, usersApi } from '@/lib/api'
import type { AppConfig } from '@/lib/types'
import { ConfirmEmailChangePage, ForgotPasswordPage, ResetPasswordPage, VerifyEmailPage } from './EmailPages'
import { MIN_PASSWORD, pairError, passwordStrength } from './password'

const config = (mailEnabled: boolean): AppConfig => ({
  currency: 'EUR',
  defaultLocation: { lat: 50.45, lng: 3.95, label: 'Mons' },
  providers: [],
  minMenuItems: 0,
  mailEnabled,
})

function renderAt(path: string, route: string, page: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path={route} element={page} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const pbError = (status: number, data: Record<string, unknown> = {}, message = 'An error occurred while validating the submitted data.') =>
  new ClientResponseError({ status, response: { status, message, data } })

beforeEach(() => vi.spyOn(occ, 'config').mockResolvedValue(config(true)))
afterEach(() => vi.restoreAllMocks())

describe('password', () => {
  it('note la robustesse', () => {
    expect(passwordStrength('court').level).toBe(0)
    expect(passwordStrength('password123').level).toBe(1)
    expect(passwordStrength('abcdefgh').level).toBe(1)
    expect(passwordStrength('abcdefgh12').level).toBe(2)
    expect(passwordStrength('Deux-Pizzas-Au-Moins').level).toBe(3)
    expect(passwordStrength('x'.repeat(MIN_PASSWORD)).label).toBe('Faible')
  })

  it('valide le couple', () => {
    expect(pairError({ password: 'abc', confirm: 'abc' })).toMatch(/8 caractères/)
    expect(pairError({ password: 'abcdefgh', confirm: 'abcdefgx' })).toMatch(/correspondent pas/)
    expect(pairError({ password: 'abcdefgh', confirm: 'abcdefgh' })).toBeNull()
  })
})

describe('ResetPasswordPage', () => {
  it('exige deux mots de passe identiques puis confirme', async () => {
    const confirm = vi.spyOn(usersApi, 'confirmPasswordReset').mockResolvedValue(true)
    renderAt('/auth/reinitialiser/tok123', '/auth/reinitialiser/:token', <ResetPasswordPage />)
    const submit = screen.getByRole('button', { name: 'Enregistrer le mot de passe' })
    await userEvent.type(screen.getByLabelText('Nouveau mot de passe'), 'Deux-Pizzas-Au-Moins')
    await userEvent.type(screen.getByLabelText('Confirme le mot de passe'), 'Deux-Pizzas')
    expect(screen.getByText('Les deux mots de passe ne correspondent pas.')).toBeInTheDocument()
    expect(submit).toBeDisabled()
    expect(screen.getByText('Robustesse : Solide')).toBeInTheDocument()

    await userEvent.type(screen.getByLabelText('Confirme le mot de passe'), '-Au-Moins')
    await userEvent.click(submit)
    expect(confirm).toHaveBeenCalledWith('tok123', 'Deux-Pizzas-Au-Moins')
    expect(await screen.findByRole('heading', { name: 'Mot de passe changé' })).toBeInTheDocument()
  })

  it('explique un lien expiré', async () => {
    vi.spyOn(usersApi, 'confirmPasswordReset').mockRejectedValue(pbError(400, { token: { message: 'Invalid or expired token.' } }))
    renderAt('/auth/reinitialiser/vieux', '/auth/reinitialiser/:token', <ResetPasswordPage />)
    await userEvent.type(screen.getByLabelText('Nouveau mot de passe'), 'abcdefgh12')
    await userEvent.type(screen.getByLabelText('Confirme le mot de passe'), 'abcdefgh12')
    await userEvent.click(screen.getByRole('button', { name: 'Enregistrer le mot de passe' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Ce lien a expiré ou a déjà servi')
    expect(screen.getByRole('link', { name: 'Nouveau lien' })).toHaveAttribute('href', '/auth/mot-de-passe-oublie')
  })
})

describe('VerifyEmailPage', () => {
  it('confirme l’adresse une seule fois', async () => {
    const verify = vi.spyOn(usersApi, 'confirmVerification').mockResolvedValue(true)
    renderAt('/auth/verifier/abc', '/auth/verifier/:token', <VerifyEmailPage />)
    expect(await screen.findByRole('heading', { name: /Adresse confirmée/ })).toBeInTheDocument()
    expect(verify).toHaveBeenCalledTimes(1)
    expect(verify).toHaveBeenCalledWith('abc')
  })

  it('signale un lien invalide', async () => {
    vi.spyOn(usersApi, 'confirmVerification').mockRejectedValue(pbError(400, { token: {} }))
    renderAt('/auth/verifier/bad', '/auth/verifier/:token', <VerifyEmailPage />)
    expect(await screen.findByRole('heading', { name: 'Lien invalide ou expiré' })).toBeInTheDocument()
  })
})

describe('ForgotPasswordPage', () => {
  it('envoie le lien pour l’adresse pré-remplie', async () => {
    const req = vi.spyOn(usersApi, 'requestPasswordReset').mockResolvedValue(true)
    renderAt('/auth/mot-de-passe-oublie?email=ana%40occ.be', '/auth/mot-de-passe-oublie', <ForgotPasswordPage />)
    expect(screen.getByLabelText('E-mail du compte')).toHaveValue('ana@occ.be')
    await userEvent.click(screen.getByRole('button', { name: 'Recevoir le lien' }))
    expect(req).toHaveBeenCalledWith('ana@occ.be')
    expect(await screen.findByRole('heading', { name: 'Regarde ta boîte mail' })).toBeInTheDocument()
  })

  it('prévient quand les e-mails sont désactivés', async () => {
    vi.spyOn(occ, 'config').mockResolvedValue(config(false))
    renderAt('/auth/mot-de-passe-oublie', '/auth/mot-de-passe-oublie', <ForgotPasswordPage />)
    expect(await screen.findByRole('heading', { name: 'E-mails indisponibles' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Recevoir le lien' })).not.toBeInTheDocument()
  })
})

describe('ConfirmEmailChangePage', () => {
  it('distingue un mauvais mot de passe', async () => {
    const confirm = vi.spyOn(usersApi, 'confirmEmailChange').mockRejectedValueOnce(pbError(400, { password: { message: 'Invalid' } })).mockResolvedValueOnce(true)
    renderAt('/auth/changer-email/t1', '/auth/changer-email/:token', <ConfirmEmailChangePage />)
    await userEvent.type(screen.getByLabelText('Mot de passe actuel'), 'faux')
    await userEvent.click(screen.getByRole('button', { name: 'Confirmer la nouvelle adresse' }))
    expect(await screen.findByText('Mot de passe incorrect.')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Confirmer la nouvelle adresse' }))
    expect(confirm).toHaveBeenLastCalledWith('t1', 'faux')
    expect(await screen.findByRole('heading', { name: 'Adresse modifiée' })).toBeInTheDocument()
  })
})
