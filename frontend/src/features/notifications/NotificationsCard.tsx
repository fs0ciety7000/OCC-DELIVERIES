import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, BellOff, Download, RotateCw, Send, Share } from 'lucide-react'
import { useSyncExternalStore } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Card, CardBody, Skeleton } from '@/components/ui'
import { Toggle } from '@/features/admin/Toggle'
import { pushApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { currentSubscription, disablePush, enablePush, isIOS, isStandalone, notificationPermission, pushSupport, requestPermission } from '@/lib/push'
import type { NotifyPrefs } from '@/lib/types'
import { canPromptInstall, promptInstall, subscribeInstall, wasInstalled } from '@/pwa/register'

const PREF_ROWS: { key: keyof NotifyPrefs; label: string; hint: string }[] = [
  { key: 'party', label: 'Étapes des commandes', hint: 'Vote ouvert, resto choisi, récap prêt, tout le monde est prêt…' },
  { key: 'payments', label: 'Remboursements', hint: 'Ce que tu dois, remboursements déclarés et confirmés.' },
  { key: 'reminders', label: "Rappels d'heure limite", hint: '2 minutes avant la fin du vote ou de la commande.' },
]

const prefsKey = (userId: string) => ['push', 'prefs', userId] as const
const DEVICE_KEY = ['push', 'device'] as const

/** Carte « Notifications » du profil (Mes infos) : activation, test, préférences, installation. */
export function NotificationsCard({ userId }: { userId: string }) {
  const qc = useQueryClient()
  const support = pushSupport()
  const prefs = useQuery({ queryKey: prefsKey(userId), queryFn: pushApi.prefs })
  // état de CET appareil (permission du navigateur + abonnement du service worker)
  const device = useQuery({
    queryKey: DEVICE_KEY,
    queryFn: async () => ({ permission: notificationPermission(), subscribed: (await currentSubscription()) !== null }),
    staleTime: 0,
    networkMode: 'always',
  })
  const permission = device.data?.permission ?? notificationPermission()
  const subscribed = device.data ? device.data.subscribed : null
  const refreshDevice = () => qc.invalidateQueries({ queryKey: DEVICE_KEY })

  const enable = useMutation({
    mutationFn: enablePush,
    onSuccess: () => toast.success('Notifications activées sur cet appareil'),
    onError: (e) => toast.error(errorMessage(e)),
    onSettled: () => {
      void refreshDevice()
      void qc.invalidateQueries({ queryKey: prefsKey(userId) })
    },
  })
  const disable = useMutation({
    mutationFn: disablePush,
    onSuccess: () => toast('Notifications désactivées sur cet appareil'),
    onError: (e) => toast.error(errorMessage(e)),
    onSettled: () => {
      void refreshDevice()
      void qc.invalidateQueries({ queryKey: prefsKey(userId) })
    },
  })
  const test = useMutation({
    mutationFn: pushApi.test,
    onSuccess: (r) => toast.success(r.devices > 1 ? `Notification de test envoyée à ${r.devices} appareils` : 'Notification de test envoyée'),
    onError: (e) => toast.error(errorMessage(e)),
  })
  const setPref = useMutation({
    mutationFn: (p: Partial<NotifyPrefs>) => pushApi.setPrefs(p),
    onMutate: (p) => {
      const prev = qc.getQueryData(prefsKey(userId))
      qc.setQueryData(prefsKey(userId), (old: typeof prefs.data) => (old ? { ...old, prefs: { ...old.prefs, ...p } } : old))
      return { prev }
    },
    onError: (e, _p, ctx) => {
      qc.setQueryData(prefsKey(userId), ctx?.prev)
      toast.error(errorMessage(e))
    },
  })

  const serverOff = prefs.data && !prefs.data.enabled
  // Préférences sans effet tant qu'aucun appareil n'est abonné (ni celui-ci, ni un autre du compte).
  const prefsLocked = subscribed !== true && (prefs.data?.devices ?? 0) === 0
  const status = subscribed ? (
    <Badge variant="success" dot>
      Activées ici
    </Badge>
  ) : permission === 'denied' ? (
    <Badge variant="danger">Bloquées</Badge>
  ) : (
    <Badge>Désactivées</Badge>
  )

  return (
    <Card>
      <CardBody className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <h2 className="flex items-center gap-2 font-display text-lg font-semibold">
            <Bell aria-hidden className="size-5 text-brand" /> Notifications
          </h2>
          {support === 'supported' && subscribed !== null && status}
        </div>
        <p className="text-sm text-muted">Sois prévenu·e même quand l'app est fermée : vote ouvert, resto choisi, ce que tu dois, rappels avant l'heure limite.</p>

        {serverOff ? (
          <p className="rounded-md border border-border bg-fg/[0.04] p-3 text-sm text-muted">Les notifications ne sont pas encore activées sur ce serveur.</p>
        ) : support === 'ios-install' ? (
          <div className="rounded-md border border-info/30 bg-info/10 p-3 text-sm">
            <p className="font-semibold">Sur iPhone / iPad : installe d'abord l'app</p>
            <p className="mt-1 text-muted">
              Touche <Share aria-label="Partager" className="inline size-4 align-text-bottom" /> puis « Sur l'écran d'accueil », ouvre OCC depuis l'icône, et reviens ici pour
              activer les notifications (iOS 16.4 ou plus récent).
            </p>
          </div>
        ) : support === 'unsupported' ? (
          <p className="rounded-md border border-border bg-fg/[0.04] p-3 text-sm text-muted">Ce navigateur ne gère pas les notifications push.</p>
        ) : (
          <>
            {permission === 'denied' && !subscribed ? (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-danger/30 bg-danger/10 p-3 text-sm">
                <p className="min-w-0 flex-1 basis-56">Notifications bloquées pour ce site : autorise-les dans les réglages du navigateur, puis réessaie.</p>
                <Button variant="ghost" size="sm" className="min-h-11" leftIcon={<RotateCw className="size-4" />} loading={device.isFetching} onClick={() => void refreshDevice()}>
                  Réessayer
                </Button>
              </div>
            ) : (
              <div className="flex flex-wrap gap-2">
                {subscribed ? (
                  <>
                    <Button variant="secondary" leftIcon={<Send className="size-4" />} loading={test.isPending} onClick={() => test.mutate()}>
                      Envoyer un test
                    </Button>
                    <Button variant="ghost" leftIcon={<BellOff className="size-4" />} loading={disable.isPending} onClick={() => disable.mutate()}>
                      Désactiver sur cet appareil
                    </Button>
                  </>
                ) : (
                  <Button leftIcon={<Bell className="size-4" />} loading={enable.isPending} disabled={subscribed === null} onClick={() => enable.mutate(requestPermission())}>
                    Activer les notifications
                  </Button>
                )}
              </div>
            )}
          </>
        )}

        {!serverOff && (
          <fieldset className="space-y-1 border-t border-border pt-3">
            <legend className="sr-only">Notifications reçues</legend>
            {prefs.isPending ? (
              <Skeleton className="h-28 rounded-md" />
            ) : prefs.isError ? (
              <p className="text-sm text-danger">{errorMessage(prefs.error)}</p>
            ) : (
              <>
              {prefsLocked && <p className="text-xs text-subtle">Active d'abord les notifications sur cet appareil.</p>}
              {PREF_ROWS.map((row) => (
                <div key={row.key} className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{row.label}</p>
                    <p className="text-xs text-muted">{row.hint}</p>
                  </div>
                  <Toggle label={row.label} disabled={prefsLocked} checked={prefs.data.prefs[row.key]} onChange={(v) => setPref.mutate({ [row.key]: v })} />
                </div>
              ))}
              </>
            )}
            {prefs.data && prefs.data.devices > 0 && (
              <p className="pt-1 text-xs text-subtle">
                {prefs.data.devices} appareil{prefs.data.devices > 1 ? 's' : ''} abonné{prefs.data.devices > 1 ? 's' : ''} à ton compte. Les préférences valent pour tous.
              </p>
            )}
          </fieldset>
        )}
        <InstallRow />
      </CardBody>
    </Card>
  )
}

function useCanInstall() {
  return useSyncExternalStore(subscribeInstall, canPromptInstall, () => false)
}

/** « Installer l'app » : invite native (Chrome, Edge, Android) ou mode d'emploi iOS. */
export function InstallRow() {
  const canInstall = useCanInstall()
  const installed = useSyncExternalStore(subscribeInstall, wasInstalled, () => false)
  if (isStandalone() || installed) return null
  if (!canInstall && !isIOS()) return null
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-3">
      <div className="min-w-0">
        <p className="text-sm font-medium">Installer l'app</p>
        <p className="text-xs text-muted">{canInstall ? "Accès en un geste depuis l'écran d'accueil, plein écran, hors ligne." : "Safari : Partager → « Sur l'écran d'accueil »."}</p>
      </div>
      {canInstall && (
        <Button
          variant="secondary"
          size="sm"
          leftIcon={<Download className="size-4" />}
          onClick={async () => {
            if (await promptInstall()) toast.success('OCC Deliveries est installée')
          }}
        >
          Installer l'app
        </Button>
      )}
    </div>
  )
}
