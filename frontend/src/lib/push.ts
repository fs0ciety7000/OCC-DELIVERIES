/** Notifications Web Push côté navigateur (abonnement, désabonnement, état). */
import { pushApi } from './api'

export type PushSupport = 'supported' | 'unsupported' | 'ios-install'

export function isIOS(): boolean {
  if (typeof navigator === 'undefined') return false
  return /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
}

/** Ouverte depuis l'écran d'accueil (PWA installée). */
export function isStandalone(): boolean {
  if (typeof window === 'undefined') return false
  const nav = navigator as Navigator & { standalone?: boolean }
  return nav.standalone === true || !!window.matchMedia?.('(display-mode: standalone)').matches
}

/** iOS n'autorise le push qu'une fois l'app ajoutée à l'écran d'accueil (iOS 16.4+). */
export function pushSupport(): PushSupport {
  if (typeof window === 'undefined' || typeof navigator === 'undefined') return 'unsupported'
  const capable = 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
  if (isIOS() && !isStandalone()) return 'ios-install'
  return capable ? 'supported' : 'unsupported'
}

export function notificationPermission(): NotificationPermission | 'unsupported' {
  return typeof Notification === 'undefined' ? 'unsupported' : Notification.permission
}

/** Clé VAPID publique (base64 URL) → octets pour `applicationServerKey`. */
export function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padded = (base64 + '='.repeat((4 - (base64.length % 4)) % 4)).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(padded)
  const out = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}

async function registration(timeoutMs = 8000): Promise<ServiceWorkerRegistration> {
  if (!('serviceWorker' in navigator)) throw new Error("Ce navigateur ne gère pas les notifications.")
  const ready = navigator.serviceWorker.ready
  const timeout = new Promise<never>((_, reject) =>
    setTimeout(() => reject(new Error("Le service de l'app n'est pas encore prêt : recharge la page puis réessaie.")), timeoutMs),
  )
  return Promise.race([ready, timeout])
}

export async function currentSubscription(): Promise<PushSubscription | null> {
  if (pushSupport() !== 'supported') return null
  try {
    const reg = await navigator.serviceWorker.getRegistration()
    return (await reg?.pushManager.getSubscription()) ?? null
  } catch {
    return null
  }
}

function toInput(sub: PushSubscription) {
  const json = sub.toJSON()
  return { endpoint: sub.endpoint, keys: { p256dh: json.keys?.p256dh ?? '', auth: json.keys?.auth ?? '' } }
}

/** Demande la permission : à appeler **dans le gestionnaire du clic**, avant tout `await` (Safari). */
export function requestPermission(): Promise<NotificationPermission> {
  if (typeof Notification === 'undefined') return Promise.resolve('denied')
  return Notification.requestPermission()
}

/**
 * Active les notifications sur cet appareil. `permission` = promesse obtenue par
 * `requestPermission()` dans le clic (jamais demandée hors geste utilisateur).
 */
export async function enablePush(permissionRequest: Promise<NotificationPermission> = requestPermission()): Promise<void> {
  if (pushSupport() !== 'supported') throw new Error('Les notifications ne sont pas disponibles sur cet appareil.')
  const permission = await permissionRequest
  if (permission !== 'granted') {
    throw new Error(
      permission === 'denied'
        ? 'Notifications bloquées : autorise-les dans les réglages du navigateur pour ce site.'
        : 'Autorisation non accordée.',
    )
  }
  const { enabled, publicKey } = await pushApi.publicKey()
  if (!enabled || !publicKey) throw new Error("Les notifications ne sont pas activées sur ce serveur.")
  const reg = await registration()
  let sub = await reg.pushManager.getSubscription()
  const key = urlBase64ToUint8Array(publicKey)
  // clé du serveur changée : réabonner
  if (sub && sub.options.applicationServerKey) {
    const current = new Uint8Array(sub.options.applicationServerKey)
    if (current.length !== key.length || current.some((b, i) => b !== key[i])) {
      await sub.unsubscribe().catch(() => undefined)
      sub = null
    }
  }
  sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
  await pushApi.subscribe(toInput(sub))
}

/** Désactive les notifications sur cet appareil. */
export async function disablePush(): Promise<void> {
  const sub = await currentSubscription()
  if (!sub) return
  await pushApi.unsubscribe(sub.endpoint).catch(() => undefined)
  await sub.unsubscribe().catch(() => undefined)
}

/** Après connexion : rattache l'abonnement existant de ce navigateur au compte courant. */
export async function syncPush(): Promise<boolean> {
  if (notificationPermission() !== 'granted') return false
  const sub = await currentSubscription()
  if (!sub) return false
  try {
    await pushApi.subscribe(toInput(sub))
    return true
  } catch {
    return false
  }
}
