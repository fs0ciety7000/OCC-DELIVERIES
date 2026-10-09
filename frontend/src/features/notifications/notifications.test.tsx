import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { pushApi } from '@/lib/api'
import * as push from '@/lib/push'
import type { PushPrefsResponse } from '@/lib/types'
import { OfflineBanner } from '@/pwa/PwaRuntime'
import { NotificationsCard } from './NotificationsCard'

const prefs = (over: Partial<PushPrefsResponse> = {}): PushPrefsResponse => ({
  prefs: { party: true, payments: true, reminders: true },
  devices: 0,
  enabled: true,
  ...over,
})

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <NotificationsCard userId="u1" />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe('NotificationsCard', () => {
  it('appareil compatible : active (permission demandée dans le clic) puis propose le test', async () => {
    vi.spyOn(push, 'pushSupport').mockReturnValue('supported')
    vi.spyOn(push, 'notificationPermission').mockReturnValue('default')
    const current = vi.spyOn(push, 'currentSubscription').mockResolvedValue(null)
    const ask = vi.spyOn(push, 'requestPermission').mockResolvedValue('granted')
    const enable = vi.spyOn(push, 'enablePush').mockResolvedValue()
    vi.spyOn(pushApi, 'prefs').mockResolvedValue(prefs())
    const test = vi.spyOn(pushApi, 'test').mockResolvedValue({ queued: true, devices: 1 })
    renderCard()

    const btn = await screen.findByRole('button', { name: 'Activer les notifications' })
    await waitFor(() => expect(btn).toBeEnabled())
    current.mockResolvedValue({ endpoint: 'https://push.test/x' } as PushSubscription)
    await userEvent.click(btn)
    expect(ask).toHaveBeenCalledTimes(1)
    expect(enable).toHaveBeenCalledTimes(1)
    await userEvent.click(await screen.findByRole('button', { name: 'Envoyer un test' }))
    expect(test).toHaveBeenCalled()
    expect(screen.getByText('Activées ici')).toBeInTheDocument()
  })

  it('préférences : un interrupteur par catégorie, enregistré sur le serveur', async () => {
    vi.spyOn(push, 'pushSupport').mockReturnValue('supported')
    vi.spyOn(push, 'currentSubscription').mockResolvedValue(null)
    vi.spyOn(pushApi, 'prefs').mockResolvedValue(prefs({ devices: 2 }))
    const save = vi.spyOn(pushApi, 'setPrefs').mockResolvedValue({ prefs: { party: true, payments: true, reminders: false } })
    renderCard()
    const reminders = await screen.findByRole('switch', { name: "Rappels d'heure limite" })
    expect(reminders).toHaveAttribute('aria-checked', 'true')
    await userEvent.click(reminders)
    expect(save).toHaveBeenCalledWith({ reminders: false })
    expect(reminders).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText(/2 appareils abonnés/)).toBeInTheDocument()
  })

  it('iPhone dans Safari : installer d’abord sur l’écran d’accueil', async () => {
    vi.spyOn(push, 'pushSupport').mockReturnValue('ios-install')
    vi.spyOn(push, 'isIOS').mockReturnValue(true)
    vi.spyOn(pushApi, 'prefs').mockResolvedValue(prefs())
    renderCard()
    expect(await screen.findByText(/installe d'abord l'app/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Activer les notifications' })).not.toBeInTheDocument()
    expect(screen.getByText("Installer l'app")).toBeInTheDocument()
  })

  it('serveur sans notifications, permission refusée', async () => {
    vi.spyOn(push, 'pushSupport').mockReturnValue('supported')
    vi.spyOn(pushApi, 'prefs').mockResolvedValue(prefs({ enabled: false }))
    renderCard()
    expect(await screen.findByText(/pas encore activées sur ce serveur/)).toBeInTheDocument()
  })

  it('permission bloquée : explication, « Réessayer » relit la permission, pas de bouton « Activer »', async () => {
    vi.spyOn(push, 'pushSupport').mockReturnValue('supported')
    const perm = vi.spyOn(push, 'notificationPermission').mockReturnValue('denied')
    vi.spyOn(push, 'currentSubscription').mockResolvedValue(null)
    vi.spyOn(pushApi, 'prefs').mockResolvedValue(prefs())
    renderCard()
    expect(await screen.findByText(/Notifications bloquées pour ce site/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Activer les notifications' })).not.toBeInTheDocument()
    const retry = await screen.findByRole('button', { name: 'Réessayer' })
    await waitFor(() => expect(retry).toBeEnabled())
    perm.mockReturnValue('default')
    await userEvent.click(retry)
    expect(await screen.findByRole('button', { name: 'Activer les notifications' })).toBeInTheDocument()
    expect(screen.queryByText(/Notifications bloquées pour ce site/)).not.toBeInTheDocument()
  })

  it('aucun appareil abonné : préférences désactivées avec une aide', async () => {
    vi.spyOn(push, 'pushSupport').mockReturnValue('supported')
    vi.spyOn(push, 'notificationPermission').mockReturnValue('default')
    vi.spyOn(push, 'currentSubscription').mockResolvedValue(null)
    vi.spyOn(pushApi, 'prefs').mockResolvedValue(prefs({ devices: 0 }))
    renderCard()
    const party = await screen.findByRole('switch', { name: 'Étapes des commandes' })
    await waitFor(() => expect(party).toBeDisabled())
    expect(screen.getByText("Active d'abord les notifications sur cet appareil.")).toBeInTheDocument()
  })
})

describe('OfflineBanner', () => {
  it('hors ligne : avertit que les données peuvent dater', () => {
    const { rerender, container } = render(<OfflineBanner online={false} pending={0} />)
    expect(screen.getByRole('status')).toHaveTextContent('Hors ligne — les données affichées peuvent dater.')
    rerender(<OfflineBanner online={false} pending={2} />)
    expect(screen.getByRole('status')).toHaveTextContent("2 actions en attente d'envoi")
    rerender(<OfflineBanner online pending={1} />)
    expect(screen.getByRole('status')).toHaveTextContent('Connexion rétablie')
    rerender(<OfflineBanner online pending={0} />)
    expect(container).toBeEmptyDOMElement()
  })
})
